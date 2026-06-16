package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"

	amqp "github.com/rabbitmq/amqp091-go"
	redis "github.com/redis/go-redis/v9"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

// SegmentationTask represents a segmentation request
type SegmentationTask struct {
	TaskID      string    `json:"task_id"`
	ScanSID     string    `json:"scan_sid"`
	SliceIndex  int       `json:"slice_index"`
	ClusterMode string    `json:"cluster_mode,omitempty"`
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
	ClusterMode string          `json:"cluster_mode,omitempty"`
	PublishedAt int64           `json:"published_at"`
	ChunkSize   int             `json:"chunk_size"`
	OverlapSize int             `json:"overlap_size"`
}

type CompareStats struct {
	ModeA       string  `json:"mode_a"`
	ModeB       string  `json:"mode_b"`
	AvgLatencyA float64 `json:"avg_latency_a_ms"`
	AvgLatencyB float64 `json:"avg_latency_b_ms"`
	P50A        float64 `json:"p50_a_ms"`
	P50B        float64 `json:"p50_b_ms"`
	P95A        float64 `json:"p95_a_ms"`
	P95B        float64 `json:"p95_b_ms"`
	P99A        float64 `json:"p99_a_ms"`
	P99B        float64 `json:"p99_b_ms"`
	Speedup     float64 `json:"speedup"`
	SampleSize  int     `json:"sample_size"`
}

type chunkRuntimeLog struct {
	TaskID       string `json:"task_id"`
	ChunkID      int    `json:"chunk_id"`
	WorkerName   string `json:"worker_name"`
	GPUType      string `json:"gpu_type"`
	ClusterMode  string `json:"cluster_mode"`
	QueueWaitMs  int    `json:"queue_wait_ms"`
	InferenceMs  int    `json:"inference_ms"`
	TotalChunkMs int    `json:"total_chunk_ms"`
}

// SegmentationService handles brain segmentation tasks
type SegmentationService struct {
	BaseService
	redisClient    *redis.Client
	rabbitConn     *amqp.Connection
	rabbitCh       *amqp.Channel
	gdcmClient     *GDCMClient
	expRepo        repositories.SegmentationExperimentRepository
	queueLegacy    string
	queueHomogen   string
	queueHeterogen string
	chunkSize      int
	overlapSize    int
	resultTTL      time.Duration
}

// NewSegmentationService creates a new segmentation service
func NewSegmentationService(
	logger *utils.Logger,
	redisClient *redis.Client,
	rabbitConn *amqp.Connection,
	gdcmClient *GDCMClient,
	expRepo repositories.SegmentationExperimentRepository,
	chunkSize int,
	overlapSize int,
	queueLegacy string,
	queueHomogen string,
	queueHeterogen string,
) (*SegmentationService, error) {

	ch, err := rabbitConn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	if chunkSize <= 0 {
		chunkSize = 256
	}
	if overlapSize < 0 {
		overlapSize = 32
	}
	if queueLegacy == "" {
		queueLegacy = "segmentation_tasks"
	}
	if queueHomogen == "" {
		queueHomogen = "segmentation_tasks_homogen"
	}
	if queueHeterogen == "" {
		queueHeterogen = "segmentation_tasks_heterogen"
	}

	// Declare queue
	args := amqp.Table{
		"x-message-ttl":  int32(1800000), // 30 minutes
		"x-max-priority": int32(10),
	}
	for _, queueName := range []string{queueLegacy, queueHomogen, queueHeterogen} {
		if _, err = ch.QueueDeclare(
			queueName,
			true,  // durable
			false, // auto-delete
			false, // exclusive
			false, // no-wait
			args,
		); err != nil {
			return nil, fmt.Errorf("failed to declare queue %s: %w", queueName, err)
		}
	}

	service := &SegmentationService{
		BaseService: BaseService{
			logger: logger,
		},
		redisClient:    redisClient,
		rabbitConn:     rabbitConn,
		rabbitCh:       ch,
		gdcmClient:     gdcmClient,
		expRepo:        expRepo,
		queueLegacy:    queueLegacy,
		queueHomogen:   queueHomogen,
		queueHeterogen: queueHeterogen,
		chunkSize:      chunkSize,
		overlapSize:    overlapSize,
		resultTTL:      1 * time.Hour,
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
	return s.SegmentSliceWithMode(ctx, scanSID, sliceIndex, "")
}

// SegmentSliceWithMode initiates segmentation with optional cluster mode routing.
// Empty mode keeps backward compatibility by publishing to legacy queue.
func (s *SegmentationService) SegmentSliceWithMode(
	ctx context.Context,
	scanSID string,
	sliceIndex int,
	clusterMode string,
) (string, error) {
	normalizedMode := strings.ToLower(strings.TrimSpace(clusterMode))
	queueName, effectiveMode, err := s.selectQueueByMode(normalizedMode)
	if err != nil {
		return "", err
	}

	// Generate task ID
	taskID := GenerateTaskID(scanSID, sliceIndex)

	s.logger.Info("Starting segmentation task: %s (scan: %s, slice: %d, mode: %s)",
		taskID, scanSID, sliceIndex, effectiveMode)

	// Fetch slice data from GDCM service
	sliceData, err := s.gdcmClient.FetchSliceData(scanSID, sliceIndex)
	if err != nil {
		return "", fmt.Errorf("failed to fetch slice data: %w", err)
	}

	// Chunk the slice with overlap
	chunks, coords := s.chunkSliceWithOverlap(sliceData)
	totalChunks := len(chunks)

	// Create task metadata
	task := SegmentationTask{
		TaskID:      taskID,
		ScanSID:     scanSID,
		SliceIndex:  sliceIndex,
		ClusterMode: effectiveMode,
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
	err = s.redisClient.Set(ctx, fmt.Sprintf("segmentation:progress:%s", taskID), 0, s.resultTTL).Err()
	if err != nil {
		return "", fmt.Errorf("failed to initialize progress percent: %w", err)
	}

	if s.expRepo != nil {
		experiment := &entities.SegmentationExperiment{
			TaskID:      taskID,
			ScanSID:     scanSID,
			SliceIndex:  sliceIndex,
			ClusterMode: effectiveMode,
			ChunkSize:   s.chunkSize,
			OverlapSize: s.overlapSize,
			TotalChunks: totalChunks,
			SubmittedAt: task.CreatedAt,
			Status:      "running",
		}
		if err := s.expRepo.Create(experiment); err != nil {
			return "", fmt.Errorf("failed to create experiment metadata: %w", err)
		}
	}

	// Publish chunks to RabbitMQ
	publishedAt := time.Now().UnixNano()
	for i, chunk := range chunks {
		job := SegmentationJob{
			TaskID:      taskID,
			ChunkID:     i,
			ChunkData:   base64.StdEncoding.EncodeToString(chunk),
			ChunkShape:  []int{s.chunkSize, s.chunkSize},
			Position:    coords[i],
			TotalChunks: totalChunks,
			RetryCount:  0,
			ClusterMode: effectiveMode,
			PublishedAt: publishedAt,
			ChunkSize:   s.chunkSize,
			OverlapSize: s.overlapSize,
		}

		jobJSON, _ := json.Marshal(job)

		err = s.rabbitCh.PublishWithContext(
			ctx,
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         jobJSON,
				Priority:     5, // Default priority
			},
		)

		if err != nil {
			s.logger.Error("Failed to publish chunk %d for task %s: %v", i, taskID, err)
			if s.expRepo != nil {
				_ = s.expRepo.UpdateStatus(taskID, "failed", fmt.Sprintf("failed to publish chunk %d: %v", i, err))
			}
			return "", fmt.Errorf("failed to publish chunk: %w", err)
		}
	}

	s.logger.Info("Published %d chunks for task %s", totalChunks, taskID)

	// Update task status
	task.Status = "processing"
	taskJSON, _ = json.Marshal(task)
	s.redisClient.Set(ctx, fmt.Sprintf("segmentation:task:%s", taskID), taskJSON, s.resultTTL)
	if s.expRepo != nil {
		_ = s.expRepo.UpdateStatus(taskID, "running", "")
	}

	return taskID, nil
}

func (s *SegmentationService) selectQueueByMode(clusterMode string) (string, string, error) {
	switch clusterMode {
	case "", "legacy":
		return s.queueLegacy, "legacy", nil
	case "homogen":
		return s.queueHomogen, "homogen", nil
	case "heterogen":
		return s.queueHeterogen, "heterogen", nil
	default:
		return "", "", utils.NewBadRequestError("cluster_mode must be one of: homogen, heterogen")
	}
}

// chunkSliceWithOverlap splits a 2D slice into overlapping chunks
func (s *SegmentationService) chunkSliceWithOverlap(
	sliceData []byte,
) ([][]byte, []ChunkCoordinate) {

	// Assume sliceData is a 512x512 slice in row-major order (float32)
	// For simplicity, we'll create a 2x2 grid of 256x256 chunks with overlap

	// TODO: Parse actual dimensions from sliceData
	width := 512
	height := 512

	var chunks [][]byte
	var coords []ChunkCoordinate

	stride := s.chunkSize - s.overlapSize
	if stride <= 0 {
		stride = s.chunkSize
	}

	for y := 0; y <= height-s.chunkSize; y += stride {
		for x := 0; x <= width-s.chunkSize; x += stride {
			// Extract chunk (simplified - actual implementation needs proper array slicing)
			chunk := extractChunk(sliceData, x, y, s.chunkSize, s.chunkSize, width)

			chunks = append(chunks, chunk)
			coords = append(coords, ChunkCoordinate{
				X:        x,
				Y:        y,
				Width:    s.chunkSize,
				Height:   s.chunkSize,
				OverlapX: s.overlapSize,
				OverlapY: s.overlapSize,
			})
		}
	}

	s.logger.Debug("Created %d chunks (%dx%d each, %dpx overlap)",
		len(chunks), s.chunkSize, s.chunkSize, s.overlapSize)

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
	fallbackCoords := s.generateChunkCoordinates(512, 512)

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

		coord := ChunkCoordinate{}
		if parsed, ok := parseChunkCoordinate(metaJSON); ok {
			coord = parsed
		} else if i < len(fallbackCoords) {
			coord = fallbackCoords[i]
		} else {
			coord = ChunkCoordinate{X: 0, Y: 0, Width: s.chunkSize, Height: s.chunkSize, OverlapX: s.overlapSize, OverlapY: s.overlapSize}
		}
		chunkMetadata[i] = coord
	}

	// Merge chunks with Gaussian blending
	mergeStart := time.Now()
	finalMask := s.mergeChunksWithBlending(chunkResults, chunkMetadata, 512, 512)
	mergeMs := int(time.Since(mergeStart).Milliseconds())

	finalKey := fmt.Sprintf("segmentation:final:%s", taskID)
	if err := s.redisClient.Set(ctx, finalKey, finalMask, 24*time.Hour).Err(); err != nil {
		s.logger.Warning("Failed to store final mask for task %s: %v", taskID, err)
	}

	if err := s.FinalizeExperiment(ctx, taskID, mergeMs); err != nil {
		s.logger.Warning("Failed to finalize experiment for task %s: %v", taskID, err)
	}

	s.logger.Info("Successfully aggregated %d chunks for task %s", task.TotalChunks, taskID)

	return finalMask, nil
}

func (s *SegmentationService) generateChunkCoordinates(width, height int) []ChunkCoordinate {
	var coords []ChunkCoordinate
	stride := s.chunkSize - s.overlapSize
	if stride <= 0 {
		stride = s.chunkSize
	}

	for y := 0; y <= height-s.chunkSize; y += stride {
		for x := 0; x <= width-s.chunkSize; x += stride {
			coords = append(coords, ChunkCoordinate{
				X:        x,
				Y:        y,
				Width:    s.chunkSize,
				Height:   s.chunkSize,
				OverlapX: s.overlapSize,
				OverlapY: s.overlapSize,
			})
		}
	}

	return coords
}

func parseChunkCoordinate(metaJSON string) (ChunkCoordinate, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil {
		return ChunkCoordinate{}, false
	}

	if pos, ok := payload["position"].(map[string]interface{}); ok {
		if coord, ok := mapToChunkCoordinate(pos); ok {
			return coord, true
		}
	}

	if coord, ok := mapToChunkCoordinate(payload); ok {
		return coord, true
	}

	return ChunkCoordinate{}, false
}

func mapToChunkCoordinate(data map[string]interface{}) (ChunkCoordinate, bool) {
	x, okX := toInt(data["x"])
	y, okY := toInt(data["y"])
	width, okW := toInt(data["width"])
	height, okH := toInt(data["height"])
	overlapX, okOX := toInt(data["overlap_x"])
	overlapY, okOY := toInt(data["overlap_y"])

	if !(okX && okY && okW && okH) {
		return ChunkCoordinate{}, false
	}

	if !okOX {
		overlapX = 0
	}
	if !okOY {
		overlapY = 0
	}

	return ChunkCoordinate{
		X:        x,
		Y:        y,
		Width:    width,
		Height:   height,
		OverlapX: overlapX,
		OverlapY: overlapY,
	}, true
}

func toInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case float32:
		return int(t), true
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	default:
		return 0, false
	}
}

func (s *SegmentationService) FinalizeExperiment(ctx context.Context, taskID string, mergeMs int) error {
	if s.expRepo == nil {
		return nil
	}

	experiment, err := s.expRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}

	logs, err := s.loadChunkLogsFromRedis(ctx, taskID)
	if err != nil {
		s.logger.Warning("Failed to load chunk logs from Redis for %s: %v", taskID, err)
	}

	avgQueueWait := averageInt(logs, func(log chunkRuntimeLog) int { return log.QueueWaitMs })
	avgInference := averageInt(logs, func(log chunkRuntimeLog) int { return log.InferenceMs })
	totalMs := int(time.Since(experiment.SubmittedAt).Milliseconds())

	if err := s.expRepo.UpdateMetrics(taskID, totalMs, avgQueueWait, avgInference, mergeMs); err != nil {
		return err
	}

	for _, log := range logs {
		chunkLog := &entities.SegmentationChunkLog{
			TaskID:       taskID,
			ChunkID:      log.ChunkID,
			WorkerName:   log.WorkerName,
			GPUType:      log.GPUType,
			ClusterMode:  log.ClusterMode,
			QueueWaitMs:  log.QueueWaitMs,
			InferenceMs:  log.InferenceMs,
			TotalChunkMs: log.TotalChunkMs,
		}
		if err := s.expRepo.InsertChunkLog(chunkLog); err != nil {
			s.logger.Warning("Failed to persist chunk log for task %s chunk %d: %v", taskID, log.ChunkID, err)
		}
	}

	return s.expRepo.UpdateStatus(taskID, "completed", "")
}

func (s *SegmentationService) loadChunkLogsFromRedis(ctx context.Context, taskID string) ([]chunkRuntimeLog, error) {
	pattern := fmt.Sprintf("segmentation:chunklog:%s:*", taskID)
	var cursor uint64
	var logs []chunkRuntimeLog

	for {
		keys, nextCursor, err := s.redisClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			payload, err := s.redisClient.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			var logEntry chunkRuntimeLog
			if err := json.Unmarshal([]byte(payload), &logEntry); err != nil {
				continue
			}
			logs = append(logs, logEntry)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].ChunkID < logs[j].ChunkID
	})

	return logs, nil
}

func averageInt(logs []chunkRuntimeLog, selector func(chunkRuntimeLog) int) int {
	if len(logs) == 0 {
		return 0
	}

	total := 0
	for _, log := range logs {
		total += selector(log)
	}

	return total / len(logs)
}

func (s *SegmentationService) ListExperiments(_ context.Context, mode string, limit int) ([]*entities.SegmentationExperiment, error) {
	if s.expRepo == nil {
		return nil, fmt.Errorf("segmentation experiment repository is not configured")
	}
	if limit <= 0 {
		limit = 50
	}

	return s.expRepo.ListByMode(strings.ToLower(strings.TrimSpace(mode)), limit)
}

func (s *SegmentationService) GetExperimentByTaskID(_ context.Context, taskID string) (*entities.SegmentationExperiment, []*entities.SegmentationChunkLog, error) {
	if s.expRepo == nil {
		return nil, nil, fmt.Errorf("segmentation experiment repository is not configured")
	}

	experiment, err := s.expRepo.GetByTaskID(taskID)
	if err != nil {
		return nil, nil, err
	}

	chunkLogs, err := s.expRepo.ListChunkLogsByTaskID(taskID)
	if err != nil {
		return nil, nil, err
	}

	return experiment, chunkLogs, nil
}

func (s *SegmentationService) CompareModes(_ context.Context, modeA, modeB string, limit int) (*CompareStats, error) {
	if s.expRepo == nil {
		return nil, fmt.Errorf("segmentation experiment repository is not configured")
	}

	modeA = strings.ToLower(strings.TrimSpace(modeA))
	modeB = strings.ToLower(strings.TrimSpace(modeB))
	if modeA == "" || modeB == "" {
		return nil, utils.NewBadRequestError("mode_a and mode_b are required")
	}
	if limit <= 0 {
		limit = 100
	}

	experimentsA, err := s.expRepo.ListByMode(modeA, limit)
	if err != nil {
		return nil, err
	}
	experimentsB, err := s.expRepo.ListByMode(modeB, limit)
	if err != nil {
		return nil, err
	}

	latA := extractLatencyValues(experimentsA)
	latB := extractLatencyValues(experimentsB)
	if len(latA) == 0 || len(latB) == 0 {
		return nil, utils.NewBadRequestError("insufficient completed experiments with latency metrics for one or both modes")
	}

	avgA := avgFloat(latA)
	avgB := avgFloat(latB)
	speedup := 0.0
	if avgA > 0 {
		speedup = avgB / avgA
	}

	sampleSize := len(latA)
	if len(latB) < sampleSize {
		sampleSize = len(latB)
	}

	return &CompareStats{
		ModeA:       modeA,
		ModeB:       modeB,
		AvgLatencyA: avgA,
		AvgLatencyB: avgB,
		P50A:        percentile(latA, 0.50),
		P50B:        percentile(latB, 0.50),
		P95A:        percentile(latA, 0.95),
		P95B:        percentile(latB, 0.95),
		P99A:        percentile(latA, 0.99),
		P99B:        percentile(latB, 0.99),
		Speedup:     speedup,
		SampleSize:  sampleSize,
	}, nil
}

func extractLatencyValues(experiments []*entities.SegmentationExperiment) []float64 {
	latencies := make([]float64, 0, len(experiments))
	for _, exp := range experiments {
		if exp == nil || exp.TotalLatencyMs == nil {
			continue
		}
		latencies = append(latencies, float64(*exp.TotalLatencyMs))
	}
	return latencies
}

func avgFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0
	for _, v := range values {
		total += v
	}

	return total / float64(len(values))
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	copyVals := make([]float64, len(values))
	copy(copyVals, values)
	sort.Float64s(copyVals)

	index := int(math.Ceil(p*float64(len(copyVals)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copyVals) {
		index = len(copyVals) - 1
	}

	return copyVals[index]
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
