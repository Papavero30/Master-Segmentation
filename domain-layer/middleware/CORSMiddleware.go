package middleware

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/rs/cors"
)

func SecureCorsMiddleware() func(http.Handler) http.Handler {
	log.Printf("[CORS] Initializing Secure CORS middleware")

	allowedOrigins := getSecureAllowedOrigins()

	log.Printf("[CORS] Configured allowed origins: %v", allowedOrigins)
	log.Printf("[CORS] AllowCredentials: true (required for JWT)")
	log.Printf("[CORS] MaxAge: 3600s (1 hour preflight cache)")

	corsHandler := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
			"OPTIONS",
			"PATCH",
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Requested-With",
			"X-CSRF-Token",
		},
		ExposedHeaders: []string{
			"Content-Disposition",
			"X-Total-Count",
		},
		AllowCredentials: true,

		MaxAge: 3600,

		Debug: os.Getenv("APP_ENV") == "development",

		AllowOriginFunc: func(origin string) bool {
			if origin == "" {
				log.Printf("[CORS] Request without Origin header (likely Electron), allowing")
				return true
			}
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}
			log.Printf("[CORS] Origin not in whitelist: %s", origin)
			return false
		},
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin == "" {
				log.Printf("[CORS] No Origin header detected, setting CORS headers for Electron")
				w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Requested-With, X-CSRF-Token")
				w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, X-Total-Count")
				w.Header().Set("Vary", "Origin")

				if r.Method == "OPTIONS" {
					w.Header().Set("Access-Control-Max-Age", "3600")
					w.WriteHeader(http.StatusOK)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			corsHandler.Handler(next).ServeHTTP(w, r)
		})
	}
}

func getSecureAllowedOrigins() []string {
	originsEnv := os.Getenv("ALLOWED_ORIGINS")

	if originsEnv == "" {
		log.Printf("[CORS] WARNING: ALLOWED_ORIGINS not set, using localhost defaults")
		return []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"https://localhost:3000",
			"https://localhost:8443",
			"http://localhost:8443",
			"file://",
		}
	}

	origins := strings.Split(originsEnv, ",")
	validOrigins := make([]string, 0, len(origins))

	for _, origin := range origins {
		origin = strings.TrimSpace(origin)

		if origin == "*" {
			log.Printf("[CORS] ERROR: Wildcard origin (*) rejected! Specify exact origins.")
			continue
		}

		if origin == "" {
			continue
		}

		if !isValidOrigin(origin) {
			log.Printf("[CORS] WARNING: Invalid origin format rejected: %s", origin)
			continue
		}

		validOrigins = append(validOrigins, origin)
		log.Printf("[CORS] Whitelisted origin: %s", origin)
	}

	if len(validOrigins) == 0 {
		log.Printf("[CORS] CRITICAL: No valid origins configured! Using localhost fallback.")
		return []string{
			"http://localhost:3000",
			"file://",
		}
	}

	return validOrigins
}

func isValidOrigin(origin string) bool {
	if strings.HasPrefix(origin, "file://") {
		return true
	}

	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		return false
	}

	if origin == "http://" || origin == "https://" {
		return false
	}

	return true
}

func CorsMiddleware() func(http.Handler) http.Handler {
	log.Printf("[CORS] DEPRECATED: CorsMiddleware called, redirecting to SecureCorsMiddleware")
	return SecureCorsMiddleware()
}

func EnhancedCorsMiddleware() func(http.Handler) http.Handler {
	log.Printf("[CORS] DEPRECATED: EnhancedCorsMiddleware called, redirecting to SecureCorsMiddleware")
	return SecureCorsMiddleware()
}
