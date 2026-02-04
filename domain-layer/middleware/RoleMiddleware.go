package middleware

import (
	"context"
	"net/http"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"gorm.io/gorm"
)

type RoleMiddleware struct {
	roleService services.RoleService
	logger      *utils.Logger
}

func NewRoleMiddleware(roleService services.RoleService, logger *utils.Logger) *RoleMiddleware {
	return &RoleMiddleware{
		roleService: roleService,
		logger:      logger,
	}
}

func (rm *RoleMiddleware) RequireRole(requiredRole entities.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			device, ok := GetDeviceFromContext(r.Context())
			if !ok {
				rm.logger.Warning("Device not found in context for role check")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Authentication required"}`))
				return
			}

			hasRole, err := rm.roleService.HasRole(device.ID, requiredRole)
			if err != nil {
				rm.logger.Error("Failed to check role for device %d: %v", device.ID, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Authorization check failed"}`))
				return
			}

			if !hasRole {
				rm.logger.Warning("Device %d attempted to access resource requiring role %s", device.ID, requiredRole)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error": "Insufficient permissions"}`))
				return
			}

			roleInfo, err := rm.roleService.GetDeviceRole(device.ID)
			if err == nil {
				ctx := context.WithValue(r.Context(), "role", roleInfo)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rm *RoleMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return rm.RequireRole(entities.RoleAdmin)(next)
}

func (rm *RoleMiddleware) RequireUser(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			device, ok := GetDeviceFromContext(r.Context())
			if !ok {
				rm.logger.Warning("Device not found in context for user role check")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Authentication required"}`))
				return
			}

			hasUserRole, err := rm.roleService.HasRole(device.ID, entities.RoleUser)
			if err != nil {
				rm.logger.Error("Failed to check user role for device %d: %v", device.ID, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Authorization check failed"}`))
				return
			}

			hasAdminRole, err := rm.roleService.HasRole(device.ID, entities.RoleAdmin)
			if err != nil {
				rm.logger.Error("Failed to check admin role for device %d: %v", device.ID, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Authorization check failed"}`))
				return
			}

			if !hasUserRole && !hasAdminRole {
				rm.logger.Warning("Device %d has no valid role assignment", device.ID)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error": "No valid role assignment"}`))
				return
			}

			roleInfo, err := rm.roleService.GetDeviceRole(device.ID)
			if err == nil {
				ctx := context.WithValue(r.Context(), "role", roleInfo)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}(next)
}

func (rm *RoleMiddleware) EnsureRoleExists(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		device, ok := GetDeviceFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		err := rm.roleService.EnsureDefaultRole(device.ID)
		if err != nil {
			rm.logger.Error("Failed to ensure default role for device %d: %v", device.ID, err)
		}

		next.ServeHTTP(w, r)
	})
}

func (rm *RoleMiddleware) EnforceHasUserRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		device, ok := GetDeviceFromContext(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Authentication required"}`))
			return
		}
		hasUser, err1 := rm.roleService.HasRole(device.ID, entities.RoleUser)
		hasAdmin, err2 := rm.roleService.HasRole(device.ID, entities.RoleAdmin)
		if (err1 != nil && err1 != gorm.ErrRecordNotFound) || (err2 != nil && err2 != gorm.ErrRecordNotFound) {
			rm.logger.Error("Role lookup failed: %v %v", err1, err2)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"Authorization failed"}`))
			return
		}
		if !hasUser && !hasAdmin {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"Role required"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GetRoleFromContext(ctx context.Context) (*entities.RoleResponse, bool) {
	role, ok := ctx.Value("role").(*entities.RoleResponse)
	return role, ok
}

func (rm *RoleMiddleware) IsOwnerOrAdmin(getResourceOwnerID func(*http.Request) (uint, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			device, ok := GetDeviceFromContext(r.Context())
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Authentication required"}`))
				return
			}

			isAdmin, err := rm.roleService.IsAdmin(device.ID)
			if err != nil {
				rm.logger.Error("Failed to check admin role: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Authorization check failed"}`))
				return
			}

			if isAdmin {
				next.ServeHTTP(w, r)
				return
			}

			ownerID, err := getResourceOwnerID(r)
			if err != nil {
				rm.logger.Error("Failed to get resource owner ID: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "Invalid resource identifier"}`))
				return
			}

			if device.ID != ownerID {
				rm.logger.Warning("Device %d attempted to access resource owned by %d", device.ID, ownerID)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error": "Access denied - not resource owner"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
