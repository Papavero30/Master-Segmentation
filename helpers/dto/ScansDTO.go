package dto

import "time"

type ScansResponse struct {
	Sid                     string                 `json:"sid"`
	Name                    string                 `json:"name"`
	PatientName             string                 `json:"patient_name"`
	Path                    []string               `json:"path"`
	Modality                map[string]interface{} `json:"modality"`
	StudyDate               string                 `json:"study_date"`
	Thumbnail               string                 `json:"thumbnail"`
	LengthSlice             int                    `json:"length_slice"`
	ProfileFirstUid         string                 `json:"profile_first_uid"`
	OwnerDeviceID           uint                   `json:"owner_device_id"`
	UploadStatus            string                 `json:"upload_status"`
	ExpectedFileCount       *int                   `json:"expected_file_count,omitempty"`
	ReceivedFileCount       int                    `json:"received_file_count"`
	ManifestChecksum        string                 `json:"manifest_checksum,omitempty"`
	LastError               string                 `json:"last_error,omitempty"`
	CompressionStatus       string                 `json:"compression_status,omitempty"`
	TransferSyntax          string                 `json:"transfer_syntax,omitempty"`
	ManifestPath            string                 `json:"manifest_path,omitempty"`
	OriginalSizeBytes       int64                  `json:"original_size_bytes,omitempty"`
	CompressedSizeBytes     int64                  `json:"compressed_size_bytes,omitempty"`
	CompressionRatioPercent float64                `json:"compression_ratio_percent,omitempty"`
	FilesProcessed          int                    `json:"files_processed,omitempty"`
	FilesFailed             int                    `json:"files_failed,omitempty"`
	CompressedAt            *time.Time             `json:"compressed_at,omitempty"`
	IsFinal                 bool                   `json:"is_final"`
	ContentRootHash         string                 `json:"content_root_hash,omitempty"`
}

type ScansCreateRequest struct {
	Sid               string                 `json:"sid" binding:"required"`
	Name              string                 `json:"name"`
	SeriesName        string                 `json:"series_name"`
	SeriesDescription string                 `json:"series_description"`
	PatientName       string                 `json:"patient_name"`
	Path              []string               `json:"path"`
	Modality          map[string]interface{} `json:"modality"`
	StudyDate         string                 `json:"study_date"`
	Thumbnail         string                 `json:"thumbnail"`
	LengthSlice       int                    `json:"length_slice"`
	ProfileFirstUid   string                 `json:"profile_first_uid" binding:"required"`
	OwnerDeviceID     uint                   `json:"owner_device_id"`
	ExpectedFileCount *int                   `json:"expected_file_count,omitempty"`
}

type ScansUpdateRequest struct {
	Name            string                 `json:"name,omitempty"`
	PatientName     string                 `json:"patient_name,omitempty"`
	Path            []string               `json:"path,omitempty"`
	Modality        map[string]interface{} `json:"modality,omitempty"`
	StudyDate       string                 `json:"study_date,omitempty"`
	Thumbnail       string                 `json:"thumbnail,omitempty"`
	LengthSlice     *int                   `json:"length_slice,omitempty"`
	ProfileFirstUid string                 `json:"profile_first_uid,omitempty"`
	OwnerDeviceID   *uint                  `json:"owner_device_id,omitempty"`
}

type ScansListResponse struct {
	Scans []ScansResponse `json:"scans"`
	Count int             `json:"count"`
}

type ScanFileResponse struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type ScanFilesListResponse struct {
	Sid   string             `json:"sid"`
	Files []ScanFileResponse `json:"files"`
	Count int                `json:"count"`
}

type ScanManifestFile struct {
	Path           string `json:"path"`
	Size           int64  `json:"size"`
	Hash           string `json:"hash"`
	InstanceNumber int    `json:"instance_number"`
	SOPInstanceUID string `json:"sop_instance_uid"`
}

type ScanManifestStats struct {
	OriginalSizeBytes       int64   `json:"original_size_bytes"`
	CompressedSizeBytes     int64   `json:"compressed_size_bytes"`
	CompressionRatioPercent float64 `json:"compression_ratio_percent"`
	FilesProcessed          int     `json:"files_processed"`
	FilesFailed             int     `json:"files_failed"`
}

type ScanManifestResponse struct {
	Sid            string             `json:"sid"`
	GeneratedAt    string             `json:"generated_at"`
	TransferSyntax string             `json:"transfer_syntax"`
	ManifestPath   string             `json:"manifest_path"`
	Files          []ScanManifestFile `json:"files"`
	Stats          ScanManifestStats  `json:"stats"`
}

// ZipUploadResponse is the response for bulk ZIP upload
type ZipUploadResponse struct {
	Message       string   `json:"message"`
	Sid           string   `json:"sid"`
	FilesExtracted int      `json:"files_extracted"`
	FileNames     []string `json:"file_names"`
	TotalBytes    int64    `json:"total_bytes"`
}

// BatchUploadResponse is the response for batch file upload (multiple files in one request)
type BatchUploadResponse struct {
	Message      string   `json:"message"`
	Sid          string   `json:"sid"`
	FilesUploaded int      `json:"files_uploaded"`
	FileNames    []string `json:"file_names"`
	TotalBytes   int64    `json:"total_bytes"`
}
