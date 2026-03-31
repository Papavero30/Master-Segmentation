package server

import (
	"net/http"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/middleware"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"
)

// ServiceProvider holds all service instances
type ServiceProvider struct {
	ProfilesService     services.ProfilesService
	ScansService        services.ScansService
	AuthService         services.AuthService
	RoleService         services.RoleService
	SegmentationService *services.SegmentationService
	WSManager           *services.WebSocketManager
}

type Server struct {
	router                 *mux.Router
	profilesController     *controller.ProfilesController
	scansController        *controller.ScansController
	scansService           services.ScansService
	authController         *controller.AuthController
	roleController         *controller.RoleController
	segmentationController *controller.SegmentationController
	websocketController    *controller.WebSocketController
	authMiddleware         *middleware.AuthMiddleware
	roleMiddleware         *middleware.RoleMiddleware
	corsMiddleware         func(http.Handler) http.Handler
	rateLimiter            *middleware.DeviceRateLimiter
	validationMiddleware   *middleware.ValidationMiddleware
	securityHeaders        *middleware.SecurityHeaders
	segmentationService    *services.SegmentationService
	wsManager              *services.WebSocketManager
}

func NewServer(serviceProvider ServiceProvider, jwtManager *utils.JWTManager, deviceRepo repositories.DeviceRepository, logger *utils.Logger) *Server {
	router := mux.NewRouter()

	profilesController := controller.NewProfilesController(serviceProvider.ProfilesService)
	scansController := controller.NewScansController(serviceProvider.ScansService)
	authController := controller.NewAuthController(serviceProvider.AuthService)
	roleController := controller.NewRoleController(serviceProvider.RoleService)

	// Initialize SegmentationController if service is available
	var segmentationController *controller.SegmentationController
	var websocketController *controller.WebSocketController
	if serviceProvider.SegmentationService != nil {
		segmentationController = controller.NewSegmentationController(logger, serviceProvider.SegmentationService)
	}

	// Initialize WebSocketController if manager is available
	if serviceProvider.WSManager != nil {
		websocketController = controller.NewWebSocketController(logger, serviceProvider.WSManager)
	}

	authMiddleware := middleware.NewAuthMiddleware(jwtManager, deviceRepo)
	roleMiddleware := middleware.NewRoleMiddleware(serviceProvider.RoleService, logger)
	corsMiddleware := middleware.SecureCorsMiddleware()

	rateLimiter := middleware.NewDeviceRateLimiter(rate.Every(time.Minute/10000), 500)
	validationMiddleware := middleware.NewValidationMiddleware().WithMaxBodySize(512 * 1024 * 1024) // 512MB for ZIP uploads
	securityHeaders := middleware.NewSecurityHeaders()

	return &Server{
		router:                 router,
		profilesController:     profilesController,
		scansController:        scansController,
		scansService:           serviceProvider.ScansService,
		authController:         authController,
		roleController:         roleController,
		segmentationController: segmentationController,
		websocketController:    websocketController,
		authMiddleware:         authMiddleware,
		roleMiddleware:         roleMiddleware,
		corsMiddleware:         corsMiddleware,
		rateLimiter:            rateLimiter,
		validationMiddleware:   validationMiddleware,
		securityHeaders:        securityHeaders,
		segmentationService:    serviceProvider.SegmentationService,
		wsManager:              serviceProvider.WSManager,
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok", "message": "BrainNav API is running"}`))
}

func debugCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{
		"status": "debug_ok", 
		"message": "BrainNav Debug API is running",
		"timestamp": "` + time.Now().Format(time.RFC3339) + `"
	}`))
}

func (s *Server) SetupRoutes() *mux.Router {
	s.router.Use(s.securityHeaders.Handler)
	s.router.Use(s.validationMiddleware.Middleware)
	s.router.Use(s.corsMiddleware)

	s.router.HandleFunc("/debug", debugCheck).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/health", healthCheck).Methods("GET", "OPTIONS")

	apiRouter := s.router.PathPrefix("/api").Subrouter()

	RegisterAuthRoutes(apiRouter, s.authController)

	protectedRouter := apiRouter.PathPrefix("").Subrouter()
	protectedRouter.Use(s.authMiddleware.RequireAuth)
	protectedRouter.Use(s.rateLimiter.Middleware)
	protectedRouter.Use(s.roleMiddleware.EnsureRoleExists)
	protectedRouter.Use(s.roleMiddleware.EnforceHasUserRole)

	RegisterProfilesRoutes(protectedRouter, s.profilesController)
	protectedRouter.HandleFunc("/scans", s.scansController.GetAllScansQuery).Methods("GET", "OPTIONS")
	RegisterScansRoutes(protectedRouter, s.scansController)
	RegisterRoleRoutes(apiRouter, s.roleController, s.authMiddleware, s.roleMiddleware)
	SetupGDCMRoutes(protectedRouter, s.scansService)
	protectedRouter.HandleFunc("/auth/me", s.authController.Me).Methods("GET", "OPTIONS")

	// Register Segmentation Routes (if service is available)
	if s.segmentationController != nil {
		RegisterSegmentationRoutes(protectedRouter, s.segmentationController, s.authMiddleware)
	}
	// Register WebSocket Routes (if controller is available)
	if s.websocketController != nil {
		RegisterWebSocketRoutes(apiRouter, s.websocketController, s.authMiddleware)
	}
	return s.router
}
