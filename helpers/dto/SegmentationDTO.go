package dto

import "time"

type SegmentRequestDTO struct {
	ScanSID     string `json:"scan_sid" validate:"required"`
	SliceIndex  int    `json:"slice_index" validate:"required,min=0"`
	ClusterMode string `json:"cluster_mode,omitempty" validate:"omitempty,oneof=homogen heterogen"`
}

type ExperimentResponseDTO struct {
	TaskID            string     `json:"task_id"`
	ScanSID           string     `json:"scan_sid"`
	SliceIndex        int        `json:"slice_index"`
	ClusterMode       string     `json:"cluster_mode"`
	ChunkSize         int        `json:"chunk_size"`
	OverlapSize       int        `json:"overlap_size"`
	TotalChunks       int        `json:"total_chunks"`
	SubmittedAt       time.Time  `json:"submitted_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	TotalLatencyMs    *int       `json:"total_latency_ms,omitempty"`
	QueueWaitMs       *int       `json:"queue_wait_ms,omitempty"`
	InferenceMs       *int       `json:"inference_ms,omitempty"`
	MergeMs           *int       `json:"merge_ms,omitempty"`
	DiceScore         *float64   `json:"dice_score,omitempty"`
	HausdorffDistance *float64   `json:"hausdorff_distance,omitempty"`
	Status            string     `json:"status"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type CompareStatsResponseDTO struct {
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
