package entities

import "time"

type SegmentationExperiment struct {
	ID                uint       `gorm:"primaryKey"`
	TaskID            string     `json:"task_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	ScanSID           string     `json:"scan_sid" gorm:"type:varchar(64);not null"`
	SliceIndex        int        `json:"slice_index" gorm:"not null"`
	ClusterMode       string     `json:"cluster_mode" gorm:"type:varchar(20);not null"`
	ChunkSize         int        `json:"chunk_size" gorm:"not null"`
	OverlapSize       int        `json:"overlap_size" gorm:"not null"`
	TotalChunks       int        `json:"total_chunks" gorm:"not null"`
	SubmittedAt       time.Time  `json:"submitted_at" gorm:"not null"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	TotalLatencyMs    *int       `json:"total_latency_ms,omitempty"`
	QueueWaitMs       *int       `json:"queue_wait_ms,omitempty"`
	InferenceMs       *int       `json:"inference_ms,omitempty"`
	MergeMs           *int       `json:"merge_ms,omitempty"`
	DiceScore         *float64   `json:"dice_score,omitempty"`
	HausdorffDistance *float64   `json:"hausdorff_distance,omitempty"`
	Status            string     `json:"status" gorm:"type:varchar(20);not null"`
	ErrorMessage      string     `json:"error_message,omitempty" gorm:"type:text"`
	CreatedAt         time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

type SegmentationChunkLog struct {
	ID           uint      `gorm:"primaryKey"`
	TaskID       string    `json:"task_id" gorm:"type:varchar(64);index;not null;uniqueIndex:idx_seg_chunk_task_chunk"`
	ChunkID      int       `json:"chunk_id" gorm:"not null;uniqueIndex:idx_seg_chunk_task_chunk"`
	WorkerName   string    `json:"worker_name" gorm:"type:varchar(128);not null;index"`
	GPUType      string    `json:"gpu_type" gorm:"column:gpu_type;type:varchar(64);index"`
	ClusterMode  string    `json:"cluster_mode" gorm:"type:varchar(20);index"`
	QueueWaitMs  int       `json:"queue_wait_ms"`
	InferenceMs  int       `json:"inference_ms"`
	TotalChunkMs int       `json:"total_chunk_ms"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}
