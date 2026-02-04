package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/middleware"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

func RegisterRoleRoutes(
	router *mux.Router,
	controller *controller.RoleController,
	authMiddleware *middleware.AuthMiddleware,
	roleMiddleware *middleware.RoleMiddleware,
) {
	roleRouter := router.PathPrefix("/admin/roles").Subrouter()

	roleRouter.Use(authMiddleware.RequireAuth)
	roleRouter.Use(roleMiddleware.RequireAdmin)

	roleRouter.HandleFunc("", controller.AssignRole).Methods("POST", "OPTIONS")
	roleRouter.HandleFunc("", controller.GetAllRoleAssignments).Methods("GET", "OPTIONS")
	roleRouter.HandleFunc("/role/{role}", controller.GetDevicesByRole).Methods("GET", "OPTIONS")
	roleRouter.HandleFunc("/device/{deviceId}", controller.GetDeviceRole).Methods("GET", "OPTIONS")
	roleRouter.HandleFunc("/device/{deviceId}", controller.UpdateRole).Methods("PUT", "OPTIONS")
	roleRouter.HandleFunc("/device/{deviceId}", controller.RemoveRole).Methods("DELETE", "OPTIONS")

	userRoleRouter := router.PathPrefix("/user/role").Subrouter()
	userRoleRouter.Use(authMiddleware.RequireAuth)
	userRoleRouter.Use(roleMiddleware.EnsureRoleExists)

	userRoleRouter.HandleFunc("/me", controller.GetMyRole).Methods("GET", "OPTIONS")
}
