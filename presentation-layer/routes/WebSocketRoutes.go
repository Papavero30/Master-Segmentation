package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/middleware"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

func RegisterWebSocketRoutes(
	r *mux.Router,
	websocketController *controller.WebSocketController,
	authMiddleware *middleware.AuthMiddleware,
) {
	// WebSocket routes (protected)
	wsRouter := r.PathPrefix("/api/ws").Subrouter()
	wsRouter.Use(authMiddleware.RequireAuth)

	// GET /api/ws/segmentation/:taskId - WebSocket for real-time progress updates
	wsRouter.HandleFunc("/segmentation/{taskId}", websocketController.HandleWebSocket).Methods("GET")
}
