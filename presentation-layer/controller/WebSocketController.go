package controller

import (
	"net/http"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type WebSocketController struct {
	BaseController
	wsManager *services.WebSocketManager
}

func NewWebSocketController(
	logger *utils.Logger,
	wsManager *services.WebSocketManager,
) *WebSocketController {
	return &WebSocketController{
		BaseController: BaseController{
			logger: logger,
		},
		wsManager: wsManager,
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// TODO: Restrict origins in production
		return true
	},
}

// HandleWebSocket handles WebSocket connections for real-time progress updates
func (c *WebSocketController) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		c.logger.Error("Missing taskId in WebSocket request")
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Error("Failed to upgrade WebSocket: %v", err)
		return
	}

	c.logger.Info("WebSocket connection established for task %s", taskID)

	// Register connection
	c.wsManager.Register(taskID, conn)

	// Start progress monitor in goroutine
	stopChan := make(chan struct{})
	go c.wsManager.StartProgressMonitor(taskID, stopChan)

	// Handle connection lifecycle
	defer func() {
		close(stopChan)
		c.wsManager.Unregister(taskID, conn)
		conn.Close()
		c.logger.Info("WebSocket connection closed for task %s", taskID)
	}()

	// Set up ping/pong for connection health
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Keep connection alive and listen for client messages
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Warning("WebSocket error: %v", err)
			}
			break
		}

		// Echo back for keepalive (optional)
		if messageType == websocket.TextMessage {
			c.logger.Debug("Received WebSocket message: %s", string(message))
		}
	}
}
