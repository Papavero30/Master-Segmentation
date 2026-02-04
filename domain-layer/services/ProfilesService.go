package services

import (
	"strings"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/dto"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"gorm.io/gorm"
)

type ProfilesService interface {
	GetAllProfiles() (*dto.ProfilesListResponse, error)
	GetProfileByID(id int) (*dto.ProfilesResponse, error)
	GetProfileByFirstUid(firstUid string) (*dto.ProfilesResponse, error)
	CreateProfile(request *dto.ProfilesCreateRequest) (*dto.ProfilesResponse, error)
	UpdateProfile(firstUid string, request *dto.ProfilesUpdateRequest) (*dto.ProfilesResponse, error)
	DeleteProfile(firstUid string) error
	UpsertProfileName(firstUid, name string) (*dto.ProfilesResponse, error)
}

type profilesServiceImpl struct {
	db           *gorm.DB
	logger       *utils.Logger
	profilesRepo repositories.ProfilesRepository
	scansRepo    repositories.ScansRepository
}

func NewProfilesService(db *gorm.DB, logger *utils.Logger, profilesRepo repositories.ProfilesRepository, scansRepo repositories.ScansRepository) ProfilesService {
	return &profilesServiceImpl{
		db:           db,
		logger:       logger,
		profilesRepo: profilesRepo,
		scansRepo:    scansRepo,
	}
}

func (s *profilesServiceImpl) GetAllProfiles() (*dto.ProfilesListResponse, error) {
	s.logger.Debug("Fetching all profiles")

	profiles, err := s.profilesRepo.GetAll()
	if err != nil {
		s.logger.Error("Failed to fetch profiles: %v", err)
		return nil, utils.NewInternalServerError("Failed to fetch profiles", err)
	}

	profileResponses := make([]dto.ProfilesResponse, len(profiles))
	for i, profile := range profiles {
		profileResponses[i] = mapProfileToDTO(profile)
	}

	s.logger.Info("Successfully fetched %d profiles", len(profileResponses))
	return &dto.ProfilesListResponse{
		Profiles: profileResponses,
		Count:    len(profileResponses),
	}, nil
}

func (s *profilesServiceImpl) GetProfileByID(id int) (*dto.ProfilesResponse, error) {
	s.logger.Debug("Fetching profile with ID: %d", id)

	profile, err := s.profilesRepo.GetByID(uint(id))
	if err != nil {
		if err.Error() == "profile not found" {
			s.logger.Info("Profile not found with ID: %d", id)
			return nil, utils.NewNotFoundError("Profile", id)
		}
		s.logger.Error("Failed to fetch profile with ID: %d: %v", id, err)
		return nil, utils.NewInternalServerError("Failed to fetch profile", err)
	}

	s.logger.Info("Successfully fetched profile with ID: %d", id)
	response := mapProfileToDTO(*profile)
	return &response, nil
}

func (s *profilesServiceImpl) GetProfileByFirstUid(firstUid string) (*dto.ProfilesResponse, error) {
	s.logger.Debug("Fetching profile with FirstUid: %s", firstUid)

	profile, err := s.profilesRepo.GetByFirstUid(firstUid)
	if err != nil {
		if err.Error() == "profile not found" {
			s.logger.Info("Profile not found with FirstUid: %s", firstUid)
			return nil, utils.NewNotFoundError("Profile", firstUid)
		}
		s.logger.Error("Failed to fetch profile with FirstUid: %s: %v", firstUid, err)
		return nil, utils.NewInternalServerError("Failed to fetch profile", err)
	}

	s.logger.Info("Successfully fetched profile with FirstUid: %s", firstUid)
	response := mapProfileToDTO(*profile)
	return &response, nil
}

func (s *profilesServiceImpl) CreateProfile(request *dto.ProfilesCreateRequest) (*dto.ProfilesResponse, error) {
	s.logger.Debug("Creating new profile with FirstUid: %s", request.FirstUid)

	validator := utils.NewValidator()
	validator.ValidateRequired("first_uid", request.FirstUid).
		ValidateRequired("name", request.Name).
		ValidateMaxLength("name", request.Name, 255)

	if validator.HasErrors() {
		errors := make(map[string]string)
		for _, err := range validator.Errors() {
			errors[err.Field] = err.Message
		}

		s.logger.Info("Validation failed for profile creation: %v", errors)
		return nil, utils.NewValidationError("Invalid profile data", errors)
	}

	existingProfile, err := s.profilesRepo.GetByFirstUidUnscoped(request.FirstUid)
	if err == nil && existingProfile != nil {
		if existingProfile.DeletedAt.Valid {
			s.logger.Info("Profile with FirstUid %s exists but is soft-deleted, restoring...", request.FirstUid)

			restored, restoreErr := s.profilesRepo.Restore(request.FirstUid)
			if restoreErr != nil {
				s.logger.Error("Failed to restore soft-deleted profile: %v", restoreErr)
				return nil, utils.NewInternalServerError("Failed to restore profile", restoreErr)
			}

			if restored.Name != request.Name {
				updated, updateErr := s.profilesRepo.Update(request.FirstUid, entities.ProfilesUpdate{
					Name: request.Name,
				})
				if updateErr != nil {
					s.logger.Error("Failed to update restored profile name: %v", updateErr)
				} else {
					restored = updated
				}
			}

			s.logger.Info("Successfully restored profile with FirstUid: %s", restored.FirstUid)
			return &dto.ProfilesResponse{
				ID:       restored.ID,
				FirstUid: restored.FirstUid,
				Name:     restored.Name,
			}, nil
		} else {
			s.logger.Info("Profile with FirstUid %s already exists (not deleted)", request.FirstUid)
			return nil, utils.NewConflictError("Profile", request.FirstUid)
		}
	}

	profileCreate := entities.ProfilesCreate{
		FirstUid: request.FirstUid,
		Name:     request.Name,
	}

	profile, err := s.profilesRepo.Create(profileCreate)
	if err != nil {
		s.logger.Error("Failed to create profile: %v", err)

		if strings.Contains(err.Error(), "duplicate key") ||
			strings.Contains(err.Error(), "idx_profiles_first_uid") ||
			strings.Contains(err.Error(), "SQLSTATE 23505") {
			s.logger.Info("Profile with FirstUid %s was created by concurrent request", request.FirstUid)

			existingProfile, getErr := s.profilesRepo.GetByFirstUid(request.FirstUid)
			if getErr == nil && existingProfile != nil {
				s.logger.Info("Retrieved concurrently created profile: %s", request.FirstUid)
				return &dto.ProfilesResponse{
					ID:       existingProfile.ID,
					FirstUid: existingProfile.FirstUid,
					Name:     existingProfile.Name,
				}, nil
			}

			return nil, utils.NewConflictError("Profile", request.FirstUid)
		}

		return nil, utils.NewInternalServerError("Failed to create profile", err)
	}

	s.logger.Info("Successfully created profile with FirstUid: %s", profile.FirstUid)
	response := mapProfileToDTO(*profile)
	return &response, nil
}

func (s *profilesServiceImpl) UpdateProfile(firstUid string, request *dto.ProfilesUpdateRequest) (*dto.ProfilesResponse, error) {
	s.logger.Debug("Updating profile with FirstUid: %s", firstUid)

	validator := utils.NewValidator()
	if request.Name != "" {
		validator.ValidateMaxLength("name", request.Name, 255)
	}

	if validator.HasErrors() {
		errors := make(map[string]string)
		for _, err := range validator.Errors() {
			errors[err.Field] = err.Message
		}

		s.logger.Info("Validation failed for profile update: %v", errors)
		return nil, utils.NewValidationError("Invalid profile data", errors)
	}

	profileUpdate := entities.ProfilesUpdate{
		Name: request.Name,
	}

	profile, err := s.profilesRepo.Update(firstUid, profileUpdate)
	if err != nil {
		if err.Error() == "profile not found" {
			s.logger.Info("Profile not found with FirstUid: %s", firstUid)
			return nil, utils.NewNotFoundError("Profile", firstUid)
		}
		s.logger.Error("Failed to update profile with FirstUid: %s: %v", firstUid, err)
		return nil, utils.NewInternalServerError("Failed to update profile", err)
	}

	s.logger.Info("Successfully updated profile with FirstUid: %s", profile.FirstUid)
	response := mapProfileToDTO(*profile)
	return &response, nil
}

func (s *profilesServiceImpl) DeleteProfile(firstUid string) error {
	s.logger.Debug("Deleting profile with FirstUid: %s", firstUid)

	var scans []entities.Scans
	if s.scansRepo != nil {
		if existingScans, err := s.scansRepo.GetAll(firstUid, 0); err == nil {
			scans = existingScans
		} else {
			s.logger.Info("Failed to enumerate scans for profile %s prior to delete: %v", firstUid, err)
		}
	}

	for _, sc := range scans {
		if err := s.scansRepo.Delete(sc.Sid, sc.OwnerDeviceID); err != nil {
			s.logger.Error("Failed to soft-delete scan %s: %v", sc.Sid, err)
		} else {
			s.logger.Info("Soft-deleted scan %s for profile %s", sc.Sid, firstUid)
		}
		removeScanArtifacts(sc.Sid)
	}

	err := s.profilesRepo.Delete(firstUid)
	if err != nil {
		if err.Error() == "profile not found" {
			s.logger.Info("Profile not found with FirstUid: %s", firstUid)
			return utils.NewNotFoundError("Profile", firstUid)
		}
		s.logger.Error("Failed to delete profile with FirstUid: %s: %v", firstUid, err)
		return utils.NewInternalServerError("Failed to delete profile", err)
	}

	s.logger.Info("Successfully deleted profile and %d scan(s) with FirstUid: %s", len(scans), firstUid)
	return nil
}

func mapProfileToDTO(profile entities.Profiles) dto.ProfilesResponse {
	return dto.ProfilesResponse{
		ID:       profile.ID,
		FirstUid: profile.FirstUid,
		Name:     profile.Name,
	}
}

func (s *profilesServiceImpl) UpsertProfileName(firstUid, name string) (*dto.ProfilesResponse, error) {
	v := utils.NewValidator()
	v.ValidateRequired("first_uid", firstUid)
	if v.HasErrors() {
		errs := map[string]string{}
		for _, e := range v.Errors() {
			errs[e.Field] = e.Message
		}
		return nil, utils.NewValidationError("Invalid profile data", errs)
	}

	existing, err := s.profilesRepo.GetByFirstUid(firstUid)
	if err == nil && existing != nil {
		if name != "" && name != existing.Name {
			upd := entities.ProfilesUpdate{Name: name}
			updated, uerr := s.profilesRepo.Update(firstUid, upd)
			if uerr != nil {
				return nil, utils.NewInternalServerError("Failed to update profile", uerr)
			}
			resp := mapProfileToDTO(*updated)
			return &resp, nil
		}
		resp := mapProfileToDTO(*existing)
		return &resp, nil
	}
	if err != nil && err.Error() != "profile not found" {
		s.logger.Error("Failed to fetch profile for upsert: %v", err)
		return nil, utils.NewInternalServerError("Failed to fetch profile", err)
	}

	if name == "" {
		name = firstUid
	}
	created, cerr := s.profilesRepo.Create(entities.ProfilesCreate{FirstUid: firstUid, Name: name})
	if cerr != nil {
		if isUniqueViolation(cerr) {
			reloaded, rerr := s.profilesRepo.GetByFirstUid(firstUid)
			if rerr == nil && reloaded != nil {
				if name != "" && name != reloaded.Name {
					upd := entities.ProfilesUpdate{Name: name}
					updated, uerr := s.profilesRepo.Update(firstUid, upd)
					if uerr != nil {
						return nil, utils.NewInternalServerError("Failed to update profile", uerr)
					}
					resp := mapProfileToDTO(*updated)
					return &resp, nil
				}
				resp := mapProfileToDTO(*reloaded)
				return &resp, nil
			}
		}
		return nil, utils.NewInternalServerError("Failed to create profile", cerr)
	}
	resp := mapProfileToDTO(*created)
	return &resp, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "23505")
}
