package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	redis "github.com/redis/go-redis/v9"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

// SegmentationTask represents a segmentation request
type SegmentationTask struct {
	TaskID      string    `json:"task_id"`
	ScanSID     string    `json:"scan_sid"`
	SliceIndex  int       `json:"slice_index"`
	TotalChunks int       `json:"total_chunks"`
	Status      string    `json:"status"` // queued, processing, completed, failed
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// ChunkCoordinate represents position of a chunk in the original image
type ChunkCoordinate struct {
	X        int `json:"x"`
	Y        int `json:"y"`
	Width    int `json:"width"`
	Height   int `json:"height"`
	OverlapX int `json:"overlap_x"`
	OverlapY int `json:"overlap_y"`
}

// SegmentationJob represents a single chunk processing job
type SegmentationJob struct {
	TaskID      string          `json:"task_id"`
	ChunkID     int             `json:"chunk_id"`
	ChunkData   string          `json:"chunk_data"` // base64 encoded
	ChunkShape  []int           `json:"chunk_shape"`
	Position    ChunkCoordinate `json:"position"`
	TotalChunks int             `json:"total_chunks"`
	RetryCount  int             `json:"retry_count"`
}

// SegmentationService handles brain segmentation tasks
type SegmentationService struct {
	BaseService
	redisClient *redis.Client
	rabbitConn  *amqp.Connection
	rabbitCh    *amqp.Channel
	gdcmClient  *GDCMClient
	queueName   string
	resultTTL   time.Duration
}

// NewSegmentationService creates a new segmentation service
func NewSegmentationService(
	logger *utils.Logger,
	redisClient *redis.Client,
	rabbitConn *amqp.Connection,
	gdcmClient *GDCMClient,
) (*SegmentationService, error) {

	ch, err := rabbitConn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	queueName := "segmentation_tasks"

	// Declare queue
	args := amqp.Table{
		"x-message-ttl":  int32(1800000), // 30 minutes
		"x-max-priority": int32(10),
	}

	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		args,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	service := &SegmentationService{
		BaseService: BaseService{
			logger: logger,
		},
		redisClient: redisClient,
		rabbitConn:  rabbitConn,
		rabbitCh:    ch,
		gdcmClient:  gdcmClient,
		queueName:   queueName,
		resultTTL:   1 * time.Hour,
	}

	logger.Info("SegmentationService initialized")
	return service, nil
}

// GenerateTaskID generates a unique task ID
func GenerateTaskID(scanSID string, sliceIndex int) string {
	data := fmt.Sprintf("%s:%d:%d", scanSID, sliceIndex, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("seg_%x", hash[:8])
}

// SegmentSlice initiates segmentation for a specific DICOM slice
func (s *SegmentationService) SegmentSlice(
	ctx context.Context,
	scanSID string,
	sliceIndex int,
) (string, error) {

	// Generate task ID
	taskID := GenerateTaskID(scanSID, sliceIndex)

	s.logger.Info("Starting segmentation task: %s (scan: %s, slice: %d)",
		taskID, scanSID, sliceIndex)

	// Fetch slice data from GDCM service
	sliceData, err := s.gdcmClient.FetchSliceData(scanSID, sliceIndex)
	if err != nil {
		return "", fmt.Errorf("failed to fetch slice data: %w", err)
	}

	// Chunk the slice with overlap
	chunks, coords := s.chunkSliceWithOverlap(sliceData, 256, 32)
	totalChunks := len(chunks)

	// Create task metadata
	task := SegmentationTask{
		TaskID:      taskID,
		ScanSID:     scanSID,
		SliceIndex:  sliceIndex,
		TotalChunks: totalChunks,
		Status:      "queued",
		CreatedAt:   time.Now(),
	}

	// Store task metadata in Redis
	taskJSON, _ := json.Marshal(task)
	err = s.redisClient.Set(ctx, fmt.Sprintf("segmentation:task:%s", taskID), taskJSON, s.resultTTL).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store task metadata: %w", err)
	}

	// Initialize progress counter
	err = s.redisClient.Set(ctx, fmt.Sprintf("segmentation:completed:%s", taskID), 0, s.resultTTL).Err()
	if err != nil {
		return "", fmt.Errorf("failed to initialize progress: %w", err)
	}

	// Publish chunks to RabbitMQ
	for i, chunk := range chunks {
		job := SegmentationJob{
			TaskID:      taskID,
			ChunkID:     i,
			ChunkData:   base64.StdEncoding.EncodeToString(chunk),
			ChunkShape:  []int{256, 256},
			Position:    coords[i],
			TotalChunks: totalChunks,
			RetryCount:  0,
		}

		jobJSON, _ := json.Marshal(job)

		err = s.rabbitCh.PublishWithContext(
			ctx,
			"",          // exchange
			s.queueName, // routing key
			false,       // mandatory
			false,       // immediate
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         jobJSON,
				Priority:     5, // Default priority
			},
		)

		if err != nil {
			s.logger.Error("Failed to publish chunk %d for task %s: %v", i, taskID, err)
			return "", fmt.Errorf("failed to publish chunk: %w", err)
		}
	}

	s.logger.Info("Published %d chunks for task %s", totalChunks, taskID)

	// Update task status
	task.Status = "processing"
	taskJSON, _ = json.Marshal(task)
	s.redisClient.Set(ctx, fmt.Sprintf("segmentation:task:%s", taskID), taskJSON, s.resultTTL)

	return taskID, nil
}

// chunkSliceWithOverlap splits a 2D slice into overlapping chunks
func (s *SegmentationService) chunkSliceWithOverlap(
	sliceData []byte,
	chunkSize int,
	overlap int,
) ([][]byte, []ChunkCoordinate) {

	// Assume sliceData is a 512x512 slice in row-major order (float32)
	// For simplicity, we'll create a 2x2 grid of 256x256 chunks with overlap

	// TODO: Parse actual dimensions from sliceData
	width := 512
	height := 512

	var chunks [][]byte
	var coords []ChunkCoordinate

	stride := chunkSize - overlap

	for y := 0; y <= height-chunkSize; y += stride {
		for x := 0; x <= width-chunkSize; x += stride {
			// Extract chunk (simplified - actual implementation needs proper array slicing)
			chunk := extractChunk(sliceData, x, y, chunkSize, chunkSize, width)

			chunks = append(chunks, chunk)
			coords = append(coords, ChunkCoordinate{
				X:        x,
				Y:        y,
				Width:    chunkSize,
				Height:   chunkSize,
				OverlapX: overlap,
				OverlapY: overlap,
			})
		}
	}

	s.logger.Debug("Created %d chunks (%dx%d each, %dpx overlap)",
		len(chunks), chunkSize, chunkSize, overlap)

	return chunks, coords
}

// extractChunk extracts a chunk from the full slice
func extractChunk(data []byte, x, y, w, h, fullWidth int) []byte {
	// Each pixel is 4 bytes (float32)
	bytesPerPixel := 4
	chunkSize := w * h * bytesPerPixel
	chunk := make([]byte, chunkSize)

	// Copy row by row
	for row := 0; row < h; row++ {
		srcRow := y + row
		srcOffset := (srcRow*fullWidth + x) * bytesPerPixel
		dstOffset := row * w * bytesPerPixel
		length := w * bytesPerPixel

		// Bounds check
		if srcOffset+length <= len(data) && dstOffset+length <= len(chunk) {
			copy(chunk[dstOffset:dstOffset+length], data[srcOffset:srcOffset+length])
		}
	}

	return chunk
}

// GetTaskStatus retrieves the current status of a segmentation task
func (s *SegmentationService) GetTaskStatus(ctx context.Context, taskID string) (*SegmentationTask, int, error) {

	// Get task metadata
	taskJSON, err := s.redisClient.Get(ctx, fmt.Sprintf("segmentation:task:%s", taskID)).Result()
	if err == redis.Nil {
		return nil, 0, fmt.Errorf("task not found: %s", taskID)
	} else if err != nil {
		return nil, 0, fmt.Errorf("failed to get task: %w", err)
	}

	var task SegmentationTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	// Get progress
	progressStr, err := s.redisClient.Get(ctx, fmt.Sprintf("segmentation:progress:%s", taskID)).Result()
	progress := 0
	if err == nil {
		fmt.Sscanf(progressStr, "%d", &progress)
	}

	return &task, progress, nil
}

// AggregateResults merges all chunk results into final segmentation mask
func (s *SegmentationService) AggregateResults(ctx context.Context, taskID string) ([]byte, error) {

	// Get task metadata
	task, _, err := s.GetTaskStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != "completed" {
		return nil, fmt.Errorf("task not completed yet")
	}

	s.logger.Info("Aggregating results for task %s (%d chunks)", taskID, task.TotalChunks)

	// Retrieve all chunk results from Redis
	chunkResults := make([][]byte, task.TotalChunks)
	chunkMetadata := make([]ChunkCoordinate, task.TotalChunks)

	for i := 0; i < task.TotalChunks; i++ {
		resultKey := fmt.Sprintf("segmentation:result:%s:%d", taskID, i)
		metadataKey := fmt.Sprintf("segmentation:metadata:%s:%d", taskID, i)

		// Get chunk result
		result, err := s.redisClient.Get(ctx, resultKey).Bytes()
		if err != nil {
			return nil, fmt.Errorf("failed to get chunk %d result: %w", i, err)
		}
		chunkResults[i] = result

		// Get chunk metadata
		metaJSON, err := s.redisClient.Get(ctx, metadataKey).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to get chunk %d metadata: %w", i, err)
		}

		var meta map[string]interface{}
		json.Unmarshal([]byte(metaJSON), &meta)

		// TODO: Parse metadata to get chunk coordinates
	}

	// Merge chunks with Gaussian blending
	finalMask := s.mergeChunksWithBlending(chunkResults, chunkMetadata, 512, 512)

	s.logger.Info("Successfully aggregated %d chunks for task %s", task.TotalChunks, taskID)

	return finalMask, nil
}

// mergeChunksWithBlending merges overlapping chunks using Gaussian weighted blending
func (s *SegmentationService) mergeChunksWithBlending(
	chunks [][]byte,
	coords []ChunkCoordinate,
	outputWidth int,
	outputHeight int,
) []byte {

	// Create output arrays
	output := make([]float32, outputWidth*outputHeight)
	weightMap := make([]float32, outputWidth*outputHeight)

	// Create Gaussian weight kernel
	chunkSize := 256
	weightKernel := createGaussianKernel2D(chunkSize, chunkSize, 32.0)

	// Blend each chunk
	for i, chunkData := range chunks {
		coord := coords[i]

		// Convert chunk bytes to float32 array
		chunkFloat := bytesToFloat32Array(chunkData)

		// Apply weighted blending
		for cy := 0; cy < coord.Height; cy++ {
			for cx := 0; cx < coord.Width; cx++ {
				outX := coord.X + cx
				outY := coord.Y + cy

				if outX < outputWidth && outY < outputHeight {
					outIdx := outY*outputWidth + outX
					chunkIdx := cy*coord.Width + cx

					weight := weightKernel[chunkIdx]
					output[outIdx] += chunkFloat[chunkIdx] * weight
					weightMap[outIdx] += weight
				}
			}
		}
	}

	// Normalize by weight map
	for i := 0; i < len(output); i++ {
		if weightMap[i] > 0 {
			output[i] /= weightMap[i]
		}
	}

	// Convert back to bytes
	return float32ArrayToBytes(output)
}

// createGaussianKernel2D creates a 2D Gaussian kernel for blending
func createGaussianKernel2D(width, height int, sigma float64) []float32 {
	kernel := make([]float32, width*height)
	centerX := float64(width) / 2.0
	centerY := float64(height) / 2.0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx := float64(x) - centerX
			dy := float64(y) - centerY

			weight := math.Exp(-(dx*dx + dy*dy) / (2 * sigma * sigma))
			kernel[y*width+x] = float32(weight)
		}
	}

	return kernel
}

// Helper functions for byte/float32 conversion
func bytesToFloat32Array(data []byte) []float32 {
	if len(data)%4 != 0 {
		return nil
	}

	count := len(data) / 4
	floatData := make([]float32, count)

	for i := 0; i < count; i++ {
		bits := binary.LittleEndian.Uint32(data[i*4 : (i+1)*4])
		floatData[i] = math.Float32frombits(bits)
	}

	return floatData
}

func float32ArrayToBytes(data []float32) []byte {
	bytes := make([]byte, len(data)*4)

	for i, f := range data {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(bytes[i*4:(i+1)*4], bits)
	}

	return bytes
}

// Close closes the RabbitMQ channel
func (s *SegmentationService) Close() error {
	if s.rabbitCh != nil {
		return s.rabbitCh.Close()
	}
	return nil
}
