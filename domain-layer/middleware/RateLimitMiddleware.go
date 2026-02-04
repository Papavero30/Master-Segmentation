package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"golang.org/x/time/rate"
)

type DeviceRateLimiter struct {
	devices map[uint]*rate.Limiter
	mu      sync.RWMutex
	rate    rate.Limit
	burst   int
}

func NewDeviceRateLimiter(rps rate.Limit, burst int) *DeviceRateLimiter {
	limiter := &DeviceRateLimiter{
		devices: make(map[uint]*rate.Limiter),
		rate:    rps,
		burst:   burst,
	}

	go limiter.cleanupDevices()

	log.Printf("[RateLimit] Device-based rate limiter initialized: %.0f req/min, burst=%d", float64(rps)*60, burst)
	return limiter
}

func (rl *DeviceRateLimiter) getDeviceLimiter(deviceID uint) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.devices[deviceID]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		if limiter, exists = rl.devices[deviceID]; !exists {
			limiter = rate.NewLimiter(rl.rate, rl.burst)
			rl.devices[deviceID] = limiter
			log.Printf("[RateLimit] Created limiter for device %d", deviceID)
		}
		rl.mu.Unlock()
	}

	return limiter
}

func (rl *DeviceRateLimiter) cleanupDevices() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		cleaned := 0
		for deviceID, limiter := range rl.devices {
			if limiter.Tokens() == float64(rl.burst) {
				delete(rl.devices, deviceID)
				cleaned++
			}
		}
		rl.mu.Unlock()

		if cleaned > 0 {
			log.Printf("[RateLimit] Cleaned up %d idle devices", cleaned)
		}
	}
}

func (rl *DeviceRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceID, err := getDeviceIDFromContext(r.Context())

		if err != nil {
			ip := getClientIP(r)
			ipLimiter := rl.getIPLimiter(ip)

			if !ipLimiter.Allow() {
				log.Printf("[RateLimit] IP rate limit exceeded: %s", ip)
				respondRateLimitError(w, "anonymous", 0)
				return
			}

			next.ServeHTTP(w, r)
			return
		}

		limiter := rl.getDeviceLimiter(deviceID)

		if !limiter.Allow() {
			log.Printf("[RateLimit] Device rate limit exceeded: device_id=%d", deviceID)
			respondRateLimitError(w, "device", deviceID)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getDeviceIDFromContext(ctx context.Context) (uint, error) {
	claimsVal := ctx.Value("claims")
	if claimsVal != nil {
		if claims, ok := claimsVal.(*utils.JWTClaims); ok {
			return claims.DeviceID, nil
		}
	}

	deviceVal := ctx.Value("device")
	if deviceVal == nil {
		return 0, fmt.Errorf("no device or claims in context")
	}

	type HasID interface {
		GetID() uint
	}

	if deviceWithID, ok := deviceVal.(HasID); ok {
		return deviceWithID.GetID(), nil
	}

	return 0, fmt.Errorf("unable to extract device ID from context")
}

func respondRateLimitError(w http.ResponseWriter, limitType string, identifier uint) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Type", limitType)
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)

	var message string
	if limitType == "device" {
		message = fmt.Sprintf("Rate limit exceeded for device %d. Please try again later.", identifier)
	} else {
		message = "Rate limit exceeded. Please try again later."
	}

	fmt.Fprintf(w, `{"error": "%s", "retry_after_seconds": 60}`, message)
}

var ipLimiters = struct {
	sync.RWMutex
	limiters map[string]*rate.Limiter
}{limiters: make(map[string]*rate.Limiter)}

func (rl *DeviceRateLimiter) getIPLimiter(ip string) *rate.Limiter {
	ipLimiters.RLock()
	limiter, exists := ipLimiters.limiters[ip]
	ipLimiters.RUnlock()

	if !exists {
		ipLimiters.Lock()
		if limiter, exists = ipLimiters.limiters[ip]; !exists {
			limiter = rate.NewLimiter(rate.Every(time.Minute/20), 5)
			ipLimiters.limiters[ip] = limiter
		}
		ipLimiters.Unlock()
	}

	return limiter
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for i, c := range xff {
			if c == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	for i := len(r.RemoteAddr) - 1; i >= 0; i-- {
		if r.RemoteAddr[i] == ':' {
			return r.RemoteAddr[:i]
		}
	}

	return r.RemoteAddr
}


type RateLimiter = DeviceRateLimiter

func NewRateLimiter(rps rate.Limit, burst int) *DeviceRateLimiter {
	log.Printf("[RateLimit] DEPRECATED: NewRateLimiter called, using NewDeviceRateLimiter")
	return NewDeviceRateLimiter(rps, burst)
}
