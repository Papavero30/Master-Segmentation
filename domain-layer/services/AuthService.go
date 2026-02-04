package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"gorm.io/gorm"
)

type AuthService interface {
	AuthenticateDevice(fingerprint, deviceName string) (*entities.DeviceLoginResponse, error)
	RefreshToken(refreshToken string) (*entities.DeviceLoginResponse, error)
	RevokeDevice(deviceID uint) error
}

type authServiceImpl struct {
	deviceRepo        repositories.DeviceRepository
	jwtManager        *utils.JWTManager
	encryptionManager *utils.EncryptionManager
	logger            *utils.Logger
	roleService       RoleService
}

func NewAuthService(
	deviceRepo repositories.DeviceRepository,
	jwtManager *utils.JWTManager,
	encryptionManager *utils.EncryptionManager,
	logger *utils.Logger,
	roleService RoleService,
) AuthService {
	return &authServiceImpl{
		deviceRepo:        deviceRepo,
		jwtManager:        jwtManager,
		encryptionManager: encryptionManager,
		logger:            logger,
		roleService:       roleService,
	}
}

func (s *authServiceImpl) AuthenticateDevice(fingerprint, deviceName string) (*entities.DeviceLoginResponse, error) {

	encryptedFingerprint, err := s.encryptionManager.Encrypt(fingerprint)
	if err != nil {
		s.logger.Error("Failed to encrypt fingerprint: %v", err)
		return nil, utils.NewInternalServerError("Authentication failed", err)
	}

	device, err := s.deviceRepo.GetByFingerprint(encryptedFingerprint)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		s.logger.Error("Database error: %v", err)
		return nil, utils.NewInternalServerError("Authentication failed", err)
	}

	if device == nil {
		s.logger.Info("Device not found by fingerprint, trying device_name fallback: %s", deviceName)
		deviceByName, nameErr := s.deviceRepo.GetByDeviceName(deviceName)

		if nameErr == nil && deviceByName != nil {
			s.logger.Info("Found existing device '%s' (ID: %d) with different fingerprint, updating fingerprint", deviceName, deviceByName.ID)

			if updateErr := s.deviceRepo.UpdateFingerprint(deviceByName.ID, encryptedFingerprint); updateErr != nil {
				s.logger.Warning("Failed to update fingerprint for device %d: %v", deviceByName.ID, updateErr)
			}

			device = deviceByName
			device.Fingerprint = encryptedFingerprint

			s.logger.Info(" Device '%s' (ID: %d) fingerprint updated successfully", deviceName, device.ID)
		}
	}

	if device == nil {
		device = &entities.Device{
			Fingerprint: encryptedFingerprint,
			DeviceName:  deviceName,
			IsActive:    true,
		}

		if err := s.deviceRepo.Create(device); err != nil {
			s.logger.Error("Failed to create device: %v", err)
			return nil, utils.NewInternalServerError("Device registration failed", err)
		}
		s.logger.Info("New device registered with ID: %d, Name: %s", device.ID, deviceName)

		if s.roleService != nil {
			if err := s.roleService.EnsureDefaultRole(device.ID); err != nil {
				s.logger.Warning("Failed to ensure default role for device %d: %v", device.ID, err)
			}
		}
	} else {
		if err := s.deviceRepo.UpdateLastAccess(device.ID); err != nil {
			s.logger.Warning("Failed to update last access time: %v", err)
		}
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(device.ID, fingerprint)
	if err != nil {
		s.logger.Error("Failed to generate access token: %v", err)
		return nil, utils.NewInternalServerError("Token generation failed", err)
	}

	refreshToken, err := s.jwtManager.GenerateRefreshToken(device.ID, fingerprint)
	if err != nil {
		s.logger.Error("Failed to generate refresh token: %v", err)
		return nil, utils.NewInternalServerError("Token generation failed", err)
	}

	refreshTokenHash := s.hashToken(refreshToken)
	if err := s.deviceRepo.UpdateRefreshToken(device.ID, refreshTokenHash); err != nil {
		s.logger.Warning("Failed to store refresh token hash: %v", err)
	}

	return &entities.DeviceLoginResponse{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		ExpiresIn:             int(s.jwtManager.AccessTTL().Seconds()),
		AccessTokenExpiresAt:  time.Now().Add(s.jwtManager.AccessTTL()).Unix(),
		RefreshTokenExpiresAt: time.Now().Add(s.jwtManager.RefreshTTL()).Unix(),
		Device:                *device,
	}, nil
}

func (s *authServiceImpl) RefreshToken(refreshToken string) (*entities.DeviceLoginResponse, error) {

	claims, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, utils.NewBadRequestError("Invalid refresh token")
	}

	device, err := s.deviceRepo.GetByID(claims.DeviceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.NewNotFoundError("Device", claims.DeviceID)
		}
		return nil, utils.NewInternalServerError("Device lookup failed", err)
	}

	storedHash := device.RefreshTokenHash
	currentHash := s.hashToken(refreshToken)
	if storedHash != currentHash {
		return nil, utils.NewBadRequestError("Invalid refresh token")
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(device.ID, claims.Fingerprint)
	if err != nil {
		return nil, utils.NewInternalServerError("Token generation failed", err)
	}
	var outRefreshToken = refreshToken
	var refreshExpiresAt = claims.ExpiresAt.Time
	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	remaining := claims.ExpiresAt.Time.Sub(time.Now())
	if remaining < lifetime/4 {
		newRefreshToken, err := s.jwtManager.GenerateRefreshToken(device.ID, claims.Fingerprint)
		if err == nil {
			outRefreshToken = newRefreshToken
			refreshExpiresAt = time.Now().Add(s.jwtManager.RefreshTTL())
			if err := s.deviceRepo.UpdateRefreshToken(device.ID, s.hashToken(newRefreshToken)); err != nil {
				s.logger.Warning("Failed to update refresh token hash: %v", err)
			}
		}
	}

	if err := s.deviceRepo.UpdateLastAccess(device.ID); err != nil {
		s.logger.Warning("Failed to update last access time: %v", err)
	}

	return &entities.DeviceLoginResponse{
		AccessToken:           accessToken,
		RefreshToken:          outRefreshToken,
		ExpiresIn:             int(s.jwtManager.AccessTTL().Seconds()),
		AccessTokenExpiresAt:  time.Now().Add(s.jwtManager.AccessTTL()).Unix(),
		RefreshTokenExpiresAt: refreshExpiresAt.Unix(),
		Device:                *device,
	}, nil
}

func (s *authServiceImpl) RevokeDevice(deviceID uint) error {
	return s.deviceRepo.DeactivateDevice(deviceID)
}

func (s *authServiceImpl) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
