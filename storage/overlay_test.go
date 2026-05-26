package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

func TestOverlayLoadImagePrefersLocal(t *testing.T) {
	local := newFakeImageStorage()
	remote := newFakeImageStorage()
	local.images["image.jpg"] = []byte("local image")
	remote.images["image.jpg"] = []byte("remote image")

	img, err := NewOverlay(local, remote).LoadImage(context.Background(), "image.jpg")
	if err != nil {
		t.Fatalf("LoadImage returned an error: %v", err)
	}
	defer img.Close()

	if string(img.Data) != "local image" {
		t.Fatalf("got %q, want local image", img.Data)
	}
	if len(remote.loadCalls) != 0 {
		t.Fatalf("remote LoadImage was called %d times, want 0", len(remote.loadCalls))
	}
}

func TestOverlayLoadImageFallsBackToRemote(t *testing.T) {
	local := newFakeImageStorage()
	remote := newFakeImageStorage()
	remote.images["image.jpg"] = []byte("remote image")

	img, err := NewOverlay(local, remote).LoadImage(context.Background(), "image.jpg")
	if err != nil {
		t.Fatalf("LoadImage returned an error: %v", err)
	}
	defer img.Close()

	if string(img.Data) != "remote image" {
		t.Fatalf("got %q, want remote image", img.Data)
	}
	if len(local.loadCalls) != 1 {
		t.Fatalf("local LoadImage was called %d times, want 1", len(local.loadCalls))
	}
	if len(remote.loadCalls) != 1 {
		t.Fatalf("remote LoadImage was called %d times, want 1", len(remote.loadCalls))
	}
}

func TestOverlayLoadImageMissReturnsNotFound(t *testing.T) {
	local := newFakeImageStorage()
	remote := newFakeImageStorage()

	img, err := NewOverlay(local, remote).LoadImage(context.Background(), "missing.jpg")
	defer img.Close()
	if !errors.Is(err, NotFoundError) {
		t.Fatalf("got error %v, want NotFoundError", err)
	}
	if len(local.loadCalls) != 1 {
		t.Fatalf("local LoadImage was called %d times, want 1", len(local.loadCalls))
	}
	if len(remote.loadCalls) != 1 {
		t.Fatalf("remote LoadImage was called %d times, want 1", len(remote.loadCalls))
	}
}

func TestOverlayLoadImageReturnsLocalError(t *testing.T) {
	localErr := errors.New("local disk error")
	local := newFakeImageStorage()
	remote := newFakeImageStorage()
	local.loadErr = localErr
	remote.images["image.jpg"] = []byte("remote image")

	img, err := NewOverlay(local, remote).LoadImage(context.Background(), "image.jpg")
	defer img.Close()
	if !errors.Is(err, localErr) {
		t.Fatalf("got error %v, want local error", err)
	}
	if len(remote.loadCalls) != 0 {
		t.Fatalf("remote LoadImage was called %d times, want 0", len(remote.loadCalls))
	}
}

func TestOverlayUploadWritesLocalOnly(t *testing.T) {
	local := newFakeImageStorage()
	remote := newFakeImageStorage()
	overlay := NewOverlay(local, remote)

	if err := overlay.Upload("upload.jpg", []byte("upload bytes")); err != nil {
		t.Fatalf("Upload returned an error: %v", err)
	}
	if err := overlay.UploadFile("upload-file.jpg", bytes.NewReader([]byte("reader bytes"))); err != nil {
		t.Fatalf("UploadFile returned an error: %v", err)
	}

	if string(local.images["upload.jpg"]) != "upload bytes" {
		t.Fatalf("local Upload stored %q, want upload bytes", local.images["upload.jpg"])
	}
	if string(local.images["upload-file.jpg"]) != "reader bytes" {
		t.Fatalf("local UploadFile stored %q, want reader bytes", local.images["upload-file.jpg"])
	}
	if len(remote.uploadCalls) != 0 {
		t.Fatalf("remote Upload was called %d times, want 0", len(remote.uploadCalls))
	}
	if len(remote.uploadFileCalls) != 0 {
		t.Fatalf("remote UploadFile was called %d times, want 0", len(remote.uploadFileCalls))
	}
}

func TestOverlayDeleteDeletesLocalOnly(t *testing.T) {
	local := newFakeImageStorage()
	remote := newFakeImageStorage()
	local.images["image.jpg"] = []byte("local image")
	remote.images["image.jpg"] = []byte("remote image")

	if err := NewOverlay(local, remote).Delete("image.jpg"); err != nil {
		t.Fatalf("Delete returned an error: %v", err)
	}

	if _, ok := local.images["image.jpg"]; ok {
		t.Fatal("local image was not deleted")
	}
	if string(remote.images["image.jpg"]) != "remote image" {
		t.Fatalf("remote image changed to %q, want remote image", remote.images["image.jpg"])
	}
	if len(remote.deleteCalls) != 0 {
		t.Fatalf("remote Delete was called %d times, want 0", len(remote.deleteCalls))
	}
}

func TestOverlayDeleteRemoteOnlyIsNoOp(t *testing.T) {
	local := newFakeImageStorage()
	remote := newFakeImageStorage()
	remote.images["image.jpg"] = []byte("remote image")

	if err := NewOverlay(local, remote).Delete("image.jpg"); err != nil {
		t.Fatalf("Delete returned an error: %v", err)
	}

	if len(local.deleteCalls) != 1 {
		t.Fatalf("local Delete was called %d times, want 1", len(local.deleteCalls))
	}
	if len(remote.deleteCalls) != 0 {
		t.Fatalf("remote Delete was called %d times, want 0", len(remote.deleteCalls))
	}
	if string(remote.images["image.jpg"]) != "remote image" {
		t.Fatalf("remote image changed to %q, want remote image", remote.images["image.jpg"])
	}
}

type fakeImageStorage struct {
	images          map[string][]byte
	loadCalls       []string
	uploadCalls     []string
	uploadFileCalls []string
	deleteCalls     []string
	loadErr         error
	pool            *sync.Pool
}

func newFakeImageStorage() *fakeImageStorage {
	return &fakeImageStorage{
		images: make(map[string][]byte),
		pool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 1024)
			},
		},
	}
}

func (st *fakeImageStorage) NewImage() SourceImage {
	return SourceImage{
		Data: st.pool.Get().([]byte),
		pool: st.pool,
	}
}

func (st *fakeImageStorage) LoadImage(ctx context.Context, key string) (SourceImage, error) {
	st.loadCalls = append(st.loadCalls, key)
	img := st.NewImage()

	select {
	case <-ctx.Done():
		return img, ctx.Err()
	default:
	}

	if st.loadErr != nil {
		return img, st.loadErr
	}

	contents, ok := st.images[key]
	if !ok {
		return img, NotFoundError
	}

	if _, err := img.ReadFrom(bytes.NewReader(contents)); err != nil {
		return img, err
	}
	return img, nil
}

func (st *fakeImageStorage) Upload(key string, contents []byte) error {
	st.uploadCalls = append(st.uploadCalls, key)
	st.images[key] = append([]byte(nil), contents...)
	return nil
}

func (st *fakeImageStorage) UploadFile(key string, r io.Reader) error {
	st.uploadFileCalls = append(st.uploadFileCalls, key)
	contents, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	st.images[key] = contents
	return nil
}

func (st *fakeImageStorage) Delete(key string) error {
	st.deleteCalls = append(st.deleteCalls, key)
	if _, ok := st.images[key]; !ok {
		return NotFoundError
	}
	delete(st.images, key)
	return nil
}
