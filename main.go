package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	redis "github.com/redis/go-redis/v9"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/config"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/entities"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/migration"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/repositories"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	server "github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/routes"
	"gorm.io/gorm"
)

func mustParseDuration(v string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}

func autoMigrate(db *gorm.DB) {
	if err := db.AutoMigrate(
		&entities.Device{},
		&entities.UserRole{},
		&entities.Profiles{},
		&entities.Scans{},
		&entities.ScanFile{},
	); err != nil {
		log.Printf("WARN: auto-migrate failed: %v", err)
	}
}

func runCustomMigrations(gormDB *gorm.DB) {
	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Fatalf("Failed to get sql.DB from GORM: %v", err)
	}

	migrationManager := migration.NewMigrationManager(sqlDB)

	if err := migrationManager.MigrateUp(); err != nil {
		log.Printf("WARN: Custom migrations failed: %v", err)
	} else {
		log.Println("Custom migrations completed successfully (indexes & foreign keys)")
	}
}

func resetStuckScansOnStartup(db *gorm.DB, logger *utils.Logger) {
	logger.Info("Checking for stuck scans on startup...")

	result := db.Exec(`
		UPDATE scans
		SET upload_status = 'uploading',
		    last_error = 'Recovered after server restart'
		WHERE upload_status IN ('pending', 'compressing')
		  AND is_final = false
	`)

	if result.Error != nil {
		logger.Error("Failed to reset stuck scans: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		logger.Info("Recovered %d stuck scans on startup", result.RowsAffected)
	} else {
		logger.Info("No stuck scans found on startup")
	}
}

func startDeletedScansCleanup(scansService services.ScansService, logger *utils.Logger) {
	retentionDays := 7
	if days := os.Getenv("DELETED_SCANS_RETENTION_DAYS"); days != "" {
		if parsed, err := time.ParseDuration(days + "d"); err == nil {
			retentionDays = int(parsed.Hours() / 24)
		}
	}

	checkInterval := 24 * time.Hour
	if interval := os.Getenv("CLEANUP_CHECK_INTERVAL"); interval != "" {
		if parsed, err := time.ParseDuration(interval); err == nil {
			checkInterval = parsed
		}
	}

	logger.Info("Deleted scans cleanup job started (retention: %d days, check interval: %v)",
		retentionDays, checkInterval)

	ticker := time.NewTicker(checkInterval)

	go func() {
		defer ticker.Stop()

		logger.Info("Running initial deleted scans cleanup...")
		if deleted, err := scansService.CleanupOldDeletedScans(nil, retentionDays); err != nil {
			logger.Error("Initial cleanup failed: %v", err)
		} else if deleted > 0 {
			logger.Info("Initial cleanup: Permanently deleted %d old scans", deleted)
		} else {
			logger.Info("Initial cleanup: No old deleted scans to remove")
		}

		for range ticker.C {
			logger.Info("Running periodic deleted scans cleanup...")

			deleted, err := scansService.CleanupOldDeletedScans(nil, retentionDays)
			if err != nil {
				logger.Error("Periodic cleanup failed: %v", err)
				continue
			}

			if deleted > 0 {
				logger.Info("Periodic cleanup: Permanently deleted %d scans older than %d days",
					deleted, retentionDays)
			} else {
				logger.Info("Periodic cleanup: No old deleted scans to remove")
			}
		}
	}()
}

func startStuckScanMonitor(db *gorm.DB, logger *utils.Logger) {
	ticker := time.NewTicker(1 * time.Hour)

	go func() {
		defer ticker.Stop()
		logger.Info("Stuck scan monitor started (checking every 1 hour)")

		for range ticker.C {
			logger.Info("Running periodic stuck scan check...")

			result := db.Exec(`
				UPDATE scans
				SET upload_status = 'uploading',
				    last_error = 'Auto-recovered from stuck state (timeout > 1 hour)'
				WHERE upload_status IN ('pending', 'compressing')
				  AND updated_at < NOW() - INTERVAL '1 hour'
				  AND is_final = false
			`)

			if result.Error != nil {
				logger.Error("Periodic stuck scan check failed: %v", result.Error)
				continue
			}

			if result.RowsAffected > 0 {
				logger.Info("Auto-recovered %d stuck scans (timeout > 1 hour)", result.RowsAffected)
			} else {
				logger.Info("Periodic check: No stuck scans found")
			}
		}
	}()
}

func main() {
	cfg := config.LoadConfig()

	gormDB, err := config.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	autoMigrate(gormDB)

	runCustomMigrations(gormDB)

	logger := utils.NewLogger(utils.INFO)
	logger.Info("Starting BrainNav backend...")

	deviceRepo := repositories.NewDeviceRepository(gormDB)
	roleRepo := repositories.NewRoleRepository(gormDB)
	profilesRepo := repositories.NewProfilesRepository(gormDB)
	scansRepo := repositories.NewScansRepository(gormDB)

	roleService := services.NewRoleService(gormDB, logger, roleRepo, deviceRepo)

	encMgr, err := utils.NewEncryptionManager(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("Invalid ENCRYPTION_KEY: %v", err)
	}

	accessTTL := mustParseDuration(cfg.JWTAccessTokenExpiration, 30*time.Minute)
	refreshTTL := mustParseDuration(cfg.JWTRefreshTokenExpiration, 7*24*time.Hour)
	jwtMgr := utils.NewJWTManager(cfg.JWTPrivateKey, cfg.JWTPublicKey, accessTTL, refreshTTL)

	authService := services.NewAuthService(deviceRepo, jwtMgr, encMgr, logger, roleService)
	profilesService := services.NewProfilesService(gormDB, logger, profilesRepo, scansRepo)
	gdcmService := services.NewGDCMService()

	gdcmURL := os.Getenv("GDCM_SERVICE_URL")
	if gdcmURL == "" {
		gdcmURL = "http://brainnav-gdcm:3000"
	}
	gdcmClient := services.NewGDCMClient(gdcmURL, logger)
	logger.Info("GDCM client initialized: %s", gdcmURL)

	// Initialize Redis
	var redisClient *redis.Client
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			logger.Error("Failed to parse REDIS_URL: %v", err)
		} else {
			redisClient = redis.NewClient(opt)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := redisClient.Ping(ctx).Err(); err != nil {
				logger.Error("Failed to connect to Redis: %v", err)
				redisClient = nil
			} else {
				logger.Info("Redis connected: %s", redisURL)
			}
		}
	}

	// Initialize RabbitMQ
	var rabbitConn *amqp.Connection
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL != "" {
		var err error
		rabbitConn, err = amqp.Dial(rabbitURL)
		if err != nil {
			logger.Error("Failed to connect to RabbitMQ: %v", err)
		} else {
			logger.Info("RabbitMQ connected: %s", rabbitURL)
		}
	}

	// Initialize background worker queue for GDCM compression
	workerQueue := services.NewWorkerQueue(100)
	logger.Info("Background job queue initialized with buffer size: 100")

	scansService := services.NewScansService(gormDB, logger, scansRepo, profilesService, gdcmService, gdcmClient, workerQueue)

	// Start worker to process compression jobs
	workerQueue.Start(func(job services.Job) bool {
		switch job.Type {
		case services.JobTypeCompress:
			logger.Info("[Worker] Processing compression job for sid: %s (attempt %d/%d)", job.SID, job.Attempts, job.MaxRetry)
			if err := scansService.ProcessCompressionJob(job.SID); err != nil {
				logger.Error("[Worker] Compression failed for sid %s: %v", job.SID, err)
				return false
			}
			logger.Info("[Worker] Compression completed for sid: %s", job.SID)
			return true
		default:
			logger.Warning("[Worker] Unknown job type: %s", job.Type)
			return true
		}
	})
	logger.Info("Background worker started for GDCM compression")

	// Initialize Segmentation Service (optional - only if Redis and RabbitMQ available)
	var segmentationService *services.SegmentationService
	var wsManager *services.WebSocketManager
	segmentationEnabled := os.Getenv("SEGMENTATION_ENABLED")
	if segmentationEnabled == "true" && redisClient != nil && rabbitConn != nil {
		var err error
		segmentationService, err = services.NewSegmentationService(logger, redisClient, rabbitConn, gdcmClient)
		if err != nil {
			logger.Error("Failed to initialize SegmentationService: %v", err)
		} else {
			logger.Info("SegmentationService initialized successfully")

			// Initialize WebSocket Manager for real-time progress
			wsManager = services.NewWebSocketManager(logger, redisClient)
			logger.Info("WebSocketManager initialized successfully")
		}
	} else {
		logger.Info("SegmentationService disabled (SEGMENTATION_ENABLED=%s, Redis=%v, RabbitMQ=%v)",
			segmentationEnabled, redisClient != nil, rabbitConn != nil)
	}

	// TODO: Implement JobQueue service

	resetStuckScansOnStartup(gormDB, logger)

	startStuckScanMonitor(gormDB, logger)

	startDeletedScansCleanup(scansService, logger)

	srv := server.NewServer(
		server.ServiceProvider{
			ProfilesService:     profilesService,
			ScansService:        scansService,
			AuthService:         authService,
			RoleService:         roleService,
			SegmentationService: segmentationService,
			WSManager:           wsManager,
		},
		jwtMgr,
		deviceRepo,
		logger,
	)
	router := srv.SetupRoutes()

	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.ServerPort
	}
	addr := "0.0.0.0:" + port
	logger.Info("HTTPS server starting on %s", addr)

	httpSrv := &http.Server{
		Addr:           addr,
		Handler:        router,
		ReadTimeout:    10 * time.Minute,  // Extended for large file uploads (ZIP bundles)
		WriteTimeout:   10 * time.Minute,  // Extended for large file uploads
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if !cfg.DisableHTTPSRedirect && cfg.HTTPRedirectPort == cfg.ServerPort {
		logger.Fatal("HTTP redirect port (%s) cannot be the same as HTTPS port (%s)", cfg.HTTPRedirectPort, cfg.ServerPort)
	}

	if !cfg.DisableHTTPSRedirect && cfg.HTTPRedirectPort != "" {
		redirectAddr := cfg.ServerHost + ":" + cfg.HTTPRedirectPort
		redirectSrv := &http.Server{
			Addr: redirectAddr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				if idx := strings.Index(host, ":"); idx != -1 {
					host = host[:idx]
				}

				target := "https://" + host
				if cfg.ServerPort != "443" {
					target += ":" + cfg.ServerPort
				}
				target += r.URL.RequestURI()

				http.Redirect(w, r, target, http.StatusPermanentRedirect)
			}),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  30 * time.Second,
		}

		go func() {
			logger.Info("HTTP redirect server listening on %s", redirectAddr)
			if err := redirectSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("HTTP redirect server stopped: %v", err)
			}
		}()
	}
	if cfg.DisableHTTPSRedirect {
		logger.Info("HTTP redirect server disabled via configuration")
	}

	if err := httpSrv.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Failed to start HTTPS server: %v", err)
		log.Fatalf("Failed to start HTTPS server: %v", err)
	}
}
