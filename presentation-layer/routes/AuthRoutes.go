package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

func RegisterAuthRoutes(router *mux.Router, controller *controller.AuthController) {
	authRouter := router.PathPrefix("/auth").Subrouter()

	authRouter.HandleFunc("/login", controller.Login).Methods("POST", "OPTIONS")
	authRouter.HandleFunc("/refresh", controller.RefreshToken).Methods("POST", "OPTIONS")
	authRouter.HandleFunc("/logout", controller.Logout).Methods("POST", "OPTIONS")
}
