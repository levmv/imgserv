package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/storage"
	"github.com/levmv/imgserv/vips"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

var (
	logo   storage.SourceImage
	inited bool
)

func initSharer(ctx context.Context, conf *config.SharerConf) error {

	if conf.Font == "" && conf.Logo == "" {
		return nil
	}

	var err error
	conf.FontFile, err = filepath.Abs(conf.FontFile)
	if err != nil {
		return fmt.Errorf("incorrect font file path: %w", err)
	}

	f, err := os.Open(conf.FontFile)
	if err != nil {
		return fmt.Errorf("couldn't open fond file: %w", err)
	}
	f.Close()

	logo, err = imgStorage.LoadImage(ctx, conf.Logo)
	if err != nil {
		return fmt.Errorf("failed to preload logo: %w", err)
	}
	inited = true

	return nil
}

func serveShareImg(w http.ResponseWriter, r *http.Request) (int, error) {

	IncSharerRequests()

	if inited == false {
		return 501, errors.New("sharer not set")
	}

	IncRequestsInProgress()
	defer DecRequestsInProgress()

	q := r.URL.Query()
	path := q.Get("key")
	text := q.Get("text")
	width, _ := strconv.Atoi(q.Get("width"))
	height, _ := strconv.Atoi(q.Get("height"))

	if path == "" || text == "" {
		return 500, errors.New("empty path or text params")
	}

	const maxWidth = 1200
	const maxHeight = 630

	if width == 0 {
		width = maxWidth
	}

	if height == 0 {
		height = maxHeight
	}

	ctx := r.Context()

	if err := queueSem.Acquire(ctx, 1); err != nil {
		panic("maxSem")
	}
	defer queueSem.Release(1)

	sourceImg, err := imgStorage.LoadImage(ctx, path)
	defer sourceImg.Close()
	if err != nil {
		if errors.Is(err, storage.NotFoundError) {
			return 404, fmt.Errorf("%v %s", err, path)
		}
		return 500, err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	image := vips.Image{}
	defer image.Close()
	defer vips.Cleanup()

	if err = image.ThumbnailFromBuffer(sourceImg.Data, width, height, vips.InterestingAttention, vips.SizeBoth); err != nil {
		return 500, err
	}

	if err := image.Linear(0.6, 0); err != nil {
		return 500, err
	}

	if err := image.Label(text, cfg.Sharer.Font, cfg.Sharer.FontFile, vips.Color{255, 255, 255}, 20, 5); err != nil {
		return 500, err
	}

	logoImage := vips.Image{}
	defer logoImage.Close()

	if err = logoImage.LoadFromBuffer(logo.Data); err != nil {
		return 500, err
	}
	if image.Width() < maxWidth {
		logoImage.Resize(float64(image.Width()) / maxWidth)
	}

	pad := image.Width() / 100 * 5

	logoImage.Embed(pad, pad, image.Width(), image.Height())
	if err = image.Composite(&logoImage); err != nil {
		return 500, err
	}

	imageBytes, _ := image.ExportJpeg(90)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(imageBytes)))
	_, err = w.Write(imageBytes)

	return 200, nil
}
