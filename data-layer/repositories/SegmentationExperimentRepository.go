package repositories

import (
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"gorm.io/gorm"
)

type SegmentationExperimentRepository interface {
	Create(experiment *entities.SegmentationExperiment) error
	UpdateStatus(taskID string, status string, errMsg string) error
	UpdateMetrics(taskID string, totalMs, queueMs, inferenceMs, mergeMs int) error
	UpdateScores(taskID string, dice, hausdorff float64) error
	GetByTaskID(taskID string) (*entities.SegmentationExperiment, error)
	ListByMode(mode string, limit int) ([]*entities.SegmentationExperiment, error)
	InsertChunkLog(log *entities.SegmentationChunkLog) error
	ListChunkLogsByTaskID(taskID string) ([]*entities.SegmentationChunkLog, error)
}

type segmentationExperimentRepositoryImpl struct {
	db *gorm.DB
}

func NewSegmentationExperimentRepository(db *gorm.DB) SegmentationExperimentRepository {
	return &segmentationExperimentRepositoryImpl{db: db}
}

func (r *segmentationExperimentRepositoryImpl) Create(experiment *entities.SegmentationExperiment) error {
	return r.db.Create(experiment).Error
}

func (r *segmentationExperimentRepositoryImpl) UpdateStatus(taskID string, status string, errMsg string) error {
	updates := map[string]interface{}{
		"status":        status,
		"error_message": nil,
	}

	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if status == "completed" || status == "failed" {
		updates["completed_at"] = time.Now()
	}

	return r.db.Model(&entities.SegmentationExperiment{}).
		Where("task_id = ?", taskID).
		Updates(updates).Error
}

func (r *segmentationExperimentRepositoryImpl) UpdateMetrics(taskID string, totalMs, queueMs, inferenceMs, mergeMs int) error {
	updates := map[string]interface{}{
		"total_latency_ms": totalMs,
		"queue_wait_ms":    queueMs,
		"inference_ms":     inferenceMs,
		"merge_ms":         mergeMs,
	}

	return r.db.Model(&entities.SegmentationExperiment{}).
		Where("task_id = ?", taskID).
		Updates(updates).Error
}

func (r *segmentationExperimentRepositoryImpl) UpdateScores(taskID string, dice, hausdorff float64) error {
	updates := map[string]interface{}{
		"dice_score":         dice,
		"hausdorff_distance": hausdorff,
	}

	return r.db.Model(&entities.SegmentationExperiment{}).
		Where("task_id = ?", taskID).
		Updates(updates).Error
}

func (r *segmentationExperimentRepositoryImpl) GetByTaskID(taskID string) (*entities.SegmentationExperiment, error) {
	var experiment entities.SegmentationExperiment
	if err := r.db.Where("task_id = ?", taskID).First(&experiment).Error; err != nil {
		return nil, err
	}

	return &experiment, nil
}

func (r *segmentationExperimentRepositoryImpl) ListByMode(mode string, limit int) ([]*entities.SegmentationExperiment, error) {
	var experiments []*entities.SegmentationExperiment

	query := r.db.Order("submitted_at DESC")
	if mode != "" {
		query = query.Where("cluster_mode = ?", mode)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&experiments).Error; err != nil {
		return nil, err
	}

	return experiments, nil
}

func (r *segmentationExperimentRepositoryImpl) InsertChunkLog(log *entities.SegmentationChunkLog) error {
	return r.db.Create(log).Error
}

func (r *segmentationExperimentRepositoryImpl) ListChunkLogsByTaskID(taskID string) ([]*entities.SegmentationChunkLog, error) {
	var logs []*entities.SegmentationChunkLog
	if err := r.db.Where("task_id = ?", taskID).Order("chunk_id ASC").Find(&logs).Error; err != nil {
		return nil, err
	}

	return logs, nil
}
