package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

type AuthMiddleware struct {
	jwtManager *utils.JWTManager
	deviceRepo repositories.DeviceRepository
}

func NewAuthMiddleware(jwtManager *utils.JWTManager, deviceRepo repositories.DeviceRepository) *AuthMiddleware {
	return &AuthMiddleware{
		jwtManager: jwtManager,
		deviceRepo: deviceRepo,
	}
}

func (a *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		writeJSON := func(code int, msg string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSON(http.StatusUnauthorized, "Authorization header required")
			return
		}

		bearerToken := strings.Split(authHeader, " ")
		if len(bearerToken) != 2 || bearerToken[0] != "Bearer" {
			writeJSON(http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		claims, err := a.jwtManager.ValidateAccessToken(bearerToken[1])
		if err != nil {
			writeJSON(http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		device, err := a.deviceRepo.GetByID(claims.DeviceID)
		if err != nil {
			writeJSON(http.StatusUnauthorized, "Device not found or inactive")
			return
		}

		ctx := context.WithValue(r.Context(), "claims", claims)
		ctx = context.WithValue(ctx, "device", device)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetClaimsFromContext(ctx context.Context) (*utils.JWTClaims, bool) {
	claims, ok := ctx.Value("claims").(*utils.JWTClaims)
	return claims, ok
}

func GetDeviceFromContext(ctx context.Context) (*entities.Device, bool) {
	device, ok := ctx.Value("device").(*entities.Device)
	return device, ok
}
