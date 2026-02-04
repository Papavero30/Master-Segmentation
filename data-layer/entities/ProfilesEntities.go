package entities

import "gorm.io/gorm"

type Profiles struct {
	gorm.Model
	FirstUid string `json:"first_uid" gorm:"uniqueIndex;not null;type:varchar(255)"`
	Name     string `json:"name" gorm:"not null;type:varchar(255)"`
}

type ProfilesCreate struct {
	FirstUid string `json:"first_uid"`
	Name     string `json:"name"`
}

type ProfilesUpdate struct {
	Name string `json:"name,omitempty"`
}
