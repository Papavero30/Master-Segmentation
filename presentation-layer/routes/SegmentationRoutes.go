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
	registerSegmentationRoutesForPrefix(r.PathPrefix("/segmentation").Subrouter(), segmentationController, authMiddleware)

	// Backward compatibility for any previous accidental /api/api/segmentation usage.
	registerSegmentationRoutesForPrefix(r.PathPrefix("/api/segmentation").Subrouter(), segmentationController, authMiddleware)
}

func registerSegmentationRoutesForPrefix(
	segmentationRouter *mux.Router,
	segmentationController *controller.SegmentationController,
	authMiddleware *middleware.AuthMiddleware,
) {
	if authMiddleware != nil {
		segmentationRouter.Use(authMiddleware.RequireAuth)
	}

	// POST /api/segmentation/segment - Initiate segmentation
	segmentationRouter.HandleFunc("/segment", segmentationController.SegmentSlice).Methods("POST", "OPTIONS")

	// GET /api/segmentation/status/:taskId - Get task status
	segmentationRouter.HandleFunc("/status/{taskId}", segmentationController.GetTaskStatus).Methods("GET", "OPTIONS")

	// GET /api/segmentation/result/:taskId - Download segmentation result
	segmentationRouter.HandleFunc("/result/{taskId}", segmentationController.GetResult).Methods("GET", "OPTIONS")

	// DELETE /api/segmentation/task/:taskId - Cancel task
	segmentationRouter.HandleFunc("/task/{taskId}", segmentationController.CancelTask).Methods("DELETE", "OPTIONS")

	// GET /api/segmentation/experiments - List experiment history
	segmentationRouter.HandleFunc("/experiments", segmentationController.ListExperiments).Methods("GET", "OPTIONS")

	// GET /api/segmentation/experiments/:taskId - Get experiment details + chunk logs
	segmentationRouter.HandleFunc("/experiments/{taskId}", segmentationController.GetExperimentDetail).Methods("GET", "OPTIONS")

	// GET /api/segmentation/stats/compare?mode_a=...&mode_b=...
	segmentationRouter.HandleFunc("/stats/compare", segmentationController.CompareStats).Methods("GET", "OPTIONS")

	// GET /api/segmentation/tasks - List all tasks (admin only)
	// segmentationRouter.HandleFunc("/tasks", segmentationController.ListTasks).Methods("GET", "OPTIONS")
}
