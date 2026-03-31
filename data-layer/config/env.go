package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"crypto/rsa"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

type Config struct {
	DBHost                    string
	DBPort                    string
	DBUser                    string
	DBPassword                string
	DBName                    string
	ServerPort                string
	ServerHost                string
	HTTPRedirectPort          string
	JWTSecretKey              string
	JWTAccessTokenExpiration  string
	JWTRefreshTokenExpiration string
	EncryptionKey             string
	TLSCertPath               string
	TLSKeyPath                string
	AppEnv                    string
	JWTPrivateKey             *rsa.PrivateKey
	JWTPublicKey              *rsa.PublicKey
	DisableHTTPSRedirect      bool
}

var (
	globalDB     *gorm.DB
	globalConfig  Config
)

func LoadConfig() Config {

	envPath := filepath.Join(findProjectRoot(), ".env")
	err := godotenv.Load(envPath)
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	serverPort := os.Getenv("PORT")
	if serverPort == "" {
		serverPort = "8443"
	}

	httpRedirectPort := os.Getenv("HTTP_REDIRECT_PORT")
	if httpRedirectPort == "" {
		httpRedirectPort = "8080"
	}

	disableRedirect := false
	if v := os.Getenv("DISABLE_HTTPS_REDIRECT"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			disableRedirect = parsed
		}
	}

	config := Config{
		DBHost:                    os.Getenv("DB_HOST"),
		DBPort:                    os.Getenv("DB_PORT"),
		DBUser:                    os.Getenv("DB_USER"),
		DBPassword:                os.Getenv("DB_PASSWORD"),
		DBName:                    os.Getenv("DB_NAME"),
		ServerPort:                serverPort,
		ServerHost:                "0.0.0.0",
		HTTPRedirectPort:          httpRedirectPort,
		JWTSecretKey:              os.Getenv("JWT_SECRET_KEY"),
		JWTAccessTokenExpiration:  getEnvOrDefault("JWT_ACCESS_TOKEN_EXPIRED_AT", "30m"),
		JWTRefreshTokenExpiration: getEnvOrDefault("JWT_REFRESH_TOKEN_EXPIRED_AT", "7d"),
		EncryptionKey:             os.Getenv("ENCRYPTION_KEY"),
		TLSCertPath:               getEnvOrDefault("TLS_CERT_PATH", "./certs/server.crt"),
		TLSKeyPath:                getEnvOrDefault("TLS_KEY_PATH", "./certs/server.key"),
		AppEnv:                    getEnvOrDefault("APP_ENV", "development"),
		DisableHTTPSRedirect:      disableRedirect,
	}

	// ✅ FIX: Use persistent JWT key manager instead of generating keys on every startup
	// This ensures tokens remain valid across server restarts
	jwtKeyDir := getEnvOrDefault("JWT_KEY_DIR", "./certs/jwt")
	jwtKeySize := 2048 // RSA 2048-bit (can be changed to 4096 for production)

	keyManager := utils.NewJWTKeyManager(jwtKeyDir, jwtKeySize)
	privateKey, publicKey, err := keyManager.LoadOrGenerateKeys()
	if err != nil {
		log.Fatalf("Failed to load or generate JWT keys: %v", err)
	}

	// Validate keys on startup
	if err := keyManager.ValidateKeys(); err != nil {
		log.Fatalf("JWT key validation failed: %v", err)
	}

	// Log key rotation status
	shouldRotate, _ := keyManager.ShouldRotate(30) // 30 days rotation policy
	if shouldRotate {
		log.Printf("⚠️  WARNING: JWT keys are older than 30 days and should be rotated (key_dir=%s)", jwtKeyDir)
		log.Printf("    Run: go run scripts/rotate-jwt-keys.go")
	}

	config.JWTPrivateKey = privateKey
	config.JWTPublicKey = publicKey

	globalConfig = config
	return config
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (c Config) PostgresConnectionString() string {

	sslMode := "require"
	if c.AppEnv == "development" {
		sslMode = "disable"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, sslMode)
}

func InitDB(config Config) (*gorm.DB, error) {
	dsn := config.PostgresConnectionString()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(time.Minute * 5)

	globalDB = db
	return db, nil
}

func GetDB() *gorm.DB {
	return globalDB
}

func GetConfig() Config {
	return globalConfig
}

func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "."
}
