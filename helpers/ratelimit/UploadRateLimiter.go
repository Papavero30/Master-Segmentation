package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

// UploadRateLimiter manages rate limiting for upload operations per session
type UploadRateLimiter struct {
	maxUploadsPerMinute int
	perSessionLimiters  map[string]*sessionRateLimiter
	mu                  sync.RWMutex
}

// sessionRateLimiter tracks upload attempts for a single session
type sessionRateLimiter struct {
	uploadTimes []time.Time
	mu          sync.Mutex
}

// RateLimitError represents a rate limit violation
type RateLimitError struct {
	SessionID           string
	MaxUploadsPerMinute int
	CurrentCount        int
	WaitSeconds         int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf(
		"upload rate limit exceeded for session %s: %d/%d uploads in last minute, wait %d seconds",
		e.SessionID,
		e.CurrentCount,
		e.MaxUploadsPerMinute,
		e.WaitSeconds,
	)
}

// NewUploadRateLimiter creates a new rate limiter with specified max uploads per minute
func NewUploadRateLimiter(maxUploadsPerMinute int) *UploadRateLimiter {
	return &UploadRateLimiter{
		maxUploadsPerMinute: maxUploadsPerMinute,
		perSessionLimiters:  make(map[string]*sessionRateLimiter),
	}
}

// CheckRateLimit verifies if an upload is allowed for the given session
// Returns RateLimitError if rate limit is exceeded, nil otherwise
func (rl *UploadRateLimiter) CheckRateLimit(sessionID string) error {
	rl.mu.Lock()
	limiter, exists := rl.perSessionLimiters[sessionID]
	if !exists {
		limiter = &sessionRateLimiter{
			uploadTimes: make([]time.Time, 0),
		}
		rl.perSessionLimiters[sessionID] = limiter
	}
	rl.mu.Unlock()

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)

	// Remove uploads older than 1 minute
	validUploads := make([]time.Time, 0)
	for _, uploadTime := range limiter.uploadTimes {
		if uploadTime.After(oneMinuteAgo) {
			validUploads = append(validUploads, uploadTime)
		}
	}
	limiter.uploadTimes = validUploads

	// Check if rate limit exceeded
	if len(limiter.uploadTimes) >= rl.maxUploadsPerMinute {
		oldestUpload := limiter.uploadTimes[0]
		waitDuration := time.Until(oldestUpload.Add(time.Minute))
		waitSeconds := int(waitDuration.Seconds()) + 1

		return &RateLimitError{
			SessionID:           sessionID,
			MaxUploadsPerMinute: rl.maxUploadsPerMinute,
			CurrentCount:        len(limiter.uploadTimes),
			WaitSeconds:         waitSeconds,
		}
	}

	// Record this upload
	limiter.uploadTimes = append(limiter.uploadTimes, now)
	return nil
}

// CleanupSession removes rate limiting data for a completed session
func (rl *UploadRateLimiter) CleanupSession(sessionID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.perSessionLimiters, sessionID)
}

// GetSessionStats returns current upload statistics for a session
func (rl *UploadRateLimiter) GetSessionStats(sessionID string) (currentCount int, maxCount int) {
	rl.mu.RLock()
	limiter, exists := rl.perSessionLimiters[sessionID]
	rl.mu.RUnlock()

	if !exists {
		return 0, rl.maxUploadsPerMinute
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)

	// Count valid uploads (within last minute)
	validCount := 0
	for _, uploadTime := range limiter.uploadTimes {
		if uploadTime.After(oneMinuteAgo) {
			validCount++
		}
	}

	return validCount, rl.maxUploadsPerMinute
}

// GetMaxUploadsPerMinute returns the configured rate limit
func (rl *UploadRateLimiter) GetMaxUploadsPerMinute() int {
	return rl.maxUploadsPerMinute
}
