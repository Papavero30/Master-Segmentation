package entities

import (
	"time"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

func (r Role) IsValid() bool {
	return r == RoleUser || r == RoleAdmin
}

func (r Role) String() string {
	return string(r)
}

type UserRole struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	DeviceID  uint      `json:"device_id" gorm:"not null;index"`
	Role      Role      `json:"role" gorm:"type:varchar(20);not null;default:'user'"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy uint      `json:"created_by,omitempty"`

	Device Device `json:"device" gorm:"foreignKey:DeviceID"`
}

type RoleCreateRequest struct {
	DeviceID uint `json:"device_id" binding:"required"`
	Role     Role `json:"role" binding:"required"`
}

type RoleUpdateRequest struct {
	Role Role `json:"role" binding:"required"`
}

type RoleResponse struct {
	ID          uint      `json:"id"`
	DeviceID    uint      `json:"device_id"`
	Role        Role      `json:"role"`
	DeviceName  string    `json:"device_name,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   uint      `json:"created_by,omitempty"`
}
