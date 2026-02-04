package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

// RegisterProfilesRoutes registers all routes for profiles data
func RegisterProfilesRoutes(router *mux.Router, controller *controller.ProfilesController) {
	router.HandleFunc("/profiles", controller.GetAllProfiles).Methods("GET", "OPTIONS")
	// Backward compatibility routes
	router.HandleFunc("/profiles/{id:[0-9]+}", controller.GetProfileByID).Methods("GET", "OPTIONS")
	// Profile-focused routes
	router.HandleFunc("/profiles/{firstUid}", controller.GetProfileByFirstUid).Methods("GET", "OPTIONS")
	router.HandleFunc("/profiles", controller.CreateProfile).Methods("POST", "OPTIONS")
	router.HandleFunc("/profiles/{firstUid}", controller.UpdateProfile).Methods("PUT", "OPTIONS")
	router.HandleFunc("/profiles/{firstUid}", controller.DeleteProfile).Methods("DELETE", "OPTIONS")
}
