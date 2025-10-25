package vips

import (
	"context"
	"fmt"
	"log"
	"runtime"
	dbg "runtime/debug"
	"unsafe"
)

/*
#cgo pkg-config: vips
#cgo CFLAGS: -O3
#include "vips.h"
*/
import "C"

type Image struct {
	VipsImage *C.VipsImage
	Data      []byte
	cancel    context.CancelFunc
}

type Interesting int

// Interesting constants represent areas of interest which smart cropping will crop based on.
const (
	InterestingNone      Interesting = C.VIPS_INTERESTING_NONE
	InterestingCentre    Interesting = C.VIPS_INTERESTING_CENTRE
	InterestingEntropy   Interesting = C.VIPS_INTERESTING_ENTROPY
	InterestingAttention Interesting = C.VIPS_INTERESTING_ATTENTION
	InterestingAll       Interesting = C.VIPS_INTERESTING_ALL
	InterestingLast      Interesting = C.VIPS_INTERESTING_LAST
)

// Size represents VipsSize type
type Size int

const (
	SizeBoth  Size = C.VIPS_SIZE_BOTH
	SizeUp    Size = C.VIPS_SIZE_UP
	SizeDown  Size = C.VIPS_SIZE_DOWN
	SizeForce Size = C.VIPS_SIZE_FORCE
	SizeLast  Size = C.VIPS_SIZE_LAST
)

// ExtendStrategy represents VIPS_EXTEND type
type ExtendStrategy int

// ExtendStrategy enum
const (
	ExtendBlack      ExtendStrategy = C.VIPS_EXTEND_BLACK
	ExtendCopy       ExtendStrategy = C.VIPS_EXTEND_COPY
	ExtendRepeat     ExtendStrategy = C.VIPS_EXTEND_REPEAT
	ExtendMirror     ExtendStrategy = C.VIPS_EXTEND_MIRROR
	ExtendWhite      ExtendStrategy = C.VIPS_EXTEND_WHITE
	ExtendBackground ExtendStrategy = C.VIPS_EXTEND_BACKGROUND
)

type Color struct{ R, G, B uint8 }

// ColorRGBA represents an RGB with alpha channel (A)
type ColorRGBA struct {
	R, G, B, A uint8
}

type MemoryStats struct {
	Mem     int64
	MemHigh int64
	Files   int64
	Allocs  int64
}

type Config struct {
	ConcurrencyLevel int
	MaxCacheMem      int
	MaxCacheSize     int
	ReportLeaks      bool
	CollectStats     bool
}

var Version = C.GoString(C.vips_version_string())

func Init(conf *Config) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := C.vips_initialize(); err != 0 {
		C.vips_shutdown()
		return fmt.Errorf("Unable to start vips code=%v", err)
	}

	if conf == nil {
		conf = &Config{
			ConcurrencyLevel: 1,
			ReportLeaks:      true,
		}
	}

	//SetLogging(conf.LogLevel)

	C.vips_concurrency_set(C.int(conf.ConcurrencyLevel))
	if conf.ReportLeaks {
		C.vips_leak_set(C.gboolean(1))
	}
	//C.vips_cache_set_trace(C.gboolean(1))
	C.vips_cache_set_max_mem(0)
	C.vips_cache_set_max(0)

	log.Printf("vips %s started with concurrency=%d",
		string(Version),
		int(C.vips_concurrency_get()))

	return nil
}

func Shutdown() {
	C.vips_shutdown()
}

func (img *Image) SetCancel(fn context.CancelFunc) {
	img.cancel = fn
}

func (img *Image) Width() int {
	return int(img.VipsImage.Xsize)
}

func (img *Image) Height() int {
	return int(img.VipsImage.Ysize)
}

func (img *Image) Close() {
	if img.VipsImage != nil {
		C.clear_image(&img.VipsImage)
	}
	if img.cancel != nil {
		img.cancel()
	}
}

func (img *Image) Resize(ratio float64) error {
	var out *C.VipsImage
	if err := C.resize(img.VipsImage, &out, C.double(ratio)); err != 0 {
		return handleImageError(out)
	}

	C.swap_and_clear(&img.VipsImage, out)
	return nil
}

func (img *Image) Crop(x int, y int, width int, height int) error {
	var out *C.VipsImage
	if err := C.crop(img.VipsImage, &out, C.int(x), C.int(y), C.int(width), C.int(height)); err != 0 {
		return handleImageError(out)
	}

	C.swap_and_clear(&img.VipsImage, out)
	return nil
}

func (img *Image) Thumbnail(width int, height int, crop Interesting, size Size) error {
	var out *C.VipsImage

	if err := C.thumbnail(img.VipsImage, &out, C.int(width), C.int(height), C.int(crop), C.int(size)); err != 0 {
		return handleImageError(out)
	}
	C.swap_and_clear(&img.VipsImage, out)

	return nil
}

func (img *Image) Flatten(bg Color) error {
	var out *C.VipsImage
	if err := C.flatten_image(img.VipsImage, &out, C.double(bg.R), C.double(bg.G), C.double(bg.B)); err != 0 {
		return handleImageError(out)
	}
	C.swap_and_clear(&img.VipsImage, out)

	return nil
}

func (img *Image) Embed(x int, y int, width int, height int) error {
	var out *C.VipsImage

	if err := C.embed_image(img.VipsImage, &out, C.int(x), C.int(y), C.int(width), C.int(height)); err != 0 {
		return handleImageError(out)
	}

	C.swap_and_clear(&img.VipsImage, out)

	return nil
}

func (img *Image) EmbedBackground(x int, y int, width int, height int, bg Color) error {
	var out *C.VipsImage

	if err := C.embed_image_background(img.VipsImage, &out, C.int(x), C.int(y), C.int(width), C.int(height),
		C.double(bg.R), C.double(bg.G), C.double(bg.B), 0); err != 0 {
		return handleImageError(out)
	}

	C.swap_and_clear(&img.VipsImage, out)

	return nil
}

func (img *Image) Composite(overlay *Image) error {
	var out *C.VipsImage

	if err := C.composite_image(img.VipsImage, overlay.VipsImage, &out); err != 0 {
		return handleImageError(out)
	}

	C.swap_and_clear(&img.VipsImage, out)

	return nil
}

func (img *Image) Strip() error {
	var out *C.VipsImage

	if err := C.strip(img.VipsImage, &out); err != 0 {
		return handleImageError(out)
	}
	C.swap_and_clear(&img.VipsImage, out)

	return nil
}

func (img *Image) ExportJpeg(quality int) ([]byte, error) {

	var ptr unsafe.Pointer
	// We use unsafe.Slice, so we need to free this memory later
	cancel := func() {
		C.g_free_go(&ptr)
	}

	imgsize := C.size_t(0)

	err := C.jpegsave(img.VipsImage, &ptr, &imgsize, C.int(quality))

	if err != 0 {
		C.g_free_go(&ptr)
		return nil, handleVipsError()
	}
	buf := unsafe.Slice((*byte)(ptr), int(imgsize))

	img.SetCancel(cancel)

	C.vips_error_clear()
	return buf, nil
}

func (img *Image) ExportWebp(quality int) ([]byte, error) {

	var ptr unsafe.Pointer
	// We use unsafe.Slice, so we need to free this memory later
	cancel := func() {
		C.g_free_go(&ptr)
	}

	imgsize := C.size_t(0)

	err := C.webpsave(img.VipsImage, &ptr, &imgsize, C.int(quality))

	if err != 0 {
		C.g_free_go(&ptr)
		return nil, handleVipsError()
	}
	buf := unsafe.Slice((*byte)(ptr), int(imgsize))

	img.SetCancel(cancel)

	return buf, nil
}

func (img *Image) LoadFromBuffer(buf []byte) error {
	img.VipsImage = C.image_new_from_buffer(unsafe.Pointer(&buf[0]), C.size_t(len(buf)))

	if img.VipsImage == nil {
		return handleVipsError()
	}
	return nil
}

func (img *Image) ThumbnailFromBuffer(buf []byte, width int, height int, crop Interesting, size Size) error {
	var out *C.VipsImage

	if err := C.thumbnail_buffer(unsafe.Pointer(&buf[0]), C.size_t(len(buf)), &out, C.int(width), C.int(height), C.int(crop), C.int(size)); err != 0 {
		return handleVipsError()
	}

	C.swap_and_clear(&img.VipsImage, out)
	return nil
}

func (img *Image) Label(text string, font string, fontFile string, color Color, x int, y int, width int, height int) error {
	var tmp *C.VipsImage

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	f := C.CString(font)
	defer C.free(unsafe.Pointer(f))

	ff := C.CString(fontFile)
	defer C.free(unsafe.Pointer(ff))

	if err := C.label(img.VipsImage, &tmp, cText, f, ff, C.double(color.R), C.double(color.G), C.double(color.B),
		C.int(x), C.int(y), C.int(width), C.int(height)); err != 0 {
		return handleImageError(tmp)
	}

	C.swap_and_clear(&img.VipsImage, tmp)

	return nil
}

func (img *Image) Linear(multiply float32, add float32) error {
	var out *C.VipsImage
	if err := C.linear(img.VipsImage, &out, C.double(multiply), C.double(add)); err != 0 {
		return handleImageError(out)
	}
	C.swap_and_clear(&img.VipsImage, out)
	return nil
}

func (img *Image) AutoRotate() error {
	var out *C.VipsImage

	err := C.autorot(img.VipsImage, &out)
	if err != 0 {
		return handleImageError(out)
	}
	C.swap_and_clear(&img.VipsImage, out)
	return nil
}

func Cleanup() {
	C.vips_cleanup()
}

func handleImageError(out *C.VipsImage) error {
	if out != nil {
		C.clear_image(&out)
	}

	return handleVipsError()
}

func handleVipsError() error {
	defer C.vips_error_clear()

	s := C.GoString(C.vips_error_buffer())
	return fmt.Errorf("%v \nStack:\n%s", s, dbg.Stack())
}

func ReadVipsMemStats(stats *MemoryStats) {
	stats.Mem = int64(C.vips_tracked_get_mem())
	stats.MemHigh = int64(C.vips_tracked_get_mem_highwater())
	stats.Allocs = int64(C.vips_tracked_get_allocs())
	stats.Files = int64(C.vips_tracked_get_files())
}
