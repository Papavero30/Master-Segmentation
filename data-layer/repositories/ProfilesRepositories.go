package repositories

import (
	"errors"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"gorm.io/gorm"
)

type ProfilesRepository interface {
	GetAll() ([]entities.Profiles, error)
	GetByID(id uint) (*entities.Profiles, error)
	GetByFirstUid(firstUid string) (*entities.Profiles, error)
	GetByFirstUidUnscoped(firstUid string) (*entities.Profiles, error)
	Create(profile entities.ProfilesCreate) (*entities.Profiles, error)
	Update(firstUid string, profile entities.ProfilesUpdate) (*entities.Profiles, error)
	Delete(firstUid string) error
	Restore(firstUid string) (*entities.Profiles, error)
}

type profilesRepositoryImpl struct {
	db *gorm.DB
}

func NewProfilesRepository(db *gorm.DB) ProfilesRepository {
	return &profilesRepositoryImpl{db: db}
}

func (r *profilesRepositoryImpl) GetAll() ([]entities.Profiles, error) {
	var profiles []entities.Profiles
	err := r.db.Find(&profiles).Error
	return profiles, err
}

func (r *profilesRepositoryImpl) GetByID(id uint) (*entities.Profiles, error) {
	var profile entities.Profiles
	err := r.db.First(&profile, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("profile not found")
		}
		return nil, err
	}
	return &profile, nil
}

func (r *profilesRepositoryImpl) GetByFirstUid(firstUid string) (*entities.Profiles, error) {
	var profile entities.Profiles
	err := r.db.Where("first_uid = ?", firstUid).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("profile not found")
		}
		return nil, err
	}
	return &profile, nil
}

func (r *profilesRepositoryImpl) GetByFirstUidUnscoped(firstUid string) (*entities.Profiles, error) {
	var profile entities.Profiles
	err := r.db.Unscoped().Where("first_uid = ?", firstUid).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("profile not found")
		}
		return nil, err
	}
	return &profile, nil
}

func (r *profilesRepositoryImpl) Create(profile entities.ProfilesCreate) (*entities.Profiles, error) {
	newProfile := entities.Profiles{
		FirstUid: profile.FirstUid,
		Name:     profile.Name,
	}

	err := r.db.Create(&newProfile).Error
	if err != nil {
		return nil, err
	}

	return &newProfile, nil
}

func (r *profilesRepositoryImpl) Update(firstUid string, profile entities.ProfilesUpdate) (*entities.Profiles, error) {
	var existingProfile entities.Profiles
	err := r.db.Where("first_uid = ?", firstUid).First(&existingProfile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("profile not found")
		}
		return nil, err
	}

	if profile.Name != "" {
		existingProfile.Name = profile.Name
	}

	err = r.db.Save(&existingProfile).Error
	if err != nil {
		return nil, err
	}

	return &existingProfile, nil
}

func (r *profilesRepositoryImpl) Delete(firstUid string) error {
	var profile entities.Profiles
	err := r.db.Where("first_uid = ?", firstUid).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("profile not found")
		}
		return err
	}

	return r.db.Delete(&profile).Error
}

func (r *profilesRepositoryImpl) Restore(firstUid string) (*entities.Profiles, error) {
	var profile entities.Profiles

	err := r.db.Unscoped().Where("first_uid = ?", firstUid).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("profile not found")
		}
		return nil, err
	}

	if !profile.DeletedAt.Valid {
		return &profile, nil
	}

	err = r.db.Unscoped().Model(&profile).Update("deleted_at", nil).Error
	if err != nil {
		return nil, err
	}

	err = r.db.Where("first_uid = ?", firstUid).First(&profile).Error
	if err != nil {
		return nil, err
	}

	return &profile, nil
}
