package services

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"gorm.io/gorm"
)

type RoleService interface {
	AssignRole(deviceID uint, role entities.Role, assignedBy uint) (*entities.RoleResponse, error)
	UpdateRole(deviceID uint, role entities.Role, updatedBy uint) (*entities.RoleResponse, error)
	RemoveRole(deviceID uint, removedBy uint) error
	GetDeviceRole(deviceID uint) (*entities.RoleResponse, error)
	GetAllRoleAssignments() ([]entities.RoleResponse, error)
	GetDevicesByRole(role entities.Role) ([]entities.RoleResponse, error)
	HasRole(deviceID uint, role entities.Role) (bool, error)
	IsAdmin(deviceID uint) (bool, error)
	IsUser(deviceID uint) (bool, error)
	EnsureDefaultRole(deviceID uint) error
}

type roleServiceImpl struct {
	db         *gorm.DB
	logger     *utils.Logger
	roleRepo   repositories.RoleRepository
	deviceRepo repositories.DeviceRepository
}

func NewRoleService(
	db *gorm.DB,
	logger *utils.Logger,
	roleRepo repositories.RoleRepository,
	deviceRepo repositories.DeviceRepository,
) RoleService {
	return &roleServiceImpl{
		db:         db,
		logger:     logger,
		roleRepo:   roleRepo,
		deviceRepo: deviceRepo,
	}
}

func (s *roleServiceImpl) AssignRole(deviceID uint, role entities.Role, assignedBy uint) (*entities.RoleResponse, error) {
	s.logger.Debug("Assigning role %s to device %d by user %d", role, deviceID, assignedBy)

	if !role.IsValid() {
		return nil, utils.NewBadRequestError("Invalid role specified")
	}

	_, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, utils.NewNotFoundError("device", deviceID)
		}
		return nil, utils.NewInternalServerError("Failed to verify device", err)
	}

	existingRole, err := s.roleRepo.GetByDeviceID(deviceID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, utils.NewInternalServerError("Failed to check existing role", err)
	}

	if existingRole != nil {
		return nil, utils.NewBadRequestError("Device already has a role assigned. Use update instead.")
	}

	userRole := &entities.UserRole{
		DeviceID:  deviceID,
		Role:      role,
		CreatedBy: assignedBy,
	}

	err = s.roleRepo.Create(userRole)
	if err != nil {
		s.logger.Error("Failed to assign role: %v", err)
		return nil, utils.NewInternalServerError("Failed to assign role", err)
	}

	s.logger.Info("Successfully assigned role %s to device %d", role, deviceID)
	return s.mapRoleToResponse(*userRole), nil
}

func (s *roleServiceImpl) UpdateRole(deviceID uint, role entities.Role, updatedBy uint) (*entities.RoleResponse, error) {
	s.logger.Debug("Updating role for device %d to %s by user %d", deviceID, role, updatedBy)

	if !role.IsValid() {
		return nil, utils.NewBadRequestError("Invalid role specified")
	}

	_, err := s.roleRepo.GetByDeviceID(deviceID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, utils.NewNotFoundError("role assignment", deviceID)
		}
		return nil, utils.NewInternalServerError("Failed to get existing role", err)
	}

	err = s.roleRepo.Update(deviceID, role, updatedBy)
	if err != nil {
		s.logger.Error("Failed to update role: %v", err)
		return nil, utils.NewInternalServerError("Failed to update role", err)
	}

	updatedRole, err := s.roleRepo.GetByDeviceID(deviceID)
	if err != nil {
		return nil, utils.NewInternalServerError("Failed to get updated role", err)
	}

	s.logger.Info("Successfully updated role for device %d to %s", deviceID, role)
	return s.mapRoleToResponse(*updatedRole), nil
}

func (s *roleServiceImpl) RemoveRole(deviceID uint, removedBy uint) error {
	s.logger.Debug("Removing role for device %d by user %d", deviceID, removedBy)

	_, err := s.roleRepo.GetByDeviceID(deviceID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return utils.NewNotFoundError("role assignment", deviceID)
		}
		return utils.NewInternalServerError("Failed to get role", err)
	}

	err = s.roleRepo.Delete(deviceID)
	if err != nil {
		s.logger.Error("Failed to remove role: %v", err)
		return utils.NewInternalServerError("Failed to remove role", err)
	}

	s.logger.Info("Successfully removed role for device %d", deviceID)
	return nil
}

func (s *roleServiceImpl) GetDeviceRole(deviceID uint) (*entities.RoleResponse, error) {
	role, err := s.roleRepo.GetByDeviceID(deviceID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, utils.NewNotFoundError("role assignment", deviceID)
		}
		return nil, utils.NewInternalServerError("Failed to get role", err)
	}

	return s.mapRoleToResponse(*role), nil
}

func (s *roleServiceImpl) GetAllRoleAssignments() ([]entities.RoleResponse, error) {
	roles, err := s.roleRepo.GetAllRoles()
	if err != nil {
		return nil, utils.NewInternalServerError("Failed to get role assignments", err)
	}

	responses := make([]entities.RoleResponse, len(roles))
	for i, role := range roles {
		responses[i] = *s.mapRoleToResponse(role)
	}

	return responses, nil
}

func (s *roleServiceImpl) GetDevicesByRole(role entities.Role) ([]entities.RoleResponse, error) {
	if !role.IsValid() {
		return nil, utils.NewBadRequestError("Invalid role specified")
	}

	roles, err := s.roleRepo.GetDevicesByRole(role)
	if err != nil {
		return nil, utils.NewInternalServerError("Failed to get devices by role", err)
	}

	responses := make([]entities.RoleResponse, len(roles))
	for i, r := range roles {
		responses[i] = *s.mapRoleToResponse(r)
	}

	return responses, nil
}

func (s *roleServiceImpl) HasRole(deviceID uint, role entities.Role) (bool, error) {
	return s.roleRepo.HasRole(deviceID, role)
}

func (s *roleServiceImpl) IsAdmin(deviceID uint) (bool, error) {
	return s.HasRole(deviceID, entities.RoleAdmin)
}

func (s *roleServiceImpl) IsUser(deviceID uint) (bool, error) {
	return s.HasRole(deviceID, entities.RoleUser)
}

func (s *roleServiceImpl) EnsureDefaultRole(deviceID uint) error {
	_, err := s.roleRepo.GetByDeviceID(deviceID)
	if err == nil {
		return nil
	}

	if err != gorm.ErrRecordNotFound {
		return utils.NewInternalServerError("Failed to check existing role", err)
	}

	userRole := &entities.UserRole{
		DeviceID: deviceID,
		Role:     entities.RoleUser,
	}

	err = s.roleRepo.Create(userRole)
	if err != nil {
		s.logger.Error("Failed to create default role for device %d: %v", deviceID, err)
		return utils.NewInternalServerError("Failed to create default role", err)
	}

	s.logger.Info("Created default user role for device %d", deviceID)
	return nil
}

func (s *roleServiceImpl) mapRoleToResponse(role entities.UserRole) *entities.RoleResponse {
	response := &entities.RoleResponse{
		ID:        role.ID,
		DeviceID:  role.DeviceID,
		Role:      role.Role,
		CreatedAt: role.CreatedAt,
		UpdatedAt: role.UpdatedAt,
		CreatedBy: role.CreatedBy,
	}

	if role.Device.ID != 0 {
		response.DeviceName = role.Device.DeviceName
		response.Fingerprint = role.Device.Fingerprint
	}

	return response
}
