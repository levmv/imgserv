package vips

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if err := Init(&Config{ConcurrencyLevel: 1}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init libvips: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	Shutdown()
	os.Exit(code)
}

func TestLoadFromBufferRejectsEmptyWithoutPanic(t *testing.T) {
	var img Image
	defer img.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LoadFromBuffer panicked on empty buffer: %v", r)
		}
	}()

	if err := img.LoadFromBuffer(nil); err == nil {
		t.Fatal("expected an error for an empty buffer")
	}
}

func TestThumbnailFromBufferRejectsEmptyWithoutPanic(t *testing.T) {
	var img Image
	defer img.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ThumbnailFromBuffer panicked on empty buffer: %v", r)
		}
	}()

	if err := img.ThumbnailFromBuffer(nil, 10, 10, InterestingCentre, SizeDown); err == nil {
		t.Fatal("expected an error for an empty buffer")
	}
}

func TestAutoRotateBeforeStripPreservesUprightDimensions(t *testing.T) {
	tests := []struct {
		name        string
		orientation uint16
		wantWidth   int
		wantHeight  int
	}{
		{name: "normal", orientation: 1, wantWidth: 3, wantHeight: 2},
		{name: "rotate_180", orientation: 3, wantWidth: 3, wantHeight: 2},
		{name: "rotate_90_cw", orientation: 6, wantWidth: 2, wantHeight: 3},
		{name: "rotate_90_ccw", orientation: 8, wantWidth: 2, wantHeight: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := jpegWithOrientation(t, tt.orientation)

			var img Image
			defer img.Close()

			if err := img.LoadFromBuffer(data); err != nil {
				t.Fatalf("LoadFromBuffer failed: %v", err)
			}
			if err := img.AutoRotate(); err != nil {
				t.Fatalf("AutoRotate failed: %v", err)
			}
			if err := img.Strip(); err != nil {
				t.Fatalf("Strip failed: %v", err)
			}

			out, err := img.ExportJpeg(90)
			if err != nil {
				t.Fatalf("ExportJpeg failed: %v", err)
			}

			gotWidth, gotHeight := jpegDimensions(t, out)
			if gotWidth != tt.wantWidth || gotHeight != tt.wantHeight {
				t.Fatalf("got %dx%d, want %dx%d", gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestStripWithoutAutoRotateDoesNotBakeOrientation(t *testing.T) {
	data := jpegWithOrientation(t, 6)

	var img Image
	defer img.Close()

	if err := img.LoadFromBuffer(data); err != nil {
		t.Fatalf("LoadFromBuffer failed: %v", err)
	}
	if err := img.Strip(); err != nil {
		t.Fatalf("Strip failed: %v", err)
	}

	out, err := img.ExportJpeg(90)
	if err != nil {
		t.Fatalf("ExportJpeg failed: %v", err)
	}

	gotWidth, gotHeight := jpegDimensions(t, out)
	if gotWidth != 3 || gotHeight != 2 {
		t.Fatalf("strip/export without autorotate produced %dx%d, want original pixel dimensions 3x2", gotWidth, gotHeight)
	}
}

func TestFlattenTransparentPNGThenExport(t *testing.T) {
	var img Image
	defer img.Close()

	if err := img.LoadFromBuffer(transparentPNG(t)); err != nil {
		t.Fatalf("LoadFromBuffer failed: %v", err)
	}
	if err := img.Flatten(Color{R: 255, G: 255, B: 255}); err != nil {
		t.Fatalf("Flatten failed: %v", err)
	}

	out, err := img.ExportJpeg(90)
	if err != nil {
		t.Fatalf("ExportJpeg failed: %v", err)
	}

	gotWidth, gotHeight := jpegDimensions(t, out)
	if gotWidth != 2 || gotHeight != 2 {
		t.Fatalf("got %dx%d, want 2x2", gotWidth, gotHeight)
	}
}

func TestLoadFromBufferRejectsInvalidImage(t *testing.T) {
	var img Image
	defer img.Close()

	if err := img.LoadFromBuffer([]byte("not an image")); err == nil {
		t.Fatal("expected invalid image data to return an error")
	}
}

func TestThumbnailFromBufferMaintainsAspectRatio(t *testing.T) {
	var img Image
	defer img.Close()

	if err := img.LoadFromBuffer(rgbJPEG(t, 100, 50)); err != nil {
		t.Fatalf("LoadFromBuffer failed: %v", err)
	}
	if err := img.ThumbnailFromBuffer(rgbJPEG(t, 100, 50), 20, 0, InterestingNone, SizeDown); err != nil {
		t.Fatalf("ThumbnailFromBuffer failed: %v", err)
	}

	out, err := img.ExportJpeg(90)
	if err != nil {
		t.Fatalf("ExportJpeg failed: %v", err)
	}

	gotWidth, gotHeight := jpegDimensions(t, out)
	if gotWidth != 20 || gotHeight != 10 {
		t.Fatalf("got %dx%d, want 20x10", gotWidth, gotHeight)
	}
}

func TestThumbnailCropDimensions(t *testing.T) {
	var img Image
	defer img.Close()

	if err := img.LoadFromBuffer(rgbJPEG(t, 100, 50)); err != nil {
		t.Fatalf("LoadFromBuffer failed: %v", err)
	}
	if err := img.Thumbnail(20, 20, InterestingCentre, SizeDown); err != nil {
		t.Fatalf("Thumbnail failed: %v", err)
	}

	out, err := img.ExportJpeg(90)
	if err != nil {
		t.Fatalf("ExportJpeg failed: %v", err)
	}

	gotWidth, gotHeight := jpegDimensions(t, out)
	if gotWidth != 20 || gotHeight != 20 {
		t.Fatalf("got %dx%d, want 20x20", gotWidth, gotHeight)
	}
}

func TestResizeAlphaImageReportsInvalidRatio(t *testing.T) {
	var img Image
	defer img.Close()

	if err := img.LoadFromBuffer(transparentPNG(t)); err != nil {
		t.Fatalf("LoadFromBuffer failed: %v", err)
	}
	if err := img.Resize(0); err == nil {
		t.Fatal("expected Resize(0) to return an error")
	}
}

func TestExportRejectsInvalidQuality(t *testing.T) {
	tests := []struct {
		name   string
		export func(*Image) ([]byte, error)
	}{
		{
			name: "jpeg",
			export: func(img *Image) ([]byte, error) {
				return img.ExportJpeg(-1)
			},
		},
		{
			name: "webp",
			export: func(img *Image) ([]byte, error) {
				return img.ExportWebp(-1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var img Image
			defer img.Close()

			if err := img.LoadFromBuffer(rgbJPEG(t, 10, 10)); err != nil {
				t.Fatalf("LoadFromBuffer failed: %v", err)
			}
			if _, err := tt.export(&img); err == nil {
				t.Fatal("expected invalid quality to return an error")
			}
		})
	}
}

func TestHEICOrientationFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "orientation_*_want_*x*.heic"))
	if err != nil {
		t.Fatalf("failed to scan HEIC fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("add HEIC fixtures named testdata/orientation_<orientation>_want_<width>x<height>.heic")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var orientation, wantWidth, wantHeight int
			if _, err := fmt.Sscanf(filepath.Base(path), "orientation_%d_want_%dx%d.heic", &orientation, &wantWidth, &wantHeight); err != nil {
				t.Fatalf("fixture name must be orientation_<orientation>_want_<width>x<height>.heic: %v", err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read fixture: %v", err)
			}

			var img Image
			defer img.Close()

			if err := img.LoadFromBuffer(data); err != nil {
				t.Fatalf("LoadFromBuffer failed for orientation %d: %v", orientation, err)
			}
			if err := img.AutoRotate(); err != nil {
				t.Fatalf("AutoRotate failed for orientation %d: %v", orientation, err)
			}
			if err := img.Strip(); err != nil {
				t.Fatalf("Strip failed for orientation %d: %v", orientation, err)
			}

			out, err := img.ExportJpeg(90)
			if err != nil {
				t.Fatalf("ExportJpeg failed for orientation %d: %v", orientation, err)
			}

			gotWidth, gotHeight := jpegDimensions(t, out)
			if gotWidth != wantWidth || gotHeight != wantHeight {
				t.Fatalf("got %dx%d, want %dx%d", gotWidth, gotHeight, wantWidth, wantHeight)
			}
		})
	}
}

func jpegWithOrientation(t *testing.T, orientation uint16) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(2, 0, color.RGBA{B: 255, A: 255})
	img.Set(0, 1, color.RGBA{R: 255, G: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, B: 255, A: 255})
	img.Set(2, 1, color.RGBA{G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("failed to encode jpeg fixture: %v", err)
	}

	return addExifOrientation(t, buf.Bytes(), orientation)
}

func rgbJPEG(t *testing.T, width int, height int) []byte {
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
	if segmentLength > 0xffff {
		t.Fatalf("EXIF payload is too large: %d", segmentLength)
	}

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

func transparentPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{G: 255, A: 128})
	img.Set(0, 1, color.NRGBA{B: 255, A: 64})
	img.Set(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 0})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode png fixture: %v", err)
	}
	return buf.Bytes()
}

func jpegDimensions(t *testing.T, data []byte) (int, int) {
	t.Helper()

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to decode exported jpeg: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("got exported format %q, want jpeg", format)
	}
	return cfg.Width, cfg.Height
}

func writeBE(t *testing.T, buf *bytes.Buffer, value interface{}) {
	t.Helper()

	if err := binary.Write(buf, binary.BigEndian, value); err != nil {
		t.Fatalf("failed to write EXIF fixture: %v", err)
	}
}
