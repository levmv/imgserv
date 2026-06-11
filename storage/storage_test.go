package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
