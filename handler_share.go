package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/storage"
	"github.com/levmv/imgserv/vips"
)

var (
	logo   storage.SourceImage
	inited bool
)

func initSharer(ctx context.Context, conf *config.SharerConf) error {

	//if conf.Font == "" && conf.Logo == "" {
	//	return nil
	//}

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
	preview := q.Has("preview") && q.Get("preview") == "1"

	fmt.Println(maxWidth, maxHeight)

	if path == "" || text == "" {
		return 500, errors.New("empty path or text params")
	}

	maxWidth := 1200.0
	maxHeight := 630.0

	if preview {
		maxWidth = maxWidth / 2
		maxHeight = maxHeight / 2
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

	if err = image.LoadFromBuffer(sourceImg.Data); err != nil {
		return 500, fmt.Errorf("failed to load. %v", err)
	}

	ratio := maxWidth / maxHeight
	width := int(maxWidth)
	height := int(maxHeight)
	if float64(image.Width()/image.Height()) > ratio {
		if image.Height() < int(maxHeight) {
			height = image.Height()
			width = int(float64(image.Height()) * ratio)
		}
	} else {
		if image.Width() < int(maxWidth) {
			width = image.Width()
			height = int(float64(image.Width()) / ratio)
		}
	}

	scale := float64(image.Width()) / maxWidth

	fmt.Println(image.Width(), 1200, float64(image.Width())/maxWidth)

	if err = image.Thumbnail(width, height, vips.InterestingAttention, vips.SizeDown); err != nil {
		return 500, err
	}

	divHor := 14.0
	divVer := 8.0

	lineH := int(float64(image.Width()) / divHor)
	lineV := int(float64(image.Height()) / divVer)

	if err := image.Linear(0.6, 0); err != nil {
		return 500, err
	}

	if err := image.Label(text, cfg.Sharer.Font, cfg.Sharer.FontFile, vips.Color{255, 255, 255},
		lineH, lineV*3, image.Width()-lineH*2, lineV*3); err != nil {
		return 500, err
	}

	logoImage := vips.Image{}
	defer logoImage.Close()

	if err = logoImage.LoadFromBuffer(logo.Data); err != nil {
		return 500, err
	}
	if scale != 1 {
		logoImage.Resize(scale)
	}

	logoImage.Embed(lineH, lineV, image.Width(), image.Height())
	if err = image.Composite(&logoImage); err != nil {
		return 500, err
	}

	quality := 90
	if preview {
		quality = 60
	}
	imageBytes, _ := image.ExportJpeg(quality)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(imageBytes)))
	_, err = w.Write(imageBytes)

	return 200, nil
}
