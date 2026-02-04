package entities

import (
	"time"
)

type Device struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	Fingerprint      string    `json:"fingerprint" gorm:"uniqueIndex;not null"`
	DeviceName       string    `json:"device_name"`
	IsActive         bool      `json:"is_active" gorm:"default:true"`
	LastAccessTime   time.Time `json:"last_access_time"`
	RefreshTokenHash string    `json:"-" gorm:"column:refresh_token_hash"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	UserRole *UserRole `json:"user_role,omitempty" gorm:"foreignKey:DeviceID"`
}

type DeviceCreateRequest struct {
	Fingerprint string `json:"fingerprint" binding:"required"`
	DeviceName  string `json:"device_name"`
}

type DeviceLoginResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int    `json:"expires_in"`
	AccessTokenExpiresAt  int64  `json:"access_token_expires_at"`
	RefreshTokenExpiresAt int64  `json:"refresh_token_expires_at"`
	Device                Device `json:"device"`
}

func (d *Device) GetID() uint {
	return d.ID
}
