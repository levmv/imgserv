package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	cacheMaxBytes           int64 = 1 << 30
	cacheLowWaterBytes            = cacheMaxBytes * 3 / 4
	cacheCleanupGrowthBytes int64 = 64 << 20
	cacheInactiveTTL              = 6 * time.Hour
	negativeCacheTTL              = 30 * time.Minute
	temporaryCacheFileTTL         = time.Hour
	cacheCleanupInterval          = 30 * time.Minute
	negativeCacheContents         = "404"
)

type cacheJanitor struct {
	trigger chan struct{}
	growth  atomic.Int64
}

type cacheEntry struct {
	path    string
	size    int64
	modTime time.Time
}

type cacheCleanupStats struct {
	Scanned      int
	Removed      int
	BytesRemoved int64
	BytesAfter   int64
}

func (cs *Cached) startCacheJanitor() {
	cs.janitor = &cacheJanitor{trigger: make(chan struct{}, 1)}
	go cs.runCacheJanitor()
}

func (cs *Cached) noteCacheWrite(size int64) {
	if cs.janitor == nil || size <= 0 {
		return
	}
	if cs.janitor.growth.Add(size) < cacheCleanupGrowthBytes {
		return
	}

	cs.janitor.growth.Swap(0)
	select {
	case cs.janitor.trigger <- struct{}{}:
	default:
	}
}

func (cs *Cached) runCacheJanitor() {
	cs.runCacheCleanup("startup")

	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cs.runCacheCleanup("interval")
		case <-cs.janitor.trigger:
			cs.runCacheCleanup("growth")
		}
	}
}

func (cs *Cached) runCacheCleanup(reason string) {
	started := time.Now()
	stats, err := cs.cleanupCache(started)
	if err != nil {
		log.Printf("storage cache cleanup reason=%s path=%q error=%v", reason, cs.basePath, err)
	}
	if stats.Removed > 0 {
		log.Printf(
			"cache cleanup %s: removed %d/%d files, freed %s, %s remain (%s)",
			reason,
			stats.Removed,
			stats.Scanned,
			formatCacheBytes(stats.BytesRemoved),
			formatCacheBytes(stats.BytesAfter),
			time.Since(started).Round(time.Millisecond),
		)
	}
}

func formatCacheBytes(size int64) string {
	const (
		kib = 1 << 10
		mib = 1 << 20
		gib = 1 << 30
	)

	switch {
	case size >= gib:
		return fmt.Sprintf("%.2f GiB", float64(size)/gib)
	case size >= mib:
		return fmt.Sprintf("%.0f MiB", float64(size)/mib)
	case size >= kib:
		return fmt.Sprintf("%.0f KiB", float64(size)/kib)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func (cs *Cached) cleanupCache(now time.Time) (cacheCleanupStats, error) {
	return cs.cleanupCacheWithLimits(now, cacheMaxBytes, cacheLowWaterBytes)
}

func (cs *Cached) cleanupCacheWithLimits(now time.Time, maxBytes int64, lowWaterBytes int64) (cacheCleanupStats, error) {
	stats := cacheCleanupStats{}
	candidates := make([]cacheEntry, 0)
	var cleanupErr error

	walkErr := filepath.WalkDir(cs.basePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if !errors.Is(walkErr, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, walkErr)
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("stat cache file %s: %w", path, err))
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		stats.Scanned++
		stats.BytesAfter += info.Size()

		age := now.Sub(info.ModTime())
		isTemporary := strings.HasPrefix(entry.Name(), "goresizer")
		isNegative, err := negativeCacheFile(path, info)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}

		expired := (isTemporary && age >= temporaryCacheFileTTL) ||
			(isNegative && age >= negativeCacheTTL) ||
			(!isTemporary && !isNegative && age >= cacheInactiveTTL)
		if expired {
			if err := removeCacheEntry(path, info.Size(), &stats); err != nil {
				cleanupErr = errors.Join(cleanupErr, err)
			}
			return nil
		}

		if !isTemporary {
			candidates = append(candidates, cacheEntry{path: path, size: info.Size(), modTime: info.ModTime()})
		}
		return nil
	})
	cleanupErr = errors.Join(cleanupErr, walkErr)

	if stats.BytesAfter > maxBytes {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].modTime.Before(candidates[j].modTime)
		})
		for _, candidate := range candidates {
			if stats.BytesAfter <= lowWaterBytes {
				break
			}
			if err := removeCacheEntry(candidate.path, candidate.size, &stats); err != nil {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
	}

	return stats, cleanupErr
}

func negativeCacheFile(path string, info fs.FileInfo) (bool, error) {
	if info.Size() != int64(len(negativeCacheContents)) {
		return false, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open possible negative cache file %s: %w", path, err)
	}
	defer file.Close()

	currentInfo, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("stat possible negative cache file %s: %w", path, err)
	}
	if currentInfo.Size() != int64(len(negativeCacheContents)) {
		return false, nil
	}

	contents := make([]byte, len(negativeCacheContents))
	if _, err := io.ReadFull(file, contents); err != nil {
		return false, fmt.Errorf("read possible negative cache file %s: %w", path, err)
	}
	return string(contents) == negativeCacheContents, nil
}

func removeCacheEntry(path string, size int64, stats *cacheCleanupStats) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove cache file %s: %w", path, err)
	}
	stats.Removed++
	stats.BytesRemoved += size
	stats.BytesAfter -= size
	return nil
}
