package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/middleware"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

func RegisterSegmentationRoutes(
	r *mux.Router,
	segmentationController *controller.SegmentationController,
	authMiddleware *middleware.AuthMiddleware,
) {
	// Segmentation routes (protected)
	segmentationRouter := r.PathPrefix("/api/segmentation").Subrouter()
	segmentationRouter.Use(authMiddleware.RequireAuth)

	// POST /api/segmentation/segment - Initiate segmentation
	segmentationRouter.HandleFunc("/segment", segmentationController.SegmentSlice).Methods("POST", "OPTIONS")

	// GET /api/segmentation/status/:taskId - Get task status
	segmentationRouter.HandleFunc("/status/{taskId}", segmentationController.GetTaskStatus).Methods("GET", "OPTIONS")

	// GET /api/segmentation/result/:taskId - Download segmentation result
	segmentationRouter.HandleFunc("/result/{taskId}", segmentationController.GetResult).Methods("GET", "OPTIONS")

	// DELETE /api/segmentation/task/:taskId - Cancel task
	segmentationRouter.HandleFunc("/task/{taskId}", segmentationController.CancelTask).Methods("DELETE", "OPTIONS")

	// GET /api/segmentation/tasks - List all tasks (admin only)
	// segmentationRouter.HandleFunc("/tasks", segmentationController.ListTasks).Methods("GET", "OPTIONS")
}
