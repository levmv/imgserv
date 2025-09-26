package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/params"
	"github.com/levmv/imgserv/storage"
	"github.com/levmv/imgserv/vips"
)

func serveImg(w http.ResponseWriter, r *http.Request) (int, error) {

	if !maxSem.TryAcquire(1) {
		IncRejectedRequests()
		return 429, errors.New("too many requests")
	}
	defer maxSem.Release(1)

	IncResizerRequests()
	IncRequestsInProgress()
	defer DecRequestsInProgress()

	inputQuery := r.URL.String()
	if len(inputQuery) == 0 {
		return 400, errors.New("no input query")
	}

	verifiedQuery, err := sign.Verify(inputQuery)
	if err != nil {
		return 403, err
	}

	ctx := r.Context()

	path, pms, err := params.Parse(verifiedQuery)
	if err != nil {
		return 500, err
	}

	// We're limiting concurrency both for loading file and processing image. Even though it seems logical to separate
	// io/cpu parts (and it was in first ver), it's more memory efficient that way and have no real performance impact
	// in real (ours) production conditions
	if err := queueSem.Acquire(ctx, 1); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 499, errors.New("request cancelled")
		}
		panic("queueSem")
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
	defer vips.Cleanup()

	image := vips.Image{}
	defer image.Close()

	if err = image.LoadFromBuffer(sourceImg.Data); err != nil {
		return 500, fmt.Errorf("failed to load %s: %v", verifiedQuery, err)
	}

	size := vips.SizeDown

	if pms.Crop.Width != 0 {
		if pms.Crop.X+pms.Crop.Width > image.Width() {
			pms.Crop.Width = image.Width() - pms.Crop.X
		}
		if pms.Crop.Y+pms.Crop.Height > image.Height() {
			pms.Crop.Height = image.Height() - pms.Crop.Y
		}
		if err = image.Crop(pms.Crop.X, pms.Crop.Y, pms.Crop.Width, pms.Crop.Height); err != nil {
			return 500, err
		}
	}

	if pms.PixelRatio > 1 {
		pms.Width = int(float64(pms.Width) * pms.PixelRatio)
		pms.Height = int(float64(pms.Height) * pms.PixelRatio)
	}

	height := pms.Height

	if pms.Resize {
		finalWidth := pms.Width
		finalHeight := pms.Height
		size = vips.SizeDown
		if pms.Mode == params.ModeCrop {
			if pms.Gravity == params.GravityNone {
				pms.Gravity = params.GravityCenter
			}

			if image.Width() < pms.Width || image.Height() < pms.Height {
				if pms.PixelRatio > 1 || pms.Upscale {
					size = vips.SizeBoth
				} else {
					pms.Mode = params.ModeFill
					if image.Height() < pms.Height {
						height = image.Height()
					}
					if image.Width() < pms.Width {
						pms.Width = image.Width()
					}
				}
			}
		}

		if pms.Crop.Width > 0 {
			err = image.Thumbnail(pms.Width, pms.Height, params.Gravity2Vips(pms.Gravity), size)
			if err != nil {
				return 500, err
			}
		} else {
			err = image.ThumbnailFromBuffer(sourceImg.Data, pms.Width, height, params.Gravity2Vips(pms.Gravity), size)
			if err != nil {
				return 500, err
			}
		}

		if pms.Mode == params.ModeFill {
			image.Flatten(vips.Color{R: 255, G: 255, B: 255}) // fixme
			if err := image.EmbedBackground(
				(finalWidth-image.Width())/2,
				(finalHeight-image.Height())/2,
				finalWidth,
				finalHeight,
				vips.Color{R: 255, G: 255, B: 255},
			); err != nil {
				return 500, err
			}
		}

		if len(pms.Watermarks) > 0 {
			var wms []*storage.SourceImage
			for i, wm := range pms.Watermarks {
				wmImg, err := imgStorage.LoadImage(ctx, wm.Path)
				if err != nil {
					return 500, fmt.Errorf("loading watermark %v", err)
				}
				wms = append(wms, &wmImg)
				defer wms[i].Close()

				if err := addWatermark(&image, wms[i], wm, pms.PixelRatio); err != nil {
					return 500, fmt.Errorf("error during watermark %v", wm)
				}
			}
		}
	}

	var imageBytes []byte

	exportWebp := func() ([]byte, error) {
		w.Header().Set("Content-Type", "image/webp")
		return image.ExportWebp(pms.Quality + cfg.Resizer.WebpQCorrection)
	}
	exportJpeg := func() ([]byte, error) {
		w.Header().Set("Content-Type", "image/jpeg")
		return image.ExportJpeg(pms.Quality + cfg.Resizer.JpegQCorrection)
	}

	if cfg.Resizer.OutputType == config.OutputTypeVary {
		w.Header().Set("Vary", "Accept")
		if strings.Contains(r.Header.Get("Accept"), "webp") {
			imageBytes, err = exportWebp()
		} else {
			imageBytes, err = exportJpeg()
		}
	} else if cfg.Resizer.OutputType == config.OutputTypeWebp {
		imageBytes, err = exportWebp()
	} else {
		imageBytes, err = exportJpeg()
	}

	if err != nil {
		return 500, err
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(imageBytes)))
	_, err = w.Write(imageBytes)

	return 200, err
}

func addWatermark(image *vips.Image, wmImg *storage.SourceImage, wm params.Watermark, pixelRatio float64) error {
	wmImage := vips.Image{}
	defer wmImage.Close()

	if err := wmImage.LoadFromBuffer(wmImg.Data); err != nil {
		return err
	}

	if pixelRatio > 1 {
		wmImage.Thumbnail(int(float64(wmImage.Width())*pixelRatio), int(float64(wmImage.Height())*pixelRatio), 0, vips.SizeBoth)
	}

	if wm.Size < 100 {
		wmImage.Resize(float64(wm.Size) / 100)
	}

	switch wm.Position { // TODO: rewrite to one embed?
	case params.PositionNorth:
		wmImage.Embed(image.Width()/2-wmImage.Width()/2, 0, image.Width(), image.Height())
	case params.PositionSouthEast:
		wmImage.Embed(image.Width()-wmImage.Width(), image.Height()-wmImage.Height(), image.Width(), image.Height())
	case params.PositionSouthWest:
		wmImage.Embed(0, image.Height()-wmImage.Height(), image.Width(), image.Height())
	case params.PositionNorthWest:
		wmImage.Embed(0, 0, image.Width(), image.Height())
	case params.PositionCenter:
		wmImage.Embed(image.Width()/2-wmImage.Width()/2, image.Height()/2-wmImage.Height()/2, image.Width(), image.Height())
	case params.PositionCoords:
		x := image.Width() - wmImage.Width() - int(float64(wm.PositionX)*pixelRatio)
		y := image.Height() - wmImage.Height() - int(float64(wm.PositionY)*pixelRatio)
		wmImage.Embed(x, y, image.Width(), image.Height())
	}
	if err := image.Composite(&wmImage); err != nil {
		return err
	}
	return nil
}
