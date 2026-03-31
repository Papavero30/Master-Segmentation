package repositories

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"gorm.io/gorm"
)

type RoleRepository interface {
	GetByDeviceID(deviceID uint) (*entities.UserRole, error)
	Create(role *entities.UserRole) error
	Update(deviceID uint, role entities.Role, updatedBy uint) error
	Delete(deviceID uint) error
	GetAllRoles() ([]entities.UserRole, error)
	GetDevicesByRole(role entities.Role) ([]entities.UserRole, error)
	HasRole(deviceID uint, role entities.Role) (bool, error)
}

type roleRepositoryImpl struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) RoleRepository {
	return &roleRepositoryImpl{db: db}
}

func (r *roleRepositoryImpl) GetByDeviceID(deviceID uint) (*entities.UserRole, error) {
	var userRole entities.UserRole
	err := r.db.Preload("Device").Where("device_id = ?", deviceID).First(&userRole).Error
	if err != nil {
		return nil, err
	}
	return &userRole, nil
}

func (r *roleRepositoryImpl) Create(role *entities.UserRole) error {
	return r.db.Create(role).Error
}

func (r *roleRepositoryImpl) Update(deviceID uint, role entities.Role, updatedBy uint) error {
	return r.db.Model(&entities.UserRole{}).
		Where("device_id = ?", deviceID).
		Updates(map[string]interface{}{
			"role":       role,
			"updated_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *roleRepositoryImpl) Delete(deviceID uint) error {
	return r.db.Where("device_id = ?", deviceID).Delete(&entities.UserRole{}).Error
}

func (r *roleRepositoryImpl) GetAllRoles() ([]entities.UserRole, error) {
	var roles []entities.UserRole
	err := r.db.Preload("Device").Find(&roles).Error
	return roles, err
}

func (r *roleRepositoryImpl) GetDevicesByRole(role entities.Role) ([]entities.UserRole, error) {
	var roles []entities.UserRole
	err := r.db.Preload("Device").Where("role = ?", role).Find(&roles).Error
	return roles, err
}

func (r *roleRepositoryImpl) HasRole(deviceID uint, role entities.Role) (bool, error) {
	var count int64
	err := r.db.Model(&entities.UserRole{}).
		Where("device_id = ? AND role = ?", deviceID, role).
		Count(&count).Error
	return count > 0, err
}

