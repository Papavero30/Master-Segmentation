package repositories

import (
	"errors"
	"fmt"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type ScansRepository interface {
	GetAll(profileFirstUid string, ownerDeviceID uint) ([]entities.Scans, error)
	GetByID(id uint, ownerDeviceID uint) (*entities.Scans, error)
	GetBySid(sid string, ownerDeviceID uint) (*entities.Scans, error)
	GetBySidUnscoped(sid string, ownerDeviceID uint) (*entities.Scans, error)
	GetBySidUnscopedAnyOwner(sid string) (*entities.Scans, error)
	GetBySidAnyOwner(sid string) (*entities.Scans, error)
	Restore(sid string, ownerDeviceID uint) (*entities.Scans, error)
	Create(item entities.ScansCreate) (*entities.Scans, error)
	Update(sid string, ownerDeviceID uint, item entities.ScansUpdate) (*entities.Scans, error)
	Delete(sid string, ownerDeviceID uint) error
	SetUploadStatus(sid string, ownerDeviceID uint, status string, lastError *string) error
	InitExpectedFileCount(sid string, ownerDeviceID uint, expected int, manifestChecksum string) error
	IncrementReceivedFileCount(sid string, ownerDeviceID uint) error
	ResetUploadState(sid string, ownerDeviceID uint) error
	SetReceivedFileCount(sid string, ownerDeviceID uint, count int) error
	SetLastError(sid string, ownerDeviceID uint, lastError string) error
	AddFileHash(sid string, ownerDeviceID uint, relPath, hash string, size int64) error
	ListFileHashes(sid string, ownerDeviceID uint) ([]entities.ScanFile, error)
	HasFile(sid string, ownerDeviceID uint, relPath string) (bool, error)
	GetFile(sid string, ownerDeviceID uint, relPath string) (*entities.ScanFile, error)
	ReplaceFiles(sid string, ownerDeviceID uint, files []entities.ScanFile) error
	UpdateCompressionResult(sid string, ownerDeviceID uint, status, transferSyntax, manifestPath string, originalSize, compressedSize int64, ratio float64, filesProcessed, filesFailed int, compressedAt time.Time) error
	FinalizeScan(sid string, ownerDeviceID uint, rootHash string, finalStatus string) error
	UpdateOwnerDeviceID(sid string, ownerDeviceID uint) error
	IsFileInManifest(sid string, ownerDeviceID uint, relPath string) (bool, error)
}

type scansRepositoryImpl struct {
	db *gorm.DB
}

func NewScansRepository(db *gorm.DB) ScansRepository {
	return &scansRepositoryImpl{db: db}
}

func (r *scansRepositoryImpl) GetAll(profileFirstUid string, ownerDeviceID uint) ([]entities.Scans, error) {
	var items []entities.Scans

	isNum := isNumeric(profileFirstUid)
	fmt.Printf("DEBUG: GetAll called with profileFirstUid='%s', isNumeric=%v\n", profileFirstUid, isNum)

	if isNumeric(profileFirstUid) {
		fmt.Printf("DEBUG: Using patient_id query\n")
		query := r.db.Where("patient_id = ?", profileFirstUid)
		if ownerDeviceID != 0 {
			query = query.Where("owner_device_id = ?", ownerDeviceID)
		}
		err := query.Preload("Profile").Find(&items).Error
		return items, err
	} else {
		fmt.Printf("DEBUG: Using profile_first_uid query\n")
		query := r.db.Where("profile_first_uid = ?", profileFirstUid)
		if ownerDeviceID != 0 {
			query = query.Where("owner_device_id = ?", ownerDeviceID)
		}
		err := query.Preload("Profile").Find(&items).Error
		return items, err
	}
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func (r *scansRepositoryImpl) GetByID(id uint, ownerDeviceID uint) (*entities.Scans, error) {
	var item entities.Scans
	q := r.db.Where("id = ?", id)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	err := q.Preload("Profile").First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}
	return &item, nil
}

func (r *scansRepositoryImpl) GetBySid(sid string, ownerDeviceID uint) (*entities.Scans, error) {
	var item entities.Scans
	q := r.db.Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	err := q.Preload("Profile").First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}
	return &item, nil
}

func (r *scansRepositoryImpl) GetBySidAnyOwner(sid string) (*entities.Scans, error) {
	var item entities.Scans
	err := r.db.Where("sid = ?", sid).Preload("Profile").First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}
	return &item, nil
}

func (r *scansRepositoryImpl) GetBySidUnscoped(sid string, ownerDeviceID uint) (*entities.Scans, error) {
	var item entities.Scans
	q := r.db.Unscoped().Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	err := q.Preload("Profile").First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}
	return &item, nil
}

func (r *scansRepositoryImpl) GetBySidUnscopedAnyOwner(sid string) (*entities.Scans, error) {
	var item entities.Scans
	err := r.db.Unscoped().Where("sid = ?", sid).Preload("Profile").First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}
	return &item, nil
}

func (r *scansRepositoryImpl) Restore(sid string, ownerDeviceID uint) (*entities.Scans, error) {
	var item entities.Scans
	err := r.db.Unscoped().Where("sid = ?", sid).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}

	if !item.DeletedAt.Valid {
		return &item, nil
	}

	err = r.db.Unscoped().Model(&item).Updates(map[string]interface{}{
		"deleted_at":    nil,
		"upload_status": "uploading",
	}).Error
	if err != nil {
		return nil, err
	}

	err = r.db.Where("sid = ?", sid).Preload("Profile").First(&item).Error
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func (r *scansRepositoryImpl) Create(item entities.ScansCreate) (*entities.Scans, error) {
	newItem := entities.Scans{
		Sid:               item.Sid,
		Name:              item.Name,
		PatientName:       item.PatientName,
		Path:              pq.StringArray(item.Path),
		Modality:          entities.Modality(item.Modality),
		StudyDate:         item.StudyDate,
		Thumbnail:         item.Thumbnail,
		LengthSlice:       item.LengthSlice,
		ProfileFirstUid:   item.ProfileFirstUid,
		OwnerDeviceID:     item.OwnerDeviceID,
		UploadStatus:      "uploading",
		ExpectedFileCount: item.ExpectedFileCount,
	}

	err := r.db.Create(&newItem).Error
	if err != nil {
		return nil, err
	}

	err = r.db.Preload("Profile").First(&newItem, "sid = ?", newItem.Sid).Error
	if err != nil {
		return nil, err
	}

	return &newItem, nil
}

func (r *scansRepositoryImpl) Update(sid string, ownerDeviceID uint, item entities.ScansUpdate) (*entities.Scans, error) {
	var existingItem entities.Scans
	q := r.db.Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	err := q.First(&existingItem).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("scan not found")
		}
		return nil, err
	}

	if item.Name != "" {
		existingItem.Name = item.Name
	}
	if item.PatientName != "" {
		existingItem.PatientName = item.PatientName
	}
	if item.Path != nil {
		existingItem.Path = pq.StringArray(item.Path)
	}
	if item.Modality != nil {
		existingItem.Modality = entities.Modality(item.Modality)
	}
	if item.StudyDate != "" {
		existingItem.StudyDate = item.StudyDate
	}
	if item.Thumbnail != "" {
		existingItem.Thumbnail = item.Thumbnail
	}
	if item.LengthSlice != nil {
		existingItem.LengthSlice = *item.LengthSlice
	}
	if item.ProfileFirstUid != "" {
		existingItem.ProfileFirstUid = item.ProfileFirstUid
	}
	if item.OwnerDeviceID != nil {
		existingItem.OwnerDeviceID = *item.OwnerDeviceID
	}

	err = r.db.Save(&existingItem).Error
	if err != nil {
		return nil, err
	}

	err = r.db.Preload("Profile").First(&existingItem, "sid = ?", existingItem.Sid).Error
	if err != nil {
		return nil, err
	}

	return &existingItem, nil
}

func (r *scansRepositoryImpl) Delete(sid string, ownerDeviceID uint) error {
	var item entities.Scans
	q := r.db.Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	err := q.First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("scan not found")
		}
		return err
	}

	return r.db.Delete(&item).Error
}

func (r *scansRepositoryImpl) SetUploadStatus(sid string, ownerDeviceID uint, status string, lastError *string) error {
	updates := map[string]interface{}{"upload_status": status}
	if lastError != nil {
		updates["last_error"] = *lastError
	}
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Updates(updates).Error
}

func (r *scansRepositoryImpl) InitExpectedFileCount(sid string, ownerDeviceID uint, expected int, manifestChecksum string) error {
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Updates(map[string]interface{}{
		"expected_file_count":       expected,
		"manifest_checksum":         manifestChecksum,
		"upload_status":             "uploading",
		"compression_status":        "",
		"transfer_syntax":           "",
		"manifest_path":             "",
		"original_size_bytes":       0,
		"compressed_size_bytes":     0,
		"compression_ratio_percent": 0,
		"files_processed":           0,
		"files_failed":              0,
		"compressed_at":             nil,
		"content_root_hash":         "",
		"is_final":                  false,
		"received_file_count":       0,
		"last_error":                "",
	}).Error
}

func (r *scansRepositoryImpl) IncrementReceivedFileCount(sid string, ownerDeviceID uint) error {
	if ownerDeviceID != 0 {
		return r.db.Exec("UPDATE scans SET received_file_count = received_file_count + 1 WHERE sid = ? AND owner_device_id = ?", sid, ownerDeviceID).Error
	}
	return r.db.Exec("UPDATE scans SET received_file_count = received_file_count + 1 WHERE sid = ?", sid).Error
}

func (r *scansRepositoryImpl) ResetUploadState(sid string, ownerDeviceID uint) error {
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Updates(map[string]interface{}{
		"upload_status":             "uploading",
		"received_file_count":       0,
		"expected_file_count":       nil,
		"last_error":                "",
		"compression_status":        "",
		"transfer_syntax":           "",
		"manifest_path":             "",
		"original_size_bytes":       0,
		"compressed_size_bytes":     0,
		"compression_ratio_percent": 0,
		"files_processed":           0,
		"files_failed":              0,
		"compressed_at":             nil,
	}).Error
}

func (r *scansRepositoryImpl) SetReceivedFileCount(sid string, ownerDeviceID uint, count int) error {
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Update("received_file_count", count).Error
}

func (r *scansRepositoryImpl) SetLastError(sid string, ownerDeviceID uint, lastError string) error {
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Update("last_error", lastError).Error
}

func (r *scansRepositoryImpl) UpdateOwnerDeviceID(sid string, ownerDeviceID uint) error {
	return r.db.Model(&entities.Scans{}).Where("sid = ?", sid).Update("owner_device_id", ownerDeviceID).Error
}

func (r *scansRepositoryImpl) AddFileHash(sid string, ownerDeviceID uint, relPath, hash string, size int64) error {
	var scan entities.Scans
	q := r.db.Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	if err := q.First(&scan).Error; err != nil {
		return err
	}

	var existing entities.ScanFile
	checkErr := r.db.Where("sid = ? AND path = ?", sid, relPath).First(&existing).Error
	if checkErr == nil {
		if existing.Hash == hash && existing.Size == size {
			return nil
		}
		existing.Hash = hash
		existing.Size = size
		existing.OwnerDeviceID = ownerDeviceID
		return r.db.Save(&existing).Error
	}

	sf := entities.ScanFile{ScanID: scan.ID, Sid: sid, Path: relPath, Hash: hash, Size: size, OwnerDeviceID: ownerDeviceID}
	return r.db.Create(&sf).Error
}

func (r *scansRepositoryImpl) ListFileHashes(sid string, ownerDeviceID uint) ([]entities.ScanFile, error) {
	var files []entities.ScanFile
	q := r.db.Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	if err := q.Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func (r *scansRepositoryImpl) HasFile(sid string, ownerDeviceID uint, relPath string) (bool, error) {
	var count int64
	q := r.db.Model(&entities.ScanFile{}).Where("sid = ? AND path = ?", sid, relPath)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *scansRepositoryImpl) GetFile(sid string, ownerDeviceID uint, relPath string) (*entities.ScanFile, error) {
	var f entities.ScanFile
	q := r.db.Where("sid = ? AND path = ?", sid, relPath)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	if err := q.First(&f).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("file not found")
		}
		return nil, err
	}
	return &f, nil
}

func (r *scansRepositoryImpl) ReplaceFiles(sid string, ownerDeviceID uint, files []entities.ScanFile) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var scan entities.Scans
		lookup := tx.Where("sid = ?", sid)
		if ownerDeviceID != 0 {
			lookup = lookup.Where("owner_device_id = ?", ownerDeviceID)
		}
		if err := lookup.First(&scan).Error; err != nil {
			return err
		}
		purge := tx.Where("sid = ?", sid)
		if ownerDeviceID != 0 {
			purge = purge.Where("owner_device_id = ?", ownerDeviceID)
		}
		if err := purge.Delete(&entities.ScanFile{}).Error; err != nil {
			return err
		}
		if len(files) == 0 {
			if err := tx.Model(&scan).Updates(map[string]interface{}{"path": pq.StringArray{}, "received_file_count": 0}).Error; err != nil {
				return err
			}
			return nil
		}
		for i := range files {
			files[i].ScanID = scan.ID
			files[i].Sid = sid
			files[i].OwnerDeviceID = ownerDeviceID
		}
		if err := tx.Create(&files).Error; err != nil {
			return err
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Path
		}
		updates := map[string]interface{}{
			"path":                pq.StringArray(paths),
			"received_file_count": len(files),
		}
		return tx.Model(&scan).Updates(updates).Error
	})
}

func (r *scansRepositoryImpl) UpdateCompressionResult(sid string, ownerDeviceID uint, status, transferSyntax, manifestPath string, originalSize, compressedSize int64, ratio float64, filesProcessed, filesFailed int, compressedAt time.Time) error {
	updates := map[string]interface{}{
		"compression_status":        status,
		"transfer_syntax":           transferSyntax,
		"manifest_path":             manifestPath,
		"original_size_bytes":       originalSize,
		"compressed_size_bytes":     compressedSize,
		"compression_ratio_percent": ratio,
		"files_processed":           filesProcessed,
		"files_failed":              filesFailed,
		"last_error":                "",
	}
	ca := compressedAt
	updates["compressed_at"] = &ca
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Updates(updates).Error
}

func (r *scansRepositoryImpl) FinalizeScan(sid string, ownerDeviceID uint, rootHash string, finalStatus string) error {
	updates := map[string]interface{}{
		"is_final":          true,
		"content_root_hash": rootHash,
	}
	if finalStatus != "" {
		updates["upload_status"] = finalStatus
	}
	q := r.db.Model(&entities.Scans{}).Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		q = q.Where("owner_device_id = ?", ownerDeviceID)
	}
	return q.Updates(updates).Error
}

func (r *scansRepositoryImpl) IsFileInManifest(sid string, ownerDeviceID uint, relPath string) (bool, error) {
	var scan entities.Scans
	query := r.db.Where("sid = ?", sid)
	if ownerDeviceID != 0 {
		query = query.Where("owner_device_id = ?", ownerDeviceID)
	}

	if err := query.First(&scan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("scan not found: %s", sid)
		}
		return false, fmt.Errorf("failed to query scan: %w", err)
	}

	if scan.ManifestChecksum == "" || scan.ExpectedFileCount == nil {
		return true, nil
	}



	return true, nil
}
