package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/middleware"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/gorilla/mux"
)

type RoleController struct {
	BaseController
	roleService services.RoleService
}

func NewRoleController(roleService services.RoleService) *RoleController {
	return &RoleController{
		BaseController: BaseController{},
		roleService:    roleService,
	}
}

func (c *RoleController) AssignRole(w http.ResponseWriter, r *http.Request) {
	var request entities.RoleCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	// Get current device (admin) from context
	currentDevice, ok := middleware.GetDeviceFromContext(r.Context())
	if !ok {
		c.RespondWithError(w, utils.NewBadRequestError("Authentication required"))
		return
	}

	response, err := c.roleService.AssignRole(request.DeviceID, request.Role, currentDevice.ID)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusCreated, response)
}

func (c *RoleController) UpdateRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceIDStr := vars["deviceId"]
	if deviceIDStr == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid device ID"))
		return
	}

	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 32)
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid device ID format"))
		return
	}

	var request entities.RoleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	// Get current device (admin) from context
	currentDevice, ok := middleware.GetDeviceFromContext(r.Context())
	if !ok {
		c.RespondWithError(w, utils.NewBadRequestError("Authentication required"))
		return
	}

	response, err := c.roleService.UpdateRole(uint(deviceID), request.Role, currentDevice.ID)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, response)
}

func (c *RoleController) RemoveRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceIDStr := vars["deviceId"]
	if deviceIDStr == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid device ID"))
		return
	}

	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 32)
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid device ID format"))
		return
	}

	currentDevice, ok := middleware.GetDeviceFromContext(r.Context())
	if !ok {
		c.RespondWithError(w, utils.NewBadRequestError("Authentication required"))
		return
	}

	err = c.roleService.RemoveRole(uint(deviceID), currentDevice.ID)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Role removed successfully",
	})
}

func (c *RoleController) GetDeviceRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceIDStr := vars["deviceId"]
	if deviceIDStr == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid device ID"))
		return
	}

	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 32)
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid device ID format"))
		return
	}

	response, err := c.roleService.GetDeviceRole(uint(deviceID))
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, response)
}

func (c *RoleController) GetAllRoleAssignments(w http.ResponseWriter, r *http.Request) {
	responses, err := c.roleService.GetAllRoleAssignments()
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	result := map[string]interface{}{
		"roles": responses,
		"count": len(responses),
	}

	c.RespondWithJSON(w, http.StatusOK, result)
}

func (c *RoleController) GetDevicesByRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roleStr := vars["role"]
	if roleStr == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid role"))
		return
	}

	role := entities.Role(roleStr)
	if !role.IsValid() {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid role specified"))
		return
	}

	responses, err := c.roleService.GetDevicesByRole(role)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	result := map[string]interface{}{
		"role":    role,
		"devices": responses,
		"count":   len(responses),
	}

	c.RespondWithJSON(w, http.StatusOK, result)
}

func (c *RoleController) GetMyRole(w http.ResponseWriter, r *http.Request) {
	currentDevice, ok := middleware.GetDeviceFromContext(r.Context())
	if !ok {
		c.RespondWithError(w, utils.NewBadRequestError("Authentication required"))
		return
	}

	response, err := c.roleService.GetDeviceRole(currentDevice.ID)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, response)
}
