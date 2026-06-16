package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/dto"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/gorilla/mux"
)

type SegmentationController struct {
	BaseController
	segmentationService *services.SegmentationService
}

func NewSegmentationController(
	logger *utils.Logger,
	segmentationService *services.SegmentationService,
) *SegmentationController {
	return &SegmentationController{
		BaseController: BaseController{
			logger: logger,
		},
		segmentationService: segmentationService,
	}
}

// SegmentSliceResponse represents the response for slice segmentation
type SegmentSliceResponse struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	TotalChunks int    `json:"total_chunks,omitempty"`
}

// TaskStatusResponse represents the status of a segmentation task
type TaskStatusResponse struct {
	TaskID      string `json:"task_id"`
	ScanSID     string `json:"scan_sid"`
	SliceIndex  int    `json:"slice_index"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"` // 0-100
	TotalChunks int    `json:"total_chunks"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// SegmentSlice handles POST /api/segmentation/segment
func (c *SegmentationController) SegmentSlice(w http.ResponseWriter, r *http.Request) {
	var req dto.SegmentRequestDTO

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.logger.Error("Failed to decode request: %v", err)
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request body"))
		return
	}

	// Validate request
	if req.ScanSID == "" {
		c.RespondWithError(w, utils.NewBadRequestError("scan_sid is required"))
		return
	}
	if req.SliceIndex < 0 {
		c.RespondWithError(w, utils.NewBadRequestError("slice_index must be >= 0"))
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.ClusterMode))
	if mode != "" && mode != "homogen" && mode != "heterogen" {
		c.RespondWithError(w, utils.NewBadRequestError("cluster_mode must be one of: homogen, heterogen"))
		return
	}

	c.logger.Info("Segmentation request: scan=%s, slice=%d, cluster_mode=%s", req.ScanSID, req.SliceIndex, mode)

	// Initiate segmentation
	taskID, err := c.segmentationService.SegmentSliceWithMode(r.Context(), req.ScanSID, req.SliceIndex, mode)
	if err != nil {
		c.logger.Error("Failed to initiate segmentation: %v", err)
		if appErr, ok := err.(*utils.AppError); ok {
			c.RespondWithError(w, appErr)
			return
		}
		c.RespondWithError(w, utils.NewInternalServerError("Failed to start segmentation", err))
		return
	}

	response := SegmentSliceResponse{
		TaskID:  taskID,
		Status:  "queued",
		Message: "Segmentation task created successfully",
	}

	c.RespondWithJSON(w, http.StatusAccepted, response)
}

// ListExperiments handles GET /api/segmentation/experiments
func (c *SegmentationController) ListExperiments(w http.ResponseWriter, r *http.Request) {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode != "" && mode != "homogen" && mode != "heterogen" && mode != "legacy" {
		c.RespondWithError(w, utils.NewBadRequestError("mode must be one of: homogen, heterogen, legacy"))
		return
	}

	limit := 50
	if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			c.RespondWithError(w, utils.NewBadRequestError("limit must be a positive integer"))
			return
		}
		if parsedLimit > 500 {
			parsedLimit = 500
		}
		limit = parsedLimit
	}

	experiments, err := c.segmentationService.ListExperiments(r.Context(), mode, limit)
	if err != nil {
		c.logger.Error("Failed to list experiments: %v", err)
		c.RespondWithError(w, utils.NewInternalServerError("Failed to list experiments", err))
		return
	}

	responseItems := make([]dto.ExperimentResponseDTO, 0, len(experiments))
	for _, exp := range experiments {
		if exp == nil {
			continue
		}
		responseItems = append(responseItems, dto.ExperimentResponseDTO{
			TaskID:            exp.TaskID,
			ScanSID:           exp.ScanSID,
			SliceIndex:        exp.SliceIndex,
			ClusterMode:       exp.ClusterMode,
			ChunkSize:         exp.ChunkSize,
			OverlapSize:       exp.OverlapSize,
			TotalChunks:       exp.TotalChunks,
			SubmittedAt:       exp.SubmittedAt,
			CompletedAt:       exp.CompletedAt,
			TotalLatencyMs:    exp.TotalLatencyMs,
			QueueWaitMs:       exp.QueueWaitMs,
			InferenceMs:       exp.InferenceMs,
			MergeMs:           exp.MergeMs,
			DiceScore:         exp.DiceScore,
			HausdorffDistance: exp.HausdorffDistance,
			Status:            exp.Status,
			ErrorMessage:      exp.ErrorMessage,
			CreatedAt:         exp.CreatedAt,
		})
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"experiments": responseItems,
		"count":       len(responseItems),
	})
}

// GetExperimentDetail handles GET /api/segmentation/experiments/:taskId
func (c *SegmentationController) GetExperimentDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := strings.TrimSpace(vars["taskId"])
	if taskID == "" {
		c.RespondWithError(w, utils.NewBadRequestError("task_id is required"))
		return
	}

	experiment, chunkLogs, err := c.segmentationService.GetExperimentByTaskID(r.Context(), taskID)
	if err != nil {
		c.logger.Error("Failed to get experiment detail: %v", err)
		if utils.IsNotFoundError(err) {
			c.RespondWithError(w, utils.NewNotFoundError("Experiment", taskID))
			return
		}
		c.RespondWithError(w, utils.NewInternalServerError("Failed to get experiment detail", err))
		return
	}

	expDTO := dto.ExperimentResponseDTO{
		TaskID:            experiment.TaskID,
		ScanSID:           experiment.ScanSID,
		SliceIndex:        experiment.SliceIndex,
		ClusterMode:       experiment.ClusterMode,
		ChunkSize:         experiment.ChunkSize,
		OverlapSize:       experiment.OverlapSize,
		TotalChunks:       experiment.TotalChunks,
		SubmittedAt:       experiment.SubmittedAt,
		CompletedAt:       experiment.CompletedAt,
		TotalLatencyMs:    experiment.TotalLatencyMs,
		QueueWaitMs:       experiment.QueueWaitMs,
		InferenceMs:       experiment.InferenceMs,
		MergeMs:           experiment.MergeMs,
		DiceScore:         experiment.DiceScore,
		HausdorffDistance: experiment.HausdorffDistance,
		Status:            experiment.Status,
		ErrorMessage:      experiment.ErrorMessage,
		CreatedAt:         experiment.CreatedAt,
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"experiment": expDTO,
		"chunk_logs": chunkLogs,
	})
}

// CompareStats handles GET /api/segmentation/stats/compare
func (c *SegmentationController) CompareStats(w http.ResponseWriter, r *http.Request) {
	modeA := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode_a")))
	modeB := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode_b")))

	if modeA == "" || modeB == "" {
		c.RespondWithError(w, utils.NewBadRequestError("mode_a and mode_b are required"))
		return
	}

	limit := 100
	if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			c.RespondWithError(w, utils.NewBadRequestError("limit must be a positive integer"))
			return
		}
		if parsedLimit > 1000 {
			parsedLimit = 1000
		}
		limit = parsedLimit
	}

	stats, err := c.segmentationService.CompareModes(r.Context(), modeA, modeB, limit)
	if err != nil {
		c.logger.Error("Failed to compare stats: %v", err)
		if appErr, ok := err.(*utils.AppError); ok {
			c.RespondWithError(w, appErr)
			return
		}
		c.RespondWithError(w, utils.NewInternalServerError("Failed to compare segmentation stats", err))
		return
	}

	response := dto.CompareStatsResponseDTO{
		ModeA:       stats.ModeA,
		ModeB:       stats.ModeB,
		AvgLatencyA: stats.AvgLatencyA,
		AvgLatencyB: stats.AvgLatencyB,
		P50A:        stats.P50A,
		P50B:        stats.P50B,
		P95A:        stats.P95A,
		P95B:        stats.P95B,
		P99A:        stats.P99A,
		P99B:        stats.P99B,
		Speedup:     stats.Speedup,
		SampleSize:  stats.SampleSize,
	}

	c.RespondWithJSON(w, http.StatusOK, response)
}

// GetTaskStatus handles GET /api/segmentation/status/:taskId
func (c *SegmentationController) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		c.RespondWithError(w, utils.NewBadRequestError("task_id is required"))
		return
	}

	task, progress, err := c.segmentationService.GetTaskStatus(r.Context(), taskID)
	if err != nil {
		c.logger.Error("Failed to get task status: %v", err)
		c.RespondWithError(w, utils.NewNotFoundError("Task", taskID))
		return
	}

	response := TaskStatusResponse{
		TaskID:      task.TaskID,
		ScanSID:     task.ScanSID,
		SliceIndex:  task.SliceIndex,
		Status:      task.Status,
		Progress:    progress,
		TotalChunks: task.TotalChunks,
		CreatedAt:   task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if !task.CompletedAt.IsZero() {
		response.CompletedAt = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	c.RespondWithJSON(w, http.StatusOK, response)
}

// GetResult handles GET /api/segmentation/result/:taskId
func (c *SegmentationController) GetResult(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		c.RespondWithError(w, utils.NewBadRequestError("task_id is required"))
		return
	}

	// Check task status first
	task, _, err := c.segmentationService.GetTaskStatus(r.Context(), taskID)
	if err != nil {
		c.logger.Error("Failed to get task status: %v", err)
		c.RespondWithError(w, utils.NewNotFoundError("Task", taskID))
		return
	}

	if task.Status != "completed" {
		c.RespondWithError(w, utils.NewBadRequestError("Task not completed yet"))
		return
	}

	// Aggregate and return result
	maskData, err := c.segmentationService.AggregateResults(r.Context(), taskID)
	if err != nil {
		c.logger.Error("Failed to aggregate results: %v", err)
		c.RespondWithError(w, utils.NewInternalServerError("Failed to retrieve segmentation result", err))
		return
	}

	// Return binary mask data
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=segmentation_"+taskID+".bin")
	w.Header().Set("Content-Length", strconv.Itoa(len(maskData)))

	w.WriteHeader(http.StatusOK)
	w.Write(maskData)
}

// CancelTask handles DELETE /api/segmentation/task/:taskId
func (c *SegmentationController) CancelTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		c.RespondWithError(w, utils.NewBadRequestError("task_id is required"))
		return
	}

	// TODO: Implement task cancellation logic
	// - Remove from RabbitMQ queue
	// - Update status in Redis
	// - Clean up partial results

	c.logger.Info("Task cancellation requested: %s", taskID)

	c.RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Task cancellation initiated",
		"task_id": taskID,
	})
}

// ListTasks handles GET /api/segmentation/tasks
func (c *SegmentationController) ListTasks(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement task listing
	// - Query Redis for all tasks
	// - Support pagination
	// - Filter by status, scan_sid, etc.

	c.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"tasks": []interface{}{},
		"total": 0,
	})
}
