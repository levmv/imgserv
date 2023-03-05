package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/levmv/go-resizer/vips"
	"golang.org/x/sync/semaphore"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const version = "0.0.5"

const help = `usage: go-resizer <action>
actions:
  server -config=<path to config.json>
  stat [-config=<path to config.json>]
  version`

var (
	queueSem *semaphore.Weighted
	sem      *semaphore.Weighted
	storage  Storage
	sign     UrlSignature
)

type appHandler func(http.ResponseWriter, *http.Request) (int, error)

func serveImg(w http.ResponseWriter, r *http.Request) (int, error) {

	IncTotalRequests()
	IncRequestsInProgress()
	defer DecRequestsInProgress()

	inputQuery := r.URL.String()

	ctx := context.TODO()

	if len(inputQuery) == 0 {
		return 400, errors.New("no input query")
	}

	inputQuery, err := sign.Verify(inputQuery)
	if err != nil {
		return 403, err
	}

	path, params, err := parseParams(inputQuery)
	if err != nil {
		return 500, err
	}

	if err := queueSem.Acquire(ctx, 1); err != nil {
		panic("sem")
	}
	defer queueSem.Release(1)

	sourceImg, err := LoadFromStorage(path)
	defer sourceImg.Close()
	if err != nil {
		if errors.Is(err, NotFoundError) {
			return 404, fmt.Errorf("%v %s", err, path)
		}
		return 500, err
	}
	defer runtime.KeepAlive(sourceImg)

	if err != nil {
		panic(err)
	}

	// Image process
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := sem.Acquire(ctx, 1); err != nil {
		panic("sem2")
	}
	defer sem.Release(1)
	defer vips.Cleanup()

	image := vips.Image{}
	defer image.Close()

	if err = image.LoadFromBuffer(sourceImg.Data); err != nil {
		return 500, err
	}

	size := vips.SizeDown

	if params.Crop.Width != 0 {

		if params.Crop.X+params.Crop.Width > image.Width() {
			params.Crop.Width = image.Width() - params.Crop.X
		}

		if params.Crop.Y+params.Crop.Height > image.Height() {
			params.Crop.Height = image.Height() - params.Crop.Y
		}

		if err = image.Crop(params.Crop.X, params.Crop.Y, params.Crop.Width, params.Crop.Height); err != nil {
			return 500, err
		}
	}

	if params.PixelRatio != 0 {
		params.Width = int(float64(params.Width) * params.PixelRatio)
		params.Height = int(float64(params.Height) * params.PixelRatio)
	}

	height := params.Height

	if params.Resize == true {

		finalWidth := params.Width
		finalHeight := params.Height
		size = vips.SizeDown

		if params.Mode == ModeCrop {

			if params.Gravity == GravityNone {
				params.Gravity = GravityCenter
			}

			if image.Width() < params.Width || image.Height() < params.Height {
				if params.PixelRatio > 0 || params.Upscale {
					size = vips.SizeBoth
				} else {
					params.Mode = ModeFill
					if image.Height() < params.Height {
						height = image.Height()
					}
					if image.Width() < params.Width {
						params.Width = image.Width()
					}
				}
			}
		}

		if params.Crop.Width > 0 {
			err = image.Thumbnail(params.Width, params.Height, Gravity2Vips(params.Gravity), size)
			if err != nil {
				return 500, err
			}
		} else {
			err = image.ThumbnailFromBuffer(sourceImg.Data, params.Width, height, Gravity2Vips(params.Gravity), size)
			if err != nil {
				return 500, err
			}
		}

		if params.Mode == ModeFill {
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

		if len(params.Watermarks) > 0 {
			for _, wm := range params.Watermarks {

				wmImg, err := LoadFromStorage(wm.Path)
				defer wmImg.Close()

				if err != nil {
					return 500, fmt.Errorf("watermark %v %s", err, path)
				}
				defer runtime.KeepAlive(wmImg)

				wmImage := vips.Image{}
				defer wmImage.Close()

				if err = wmImage.LoadFromBuffer(wmImg.Data); err != nil {
					return 500, err
				}
				if wm.Size < 100 {
					wmImage.Resize(float64(wm.Size) / 100)
				}

				// TODO wm.Position with coords
				switch wm.Position {
				case PositionNorth:
					wmImage.Embed(image.Width()/2-wmImage.Width()/2, 0, image.Width(), image.Height())
				case PositionSouthEast:
					wmImage.Embed(image.Width()-wmImage.Width(), image.Height()-wmImage.Height(), image.Width(), image.Height())
				case PositionSouthWest:
					wmImage.Embed(0, image.Height()-wmImage.Height(), image.Width(), image.Height())
				case PositionNorthWest:
					wmImage.Embed(0, 0, image.Width(), image.Height())
				case PositionCenter:
					wmImage.Embed(image.Width()/2-wmImage.Width()/2, image.Height()/2-wmImage.Height()/2, image.Width(), image.Height())
				case PositionCoords:
					x := image.Width() - wmImage.Width() - wm.PositionX
					y := image.Height() - wmImage.Height() - wm.PositionY

					wmImage.Embed(x, y, image.Width(), image.Height())
				}
				if err = image.Composite(&wmImage); err != nil {
					return 0, err
				}
			}
		}
	}

	var imageBytes []byte

	if strings.Contains(r.Header.Get("Accept"), "webp") {
		imageBytes, _ = image.ExportWebp(params.Quality + cfg.WebpQCorrection)
		w.Header().Set("Content-Type", "image/webp")
	} else {
		imageBytes, _ = image.ExportJpeg(params.Quality)
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("Vary", "Accept")

	w.Header().Set("Content-Length", strconv.Itoa(len(imageBytes)))
	_, err = w.Write(imageBytes)

	return 200, nil
}

func serveStat(w http.ResponseWriter, r *http.Request) (int, error) {
	var curStats = newStats()

	out, err := json.MarshalIndent(curStats, "", "    ")
	if err != nil {
		return 500, err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)

	return 200, nil
}

func startServer(bindAddr string) {
	queueSem = semaphore.NewWeighted(int64(cfg.WorkingQueueSize))
	sem = semaphore.NewWeighted(int64(cfg.Concurrency))

	go func() {
		for range time.Tick(time.Duration(cfg.FreeMemoryInterval) * time.Second) {
			Free()
		}
	}()

	http.Handle("/", appHandler(serveImg))
	http.Handle("/stat", appHandler(serveStat))
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(404)
	})

	log.Printf("Starting server on %s", bindAddr)

	go func() {
		err := http.ListenAndServe(bindAddr, nil)
		if err != nil {
			log.Fatal(err)
		}
	}()
}

func run(config string) error {

	cfg, err := ParseConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Presets != nil {
		if err = parsePresets(string(cfg.Presets)); err != nil {
			log.Fatal(err)
		}
	}
	if cfg.MemoryLimit > 0 {
		debug.SetMemoryLimit(cfg.MemoryLimit)
	}

	if cfg.LogFile != "" {
		file, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(file)
	}

	sign = NewUrlSignature(cfg.SignatureMethod, cfg.SignatureSecret)

	if err = vips.Init(nil); err != nil {
		log.Fatal(err)
	}
	defer vips.Shutdown()

	initSourcePool()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	if cfg.RemoteStorage {
		storage, err = NewS3Storage(cfg.S3.Bucket, cfg.CachePath)
	} else {
		storage, err = NewFileStorage(cfg.BasePath)
	}
	if err != nil {
		log.Fatal(err)
	}

	startServer(cfg.BindTo)

	<-stop
	return nil
}

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, err := fn(w, r); err != nil {
		log.Printf("Error %d %v", status, err)
		switch status {
		case http.StatusNotFound:
			http.Error(w, http.StatusText(http.StatusNotFound), status)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(help)
		os.Exit(0)
	}

	var configArg string
	serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
	serverCmd.StringVar(&configArg, "config", "./config.json", "path to config json file")
	serverCmd.StringVar(&configArg, "c", "./config.json", "path to config json file (shorthand)")

	action := os.Args[1]

	switch action {
	case "version":
		fmt.Println(version)
	case "server":
		serverCmd.Parse(os.Args[2:])
		if err := run(configArg); err != nil {
			log.Fatal(err)
		}
	case "stat":
		serverCmd.Parse(os.Args[2:])
		if err := showStats(configArg); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("expected 'server', `stat` or 'version'")
		os.Exit(1)
	}
}
