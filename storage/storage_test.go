package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInitCachePathRequiresNonEmptyPath(t *testing.T) {
	if _, err := initCachePath(""); err == nil {
		t.Fatal("expected empty cache path to return an error")
	}
}

func TestInitCachePathCreatesDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache", "nested")

	got, err := initCachePath(base)
	if err != nil {
		t.Fatalf("initCachePath returned an error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("got %q, want absolute path", got)
	}
	if info, err := os.Stat(got); err != nil {
		t.Fatalf("cache path was not created: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("cache path %q is not a directory", got)
	}
}

func TestCacheFileAndGetCachedRoundTrip(t *testing.T) {
	cs := newTestCache(t)

	if err := cs.cacheFile("some/key.jpg", []byte("image bytes")); err != nil {
		t.Fatalf("cacheFile returned an error: %v", err)
	}

	r, err := cs.getCached("some/key.jpg")
	if err != nil {
		t.Fatalf("getCached returned an error: %v", err)
	}
	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read cached file: %v", err)
	}
	if string(got) != "image bytes" {
		t.Fatalf("got %q, want cached contents", got)
	}
}

func TestGetCachedRefreshesPositiveEntryMTime(t *testing.T) {
	cs := newTestCache(t)
	cachePath := writeTestCacheFile(t, cs, "some/key.jpg", []byte("image bytes"), time.Now().Add(-time.Hour))

	r, err := cs.getCached("some/key.jpg")
	if err != nil {
		t.Fatalf("getCached returned an error: %v", err)
	}
	r.Close()

	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat cached file: %v", err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Fatalf("positive cache mtime was not refreshed: %s", info.ModTime())
	}
}

func TestGetCachedMiss(t *testing.T) {
	cs := newTestCache(t)

	r, err := cs.getCached("missing.jpg")
	if r != nil {
		t.Fatal("expected nil reader on cache miss")
	}
	if !errors.Is(err, NotCached) {
		t.Fatalf("got error %v, want NotCached", err)
	}
}

func TestGetCachedNotFoundSentinel(t *testing.T) {
	cs := newTestCache(t)

	if err := cs.cacheFile("missing.jpg", []byte("404")); err != nil {
		t.Fatalf("cacheFile returned an error: %v", err)
	}

	r, err := cs.getCached("missing.jpg")
	if r != nil {
		t.Fatal("expected nil reader for cached 404 sentinel")
	}
	if !errors.Is(err, NotFoundError) {
		t.Fatalf("got error %v, want NotFoundError", err)
	}
}

func TestGetCachedNotFoundSentinelDoesNotRefreshTTL(t *testing.T) {
	cs := newTestCache(t)
	if err := cs.cacheFile("missing.jpg", []byte(negativeCacheContents)); err != nil {
		t.Fatalf("cacheFile returned an error: %v", err)
	}

	cachePath := cs.hashName("missing.jpg")
	createdAt := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	if err := os.Chtimes(cachePath, createdAt, createdAt); err != nil {
		t.Fatalf("set sentinel mtime: %v", err)
	}

	r, err := cs.getCached("missing.jpg")
	if r != nil {
		t.Fatal("expected nil reader for cached 404 sentinel")
	}
	if !errors.Is(err, NotFoundError) {
		t.Fatalf("got error %v, want NotFoundError", err)
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat sentinel: %v", err)
	}
	if !info.ModTime().Equal(createdAt) {
		t.Fatalf("sentinel mtime changed from %s to %s", createdAt, info.ModTime())
	}
}

func TestGetCachedExpiresNotFoundSentinel(t *testing.T) {
	cs := newTestCache(t)
	if err := cs.cacheFile("missing.jpg", []byte(negativeCacheContents)); err != nil {
		t.Fatalf("cacheFile returned an error: %v", err)
	}

	cachePath := cs.hashName("missing.jpg")
	expiredAt := time.Now().Add(-negativeCacheTTL - time.Minute)
	if err := os.Chtimes(cachePath, expiredAt, expiredAt); err != nil {
		t.Fatalf("set sentinel mtime: %v", err)
	}

	r, err := cs.getCached("missing.jpg")
	if r != nil {
		t.Fatal("expected nil reader for expired 404 sentinel")
	}
	if !errors.Is(err, NotCached) {
		t.Fatalf("got error %v, want NotCached", err)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired sentinel still exists: %v", err)
	}
}

func TestGetCachedAllowsThreeByteImage(t *testing.T) {
	cs := newTestCache(t)
	if err := cs.cacheFile("tiny.jpg", []byte("img")); err != nil {
		t.Fatalf("cacheFile returned an error: %v", err)
	}

	r, err := cs.getCached("tiny.jpg")
	if err != nil {
		t.Fatalf("getCached returned an error: %v", err)
	}
	defer r.Close()

	contents, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read cached file: %v", err)
	}
	if string(contents) != "img" {
		t.Fatalf("got %q, want three-byte contents", contents)
	}
}

func TestCleanupCacheExpiresInactiveAndTemporaryFiles(t *testing.T) {
	cs := newTestCache(t)
	now := time.Now()

	freshImage := writeTestCacheFile(t, cs, "fresh.jpg", []byte("fresh"), now.Add(-time.Hour))
	expiredImage := writeTestCacheFile(t, cs, "expired.jpg", []byte("expired"), now.Add(-cacheInactiveTTL-time.Minute))
	freshNegative := writeTestCacheFile(t, cs, "fresh-missing.jpg", []byte(negativeCacheContents), now.Add(-time.Minute))
	expiredNegative := writeTestCacheFile(t, cs, "expired-missing.jpg", []byte(negativeCacheContents), now.Add(-negativeCacheTTL-time.Minute))

	tempDir := filepath.Join(cs.basePath, "ff")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatalf("create temp cache directory: %v", err)
	}
	expiredTemp := filepath.Join(tempDir, "goresizer-old")
	if err := os.WriteFile(expiredTemp, []byte("temporary"), 0o644); err != nil {
		t.Fatalf("create temporary cache file: %v", err)
	}
	setTestFileTime(t, expiredTemp, now.Add(-temporaryCacheFileTTL-time.Minute))
	freshTemp := filepath.Join(tempDir, "goresizer-active")
	if err := os.WriteFile(freshTemp, []byte("temporary"), 0o644); err != nil {
		t.Fatalf("create fresh temporary cache file: %v", err)
	}

	stats, err := cs.cleanupCache(now)
	if err != nil {
		t.Fatalf("cleanupCache returned an error: %v", err)
	}
	if stats.Removed != 3 {
		t.Fatalf("removed %d files, want 3", stats.Removed)
	}
	assertTestFileExists(t, freshImage, true)
	assertTestFileExists(t, freshNegative, true)
	assertTestFileExists(t, expiredImage, false)
	assertTestFileExists(t, expiredNegative, false)
	assertTestFileExists(t, expiredTemp, false)
	assertTestFileExists(t, freshTemp, true)
}

func TestCleanupCacheTrimsOldestFilesBelowLowWatermark(t *testing.T) {
	cs := newTestCache(t)
	now := time.Now()
	const (
		entrySize     int64 = 400
		maxBytes            = 1000
		lowWaterBytes       = 750
	)

	oldest := writeSizedTestCacheFile(t, cs, "oldest.jpg", entrySize, now.Add(-3*time.Hour))
	middle := writeSizedTestCacheFile(t, cs, "middle.jpg", entrySize, now.Add(-2*time.Hour))
	newest := writeSizedTestCacheFile(t, cs, "newest.jpg", entrySize, now.Add(-time.Hour))

	stats, err := cs.cleanupCacheWithLimits(now, maxBytes, lowWaterBytes)
	if err != nil {
		t.Fatalf("cleanupCache returned an error: %v", err)
	}
	if stats.Removed != 2 {
		t.Fatalf("removed %d files, want 2", stats.Removed)
	}
	if stats.BytesAfter != entrySize {
		t.Fatalf("bytes after cleanup = %d, want %d", stats.BytesAfter, entrySize)
	}
	assertTestFileExists(t, oldest, false)
	assertTestFileExists(t, middle, false)
	assertTestFileExists(t, newest, true)
}

func TestCacheWriteSchedulesCleanupAfterGrowthThreshold(t *testing.T) {
	cs := newTestCache(t)
	cs.janitor = &cacheJanitor{trigger: make(chan struct{}, 1)}

	cs.noteCacheWrite(cacheCleanupGrowthBytes - 1)
	select {
	case <-cs.janitor.trigger:
		t.Fatal("cleanup was scheduled below growth threshold")
	default:
	}

	cs.noteCacheWrite(1)
	select {
	case <-cs.janitor.trigger:
	default:
		t.Fatal("cleanup was not scheduled at growth threshold")
	}
	if got := cs.janitor.growth.Load(); got != 0 {
		t.Fatalf("growth after scheduling = %d, want 0", got)
	}
}

func TestCacheReadWriteAndCleanupAreConcurrentSafe(t *testing.T) {
	cs := newTestCache(t)
	first := []byte(strings.Repeat("a", 4096))
	second := []byte(strings.Repeat("b", 4096))
	if err := cs.cacheFile("shared.jpg", first); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			contents := first
			if i%2 == 1 {
				contents = second
			}
			if err := cs.cacheFile("shared.jpg", contents); err != nil {
				errs <- fmt.Errorf("write cached file: %w", err)
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 400; i++ {
			reader, err := cs.getCached("shared.jpg")
			if errors.Is(err, NotCached) {
				continue
			}
			if err != nil {
				errs <- fmt.Errorf("open cached file: %w", err)
				return
			}
			contents, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil {
				errs <- fmt.Errorf("read cached file: %w", readErr)
				return
			}
			if closeErr != nil {
				errs <- fmt.Errorf("close cached file: %w", closeErr)
				return
			}
			if !bytes.Equal(contents, first) && !bytes.Equal(contents, second) {
				errs <- fmt.Errorf("read partial cached contents of %d bytes", len(contents))
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			if _, err := cs.cleanupCacheWithLimits(time.Now(), 1, 0); err != nil {
				errs <- fmt.Errorf("clean cache: %w", err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestLoadImageRejectsEmptyCachedFile(t *testing.T) {
	cs := newTestCache(t)

	if err := cs.cacheFile("empty.jpg", nil); err != nil {
		t.Fatalf("cacheFile returned an error: %v", err)
	}

	img, err := cs.LoadImage(t.Context(), "empty.jpg")
	defer img.Close()
	if err == nil || !strings.Contains(err.Error(), "empty input file") {
		t.Fatalf("got error %v, want empty input file", err)
	}
}

func TestHashNameDependsOnBucketAndPath(t *testing.T) {
	cs := newTestCache(t)
	other := newTestCache(t)
	other.s3.Bucket = "other-bucket"

	first := cs.hashName("same/path")
	second := cs.hashName("same/path")
	if first != second {
		t.Fatalf("hashName is not stable: %q != %q", first, second)
	}
	if first == cs.hashName("different/path") {
		t.Fatal("hashName should differ for different paths")
	}
	if first == other.hashName("same/path") {
		t.Fatal("hashName should differ for different buckets")
	}
}

func TestSourceImageReadFromResetsExistingData(t *testing.T) {
	si := SourceImage{Data: []byte("old-data")}

	n, err := si.ReadFrom(bytes.NewBufferString("new-data"))
	if err != nil {
		t.Fatalf("ReadFrom returned an error: %v", err)
	}
	if n != int64(len("new-data")) {
		t.Fatalf("got read count %d, want %d", n, len("new-data"))
	}
	if string(si.Data) != "new-data" {
		t.Fatalf("got %q, want new data", si.Data)
	}
}

func TestLocalStorageRoundTrip(t *testing.T) {
	ls, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal returned an error: %v", err)
	}

	if err := ls.Upload("nested/image.jpg", []byte("local image")); err != nil {
		t.Fatalf("Upload returned an error: %v", err)
	}

	img, err := ls.LoadImage(context.Background(), "nested/image.jpg")
	if err != nil {
		t.Fatalf("LoadImage returned an error: %v", err)
	}
	defer img.Close()

	if string(img.Data) != "local image" {
		t.Fatalf("got %q, want uploaded contents", img.Data)
	}
}

func TestLocalStorageUploadFileAndDelete(t *testing.T) {
	ls, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal returned an error: %v", err)
	}

	if err := ls.UploadFile("image.jpg", strings.NewReader("from reader")); err != nil {
		t.Fatalf("UploadFile returned an error: %v", err)
	}
	if err := ls.Delete("image.jpg"); err != nil {
		t.Fatalf("Delete returned an error: %v", err)
	}

	img, err := ls.LoadImage(context.Background(), "image.jpg")
	defer img.Close()
	if !errors.Is(err, NotFoundError) {
		t.Fatalf("got error %v, want NotFoundError", err)
	}
}

func TestLocalStorageRejectsUnsafeKeys(t *testing.T) {
	ls, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal returned an error: %v", err)
	}

	for _, key := range []string{
		"",
		"/absolute.jpg",
		"../escape.jpg",
		"nested/../escape.jpg",
		"nested//image.jpg",
		"nested/./image.jpg",
		`nested\image.jpg`,
	} {
		t.Run(key, func(t *testing.T) {
			if err := ls.Upload(key, []byte("data")); err == nil {
				t.Fatalf("expected Upload to reject key %q", key)
			}
		})
	}
}

func newTestCache(t *testing.T) *Cached {
	t.Helper()

	return &Cached{
		s3:       S3Storage{Bucket: "test-bucket"},
		basePath: t.TempDir(),
		pool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 1024)
			},
		},
	}
}

func writeTestCacheFile(t *testing.T, cs *Cached, key string, contents []byte, modTime time.Time) string {
	t.Helper()
	if err := cs.cacheFile(key, contents); err != nil {
		t.Fatalf("cache %s: %v", key, err)
	}
	path := cs.hashName(key)
	setTestFileTime(t, path, modTime)
	return path
}

func writeSizedTestCacheFile(t *testing.T, cs *Cached, key string, size int64, modTime time.Time) string {
	t.Helper()
	path := cs.hashName(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create cache directory: %v", err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("create sized cache file: %v", err)
	}
	setTestFileTime(t, path, modTime)
	return path
}

func setTestFileTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set mtime for %s: %v", path, err)
	}
}

func assertTestFileExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if want && err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if !want && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, got: %v", path, err)
	}
}
