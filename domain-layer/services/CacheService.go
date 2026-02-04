package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CacheService struct {
	cacheDir     string
	cacheTTL     time.Duration
	mu           sync.RWMutex
	cacheMetrics map[string]*CacheMetadata
}

type CacheMetadata struct {
	SID          string
	FilePath     string
	LastAccessed time.Time
	CreatedAt    time.Time
	FileSize     int64
}

func NewCacheService(cacheDir string, cacheTTL time.Duration) *CacheService {
	if cacheDir == "" {
		cacheDir = "/app/cache"
	}
	if cacheTTL == 0 {
		cacheTTL = 24 * time.Hour
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Printf("[CacheService] Failed to create cache directory: %v\n", err)
	}

	cs := &CacheService{
		cacheDir:     cacheDir,
		cacheTTL:     cacheTTL,
		cacheMetrics: make(map[string]*CacheMetadata),
	}

	go cs.cleanupExpiredCache()

	return cs
}

func (cs *CacheService) GetCachePath(sid, filename string) string {
	return filepath.Join(cs.cacheDir, sid, "decompressed", filename)
}

func (cs *CacheService) IsCached(sid, filename string) bool {
	cachePath := cs.GetCachePath(sid, filename)

	info, err := os.Stat(cachePath)
	if err != nil || info.IsDir() {
		return false
	}

	cs.mu.RLock()
	meta, exists := cs.cacheMetrics[cachePath]
	cs.mu.RUnlock()

	if !exists {
		cs.mu.Lock()
		cs.cacheMetrics[cachePath] = &CacheMetadata{
			SID:          sid,
			FilePath:     cachePath,
			LastAccessed: time.Now(),
			CreatedAt:    info.ModTime(),
			FileSize:     info.Size(),
		}
		cs.mu.Unlock()
		return true
	}

	if time.Since(meta.CreatedAt) > cs.cacheTTL {
		cs.DeleteCacheFile(sid, filename)
		return false
	}

	cs.mu.Lock()
	meta.LastAccessed = time.Now()
	cs.mu.Unlock()

	return true
}

func (cs *CacheService) CacheFile(sid, filename string, data []byte) error {
	cachePath := cs.GetCachePath(sid, filename)
	cacheDir := filepath.Dir(cachePath)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	cs.mu.Lock()
	cs.cacheMetrics[cachePath] = &CacheMetadata{
		SID:          sid,
		FilePath:     cachePath,
		LastAccessed: time.Now(),
		CreatedAt:    time.Now(),
		FileSize:     int64(len(data)),
	}
	cs.mu.Unlock()

	fmt.Printf("[CacheService]  Cached file: %s (size: %d bytes)\n", filename, len(data))
	return nil
}

func (cs *CacheService) ReadCacheFile(sid, filename string) ([]byte, error) {
	cachePath := cs.GetCachePath(sid, filename)

	cs.mu.Lock()
	if meta, exists := cs.cacheMetrics[cachePath]; exists {
		meta.LastAccessed = time.Now()
	}
	cs.mu.Unlock()

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	fmt.Printf("[CacheService]  Cache hit: %s (size: %d bytes)\n", filename, len(data))
	return data, nil
}

func (cs *CacheService) DeleteCacheFile(sid, filename string) error {
	cachePath := cs.GetCachePath(sid, filename)

	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete cache file: %w", err)
	}

	cs.mu.Lock()
	delete(cs.cacheMetrics, cachePath)
	cs.mu.Unlock()

	fmt.Printf("[CacheService]  Deleted cache file: %s\n", filename)
	return nil
}

func (cs *CacheService) DeleteScanCache(sid string) error {
	scanCacheDir := filepath.Join(cs.cacheDir, sid)

	if err := os.RemoveAll(scanCacheDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete scan cache: %w", err)
	}

	cs.mu.Lock()
	for path, meta := range cs.cacheMetrics {
		if meta.SID == sid {
			delete(cs.cacheMetrics, path)
		}
	}
	cs.mu.Unlock()

	fmt.Printf("[CacheService]  Deleted all cache for SID: %s\n", sid)
	return nil
}

func (cs *CacheService) cleanupExpiredCache() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cs.mu.Lock()
		now := time.Now()
		expiredPaths := []string{}

		for path, meta := range cs.cacheMetrics {
			if now.Sub(meta.CreatedAt) > cs.cacheTTL {
				expiredPaths = append(expiredPaths, path)
			}
		}

		for _, path := range expiredPaths {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				fmt.Printf("[CacheService]  Failed to delete expired cache: %s - %v\n", path, err)
			} else {
				fmt.Printf("[CacheService]  Cleaned expired cache: %s\n", path)
			}
			delete(cs.cacheMetrics, path)
		}
		cs.mu.Unlock()

		if len(expiredPaths) > 0 {
			fmt.Printf("[CacheService]  Cleanup completed: %d expired files removed\n", len(expiredPaths))
		}
	}
}

func (cs *CacheService) GetCacheStats() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	totalSize := int64(0)
	totalFiles := len(cs.cacheMetrics)

	for _, meta := range cs.cacheMetrics {
		totalSize += meta.FileSize
	}

	return map[string]interface{}{
		"total_files":      totalFiles,
		"total_size_bytes": totalSize,
		"total_size_mb":    float64(totalSize) / 1024 / 1024,
		"cache_directory":  cs.cacheDir,
		"cache_ttl_hours":  cs.cacheTTL.Hours(),
	}
}
