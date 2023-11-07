package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

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

	q := r.URL.Query()
	key := q.Get("key")
	if key == "" {
		key, _ = genUuid()
	}

	if err := queueSem.Acquire(r.Context(), 1); err != nil {
		panic("maxSem")
	}
	defer queueSem.Release(1)

	upInfo, err := uploadPhoto(key, r.Body)
	if err != nil {
		return 500, err
	}

	js, _ := json.Marshal(upInfo)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(js)

	Free()

	return 200, nil
}

func UploadFileHandler(w http.ResponseWriter, r *http.Request) (int, error) {

	IncUploaderRequests()

	q := r.URL.Query()
	key := q.Get("key")
	if key == "" {
		key, _ = genUuid()
	}

	filename := q.Get("filename")

	file, err := os.Open(filename)
	defer file.Close()

	if err != nil {
		return 500, fmt.Errorf("failed to open file %v (%w)", filename, err)
	}

	if err := queueSem.Acquire(r.Context(), 1); err != nil {
		panic("maxSem")
	}
	defer queueSem.Release(1)

	upInfo, err := uploadPhoto(key, file)
	if err != nil {
		return 500, err
	}

	js, _ := json.Marshal(upInfo)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(js)

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

	if image.Width()*image.Height() > 16000*16000 {
		return nil, fmt.Errorf("input image is too big %vx%v", image.Width(), image.Height())
	}

	if err := image.Strip(); err != nil {
		return nil, err
	}

	if err := preprocess(&image); err != nil {
		return nil, err
	}

	imageBytes, _ := image.ExportJpeg(95)

	if err := imgStorage.Upload(name, imageBytes); err != nil {
		return nil, err
	}

	return &UploadedInfo{
		Name:   name,
		Width:  image.Width(),
		Height: image.Height(),
	}, nil
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
