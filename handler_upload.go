package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/vips"
)

type UploadedInfo struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type preprocessor func(image *vips.Image) error

func nope(image *vips.Image) error {
	return nil
}

var (
	preprocess preprocessor
	maxWidth   int
	maxHeight  int
)

const maxSourcePixels = 16000 * 16000

func resize(image *vips.Image) error {
	if image.Width() > maxWidth || image.Height() > maxHeight {
		if err := image.Thumbnail(maxWidth, maxHeight, 0, vips.SizeDown); err != nil {
			return err
		}
	}
	return nil
}

func initUpload(conf config.StorageConf) {
	if conf.MaxWidth > 0 && conf.MaxHeight > 0 {
		maxWidth = conf.MaxWidth
		maxHeight = conf.MaxHeight
		preprocess = resize
	} else {
		preprocess = nope
	}
}

func UploadHandler(w http.ResponseWriter, r *http.Request) (int, error) {

	IncUploaderRequests()
	if cfg.Server.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, cfg.Server.MaxUploadSize)
	}

	q := r.URL.Query()
	key := q.Get("key")
	if key == "" {
		var err error
		key, err = genUuid()
		if err != nil {
			return 500, err
		}
	}

	if err := queueSem.Acquire(r.Context(), 1); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 499, errors.New("request cancelled")
		}
		panic("queueSem")
	}
	defer queueSem.Release(1)

	upInfo, err := uploadPhoto(key, r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return http.StatusRequestEntityTooLarge, err
		}
		return 500, err
	}

	js, _ := json.Marshal(upInfo)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(js); err != nil {
		return 200, err
	}

	Free()

	return 200, nil
}

func UploadFileHandler(w http.ResponseWriter, r *http.Request) (int, error) {

	IncUploaderRequests()

	q := r.URL.Query()
	key := q.Get("key")
	if key == "" {
		var err error
		key, err = genUuid()
		if err != nil {
			return 500, err
		}
	}

	filename := q.Get("filename")
	if filename == "" {
		return 400, errors.New("empty filename arg")
	}
	filename, err := uploadFilePath(filename)
	if err != nil {
		return 400, err
	}

	file, err := os.Open(filename)
	if err != nil {
		return 500, fmt.Errorf("failed to open file %v (%w)", filename, err)
	}
	defer file.Close()

	if err := queueSem.Acquire(r.Context(), 1); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 499, errors.New("request cancelled")
		}
		panic("queueSem")
	}
	defer queueSem.Release(1)

	upInfo, err := uploadPhoto(key, file)
	if err != nil {
		return 500, err
	}

	js, _ := json.Marshal(upInfo)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(js); err != nil {
		return 200, err
	}

	Free()

	return 200, nil
}

func uploadPhoto(name string, r io.Reader) (*UploadedInfo, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	newImg := imgStorage.NewImage()
	defer newImg.Close()

	if _, err := newImg.ReadFrom(r); err != nil {
		return nil, err
	}

	image := vips.Image{}
	defer image.Close()
	defer vips.Cleanup()

	if err := image.LoadFromBuffer(newImg.Data); err != nil {
		return nil, err
	}

	if err := image.AutoRotate(); err != nil {
		return nil, err
	}

	if image.Width()*image.Height() > maxSourcePixels {
		return nil, fmt.Errorf("input image is too big %vx%v", image.Width(), image.Height())
	}

	if err := preprocess(&image); err != nil {
		return nil, err
	}

	if err := image.Strip(); err != nil {
		return nil, err
	}

	imageBytes, err := image.ExportJpeg(95)
	if err != nil {
		return nil, err
	}

	if err := imgStorage.Upload(name, imageBytes); err != nil {
		return nil, err
	}

	return &UploadedInfo{
		Name:   name,
		Width:  image.Width(),
		Height: image.Height(),
	}, nil
}

func uploadFilePath(filename string) (string, error) {
	basePath := cfg.Server.UploadFileBasePath
	if basePath == "" {
		return filename, nil
	}

	path := filename
	if !filepath.IsAbs(path) {
		path = filepath.Join(basePath, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid filename %q: %w", filename, err)
	}

	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return "", fmt.Errorf("invalid filename %q: %w", filename, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("filename %q is outside upload_file_base_path", filename)
	}

	return path, nil
}

func genUuid() (string, error) {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", err
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10

	return base64.RawURLEncoding.EncodeToString(uuid), nil
}
