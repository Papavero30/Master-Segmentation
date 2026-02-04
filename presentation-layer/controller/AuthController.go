package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

type AuthController struct {
	BaseController
	authService services.AuthService
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func NewAuthController(authService services.AuthService) *AuthController {
	return &AuthController{
		BaseController: BaseController{},
		authService:    authService,
	}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var request entities.DeviceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	validator := utils.NewValidator()
	validator.ValidateRequired("fingerprint", request.Fingerprint).
		ValidateMinLength("fingerprint", request.Fingerprint, 10).
		ValidateMaxLength("fingerprint", request.Fingerprint, 255).
		ValidateRequired("device_name", request.DeviceName).
		ValidateMinLength("device_name", request.DeviceName, 3).
		ValidateMaxLength("device_name", request.DeviceName, 100)

	if validator.HasErrors() {
		c.RespondWithError(w, utils.NewValidationError("Invalid device data",
			map[string]string{"validation": validator.Errors().Error()}))
		return
	}

	response, err := c.authService.AuthenticateDevice(request.Fingerprint, request.DeviceName)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	// Unified response shape: keep flat fields for backward compatibility and provide nested data object.
	respPayload := map[string]interface{}{
		"access_token":  response.AccessToken,
		"refresh_token": response.RefreshToken,
		"expires_in":    response.ExpiresIn,
		"token_type":    "Bearer",
		"device":        response.Device,
		"data": map[string]interface{}{
			"access_token":  response.AccessToken,
			"refresh_token": response.RefreshToken,
			"expires_in":    response.ExpiresIn,
			"token_type":    "Bearer",
			"device":        response.Device,
		},
	}

	c.RespondWithJSON(w, http.StatusOK, respPayload)
}

func (c *AuthController) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var request RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	response, err := c.authService.RefreshToken(request.RefreshToken)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	respPayload := map[string]interface{}{
		"access_token":  response.AccessToken,
		"refresh_token": response.RefreshToken,
		"expires_in":    response.ExpiresIn,
		"token_type":    "Bearer",
		"device":        response.Device,
		"data": map[string]interface{}{
			"access_token":  response.AccessToken,
			"refresh_token": response.RefreshToken,
			"expires_in":    response.ExpiresIn,
			"token_type":    "Bearer",
			"device":        response.Device,
		},
	}

	c.RespondWithJSON(w, http.StatusOK, respPayload)
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	device, ok := GetDeviceFromContext(r.Context())
	if !ok {
		c.RespondWithError(w, utils.NewBadRequestError("Device context not found"))
		return
	}

	if err := c.authService.RevokeDevice(device.ID); err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Device logged out successfully"})
}

func (c *AuthController) Me(w http.ResponseWriter, r *http.Request) {
	device, ok := GetDeviceFromContext(r.Context())
	if !ok {
		c.RespondWithError(w, utils.NewUnauthorizedError("Authentication required"))
		return
	}
	claims, _ := GetClaimsFromContext(r.Context())
	resp := map[string]interface{}{
		"device": device,
		"claims": claims,
	}
	c.RespondWithJSON(w, http.StatusOK, resp)
}

func GetClaimsFromContext(ctx context.Context) (*utils.JWTClaims, bool) {
	claims, ok := ctx.Value("claims").(*utils.JWTClaims)
	return claims, ok
}

func GetDeviceFromContext(ctx context.Context) (*entities.Device, bool) {
	device, ok := ctx.Value("device").(*entities.Device)
	return device, ok
}
