package entities

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Modality map[string]interface{}

func (m Modality) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *Modality) Scan(value interface{}) error {
	if value == nil {
		*m = make(Modality)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(bytes, m)
}

type Scans struct {
	gorm.Model
	Sid             string         `json:"sid" gorm:"uniqueIndex;not null;type:varchar(255)"`
	Name            string         `json:"name" gorm:"type:varchar(255)"`
	PatientName     string         `json:"patient_name" gorm:"type:varchar(255)"`
	Path            pq.StringArray `json:"path" gorm:"type:text[]"`
	Modality        Modality       `json:"modality" gorm:"type:jsonb"`
	StudyDate       string         `json:"study_date" gorm:"type:varchar(255)"`
	Thumbnail       string         `json:"thumbnail" gorm:"type:text"`
	LengthSlice     int            `json:"length_slice" gorm:"type:integer"`
	ProfileFirstUid string         `json:"profile_first_uid" gorm:"type:varchar(255);not null"`
	OwnerDeviceID   uint           `json:"owner_device_id" gorm:"index;not null;default:0"`

	UploadStatus            string     `json:"upload_status" gorm:"type:varchar(50);default:'pending'"`
	ExpectedFileCount       *int       `json:"expected_file_count"`
	ReceivedFileCount       int        `json:"received_file_count" gorm:"default:0"`
	ManifestChecksum        string     `json:"manifest_checksum" gorm:"type:varchar(128)"`
	LastError               string     `json:"last_error" gorm:"type:text"`
	CompressionStatus       string     `json:"compression_status" gorm:"type:varchar(50)"`
	TransferSyntax          string     `json:"transfer_syntax" gorm:"type:varchar(128)"`
	ManifestPath            string     `json:"manifest_path" gorm:"type:text"`
	OriginalSizeBytes       int64      `json:"original_size_bytes"`
	CompressedSizeBytes     int64      `json:"compressed_size_bytes"`
	CompressionRatioPercent float64    `json:"compression_ratio_percent"`
	FilesProcessed          int        `json:"files_processed"`
	FilesFailed             int        `json:"files_failed"`
	CompressedAt            *time.Time `json:"compressed_at"`
	IsFinal         bool   `json:"is_final" gorm:"default:false"`
	ContentRootHash string `json:"content_root_hash" gorm:"type:char(64)"`

	Profile Profiles `json:"profile" gorm:"foreignKey:ProfileFirstUid;references:FirstUid;constraint:OnDelete:CASCADE"`
}

type ScansCreate struct {
	Sid               string                 `json:"sid"`
	Name              string                 `json:"name"`
	PatientName       string                 `json:"patient_name"`
	Path              []string               `json:"path"`
	Modality          map[string]interface{} `json:"modality"`
	StudyDate         string                 `json:"study_date"`
	Thumbnail         string                 `json:"thumbnail"`
	LengthSlice       int                    `json:"length_slice"`
	ProfileFirstUid   string                 `json:"profile_first_uid"`
	OwnerDeviceID     uint                   `json:"owner_device_id"`
	ExpectedFileCount *int                   `json:"expected_file_count,omitempty"`
}

type ScansUpdate struct {
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

type ScanFile struct {
	gorm.Model
	ScanID        uint   `json:"scan_id" gorm:"index"`
	Sid           string `json:"sid" gorm:"type:varchar(255);uniqueIndex:idx_scanfile_sid_path"`
	Path          string `json:"path" gorm:"type:text;not null;uniqueIndex:idx_scanfile_sid_path"`
	Hash          string `json:"hash" gorm:"type:char(64);not null"`
	Size          int64  `json:"size" gorm:"not null"`
	OwnerDeviceID uint   `json:"owner_device_id" gorm:"index"`
}
