package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/dto"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/ratelimit"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"gorm.io/gorm"
)

func sharedInputDir() string {
	if dir := os.Getenv("GDCM_SHARED_INPUT"); dir != "" {
		return dir
	}
	return "/app/data"
}

func sharedOutputDir() string {
	if dir := os.Getenv("GDCM_SHARED_OUTPUT"); dir != "" {
		return dir
	}
	return "/app/data"
}

func removeScanArtifacts(sid string) {
	if strings.TrimSpace(sid) == "" {
		return
	}
	_ = os.RemoveAll(filepath.Join(sharedInputDir(), sid))
	_ = os.RemoveAll(filepath.Join(sharedOutputDir(), sid))
}

func deleteOriginalFilesDir(sid string) error {
	if strings.TrimSpace(sid) == "" {
		return fmt.Errorf("sid is required")
	}

	originalDir := filepath.Join(sharedInputDir(), sid, "original")

	info, err := os.Stat(originalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat original directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("original path exists but is not a directory")
	}

	var totalSize int64
	_ = filepath.Walk(originalDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if err := os.RemoveAll(originalDir); err != nil {
		return fmt.Errorf("failed to remove original directory: %w", err)
	}

	sizeMB := float64(totalSize) / (1024 * 1024)
	_ = fmt.Sprintf("Deleted original files directory for sid %s (freed %.2f MB)", sid, sizeMB)

	return nil
}

var dicomExtensions = map[string]struct{}{
	".dcm":   {},
	".dic":   {},
	".dicm":  {},
	".dicom": {},
	".dc3":   {},
	".ima":   {},
	".file":  {},
}

func isPotentialDicomRelPath(rel string) bool {
	return utils.IsDicomFile(rel)
}

const (
	manifestPathMaxLen       = 255
	manifestHashSuffixLength = 12
)

func normalizeManifestPath(raw string) string {
	return utils.NormalizeAndCleanPath(raw)
}

func shortenManifestPath(original string) string {
	if len(original) <= manifestPathMaxLen {
		return original
	}
	parts := strings.Split(original, "/")
	if len(parts) == 0 {
		return original
	}
	filename := parts[len(parts)-1]
	prefixParts := append([]string{}, parts[:len(parts)-1]...)
	ext := ""
	if dot := strings.LastIndex(filename, "."); dot > 0 {
		ext = filename[dot:]
		filename = filename[:dot]
	}
	hashBytes := sha256.Sum256([]byte(original))
	hashStr := fmt.Sprintf("%x", hashBytes[:])
	if len(hashStr) > manifestHashSuffixLength {
		hashStr = hashStr[:manifestHashSuffixLength]
	}
	available := manifestPathMaxLen - len(ext) - len(hashStr) - 1
	prefix := strings.Join(prefixParts, "/")
	if prefix != "" {
		available -= len(prefix) + 1
	}
	if available < 8 {
		available = 8
	}
	if len(filename) > available {
		filename = filename[:available]
	}
	truncated := filename + "_" + hashStr + ext
	result := truncated
	if prefix != "" {
		result = prefix + "/" + truncated
	}
	for len(result) > manifestPathMaxLen && len(prefixParts) > 0 {
		prefixParts = prefixParts[1:]
		if len(prefixParts) > 0 {
			result = strings.Join(prefixParts, "/") + "/" + truncated
		} else {
			result = truncated
		}
	}
	if len(result) > manifestPathMaxLen {
		result = result[len(result)-manifestPathMaxLen:]
	}
	return result
}

func filterDicomFileNames(names []string) (kept []string, skipped []string) {
	seen := make(map[string]struct{})
	for _, raw := range names {
		normalized := filepath.ToSlash(strings.TrimSpace(raw))
		if normalized == "" {
			skipped = append(skipped, raw)
			continue
		}
		if !isPotentialDicomRelPath(normalized) {
			skipped = append(skipped, raw)
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		kept = append(kept, normalized)
	}
	return kept, skipped
}

type decompressCacheEntry struct {
	preparedAt time.Time
	lastAccess time.Time
	expiresAt  time.Time
}

type ScansService interface {
	GetAllScans(ctx context.Context, profileFirstUid string) (*dto.ScansListResponse, error)
	GetScanByID(ctx context.Context, id int) (*dto.ScansResponse, error)
	GetScanBySid(ctx context.Context, sid string) (*dto.ScansResponse, error)
	CreateScan(ctx context.Context, req *dto.ScansCreateRequest) (*dto.ScansResponse, error)
	UpdateScan(ctx context.Context, sid string, req *dto.ScansUpdateRequest) (*dto.ScansResponse, error)
	DeleteScan(ctx context.Context, sid string) error
	CleanupOldDeletedScans(ctx context.Context, olderThanDays int) (int, error)
	InitUpload(ctx context.Context, sid, profileFirstUid string, expected int, fileNames []string, patientNameHint string) (*dto.ScansResponse, error)
	GetStatus(ctx context.Context, sid string) (*dto.ScansResponse, error)
	UploadFileChunk(ctx context.Context, sid, relPath string, r io.Reader, size int64) error
	UploadZip(ctx context.Context, sid string, r io.Reader, size int64) (*dto.ZipUploadResponse, error)
	UploadBatch(ctx context.Context, sid string, files []*multipart.FileHeader) (*dto.BatchUploadResponse, error)
	CompleteUpload(ctx context.Context, sid string, force bool) (*dto.ScansResponse, error)
	AbortUpload(ctx context.Context, sid string) error
	GetManifest(ctx context.Context, sid string) (*dto.ScanManifestResponse, error)
	ListFiles(ctx context.Context, sid string) (*dto.ScanFilesListResponse, error)
	ResolveFilePath(ctx context.Context, sid, relPath string) (string, *entities.ScanFile, error)
	ProcessCompressionJob(sid string) error
	DecompressFile(ctx context.Context, sid, filename string) ([]byte, error)
	DecompressBatch(ctx context.Context, sid string, filenames []string) (map[string][]byte, error)
	GetCacheStats(ctx context.Context) map[string]interface{}
	ClearScanCache(ctx context.Context, sid string) error
}

type scansServiceImpl struct {
	db                *gorm.DB
	logger            *utils.Logger
	scansRepo         repositories.ScansRepository
	uploadRateLimit   *ratelimit.UploadRateLimiter
	profiles          ProfilesService
	gdcm              *GDCMService
	gdcmClient        *GDCMClient
	workerQueue       *WorkerQueue
	metadataExtractor *DICOMMetadataExtractor
	cacheService      *CacheService
	cacheTTL          time.Duration
	cacheMu           sync.Mutex
	cache             map[string]*decompressCacheEntry
	prepareMu         sync.Map
}

func NewScansService(db *gorm.DB, logger *utils.Logger, scansRepo repositories.ScansRepository, profilesService ProfilesService, gdcmService *GDCMService, gdcmClient *GDCMClient, workerQueue *WorkerQueue) ScansService {
	ttl := 30 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("SCAN_DECOMP_CACHE_TTL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			ttl = parsed
		}
	}

	maxUploadsPerMin := 5
	if raw := strings.TrimSpace(os.Getenv("MAX_UPLOADS_PER_MINUTE")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxUploadsPerMin = parsed
		}
	}

	gdcmBaseURL := "http://brainnav-gdcm:3000"
	if url := strings.TrimSpace(os.Getenv("GDCM_SERVICE_URL")); url != "" {
		gdcmBaseURL = url
	}
	metadataExtractor := NewDICOMMetadataExtractor(gdcmBaseURL, logger)

	cacheService := NewCacheService("/app/cache", 24*time.Hour)

	return &scansServiceImpl{
		db:                db,
		logger:            logger,
		scansRepo:         scansRepo,
		uploadRateLimit:   ratelimit.NewUploadRateLimiter(maxUploadsPerMin),
		profiles:          profilesService,
		gdcm:              gdcmService,
		gdcmClient:        gdcmClient,
		workerQueue:       workerQueue,
		metadataExtractor: metadataExtractor,
		cacheService:      cacheService,
		cacheTTL:          ttl,
		cache:             map[string]*decompressCacheEntry{},
	}
}

func (s *scansServiceImpl) sanitizeManifestPath(sid, rel string) string {
	normalized := normalizeManifestPath(rel)
	if normalized == "" {
		return ""
	}
	if len(normalized) <= manifestPathMaxLen {
		return normalized
	}
	shortened := shortenManifestPath(normalized)
	if s.logger != nil && shortened != normalized {
		s.logger.Info("Truncated manifest path for sid %s from %d to %d characters", sid, len(normalized), len(shortened))
	}
	return shortened
}

func (s *scansServiceImpl) GetAllScans(ctx context.Context, profileFirstUid string) (*dto.ScansListResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	scans, err := s.scansRepo.GetAll(profileFirstUid, ownerID)
	if err != nil {
		return nil, utils.NewInternalServerError("Failed to fetch scans", err)
	}
	resp := make([]dto.ScansResponse, len(scans))
	for i, sc := range scans {
		resp[i] = mapScanToDTO(sc)
	}
	return &dto.ScansListResponse{Scans: resp, Count: len(resp)}, nil
}
func (s *scansServiceImpl) GetScanByID(ctx context.Context, id int) (*dto.ScansResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	sc, err := s.scansRepo.GetByID(uint(id), ownerID)
	if err != nil {
		if err.Error() == "scan not found" {
			return nil, utils.NewNotFoundError("Scan", id)
		}
		return nil, utils.NewInternalServerError("Failed to fetch scan", err)
	}
	r := mapScanToDTO(*sc)
	return &r, nil
}
func (s *scansServiceImpl) GetScanBySid(ctx context.Context, sid string) (*dto.ScansResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		if err.Error() == "scan not found" {
			return nil, utils.NewNotFoundError("Scan", sid)
		}
		return nil, utils.NewInternalServerError("Failed to fetch scan", err)
	}
	r := mapScanToDTO(*sc)
	return &r, nil
}

func (s *scansServiceImpl) CreateScan(ctx context.Context, req *dto.ScansCreateRequest) (*dto.ScansResponse, error) {
	if err := s.validateCreateScanRequest(req); err != nil {
		return nil, err
	}

	ownerID := getOwnerDeviceIDFromCtx(ctx)

	existingScan, err := s.scansRepo.GetBySidUnscoped(req.Sid, ownerID)
	if err == nil && existingScan != nil {
		return s.handleExistingScan(ctx, existingScan, req, ownerID)
	}

	return s.createNewScan(req, ownerID)
}

func (s *scansServiceImpl) validateCreateScanRequest(req *dto.ScansCreateRequest) error {
	v := utils.NewValidator()
	v.ValidateRequired("sid", req.Sid).ValidateRequired("profile_first_uid", req.ProfileFirstUid)

	if v.HasErrors() {
		errs := map[string]string{}
		for _, e := range v.Errors() {
			errs[e.Field] = e.Message
		}
		return utils.NewValidationError("Invalid scan data", errs)
	}

	return nil
}

func (s *scansServiceImpl) handleExistingScan(ctx context.Context, existing *entities.Scans, req *dto.ScansCreateRequest, ownerID uint) (*dto.ScansResponse, error) {
	if existing.DeletedAt.Valid {
		s.logger.Info("Scan with SID %s exists but is soft-deleted, restoring...", req.Sid)
		return s.restoreDeletedScan(existing, req, ownerID)
	}

	s.logger.Info("Scan with SID %s already exists (not deleted), returning existing", req.Sid)
	r := mapScanToDTO(*existing)
	return &r, nil
}

func (s *scansServiceImpl) restoreDeletedScan(existing *entities.Scans, req *dto.ScansCreateRequest, ownerID uint) (*dto.ScansResponse, error) {
	restored, restoreErr := s.scansRepo.Restore(req.Sid, ownerID)
	if restoreErr != nil {
		s.logger.Error("Failed to restore soft-deleted scan %s: %v", req.Sid, restoreErr)
		return nil, utils.NewInternalServerError("Failed to restore scan", restoreErr)
	}

	if resetErr := s.scansRepo.ResetUploadState(req.Sid, ownerID); resetErr != nil {
		s.logger.Error("Failed to reset upload state for restored scan %s: %v", req.Sid, resetErr)
	} else {
		s.logger.Info("Reset upload state for restored scan: %s", req.Sid)
	}

	restored = s.updateRestoredScanFields(restored, req, ownerID)

	s.logger.Info("Successfully restored scan with SID: %s", restored.Sid)
	r := mapScanToDTO(*restored)
	return &r, nil
}

func (s *scansServiceImpl) updateRestoredScanFields(scan *entities.Scans, req *dto.ScansCreateRequest, ownerID uint) *entities.Scans {
	if scan.Name != req.Name || scan.PatientName != req.PatientName {
		update := entities.ScansUpdate{
			Name:        req.Name,
			PatientName: req.PatientName,
		}
		updated, updateErr := s.scansRepo.Update(req.Sid, ownerID, update)
		if updateErr != nil {
			s.logger.Error("Failed to update restored scan %s: %v", req.Sid, updateErr)
			return scan
		}
		return updated
	}
	return scan
}

func (s *scansServiceImpl) createNewScan(req *dto.ScansCreateRequest, ownerID uint) (*dto.ScansResponse, error) {
	scanName := req.Name
	if scanName == "" && req.SeriesName != "" {
		scanName = req.SeriesName
	}

	create := entities.ScansCreate{
		Sid:               req.Sid,
		Name:              scanName,
		PatientName:       req.PatientName,
		Path:              req.Path,
		Modality:          req.Modality,
		StudyDate:         req.StudyDate,
		Thumbnail:         req.Thumbnail,
		LengthSlice:       req.LengthSlice,
		ProfileFirstUid:   req.ProfileFirstUid,
		OwnerDeviceID:     ownerID,
		ExpectedFileCount: req.ExpectedFileCount,
	}

	sc, err := s.scansRepo.Create(create)
	if err != nil {
		if s.isDuplicateKeyError(err) {
			return s.handleDuplicateKeyError(req, ownerID)
		}
		return nil, utils.NewInternalServerError("Failed to create scan", err)
	}

	r := mapScanToDTO(*sc)
	return &r, nil
}

func (s *scansServiceImpl) isDuplicateKeyError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "idx_scans_sid") ||
		strings.Contains(errStr, "SQLSTATE 23505")
}

func (s *scansServiceImpl) handleDuplicateKeyError(req *dto.ScansCreateRequest, ownerID uint) (*dto.ScansResponse, error) {
	s.logger.Info("Duplicate key error for SID %s - checking if soft-deleted, race condition, or different owner", req.Sid)

	existingScan, err := s.scansRepo.GetBySidUnscoped(req.Sid, ownerID)

	if err != nil || existingScan == nil {
		s.logger.Info("Scan not found with current owner %d, checking any owner...", ownerID)
		existingScan, err = s.scansRepo.GetBySidUnscopedAnyOwner(req.Sid)
	}

	if err != nil || existingScan == nil {
		s.logger.Error("Failed to find scan after duplicate key error for SID: %s", req.Sid)
		return nil, utils.NewConflictError("Scan", req.Sid)
	}

	if existingScan.DeletedAt.Valid {
		return s.restoreDeletedScanWithOwnershipUpdate(existingScan, req, ownerID)
	}

	return s.returnExistingScanWithOwnershipUpdate(existingScan, req.Sid, ownerID)
}

func (s *scansServiceImpl) restoreDeletedScanWithOwnershipUpdate(existing *entities.Scans, req *dto.ScansCreateRequest, ownerID uint) (*dto.ScansResponse, error) {
	s.logger.Info("Scan with SID %s is soft-deleted (owner_id: %d), restoring with new owner %d...", req.Sid, existing.OwnerDeviceID, ownerID)

	if existing.OwnerDeviceID != ownerID {
		s.logger.Info("Updating scan ownership from device %d to %d", existing.OwnerDeviceID, ownerID)
		if updateErr := s.scansRepo.UpdateOwnerDeviceID(req.Sid, ownerID); updateErr != nil {
			s.logger.Error("Failed to update scan ownership for SID %s: %v", req.Sid, updateErr)
		}
	}

	restored, restoreErr := s.scansRepo.Restore(req.Sid, ownerID)
	if restoreErr != nil {
		s.logger.Error("Failed to restore soft-deleted scan %s: %v", req.Sid, restoreErr)
		return nil, utils.NewInternalServerError("Failed to restore scan", restoreErr)
	}

	if resetErr := s.scansRepo.ResetUploadState(req.Sid, ownerID); resetErr != nil {
		s.logger.Error("Failed to reset upload state for restored scan %s: %v", req.Sid, resetErr)
	} else {
		s.logger.Info("Reset upload state for restored scan: %s", req.Sid)
	}

	restored = s.updateRestoredScanFields(restored, req, ownerID)

	s.logger.Info("Successfully restored scan with SID: %s (new owner: %d)", restored.Sid, ownerID)
	r := mapScanToDTO(*restored)
	return &r, nil
}

func (s *scansServiceImpl) returnExistingScanWithOwnershipUpdate(existing *entities.Scans, sid string, ownerID uint) (*dto.ScansResponse, error) {
	if existing.OwnerDeviceID != ownerID {
		s.logger.Info("Scan %s exists with different owner %d, updating to %d", sid, existing.OwnerDeviceID, ownerID)
		if updateErr := s.scansRepo.UpdateOwnerDeviceID(sid, ownerID); updateErr != nil {
			s.logger.Error("Failed to update scan ownership for SID %s: %v", sid, updateErr)
		}
		existing, _ = s.scansRepo.GetBySid(sid, ownerID)
	}

	s.logger.Info("Retrieved existing scan: %s (owner: %d)", sid, ownerID)
	r := mapScanToDTO(*existing)
	return &r, nil
}
func (s *scansServiceImpl) UpdateScan(ctx context.Context, sid string, req *dto.ScansUpdateRequest) (*dto.ScansResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	scExisting, _ := s.scansRepo.GetBySid(sid, ownerID)
	if scExisting != nil && scExisting.IsFinal {
		return nil, utils.NewBadRequestError(fmt.Sprintf("Cannot update scan '%s': scan is finalized and cannot be modified", sid))
	}
	upd := entities.ScansUpdate{Name: req.Name, PatientName: req.PatientName, Path: req.Path, Modality: req.Modality, StudyDate: req.StudyDate, Thumbnail: req.Thumbnail, LengthSlice: req.LengthSlice, ProfileFirstUid: req.ProfileFirstUid}
	sc, err := s.scansRepo.Update(sid, ownerID, upd)
	if err != nil {
		if err.Error() == "scan not found" {
			return nil, utils.NewNotFoundError("Scan", sid)
		}
		return nil, utils.NewInternalServerError("Failed to update scan", err)
	}
	r := mapScanToDTO(*sc)
	return &r, nil
}
func (s *scansServiceImpl) DeleteScan(ctx context.Context, sid string) error {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	if sc, err := s.scansRepo.GetBySid(sid, ownerID); err == nil && sc.IsFinal {
		return utils.NewBadRequestError(fmt.Sprintf("Cannot delete scan '%s': scan is finalized (status: %s). Finalized scans cannot be deleted.", sid, sc.UploadStatus))
	}
	if err := s.scansRepo.Delete(sid, ownerID); err != nil {
		if err.Error() == "scan not found" {
			return utils.NewNotFoundError("Scan", sid)
		}
		return utils.NewInternalServerError("Failed to delete scan", err)
	}
	s.purgeCacheForSID(sid)
	removeScanArtifacts(sid)
	s.uploadRateLimit.CleanupSession(sid)
	return nil
}

func (s *scansServiceImpl) CleanupOldDeletedScans(ctx context.Context, olderThanDays int) (int, error) {
	if olderThanDays <= 0 {
		return 0, fmt.Errorf("olderThanDays must be positive, got: %d", olderThanDays)
	}

	cutoffDate := time.Now().AddDate(0, 0, -olderThanDays)
	s.logger.Info("[CleanupOldDeletedScans] Starting cleanup for scans deleted before %s", cutoffDate.Format("2006-01-02 15:04:05"))

	var deletedScans []entities.Scans
	err := s.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Where("deleted_at < ?", cutoffDate).
		Find(&deletedScans).Error

	if err != nil {
		s.logger.Error("[CleanupOldDeletedScans] Failed to query soft-deleted scans: %v", err)
		return 0, fmt.Errorf("failed to query deleted scans: %w", err)
	}

	if len(deletedScans) == 0 {
		s.logger.Info("[CleanupOldDeletedScans] No scans to cleanup (cutoff: %d days)", olderThanDays)
		return 0, nil
	}

	s.logger.Info("[CleanupOldDeletedScans] Found %d scans to permanently delete", len(deletedScans))

	deletedCount := 0
	for _, scan := range deletedScans {
		s.logger.Info("[CleanupOldDeletedScans] Permanently deleting scan: %s (deleted_at: %s)",
			scan.Sid, scan.DeletedAt.Time.Format("2006-01-02 15:04:05"))

		removeScanArtifacts(scan.Sid)

		if err := s.cacheService.DeleteScanCache(scan.Sid); err != nil {
			s.logger.Info("[CleanupOldDeletedScans]   Failed to delete cache for %s: %v", scan.Sid, err)
		}

		if err := s.db.Unscoped().Where("sid = ?", scan.Sid).Delete(&entities.ScanFile{}).Error; err != nil {
			s.logger.Error("[CleanupOldDeletedScans] Failed to delete scan_files for %s: %v", scan.Sid, err)
			continue
		}

		if err := s.db.Unscoped().Delete(&scan).Error; err != nil {
			s.logger.Error("[CleanupOldDeletedScans] Failed to hard delete scan %s: %v", scan.Sid, err)
			continue
		}

		s.logger.Info("[CleanupOldDeletedScans]  Successfully deleted scan: %s", scan.Sid)
		deletedCount++
	}

	s.logger.Info("[CleanupOldDeletedScans] Cleanup complete: %d/%d scans permanently deleted",
		deletedCount, len(deletedScans))

	return deletedCount, nil
}

func (s *scansServiceImpl) InitUpload(ctx context.Context, sid, profileFirstUid string, expected int, fileNames []string, patientNameHint string) (*dto.ScansResponse, error) {
	if strings.TrimSpace(sid) == "" || strings.TrimSpace(profileFirstUid) == "" {
		return nil, utils.NewBadRequestError("Missing required parameters: 'sid' and 'profile_first_uid' must be provided")
	}
	if len(fileNames) == 0 {
		return nil, utils.NewBadRequestError("File list cannot be empty: 'file_names' array must contain at least one file")
	}
	s.purgeCacheForSID(sid)
	dicomNames, skipped := filterDicomFileNames(fileNames)
	if len(skipped) > 0 {
		s.logger.Info("InitUpload skipping %d non-DICOM entries for sid %s", len(skipped), sid)
	}
	sanitized := make([]string, 0, len(dicomNames))
	for _, rel := range dicomNames {
		clean := s.sanitizeManifestPath(sid, rel)
		if clean == "" {
			s.logger.Info("InitUpload dropping empty sanitized path for sid %s", sid)
			continue
		}
		if !isPotentialDicomRelPath(clean) {
			s.logger.Info("InitUpload dropping sanitized non-DICOM path %s for sid %s", clean, sid)
			continue
		}
		sanitized = append(sanitized, clean)
	}
	if len(sanitized) == 0 {
		return nil, utils.NewBadRequestError(fmt.Sprintf("No valid DICOM files found: provided %d files but none have DICOM extensions (.dcm, .dicom, or no extension). Skipped %d non-DICOM files.", len(fileNames), len(skipped)))
	}

	names := make([]string, 0, len(sanitized))
	for _, rel := range sanitized {
		manifestRel := s.sanitizeManifestPath(sid, filepath.ToSlash(filepath.Join("original", rel)))
		if manifestRel != "" {
			names = append(names, manifestRel)
		}
	}

	for i := 0; i < len(names)-1; i++ {
		for j := 0; j < len(names)-i-1; j++ {
			if names[j+1] < names[j] {
				names[j], names[j+1] = names[j+1], names[j]
			}
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(names, "\n")))
	manifest := fmt.Sprintf("%x", sum[:])
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	existing, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return nil, utils.NewInternalServerError("lookup failed", err)
		}
		if any, anyErr := s.scansRepo.GetBySidAnyOwner(sid); anyErr == nil {
			existing = any
		} else {
			if _, cerr := s.scansRepo.Create(entities.ScansCreate{Sid: sid, ProfileFirstUid: profileFirstUid, Name: sid, OwnerDeviceID: ownerID, Path: []string{}}); cerr != nil {
				errMsg := strings.ToLower(cerr.Error())
				if strings.Contains(errMsg, "duplicate key value") || strings.Contains(errMsg, "unique constraint") {
					if dup, derr := s.scansRepo.GetBySidAnyOwner(sid); derr == nil {
						existing = dup
					} else {
						return nil, utils.NewInternalServerError("duplicate sid detected but failed to load existing scan", derr)
					}
				} else {
					return nil, utils.NewInternalServerError("failed to create scan", cerr)
				}
			} else {
				if fetched, ferr := s.scansRepo.GetBySid(sid, ownerID); ferr == nil {
					existing = fetched
				}
			}
		}
	}
	if existing != nil && ownerID != 0 && existing.OwnerDeviceID != ownerID {
		if existing.OwnerDeviceID == 0 {
			if uerr := s.scansRepo.UpdateOwnerDeviceID(sid, ownerID); uerr != nil {
				return nil, utils.NewInternalServerError("failed to claim scan ownership", uerr)
			}
			existing.OwnerDeviceID = ownerID
		} else {
			s.logger.Info("scan %s already owned by device %d, requested owner %d", sid, existing.OwnerDeviceID, ownerID)
			return nil, utils.NewBadRequestError("scan already associated with another device")
		}
	}
	if s.profiles != nil {
		_, _ = s.profiles.UpsertProfileName(profileFirstUid, strings.TrimSpace(patientNameHint))
	}
	expectedCount := len(names)
	if err := s.scansRepo.InitExpectedFileCount(sid, ownerID, expectedCount, manifest); err != nil {
		return nil, utils.NewInternalServerError("failed to init upload", err)
	}

	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		return nil, utils.NewInternalServerError("failed to load scan", err)
	}
	r := mapScanToDTO(*sc)
	return &r, nil
}
func (s *scansServiceImpl) GetStatus(ctx context.Context, sid string) (*dto.ScansResponse, error) {
	return s.GetScanBySid(ctx, sid)
}
func (s *scansServiceImpl) UploadFileChunk(ctx context.Context, sid, relPath string, r io.Reader, size int64) error {
	ownerID := getOwnerDeviceIDFromCtx(ctx)

	s.logger.Info("[UPLOAD-DEBUG] ========== START UPLOAD ==========")
	s.logger.Info("[UPLOAD-DEBUG] SID: %s", sid)
	s.logger.Info("[UPLOAD-DEBUG] RelPath: %s", relPath)
	s.logger.Info("[UPLOAD-DEBUG] Size: %d bytes", size)
	s.logger.Info("[UPLOAD-DEBUG] OwnerID: %d", ownerID)

	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		s.logger.Info("[UPLOAD-DEBUG] GetBySid failed, trying GetBySidAnyOwner...")
		scAny, errAny := s.scansRepo.GetBySidAnyOwner(sid)
		if errAny != nil {
			s.logger.Error("[UPLOAD-DEBUG] GetBySidAnyOwner also failed: %v", errAny)
			return err
		}

		if scAny.OwnerDeviceID != ownerID {
			s.logger.Info("Scan %s has different owner %d, updating to %d for upload", sid, scAny.OwnerDeviceID, ownerID)
			updateErr := s.scansRepo.UpdateOwnerDeviceID(sid, ownerID)
			if updateErr != nil {
				s.logger.Error("Failed to update scan ownership during upload: %v", updateErr)
				return err
			}
			sc, err = s.scansRepo.GetBySid(sid, ownerID)
			if err != nil {
				return err
			}
		} else {
			sc = scAny
		}
	}

	s.logger.Info("[UPLOAD-DEBUG] Scan retrieved successfully: IsFinal=%v, UploadStatus=%s", sc.IsFinal, sc.UploadStatus)

	if sc.IsFinal {
		s.logger.Error("[UPLOAD-DEBUG] Scan is finalized - REJECTING upload")
		return utils.NewBadRequestError(fmt.Sprintf("Cannot upload: scan is already finalized (current status: %s)", sc.UploadStatus))
	}
	if sc.UploadStatus != "uploading" {
		s.logger.Error("[UPLOAD-DEBUG] UploadStatus is not 'uploading' (current: %s) - REJECTING upload", sc.UploadStatus)
		return utils.NewBadRequestError(fmt.Sprintf("Cannot upload: scan status must be 'uploading' (current status: %s). Call InitUpload first or check scan status.", sc.UploadStatus))
	}

	s.logger.Info("[UPLOAD-DEBUG] Scan status checks passed")
	s.logger.Info("[UPLOAD-DEBUG] Validating relPath...")

	if err := utils.ValidatePathSecurity(relPath); err != nil {
		s.logger.Error("[UPLOAD-DEBUG] Path security validation failed: %v", err)
		if pathErr, ok := err.(*utils.PathValidationError); ok {
			return utils.NewBadRequestError(fmt.Sprintf(
				"Invalid file path '%s': %s",
				pathErr.Path,
				pathErr.Reason,
			))
		}
		return utils.NewBadRequestError(fmt.Sprintf("Invalid file path: %v", err))
	}
	s.logger.Info("[UPLOAD-DEBUG] Path security validation passed")

	cleanedPath := utils.NormalizeAndCleanPath(relPath)
	if cleanedPath == "" {
		s.logger.Error("[UPLOAD-DEBUG] Path normalization resulted in empty string")
		return utils.NewBadRequestError("File path is invalid or empty after normalization")
	}
	s.logger.Info("[UPLOAD-DEBUG] Path after normalization: %s", cleanedPath)

	relPath = s.sanitizeManifestPath(sid, cleanedPath)
	if relPath == "" {
		s.logger.Error("[UPLOAD-DEBUG] Path is invalid or too long")
		return utils.NewBadRequestError(fmt.Sprintf("File path is invalid or too long (exceeds %d characters)", utils.MaxPathLength))
	}
	s.logger.Info("[UPLOAD-DEBUG] Final relPath: %s", relPath)

	manifestRel := s.sanitizeManifestPath(sid, filepath.ToSlash(filepath.Join("original", relPath)))
	if manifestRel == "" {
		s.logger.Error("[UPLOAD-DEBUG] ManifestRel is invalid")
		return utils.NewBadRequestError("Internal path validation failed (manifest path invalid)")
	}
	s.logger.Info("[UPLOAD-DEBUG] ManifestRel: %s", manifestRel)

	isValid, err := s.scansRepo.IsFileInManifest(sid, ownerID, manifestRel)
	if err != nil {
		s.logger.Error("[UPLOAD-DEBUG] Failed to validate manifest: %v", err)
		return utils.NewInternalServerError("manifest validation failed", err)
	}
	if !isValid {
		s.logger.Error("[UPLOAD-DEBUG] File not declared in manifest: relPath=%s, manifestRel=%s", relPath, manifestRel)
		return utils.NewBadRequestError(fmt.Sprintf(
			"File '%s' not declared in manifest. Please call InitUpload with correct file list first.",
			relPath,
		))
	}
	s.logger.Info("[UPLOAD-DEBUG] Manifest validation passed")

	if !isPotentialDicomRelPath(relPath) {
		s.logger.Error("[UPLOAD-DEBUG] File is not a DICOM file: %s", relPath)
		return utils.NewBadRequestError("only DICOM files are supported")
	}
	s.logger.Info("[UPLOAD-DEBUG] DICOM file check passed")

	if has, herr := s.scansRepo.HasFile(sid, ownerID, manifestRel); herr == nil && has {
		s.logger.Error("[UPLOAD-DEBUG] Duplicate file detected: %s", manifestRel)
		return utils.NewBadRequestError("duplicate relPath")
	}
	s.logger.Info("[UPLOAD-DEBUG] Duplicate check passed")

	if err := s.uploadRateLimit.CheckRateLimit(sid); err != nil {
		if rateLimitErr, ok := err.(*ratelimit.RateLimitError); ok {
			s.logger.Error("[UPLOAD-DEBUG] Rate limit exceeded for SID: %s - %d/%d uploads, wait %d seconds",
				sid, rateLimitErr.CurrentCount, rateLimitErr.MaxUploadsPerMinute, rateLimitErr.WaitSeconds)

			return utils.NewRateLimitErrorWithDetails(
				fmt.Sprintf("Upload rate limit exceeded for session %s", sid),
				"UPLOAD_RATE_LIMIT_EXCEEDED",
				map[string]interface{}{
					"max_uploads_per_minute": rateLimitErr.MaxUploadsPerMinute,
					"current_count":          rateLimitErr.CurrentCount,
					"wait_seconds":           rateLimitErr.WaitSeconds,
					"session_id":             rateLimitErr.SessionID,
				},
			)
		}
		s.logger.Error("[UPLOAD-DEBUG] Rate limit check error: %v", err)
		return utils.NewTooManyRequestsError("Upload rate limit exceeded")
	}
	s.logger.Info("[UPLOAD-DEBUG] Rate limit check passed")

	if maxStr := os.Getenv("MAX_BYTES_PER_FILE"); maxStr != "" {
		if mv, e := strconv.ParseInt(maxStr, 10, 64); e == nil && size > mv {
			s.logger.Error("[UPLOAD-DEBUG] File too large: %d bytes > %d bytes max", size, mv)
			return utils.NewBadRequestError(fmt.Sprintf("File too large: %d bytes exceeds maximum allowed size of %d bytes (%.2f MB)", size, mv, float64(mv)/(1024*1024)))
		}
	}
	s.logger.Info("[UPLOAD-DEBUG] File size check passed")

	base := sharedInputDir()
	dir := filepath.Join(base, sid, "original", filepath.Dir(relPath))
	dst := filepath.Join(base, sid, "original", relPath)

	s.logger.Info("[UPLOAD-DEBUG] ========== FILE WRITE SECTION ==========")
	s.logger.Info("[UPLOAD-DEBUG] Base dir: %s", base)
	s.logger.Info("[UPLOAD-DEBUG] Target directory: %s", dir)
	s.logger.Info("[UPLOAD-DEBUG] Target file path: %s", dst)
	s.logger.Info("[UPLOAD-DEBUG] Creating directory with MkdirAll...")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.logger.Error("[UPLOAD-DEBUG]  MkdirAll FAILED: %v", err)
		return err
	}
	s.logger.Info("[UPLOAD-DEBUG]  MkdirAll SUCCESS")

	s.logger.Info("[UPLOAD-DEBUG] Creating file with os.Create...")
	f, err := os.Create(dst)
	if err != nil {
		s.logger.Error("[UPLOAD-DEBUG]  os.Create FAILED: %v", err)
		return err
	}
	defer f.Close()
	s.logger.Info("[UPLOAD-DEBUG]  os.Create SUCCESS")

	s.logger.Info("[UPLOAD-DEBUG] Starting io.Copy (size: %d bytes)...", size)
	var h hash.Hash = sha256.New()
	mw := io.MultiWriter(f, h)
	written, err := io.Copy(mw, r)
	if err != nil {
		s.logger.Error("[UPLOAD-DEBUG]  io.Copy FAILED: %v", err)
		return err
	}
	s.logger.Info("[UPLOAD-DEBUG]  io.Copy SUCCESS - Written: %d bytes", written)

	if written != size {
		s.logger.Info("[UPLOAD-DEBUG]   Size mismatch: expected %d, got %d bytes (continuing...)", size, written)
	}

	fileHash := fmt.Sprintf("%x", h.Sum(nil))
	s.logger.Info("[UPLOAD-DEBUG] File hash: %s", fileHash)
	s.logger.Info("[UPLOAD-DEBUG] Adding file to database...")

	if err := s.scansRepo.AddFileHash(sid, ownerID, manifestRel, fileHash, written); err != nil {
		s.logger.Error("[UPLOAD-DEBUG]  AddFileHash FAILED: %v", err)
		return err
	}
	s.logger.Info("[UPLOAD-DEBUG]  AddFileHash SUCCESS")

	s.logger.Info("[UPLOAD-DEBUG] Incrementing received file count...")
	if err := s.scansRepo.IncrementReceivedFileCount(sid, ownerID); err != nil {
		s.logger.Error("[UPLOAD-DEBUG]  IncrementReceivedFileCount FAILED: %v", err)
		return err
	}
	s.logger.Info("[UPLOAD-DEBUG]  IncrementReceivedFileCount SUCCESS")
	s.logger.Info("[UPLOAD-DEBUG] ========== UPLOAD COMPLETE ==========")

	return nil
}

// UploadZip handles bulk upload of DICOM files as a ZIP archive
// This is much faster than uploading files one by one over the network
func (s *scansServiceImpl) UploadZip(ctx context.Context, sid string, r io.Reader, size int64) (*dto.ZipUploadResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	s.logger.Info("[ZIP-UPLOAD] Starting ZIP upload for sid: %s, size: %d bytes", sid, size)

	// Verify scan exists and is in uploading state
	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		return nil, utils.NewNotFoundError("Scan", sid)
	}
	if sc.IsFinal {
		return nil, utils.NewBadRequestError("Cannot upload to finalized scan")
	}
	if sc.UploadStatus != "uploading" {
		return nil, utils.NewBadRequestError(fmt.Sprintf("Scan is not in uploading state (current: %s)", sc.UploadStatus))
	}

	// Read ZIP content into memory
	zipData, err := io.ReadAll(r)
	if err != nil {
		s.logger.Error("[ZIP-UPLOAD] Failed to read ZIP data: %v", err)
		return nil, utils.NewInternalServerError("Failed to read ZIP file", err)
	}

	// Open ZIP archive
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		s.logger.Error("[ZIP-UPLOAD] Failed to open ZIP archive: %v", err)
		return nil, utils.NewBadRequestError("Invalid ZIP file: " + err.Error())
	}

	// Prepare target directory
	targetDir := filepath.Join(sharedInputDir(), sid, "original")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		s.logger.Error("[ZIP-UPLOAD] Failed to create target directory: %v", err)
		return nil, utils.NewInternalServerError("Failed to create upload directory", err)
	}

	// Extract files
	var extractedFiles []string
	var totalBytes int64

	for _, zipFile := range zipReader.File {
		// Skip directories
		if zipFile.FileInfo().IsDir() {
			continue
		}

		// Get just the filename (ignore directory structure in ZIP)
		fileName := filepath.Base(zipFile.Name)

		// Skip hidden files and non-DICOM looking files
		if strings.HasPrefix(fileName, ".") || strings.HasPrefix(fileName, "__") {
			continue
		}

		// Open file in ZIP
		rc, err := zipFile.Open()
		if err != nil {
			s.logger.Warning("[ZIP-UPLOAD] Failed to open file in ZIP: %s - %v", fileName, err)
			continue
		}

		// Read file content
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			s.logger.Warning("[ZIP-UPLOAD] Failed to read file from ZIP: %s - %v", fileName, err)
			continue
		}

		// Write to target directory
		targetPath := filepath.Join(targetDir, fileName)
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			s.logger.Error("[ZIP-UPLOAD] Failed to write file: %s - %v", fileName, err)
			continue
		}

		// Calculate hash and add to database
		hash := sha256.Sum256(content)
		hashStr := fmt.Sprintf("%x", hash[:])
		relPath := "original/" + fileName

		if err := s.scansRepo.AddFileHash(sid, ownerID, relPath, hashStr, int64(len(content))); err != nil {
			s.logger.Warning("[ZIP-UPLOAD] Failed to add file hash: %s - %v", fileName, err)
		}

		extractedFiles = append(extractedFiles, fileName)
		totalBytes += int64(len(content))

		// Increment received file count
		if err := s.scansRepo.IncrementReceivedFileCount(sid, ownerID); err != nil {
			s.logger.Warning("[ZIP-UPLOAD] Failed to increment file count: %v", err)
		}
	}

	s.logger.Info("[ZIP-UPLOAD] Extracted %d files (%d bytes) for sid: %s", len(extractedFiles), totalBytes, sid)

	return &dto.ZipUploadResponse{
		Message:        fmt.Sprintf("Successfully extracted %d files from ZIP", len(extractedFiles)),
		Sid:            sid,
		FilesExtracted: len(extractedFiles),
		FileNames:      extractedFiles,
		TotalBytes:     totalBytes,
	}, nil
}

// UploadBatch handles batch upload of multiple files in a single request
// This reduces HTTP overhead compared to uploading files one by one
func (s *scansServiceImpl) UploadBatch(ctx context.Context, sid string, files []*multipart.FileHeader) (*dto.BatchUploadResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		return nil, err
	}
	if sc.IsFinal {
		return nil, utils.NewBadRequestError("Cannot upload to finalized scan")
	}
	if sc.UploadStatus != "uploading" {
		return nil, utils.NewBadRequestError(fmt.Sprintf("Scan is not in uploading state (current: %s)", sc.UploadStatus))
	}

	// Prepare target directory
	targetDir := filepath.Join(sharedInputDir(), sid, "original")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		s.logger.Error("[BATCH-UPLOAD] Failed to create target directory: %v", err)
		return nil, utils.NewInternalServerError("Failed to create upload directory", err)
	}

	var uploadedFiles []string
	var totalBytes int64

	for _, fileHeader := range files {
		fileName := filepath.Base(fileHeader.Filename)

		// Skip hidden files
		if strings.HasPrefix(fileName, ".") || strings.HasPrefix(fileName, "__") {
			continue
		}

		// Open the uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			s.logger.Warning("[BATCH-UPLOAD] Failed to open file: %s - %v", fileName, err)
			continue
		}

		// Read file content
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			s.logger.Warning("[BATCH-UPLOAD] Failed to read file: %s - %v", fileName, err)
			continue
		}

		// Write to target directory
		targetPath := filepath.Join(targetDir, fileName)
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			s.logger.Error("[BATCH-UPLOAD] Failed to write file: %s - %v", fileName, err)
			continue
		}

		// Calculate hash and add to database
		hash := sha256.Sum256(content)
		hashStr := fmt.Sprintf("%x", hash[:])
		relPath := "original/" + fileName

		if err := s.scansRepo.AddFileHash(sid, ownerID, relPath, hashStr, int64(len(content))); err != nil {
			s.logger.Warning("[BATCH-UPLOAD] Failed to add file hash: %s - %v", fileName, err)
		}

		uploadedFiles = append(uploadedFiles, fileName)
		totalBytes += int64(len(content))

		// Increment received file count
		if err := s.scansRepo.IncrementReceivedFileCount(sid, ownerID); err != nil {
			s.logger.Warning("[BATCH-UPLOAD] Failed to increment file count: %v", err)
		}
	}

	s.logger.Info("[BATCH-UPLOAD] Uploaded %d files (%d bytes) for sid: %s", len(uploadedFiles), totalBytes, sid)

	return &dto.BatchUploadResponse{
		Message:      fmt.Sprintf("Successfully uploaded %d files", len(uploadedFiles)),
		Sid:          sid,
		FilesUploaded: len(uploadedFiles),
		FileNames:    uploadedFiles,
		TotalBytes:   totalBytes,
	}, nil
}

func (s *scansServiceImpl) CompleteUpload(ctx context.Context, sid string, force bool) (*dto.ScansResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		return nil, err
	}
	s.purgeCacheForSID(sid)
	if sc.IsFinal {
		updated, _ := s.scansRepo.GetBySid(sid, ownerID)
		r := mapScanToDTO(*updated)
		return &r, nil
	}
	if sc.ExpectedFileCount == nil {
		return nil, utils.NewBadRequestError("Cannot complete upload: expected file count is missing. Call InitUpload first to declare the number of files.")
	}
	if sc.ReceivedFileCount != *sc.ExpectedFileCount && !force {
		return nil, utils.NewBadRequestError(fmt.Sprintf("Upload incomplete: received %d files but expected %d files. Upload remaining %d files or use force=1 to complete anyway.", sc.ReceivedFileCount, *sc.ExpectedFileCount, *sc.ExpectedFileCount-sc.ReceivedFileCount))
	}
	files, ferr := s.scansRepo.ListFileHashes(sid, ownerID)
	if ferr != nil {
		return nil, utils.NewInternalServerError("list file hashes failed", ferr)
	}
	dicomFiles := make([]entities.ScanFile, 0, len(files))
	for _, f := range files {
		cleanPath := s.sanitizeManifestPath(sid, f.Path)
		if cleanPath == "" {
			s.logger.Info("CompleteUpload dropping empty stored path for sid %s", sid)
			continue
		}
		if !isPotentialDicomRelPath(cleanPath) {
			s.logger.Info("CompleteUpload ignoring non-DICOM path %s for sid %s", cleanPath, sid)
			continue
		}
		f.Path = cleanPath
		dicomFiles = append(dicomFiles, f)
	}
	if len(dicomFiles) == 0 {
		msg := "No valid DICOM files found after filtering. All uploaded files were non-DICOM format."
		_ = s.scansRepo.SetUploadStatus(sid, ownerID, "failed", &msg)
		return nil, utils.NewBadRequestError(fmt.Sprintf("Compression failed: no valid DICOM files detected. Uploaded %d files but none were valid DICOM format.", len(files)))
	}
	if len(dicomFiles) != len(files) {
		if err := s.scansRepo.ReplaceFiles(sid, ownerID, dicomFiles); err != nil {
			return nil, utils.NewInternalServerError("failed to prune non-dicom files", err)
		}
		files = dicomFiles
		sc, _ = s.scansRepo.GetBySid(sid, ownerID)
	} else {
		files = dicomFiles
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	manifestRecalc := func() string {
		sorted := append([]string{}, paths...)
		for i := 0; i < len(sorted)-1; i++ {
			for j := 0; j < len(sorted)-i-1; j++ {
				if sorted[j+1] < sorted[j] {
					sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
				}
			}
		}
		sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
		return fmt.Sprintf("%x", sum[:])
	}()
	if sc.ManifestChecksum != "" && manifestRecalc != sc.ManifestChecksum && !force {
		return nil, utils.NewBadRequestError(fmt.Sprintf("Manifest mismatch: uploaded files don't match the declared file list from InitUpload. Expected checksum: %s, Got: %s. Use force=1 to override.", sc.ManifestChecksum[:16]+"...", manifestRecalc[:16]+"..."))
	}

	s.logger.Info("[CompleteUpload] Extracting DICOM metadata for sid: %s", sid)
	if err := s.extractAndStoreMetadata(sid, ownerID); err != nil {
		s.logger.Warning("Failed to extract metadata for sid %s: %v", sid, err)
	} else {
		s.logger.Info("[CompleteUpload]  Successfully extracted and stored metadata for sid: %s", sid)
	}

	// If workerQueue is available, enqueue compression job
	// Otherwise, mark as completed without compression (compression on-demand via decompress endpoint)
	if s.workerQueue != nil {
		if err := s.scansRepo.SetUploadStatus(sid, ownerID, "pending", nil); err != nil {
			return nil, utils.NewInternalServerError("failed to update status", err)
		}

		s.workerQueue.Enqueue(Job{
			ID:       fmt.Sprintf("compress-%s-%d", sid, time.Now().Unix()),
			Type:     JobTypeCompress,
			SID:      sid,
			Enqueued: time.Now(),
			Attempts: 0,
			MaxRetry: 3,
		})

		s.logger.Info("[CompleteUpload] Compression job enqueued for sid: %s", sid)
	} else {
		// No worker queue - mark upload as completed without compression
		// Original files are available, compression will happen on-demand when user requests decompress
		if err := s.scansRepo.FinalizeScan(sid, ownerID, "", "completed"); err != nil {
			return nil, utils.NewInternalServerError("failed to finalize scan", err)
		}
		s.logger.Info("[CompleteUpload] Upload completed without background compression for sid: %s (workerQueue not configured)", sid)
	}

	updated, _ := s.scansRepo.GetBySid(sid, ownerID)
	r := mapScanToDTO(*updated)
	return &r, nil
}

func (s *scansServiceImpl) ProcessCompressionJob(sid string) error {
	scan, err := s.scansRepo.GetBySidAnyOwner(sid)
	if err != nil {
		s.logger.Error("[ProcessCompressionJob] Failed to get scan %s: %v", sid, err)
		return err
	}
	ownerID := scan.OwnerDeviceID

	s.logger.Info("[ProcessCompressionJob] Starting compression for sid: %s (owner: %d)", sid, ownerID)

	if err := s.scansRepo.SetUploadStatus(sid, ownerID, "compressing", nil); err != nil {
		s.logger.Error("[ProcessCompressionJob] Failed to set status for sid %s: %v", sid, err)
		return err
	}

	inputPath := filepath.Join(sharedInputDir(), sid, "original")
	outputPath := filepath.Join(sharedOutputDir(), sid, "compressed")

	s.logger.Info("[ProcessCompressionJob] Calling GDCM service - Input: %s, Output: %s", inputPath, outputPath)

	compressResp, err := s.gdcmClient.CompressSeries(sid, inputPath, outputPath)
	if err != nil {
		msg := err.Error()
		_ = s.scansRepo.SetUploadStatus(sid, ownerID, "failed", &msg)
		s.logger.Error("[ProcessCompressionJob] GDCM compression failed for sid %s: %v", sid, err)
		return fmt.Errorf("compression failed: %w", err)
	}

	s.logger.Info("[ProcessCompressionJob] GDCM compression successful: %d files processed", compressResp.FilesProcessed)

	var allFiles []entities.ScanFile
	totalSliceCount := 0

	if compressResp.Manifest.TotalGroups > 0 && len(compressResp.Manifest.Groups) > 0 {
		s.logger.Info("[ProcessCompressionJob] Manifest contains %d slice groups for sid %s", compressResp.Manifest.TotalGroups, sid)

		for _, group := range compressResp.Manifest.Groups {
			s.logger.Info("[ProcessCompressionJob] Processing group %d with %d slices", group.GroupIndex, group.SliceCount)
			totalSliceCount += group.SliceCount

			for _, mf := range group.Files {
				cleanPath := s.sanitizeManifestPath(sid, mf.Path)
				if cleanPath == "" {
					continue
				}
				if !isPotentialDicomRelPath(cleanPath) {
					continue
				}
				allFiles = append(allFiles, entities.ScanFile{
					Sid:           sid,
					Path:          cleanPath,
					Hash:          mf.Hash,
					Size:          mf.Size,
					OwnerDeviceID: ownerID,
				})
			}
		}
		s.logger.Info("[ProcessCompressionJob] Extracted %d files from %d groups, total slices: %d for sid %s",
			len(allFiles), compressResp.Manifest.TotalGroups, totalSliceCount, sid)
	} else {
		s.logger.Info("[ProcessCompressionJob] Using legacy flat manifest format for sid %s", sid)
		for _, mf := range compressResp.Manifest.Files {
			cleanPath := s.sanitizeManifestPath(sid, mf.Path)
			if cleanPath == "" {
				continue
			}
			if !isPotentialDicomRelPath(cleanPath) {
				continue
			}
			allFiles = append(allFiles, entities.ScanFile{
				Sid:           sid,
				Path:          cleanPath,
				Hash:          mf.Hash,
				Size:          mf.Size,
				OwnerDeviceID: ownerID,
			})
		}
		totalSliceCount = len(allFiles)
	}

	if len(allFiles) == 0 {
		msg := "no dicom files detected in manifest"
		_ = s.scansRepo.SetUploadStatus(sid, ownerID, "failed", &msg)
		return fmt.Errorf("%s", msg)
	}

	if err := s.scansRepo.ReplaceFiles(sid, ownerID, allFiles); err != nil {
		msg := err.Error()
		_ = s.scansRepo.SetLastError(sid, ownerID, msg)
		return fmt.Errorf("failed to persist manifest files: %w", err)
	}

	sliceCount := totalSliceCount
	if sliceCount == 0 {
		sliceCount = len(allFiles)
	}

	s.logger.Info("[ProcessCompressionJob] Final slice count for sid %s: %d", sid, sliceCount)

	thumbURL := fmt.Sprintf("/api/v1/gdcm/thumbnail?sid=%s", url.QueryEscape(sid))
	if err := s.applySeriesMetadata(sid, ownerID, sliceCount, thumbURL); err != nil {
		msg := err.Error()
		_ = s.scansRepo.SetLastError(sid, ownerID, msg)
		return fmt.Errorf("failed to update scan metadata: %w", err)
	}

	for i := 0; i < len(allFiles)-1; i++ {
		for j := 0; j < len(allFiles)-i-1; j++ {
			if allFiles[j+1].Path < allFiles[j].Path {
				allFiles[j], allFiles[j+1] = allFiles[j+1], allFiles[j]
			}
		}
	}

	agg := sha256.New()
	for _, f := range allFiles {
		agg.Write([]byte(f.Hash))
	}
	rootHash := fmt.Sprintf("%x", agg.Sum(nil))

	compressedAt := time.Now().UTC()
	if ts := strings.TrimSpace(compressResp.Manifest.GeneratedAt); ts != "" {
		if parsed, perr := time.Parse(time.RFC3339, ts); perr == nil {
			compressedAt = parsed
		}
	}

	if err := s.scansRepo.UpdateCompressionResult(sid, ownerID, compressResp.Status, compressResp.TransferSyntax, compressResp.ManifestPath, compressResp.OriginalSizeBytes, compressResp.CompressedSizeBytes, compressResp.CompressionRatioPercent, compressResp.FilesProcessed, compressResp.FilesFailed, compressedAt); err != nil {
		msg := err.Error()
		_ = s.scansRepo.SetLastError(sid, ownerID, msg)
		return fmt.Errorf("failed to update compression metadata: %w", err)
	}

	if err := s.scansRepo.FinalizeScan(sid, ownerID, rootHash, "ready"); err != nil {
		return fmt.Errorf("finalize failed: %w", err)
	}

	s.uploadRateLimit.CleanupSession(sid)

	if err := s.deleteOriginalFiles(sid, ownerID); err != nil {
		s.logger.Warning("Failed to delete original files for sid %s: %v", sid, err)
	} else {
		s.logger.Info(" Successfully deleted original files for sid: %s", sid)
	}

	s.logger.Info("[ProcessCompressionJob] Compression completed successfully for sid: %s", sid)
	return nil
}
func (s *scansServiceImpl) AbortUpload(ctx context.Context, sid string) error {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	if sc, err := s.scansRepo.GetBySid(sid, ownerID); err == nil && sc.IsFinal {
		return utils.NewBadRequestError("scan is finalized")
	}
	removeScanArtifacts(sid)
	if err := s.scansRepo.ResetUploadState(sid, ownerID); err != nil {
		return err
	}
	s.purgeCacheForSID(sid)
	s.uploadRateLimit.CleanupSession(sid)
	return nil
}

func (s *scansServiceImpl) GetManifest(ctx context.Context, sid string) (*dto.ScanManifestResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	sc, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, utils.NewNotFoundError("Scan", sid)
		}
		return nil, utils.NewInternalServerError("failed to lookup scan", err)
	}
	if s.gdcm == nil {
		return nil, utils.NewInternalServerError("compression service unavailable", nil)
	}
	manifest, err := s.gdcm.FetchSeriesManifest(sid)
	if err != nil {
		return nil, utils.NewInternalServerError("failed to fetch manifest", err)
	}

	var allManifestFiles []dto.ScanManifestFile

	if manifest.Manifest.TotalGroups > 0 && len(manifest.Manifest.Groups) > 0 {
		s.logger.Info("[GetManifest] Processing %d groups for sid %s", manifest.Manifest.TotalGroups, sid)
		for _, group := range manifest.Manifest.Groups {
			for _, f := range group.Files {
				cleanPath := s.sanitizeManifestPath(sid, f.Path)
				if cleanPath == "" {
					continue
				}
				allManifestFiles = append(allManifestFiles, dto.ScanManifestFile{
					Path:           cleanPath,
					Size:           f.Size,
					Hash:           f.Hash,
					InstanceNumber: f.InstanceNumber,
					SOPInstanceUID: f.SOPInstanceUID,
				})
			}
		}
	} else {
		for _, f := range manifest.Manifest.Files {
			cleanPath := s.sanitizeManifestPath(sid, f.Path)
			if cleanPath == "" {
				continue
			}
			allManifestFiles = append(allManifestFiles, dto.ScanManifestFile{
				Path:           cleanPath,
				Size:           f.Size,
				Hash:           f.Hash,
				InstanceNumber: f.InstanceNumber,
				SOPInstanceUID: f.SOPInstanceUID,
			})
		}
	}

	stats := manifest.Manifest.Stats
	resp := dto.ScanManifestResponse{
		Sid:            manifest.Sid,
		GeneratedAt:    manifest.Manifest.GeneratedAt,
		TransferSyntax: manifest.Manifest.TransferSyntax,
		ManifestPath:   sc.ManifestPath,
		Files:          allManifestFiles,
		Stats: dto.ScanManifestStats{
			OriginalSizeBytes:       stats.OriginalSizeBytes,
			CompressedSizeBytes:     stats.CompressedSizeBytes,
			CompressionRatioPercent: stats.CompressionRatioPercent,
			FilesProcessed:          stats.FilesProcessed,
			FilesFailed:             stats.FilesFailed,
		},
	}
	return &resp, nil
}

func (s *scansServiceImpl) ListFiles(ctx context.Context, sid string) (*dto.ScanFilesListResponse, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	if _, err := s.scansRepo.GetBySid(sid, ownerID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, utils.NewNotFoundError("Scan", sid)
		}
		return nil, utils.NewInternalServerError("failed to lookup scan", err)
	}
	files, err := s.scansRepo.ListFileHashes(sid, ownerID)
	if err != nil {
		return nil, utils.NewInternalServerError("failed to list files", err)
	}
	out := make([]dto.ScanFileResponse, 0, len(files))
	for _, f := range files {
		cleanPath := s.sanitizeManifestPath(sid, f.Path)
		if cleanPath == "" {
			continue
		}
		out = append(out, dto.ScanFileResponse{Path: cleanPath, Size: f.Size, Hash: f.Hash})
	}
	return &dto.ScanFilesListResponse{Sid: sid, Files: out, Count: len(out)}, nil
}

func (s *scansServiceImpl) ResolveFilePath(ctx context.Context, sid, relPath string) (string, *entities.ScanFile, error) {
	ownerID := getOwnerDeviceIDFromCtx(ctx)
	relPath = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./"))
	if relPath == "" || strings.Contains(relPath, "..") || strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, string(os.PathSeparator)) {
		return "", nil, utils.NewBadRequestError("invalid file path")
	}
	sf, err := s.scansRepo.GetFile(sid, ownerID, relPath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", nil, utils.NewNotFoundError("Scan file", relPath)
		}
		return "", nil, utils.NewInternalServerError("failed to lookup file", err)
	}
	base := sharedOutputDir()
	abs := filepath.Join(base, sid, filepath.FromSlash(sf.Path))

	if strings.HasPrefix(sf.Path, "compressed/") {
		rel := strings.TrimPrefix(sf.Path, "compressed/")
		if err := s.ensureSeriesPreparedForView(sid); err != nil {
			return "", nil, err
		}
		abs = filepath.Join(base, sid, "decompressed", filepath.FromSlash(rel))
	} else if strings.HasPrefix(sf.Path, "original/") {
		rel := strings.TrimPrefix(sf.Path, "original/")
		orig := filepath.Join(sharedInputDir(), sid, filepath.FromSlash(rel))
		if st, err := os.Stat(orig); err == nil && !st.IsDir() {
			abs = orig
		} else {
			if err := s.ensureSeriesPreparedForView(sid); err != nil {
				return "", nil, err
			}
			abs = filepath.Join(base, sid, "decompressed", filepath.FromSlash(rel))
		}
	} else if strings.HasPrefix(sf.Path, "decompressed/") {
		if err := s.ensureSeriesPreparedForView(sid); err != nil {
			return "", nil, err
		}
	}

	st, err := os.Stat(abs)
	if err != nil {
		return "", nil, utils.NewInternalServerError("scan file not available on disk", err)
	}
	if st.IsDir() {
		return "", nil, utils.NewBadRequestError("path is a directory")
	}
	return abs, sf, nil
}

func (s *scansServiceImpl) ensureSeriesPreparedForView(sid string) error {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return utils.NewBadRequestError("sid required")
	}
	if s.gdcm == nil {
		return utils.NewInternalServerError("decompression service unavailable", nil)
	}
	now := time.Now()
	s.evictExpiredCaches(now)
	if s.hasFreshCache(sid, now) {
		return nil
	}
	lock := s.lockForSID(sid)
	lock.Lock()
	defer lock.Unlock()

	now = time.Now()
	if s.hasFreshCache(sid, now) {
		return nil
	}

	if _, err := s.gdcm.SeriesDecompress(sid); err != nil {
		return utils.NewInternalServerError("failed to prepare scan for viewing", err)
	}
	if info, err := os.Stat(filepath.Join(sharedOutputDir(), sid, "decompressed")); err == nil && info.IsDir() {
		s.rememberCachePrepared(sid, time.Now())
	}
	return nil
}

func (s *scansServiceImpl) hasFreshCache(sid string, now time.Time) bool {
	if s.cacheTTL <= 0 {
		return false
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	entry, ok := s.cache[sid]
	if !ok {
		return false
	}
	if now.After(entry.expiresAt) {
		delete(s.cache, sid)
		return false
	}
	entry.lastAccess = now
	entry.expiresAt = now.Add(s.cacheTTL)
	return true
}

func (s *scansServiceImpl) rememberCachePrepared(sid string, now time.Time) {
	if s.cacheTTL <= 0 {
		return
	}
	s.cacheMu.Lock()
	s.cache[sid] = &decompressCacheEntry{preparedAt: now, lastAccess: now, expiresAt: now.Add(s.cacheTTL)}
	s.cacheMu.Unlock()
}

func (s *scansServiceImpl) evictExpiredCaches(now time.Time) {
	if s.cacheTTL <= 0 {
		return
	}
	var expired []string
	s.cacheMu.Lock()
	for sid, entry := range s.cache {
		if now.After(entry.expiresAt) {
			delete(s.cache, sid)
			expired = append(expired, sid)
		}
	}
	s.cacheMu.Unlock()
	for _, sid := range expired {
		_ = os.RemoveAll(filepath.Join(sharedOutputDir(), sid, "decompressed"))
		s.prepareMu.Delete(sid)
	}
}

func (s *scansServiceImpl) purgeCacheForSID(sid string) {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return
	}
	s.cacheMu.Lock()
	delete(s.cache, sid)
	s.cacheMu.Unlock()
	s.prepareMu.Delete(sid)
	_ = os.RemoveAll(filepath.Join(sharedOutputDir(), sid, "decompressed"))
}

func (s *scansServiceImpl) lockForSID(sid string) *sync.Mutex {
	val, _ := s.prepareMu.LoadOrStore(sid, &sync.Mutex{})
	return val.(*sync.Mutex)
}

func (s *scansServiceImpl) extractAndStoreMetadata(sid string, ownerID uint) error {
	if s.metadataExtractor == nil {
		return fmt.Errorf("metadata extractor not available")
	}

	originalDir := filepath.Join(sharedInputDir(), sid, "original")

	if _, err := os.Stat(originalDir); err != nil {
		return fmt.Errorf("original directory not found: %w", err)
	}

	seriesMetadata, err := s.metadataExtractor.ExtractFromDirectory(originalDir)
	if err != nil {
		return fmt.Errorf("failed to extract metadata: %w", err)
	}

	modalityMap := entities.Modality{
		"modality": seriesMetadata.Modality,
	}

	update := entities.ScansUpdate{
		PatientName: seriesMetadata.PatientName,
		Modality:    modalityMap,
		StudyDate:   seriesMetadata.StudyDate,
		LengthSlice: &seriesMetadata.TotalSlices,
	}

	if seriesMetadata.SeriesDescription != "" {
		update.Name = seriesMetadata.SeriesDescription
	}

	if _, err := s.scansRepo.Update(sid, ownerID, update); err != nil {
		return fmt.Errorf("failed to update scan metadata: %w", err)
	}

	s.logger.Info("Extracted metadata for sid %s: Patient=%s, Series=%s, Modality=%s, Slices=%d",
		sid, seriesMetadata.PatientName, seriesMetadata.SeriesDescription,
		seriesMetadata.Modality, seriesMetadata.TotalSlices)

	return nil
}

func (s *scansServiceImpl) deleteOriginalFiles(sid string, ownerID uint) error {
	if err := deleteOriginalFilesDir(sid); err != nil {
		s.logger.LogWithError(utils.ERROR, err, "Failed to delete original files for sid %s (owner: %d)", sid, ownerID)
		return err
	}

	s.logger.Info("Successfully deleted original files for sid %s (owner: %d)", sid, ownerID)
	return nil
}

func (s *scansServiceImpl) applySeriesMetadata(sid string, ownerID uint, sliceCount int, thumbnailPath string) error {
	lengthPtr := sliceCount
	update := entities.ScansUpdate{Thumbnail: strings.TrimSpace(thumbnailPath), LengthSlice: &lengthPtr}
	if update.Thumbnail == "" {
		update.Thumbnail = thumbnailPath
	}
	_, err := s.scansRepo.Update(sid, ownerID, update)
	return err
}

func mapScanToDTO(sc entities.Scans) dto.ScansResponse {
	return dto.ScansResponse{
		Sid:                     sc.Sid,
		Name:                    sc.Name,
		PatientName:             sc.PatientName,
		Path:                    []string(sc.Path),
		Modality:                map[string]interface{}(sc.Modality),
		StudyDate:               sc.StudyDate,
		Thumbnail:               sc.Thumbnail,
		LengthSlice:             sc.LengthSlice,
		ProfileFirstUid:         sc.ProfileFirstUid,
		OwnerDeviceID:           sc.OwnerDeviceID,
		UploadStatus:            sc.UploadStatus,
		ExpectedFileCount:       sc.ExpectedFileCount,
		ReceivedFileCount:       sc.ReceivedFileCount,
		ManifestChecksum:        sc.ManifestChecksum,
		LastError:               sc.LastError,
		CompressionStatus:       sc.CompressionStatus,
		TransferSyntax:          sc.TransferSyntax,
		ManifestPath:            sc.ManifestPath,
		OriginalSizeBytes:       sc.OriginalSizeBytes,
		CompressedSizeBytes:     sc.CompressedSizeBytes,
		CompressionRatioPercent: sc.CompressionRatioPercent,
		FilesProcessed:          sc.FilesProcessed,
		FilesFailed:             sc.FilesFailed,
		CompressedAt:            sc.CompressedAt,
		IsFinal:                 sc.IsFinal,
		ContentRootHash:         sc.ContentRootHash,
	}
}

func getOwnerDeviceIDFromCtx(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	if dev, ok := middlewareDeviceFromContext(ctx); ok {
		return dev.ID
	}
	return 0
}

type deviceLike interface{ GetID() uint }

func middlewareDeviceFromContext(ctx context.Context) (*entities.Device, bool) {
	v := ctx.Value("device")
	if d, ok := v.(*entities.Device); ok {
		return d, true
	}
	return nil, false
}

func (s *scansServiceImpl) DecompressFile(ctx context.Context, sid, filename string) ([]byte, error) {
	if strings.TrimSpace(sid) == "" || strings.TrimSpace(filename) == "" {
		return nil, utils.NewBadRequestError("sid and filename are required")
	}

	if s.cacheService.IsCached(sid, filename) {
		data, err := s.cacheService.ReadCacheFile(sid, filename)
		if err == nil {
			s.logger.Info("[DecompressFile]  Cache HIT for %s/%s", sid, filename)
			return data, nil
		}
		s.logger.Error("[DecompressFile]  Cache read failed for %s/%s: %v", sid, filename, err)
	} else {
		s.logger.Info("[DecompressFile]  Cache MISS for %s/%s", sid, filename)
	}

	ownerID := getOwnerDeviceIDFromCtx(ctx)
	scan, err := s.scansRepo.GetBySid(sid, ownerID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, utils.NewNotFoundError("Scan not found", nil)
		}
		return nil, utils.NewInternalServerError("Failed to fetch scan", err)
	}

	compressedPath := filepath.Join(sharedOutputDir(), sid, "compressed", filename)
	if _, err := os.Stat(compressedPath); os.IsNotExist(err) {
		return nil, utils.NewNotFoundError(fmt.Sprintf("Compressed file not found: %s", filename), nil)
	}

	s.logger.Info("[DecompressFile]  Decompressing %s/%s...", sid, filename)
	decompressedData, err := s.gdcmClient.DecompressFile(sid, filename)
	if err != nil {
		return nil, utils.NewInternalServerError(fmt.Sprintf("Decompression failed for %s", filename), err)
	}

	if err := s.cacheService.CacheFile(sid, filename, decompressedData); err != nil {
		s.logger.Error("[DecompressFile]  Failed to cache file %s/%s: %v", sid, filename, err)
	}

	_ = scan

	s.logger.Info("[DecompressFile]  Successfully decompressed %s/%s (size: %d bytes)", sid, filename, len(decompressedData))
	return decompressedData, nil
}

func (s *scansServiceImpl) DecompressBatch(ctx context.Context, sid string, filenames []string) (map[string][]byte, error) {
	if strings.TrimSpace(sid) == "" || len(filenames) == 0 {
		return nil, utils.NewBadRequestError("sid and filenames are required")
	}

	s.logger.Info("[DecompressBatch]  Decompressing batch of %d files for sid: %s", len(filenames), sid)

	result := make(map[string][]byte)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(filenames))

	maxWorkers := 5
	sem := make(chan struct{}, maxWorkers)

	for _, filename := range filenames {
		wg.Add(1)
		go func(fn string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			data, err := s.DecompressFile(ctx, sid, fn)
			if err != nil {
				errChan <- fmt.Errorf("failed to decompress %s: %w", fn, err)
				return
			}

			mu.Lock()
			result[fn] = data
			mu.Unlock()
		}(filename)
	}

	wg.Wait()
	close(errChan)

	var errors []string
	for err := range errChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return result, fmt.Errorf("batch decompression completed with %d errors: %v", len(errors), errors)
	}

	s.logger.Info("[DecompressBatch]  Successfully decompressed %d files for sid: %s", len(result), sid)
	return result, nil
}

func (s *scansServiceImpl) GetCacheStats(ctx context.Context) map[string]interface{} {
	return s.cacheService.GetCacheStats()
}

func (s *scansServiceImpl) ClearScanCache(ctx context.Context, sid string) error {
	if strings.TrimSpace(sid) == "" {
		return utils.NewBadRequestError("sid is required")
	}

	if err := s.cacheService.DeleteScanCache(sid); err != nil {
		return utils.NewInternalServerError("Failed to clear cache", err)
	}

	s.logger.Info("[ClearScanCache]  Cleared cache for sid: %s", sid)
	return nil
}
