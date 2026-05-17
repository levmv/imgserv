package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/storage"
	"github.com/levmv/imgserv/vips"
)

var (
	uploadTestVipsOnce sync.Once
	uploadTestVipsErr  error
)

func TestUploadPhotoStoresGeneratedJPEGInLocalStorage(t *testing.T) {
	withLocalUploadStorage(t)

	info, err := uploadPhoto("generated/photo.jpg", bytes.NewReader(testJPEG(t, 4, 3)))
	if err != nil {
		t.Fatalf("uploadPhoto returned an error: %v", err)
	}
	if info.Name != "generated/photo.jpg" || info.Width != 4 || info.Height != 3 {
		t.Fatalf("got info %+v, want name generated/photo.jpg and size 4x3", info)
	}

	stored := loadStoredImage(t, "generated/photo.jpg")
	width, height := jpegDimensions(t, stored.Data)
	if width != 4 || height != 3 {
		t.Fatalf("stored image is %dx%d, want 4x3", width, height)
	}
}

func TestUploadPhotoAppliesOrientationBeforeStripping(t *testing.T) {
	withLocalUploadStorage(t)

	info, err := uploadPhoto("generated/oriented.jpg", bytes.NewReader(jpegWithOrientation(t, 6)))
	if err != nil {
		t.Fatalf("uploadPhoto returned an error: %v", err)
	}
	if info.Width != 2 || info.Height != 3 {
		t.Fatalf("got upload info size %dx%d, want upright size 2x3", info.Width, info.Height)
	}

	stored := loadStoredImage(t, "generated/oriented.jpg")
	width, height := jpegDimensions(t, stored.Data)
	if width != 2 || height != 3 {
		t.Fatalf("stored image is %dx%d, want upright size 2x3", width, height)
	}
}

func withLocalUploadStorage(t *testing.T) {
	t.Helper()

	uploadTestVipsOnce.Do(func() {
		uploadTestVipsErr = vips.Init(&vips.Config{ConcurrencyLevel: 1})
	})
	if uploadTestVipsErr != nil {
		t.Fatalf("failed to init vips: %v", uploadTestVipsErr)
	}

	oldStorage := imgStorage
	oldPreprocess := preprocess
	oldMaxWidth := maxWidth
	oldMaxHeight := maxHeight
	t.Cleanup(func() {
		imgStorage = oldStorage
		preprocess = oldPreprocess
		maxWidth = oldMaxWidth
		maxHeight = oldMaxHeight
	})

	localStorage, err := storage.New(config.StorageConf{
		Type:      "local",
		LocalPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("failed to create local storage: %v", err)
	}
	imgStorage = localStorage
	initUpload(config.StorageConf{})
}

func loadStoredImage(t *testing.T, key string) storage.SourceImage {
	t.Helper()

	img, err := imgStorage.LoadImage(t.Context(), key)
	if err != nil {
		t.Fatalf("failed to load stored image %q: %v", key, err)
	}
	t.Cleanup(img.Close)
	return img
}

func testJPEG(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / max(1, width-1)),
				G: uint8((y * 255) / max(1, height-1)),
				B: uint8(((x + y) * 255) / max(1, width+height-2)),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("failed to encode jpeg fixture: %v", err)
	}
	return buf.Bytes()
}

func jpegWithOrientation(t *testing.T, orientation uint16) []byte {
	t.Helper()

	return addExifOrientation(t, testJPEG(t, 3, 2), orientation)
}

func addExifOrientation(t *testing.T, jpegData []byte, orientation uint16) []byte {
	t.Helper()

	if len(jpegData) < 2 || jpegData[0] != 0xff || jpegData[1] != 0xd8 {
		t.Fatal("jpeg fixture is missing SOI marker")
	}

	var tiff bytes.Buffer
	tiff.Write([]byte{'M', 'M'})
	writeBE(t, &tiff, uint16(42))
	writeBE(t, &tiff, uint32(8))
	writeBE(t, &tiff, uint16(1))
	writeBE(t, &tiff, uint16(0x0112))
	writeBE(t, &tiff, uint16(3))
	writeBE(t, &tiff, uint32(1))
	writeBE(t, &tiff, orientation)
	writeBE(t, &tiff, uint16(0))
	writeBE(t, &tiff, uint32(0))

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	segmentLength := len(payload) + 2

	app1 := []byte{
		0xff,
		0xe1,
		byte(segmentLength >> 8),
		byte(segmentLength),
	}
	app1 = append(app1, payload...)

	out := make([]byte, 0, len(jpegData)+len(app1))
	out = append(out, jpegData[:2]...)
	out = append(out, app1...)
	out = append(out, jpegData[2:]...)
	return out
}

func jpegDimensions(t *testing.T, data []byte) (int, int) {
	t.Helper()

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to decode jpeg: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("got format %q, want jpeg", format)
	}
	return cfg.Width, cfg.Height
}

func writeBE(t *testing.T, buf *bytes.Buffer, value interface{}) {
	t.Helper()

	if err := binary.Write(buf, binary.BigEndian, value); err != nil {
		t.Fatalf("failed to write EXIF fixture: %v", err)
	}
}
