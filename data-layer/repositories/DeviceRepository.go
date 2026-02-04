package repositories

import (
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"gorm.io/gorm"
)

type DeviceRepository interface {
	GetByFingerprint(fingerprint string) (*entities.Device, error)
	GetByDeviceName(deviceName string) (*entities.Device, error)
	UpdateFingerprint(id uint, fingerprint string) error
	Create(device *entities.Device) error
	UpdateLastAccess(id uint) error
	UpdateRefreshToken(id uint, tokenHash string) error
	DeactivateDevice(id uint) error
	GetByID(id uint) (*entities.Device, error)
	GetByIDWithRole(id uint) (*entities.Device, error)
}

type deviceRepositoryImpl struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) DeviceRepository {
	return &deviceRepositoryImpl{db: db}
}

func (r *deviceRepositoryImpl) GetByFingerprint(fingerprint string) (*entities.Device, error) {
	var device entities.Device
	err := r.db.Where("fingerprint = ? AND is_active = ?", fingerprint, true).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *deviceRepositoryImpl) GetByDeviceName(deviceName string) (*entities.Device, error) {
	var device entities.Device
	err := r.db.Where("device_name = ? AND is_active = ?", deviceName, true).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *deviceRepositoryImpl) UpdateFingerprint(id uint, fingerprint string) error {
	return r.db.Model(&entities.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"fingerprint": fingerprint,
		"updated_at":  time.Now(),
	}).Error
}

func (r *deviceRepositoryImpl) Create(device *entities.Device) error {
	device.CreatedAt = time.Now()
	device.UpdatedAt = time.Now()
	device.LastAccessTime = time.Now()
	return r.db.Create(device).Error
}

func (r *deviceRepositoryImpl) UpdateLastAccess(id uint) error {
	return r.db.Model(&entities.Device{}).Where("id = ?", id).Update("last_access_time", time.Now()).Error
}

func (r *deviceRepositoryImpl) UpdateRefreshToken(id uint, tokenHash string) error {
	return r.db.Model(&entities.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"refresh_token_hash": tokenHash,
		"updated_at":         time.Now(),
	}).Error
}

func (r *deviceRepositoryImpl) DeactivateDevice(id uint) error {
	return r.db.Model(&entities.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"is_active":          false,
		"refresh_token_hash": "",
		"updated_at":         time.Now(),
	}).Error
}

func (r *deviceRepositoryImpl) GetByID(id uint) (*entities.Device, error) {
	var device entities.Device
	err := r.db.Where("id = ? AND is_active = ?", id, true).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *deviceRepositoryImpl) GetByIDWithRole(id uint) (*entities.Device, error) {
	var device entities.Device
	err := r.db.Preload("UserRole").Where("id = ? AND is_active = ?", id, true).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}
