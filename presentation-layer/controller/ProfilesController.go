package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/dto"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/gorilla/mux"
)

type ProfilesController struct {
	BaseController
	profilesService services.ProfilesService
}

func NewProfilesController(profilesService services.ProfilesService) *ProfilesController {
	return &ProfilesController{
		BaseController:  BaseController{},
		profilesService: profilesService,
	}
}

func (c *ProfilesController) GetAllProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := c.profilesService.GetAllProfiles()
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, profiles)
}

func (c *ProfilesController) GetProfileByFirstUid(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	firstUid := vars["firstUid"]
	if firstUid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid profile FirstUid"))
		return
	}

	profile, err := c.profilesService.GetProfileByFirstUid(firstUid)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, profile)
}

func (c *ProfilesController) GetProfileByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid profile ID"))
		return
	}

	profile, err := c.profilesService.GetProfileByID(id)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, profile)
}

func (c *ProfilesController) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var request dto.ProfilesCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	profile, err := c.profilesService.CreateProfile(&request)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusCreated, profile)
}

func (c *ProfilesController) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	firstUid := vars["firstUid"]
	if firstUid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid profile FirstUid"))
		return
	}

	var request dto.ProfilesUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	profile, err := c.profilesService.UpdateProfile(firstUid, &request)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, profile)
}

func (c *ProfilesController) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	firstUid := vars["firstUid"]
	if firstUid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid profile FirstUid"))
		return
	}

	if err := c.profilesService.DeleteProfile(firstUid); err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Profile deleted successfully"})
}
