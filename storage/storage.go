package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	time2 "time"

	"github.com/levmv/imgserv/config"
)

var NotCached = errors.New("not cached")

type Cached struct {
	s3       S3Storage
	pool     *sync.Pool
	basePath string
}

func NewCached(conf config.StorageConf) (*Cached, error) {
	cachePath, err := initCachePath(conf.CachePath)
	if err != nil {
		return nil, err
	}

	st, err := NewS3Storage(conf.Bucket, conf.Credentials)
	if err != nil {
		return nil, err
	}

	cs := Cached{
		s3:       st,
		basePath: cachePath,
		pool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 1024)
			},
		},
	}

	return &cs, nil
}

func initCachePath(base string) (string, error) {
	var err error

	if base == "" {
		return "", errors.New("empty cache path")
	}

	base, err = filepath.Abs(base)
	if err != nil {
		return base, fmt.Errorf("incorrect cachePath %s (%w)", base, err)
	}
	_, err = os.Stat(base)

	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return base, fmt.Errorf("can't access cache directory: %s (%w)", base, err)
		}

		if err := os.MkdirAll(base, os.ModePerm); err != nil {
			return base, fmt.Errorf("failed to create cache directory: %s (%w)", base, err)
		}
	}

	return base, nil
}

func (cs *Cached) NewImage() SourceImage {
	return SourceImage{
		Data: cs.pool.Get().([]byte),
		pool: cs.pool, // save link to pool to have ability to close
	}
}

func (cs *Cached) LoadImage(ctx context.Context, path string) (SourceImage, error) {
	si := cs.NewImage()

	if err := cs.readImage(ctx, path, &si); err != nil {
		return si, err
	}

	if len(si.Data) == 0 {
		return si, errors.New("empty input file")
	}

	return si, nil
}

func (cs *Cached) Upload(path string, contents []byte) error {
	if err := cs.s3.Save(path, bytes.NewReader(contents)); err != nil {
		return err
	}
	if err := cs.cacheFile(path, contents); err != nil {
		// TODO: try to delete uploaded?
		return err
	}
	return nil
}

func (cs *Cached) UploadFile(path string, r io.Reader) error {
	return cs.s3.Save(path, r)
}

func (cs *Cached) Delete(path string) error {
	if err := cs.s3.Delete(path); err != nil {
		return err
	}

	err := os.Remove(cs.hashName(path))
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return err
}

func (cs *Cached) readImage(ctx context.Context, path string, si *SourceImage) error {

	r, err := cs.getCached(path)
	if err == nil {
		_, err = si.ReadFrom(r)
		return err
	}
	if !errors.Is(err, NotCached) {
		return err
	}

	r, err = cs.s3.Open(ctx, path)
	if err != nil {
		if errors.Is(err, NotFoundError) {
			cs.cacheFile(path, []byte("404"))
		}
		return err
	}
	defer r.Close()

	_, err = si.ReadFrom(r)
	if err != nil {
		return err
	}

	err = cs.cacheFile(path, si.Data)

	return err
}

func (cs *Cached) getCached(path string) (io.ReadCloser, error) {
	r, err := os.Open(cs.hashName(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, NotCached
		}
		return nil, err
	}
	curTime := time2.Now().Local()
	_ = os.Chtimes(path, curTime, curTime)

	if info, err := r.Stat(); err == nil {
		if info.Size() == 3 {
			var c = make([]byte, 3)
			if _, err := r.Read(c); err != nil {
				return r, err
			}
			if string(c) == "404" {
				return r, NotFoundError
			}
		}
	}

	return r, err
}

func (cs *Cached) cacheFile(path string, data []byte) error {
	path = cs.hashName(path)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("can't create cache directory: %w", err)
	}

	// We save temp in the same folder to avoid "invalid cross-device link"
	tempFile, err := os.CreateTemp(dir, "goresizer")
	defer tempFile.Close()
	if err != nil {
		return err
	}

	if _, err = tempFile.Write(data); err != nil {
		return err
	}

	if err = tempFile.Sync(); err != nil {
		return err
	}

	if err = os.Rename(tempFile.Name(), path); err != nil {
		return err
	}
	return nil
}

func (cs *Cached) hashName(path string) string {
	hash := md5.Sum([]byte(cs.s3.Bucket + path))
	hashed := hex.EncodeToString(hash[:])
	prefix := hashed[:2]

	return cs.basePath + "/" + prefix + "/" + hashed
}
