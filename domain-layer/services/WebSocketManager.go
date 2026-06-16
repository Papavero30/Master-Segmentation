package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/gorilla/websocket"
	redis "github.com/redis/go-redis/v9"
)

// ProgressUpdate represents a real-time progress update
type ProgressUpdate struct {
	TaskID      string         `json:"task_id"`
	Status      string         `json:"status"`
	Progress    int            `json:"progress"`
	TotalChunks int            `json:"total_chunks"`
	Completed   int            `json:"completed"`
	Message     string         `json:"message"`
	Timestamp   time.Time      `json:"timestamp"`
	ClusterMode string         `json:"cluster_mode"`
	WorkerStats map[string]int `json:"worker_stats"`
}

type chunkLogPayload struct {
	WorkerName string `json:"worker_name"`
}

// WebSocketConnection wraps a websocket connection with metadata
type WebSocketConnection struct {
	conn   *websocket.Conn
	taskID string
	mu     sync.Mutex
}

// WebSocketManager manages WebSocket connections for real-time updates
type WebSocketManager struct {
	connections map[string][]*WebSocketConnection
	mu          sync.RWMutex
	logger      *utils.Logger
	redisClient *redis.Client
	ctx         context.Context
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(logger *utils.Logger, redisClient *redis.Client) *WebSocketManager {
	return &WebSocketManager{
		connections: make(map[string][]*WebSocketConnection),
		logger:      logger,
		redisClient: redisClient,
		ctx:         context.Background(),
	}
}

// Register adds a new WebSocket connection for a task
func (m *WebSocketManager) Register(taskID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	wsConn := &WebSocketConnection{
		conn:   conn,
		taskID: taskID,
	}

	m.connections[taskID] = append(m.connections[taskID], wsConn)
	m.logger.Info("WebSocket registered for task %s (total: %d)", taskID, len(m.connections[taskID]))
}

// Unregister removes a WebSocket connection
func (m *WebSocketManager) Unregister(taskID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	connections := m.connections[taskID]
	for i, wsConn := range connections {
		if wsConn.conn == conn {
			// Remove from slice
			m.connections[taskID] = append(connections[:i], connections[i+1:]...)
			break
		}
	}

	// Clean up empty task entries
	if len(m.connections[taskID]) == 0 {
		delete(m.connections, taskID)
	}

	m.logger.Info("WebSocket unregistered for task %s", taskID)
}

// BroadcastProgress sends progress update to all connections for a task
func (m *WebSocketManager) BroadcastProgress(taskID string, update ProgressUpdate) {
	m.mu.RLock()
	connections := m.connections[taskID]
	m.mu.RUnlock()

	if len(connections) == 0 {
		return
	}

	message, err := json.Marshal(update)
	if err != nil {
		m.logger.Error("Failed to marshal progress update: %v", err)
		return
	}

	// Send to all connections
	for _, wsConn := range connections {
		wsConn.mu.Lock()
		err := wsConn.conn.WriteMessage(websocket.TextMessage, message)
		wsConn.mu.Unlock()

		if err != nil {
			m.logger.Warning("Failed to send WebSocket message: %v", err)
			// Connection will be cleaned up when client disconnects
		}
	}

	m.logger.Debug("Broadcasted progress to %d connections for task %s", len(connections), taskID)
}

// StartProgressMonitor starts monitoring Redis for progress updates
func (m *WebSocketManager) StartProgressMonitor(taskID string, stopChan <-chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond) // Poll every 500ms
	defer ticker.Stop()

	m.logger.Info("Started progress monitor for task %s", taskID)

	for {
		select {
		case <-ticker.C:
			// Get current progress from Redis
			progressKey := "segmentation:progress:" + taskID
			completedKey := "segmentation:completed:" + taskID
			taskKey := "segmentation:task:" + taskID

			progress, err := m.redisClient.Get(m.ctx, progressKey).Int()
			if err != nil && err != redis.Nil {
				m.logger.Error("Failed to get progress: %v", err)
				continue
			}

			completed, err := m.redisClient.Get(m.ctx, completedKey).Int()
			if err != nil && err != redis.Nil {
				completed = 0
			}

			// Get task metadata
			taskJSON, err := m.redisClient.Get(m.ctx, taskKey).Result()
			if err != nil {
				if err == redis.Nil {
					m.logger.Info("Task %s completed or expired, stopping monitor", taskID)
					return
				}
				continue
			}

			var task struct {
				Status      string `json:"status"`
				TotalChunks int    `json:"total_chunks"`
				ClusterMode string `json:"cluster_mode"`
			}
			if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
				m.logger.Error("Failed to unmarshal task: %v", err)
				continue
			}

			workerStats, err := m.collectWorkerStats(taskID)
			if err != nil {
				m.logger.Warning("Failed to collect worker stats for task %s: %v", taskID, err)
				workerStats = map[string]int{}
			}

			clusterMode := task.ClusterMode
			if clusterMode == "" {
				clusterMode = "legacy"
			}

			// Broadcast update
			update := ProgressUpdate{
				TaskID:      taskID,
				Status:      task.Status,
				Progress:    progress,
				TotalChunks: task.TotalChunks,
				Completed:   completed,
				Message:     getProgressMessage(progress, task.Status),
				Timestamp:   time.Now(),
				ClusterMode: clusterMode,
				WorkerStats: workerStats,
			}

			m.BroadcastProgress(taskID, update)

			// Stop if completed or failed
			if task.Status == "completed" || task.Status == "failed" {
				m.logger.Info("Task %s %s, stopping monitor", taskID, task.Status)
				return
			}

		case <-stopChan:
			m.logger.Info("Progress monitor stopped for task %s", taskID)
			return
		}
	}
}

func (m *WebSocketManager) collectWorkerStats(taskID string) (map[string]int, error) {
	stats := make(map[string]int)
	pattern := fmt.Sprintf("segmentation:chunklog:%s:*", taskID)
	var cursor uint64

	for {
		keys, nextCursor, err := m.redisClient.Scan(m.ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			payload, err := m.redisClient.Get(m.ctx, key).Result()
			if err != nil {
				continue
			}

			entry := chunkLogPayload{}
			if err := json.Unmarshal([]byte(payload), &entry); err != nil {
				continue
			}

			if entry.WorkerName == "" {
				continue
			}

			stats[entry.WorkerName]++
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return stats, nil
}

func getProgressMessage(progress int, status string) string {
	switch status {
	case "queued":
		return "Task queued, waiting for processing..."
	case "processing":
		if progress < 25 {
			return "Starting segmentation..."
		} else if progress < 75 {
			return "Processing chunks..."
		} else {
			return "Finalizing results..."
		}
	case "completed":
		return "Segmentation completed successfully"
	case "failed":
		return "Segmentation failed"
	default:
		return "Processing..."
	}
}

// CloseAll closes all WebSocket connections
func (m *WebSocketManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for taskID, connections := range m.connections {
		for _, wsConn := range connections {
			wsConn.conn.Close()
		}
		m.logger.Info("Closed %d connections for task %s", len(connections), taskID)
	}

	m.connections = make(map[string][]*WebSocketConnection)
}
