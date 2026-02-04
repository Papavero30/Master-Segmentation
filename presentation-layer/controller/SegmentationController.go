package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
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

// SegmentSliceRequest represents the request body for slice segmentation
type SegmentSliceRequest struct {
	ScanSID    string `json:"scan_sid" validate:"required"`
	SliceIndex int    `json:"slice_index" validate:"required,min=0"`
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
	var req SegmentSliceRequest

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

	c.logger.Info("Segmentation request: scan=%s, slice=%d", req.ScanSID, req.SliceIndex)

	// Initiate segmentation
	taskID, err := c.segmentationService.SegmentSlice(r.Context(), req.ScanSID, req.SliceIndex)
	if err != nil {
		c.logger.Error("Failed to initiate segmentation: %v", err)
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
