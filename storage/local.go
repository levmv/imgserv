package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

type Local struct {
	basePath string
	pool     *sync.Pool
}

func NewLocal(basePath string) (*Local, error) {
	basePath, err := initCachePath(basePath)
	if err != nil {
		return nil, err
	}

	return &Local{
		basePath: basePath,
		pool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 1024)
			},
		},
	}, nil
}

func (ls *Local) NewImage() SourceImage {
	return SourceImage{
		Data: ls.pool.Get().([]byte),
		pool: ls.pool,
	}
}

func (ls *Local) LoadImage(ctx context.Context, key string) (SourceImage, error) {
	si := ls.NewImage()

	select {
	case <-ctx.Done():
		return si, ctx.Err()
	default:
	}

	filePath, err := ls.filePath(key)
	if err != nil {
		return si, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return si, NotFoundError
		}
		return si, err
	}
	defer f.Close()

	if _, err := si.ReadFrom(f); err != nil {
		return si, err
	}
	if len(si.Data) == 0 {
		return si, errors.New("empty input file")
	}

	return si, nil
}

func (ls *Local) Upload(key string, contents []byte) error {
	return ls.writeReader(key, bytes.NewReader(contents))
}

func (ls *Local) UploadFile(key string, r io.Reader) error {
	return ls.writeReader(key, r)
}

func (ls *Local) Delete(key string) error {
	filePath, err := ls.filePath(key)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NotFoundError
		}
		return err
	}
	return nil
}

func (ls *Local) writeReader(key string, r io.Reader) error {
	filePath, err := ls.filePath(key)
	if err != nil {
		return err
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("can't create local storage directory: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, ".imgserv-*")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempName)
	}()

	if _, err := io.Copy(tempFile, r); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	closed = true

	if err := os.Rename(tempName, filePath); err != nil {
		return err
	}
	return nil
}

func (ls *Local) filePath(key string) (string, error) {
	cleanKey, err := cleanKey(key)
	if err != nil {
		return "", err
	}

	return filepath.Join(ls.basePath, filepath.FromSlash(cleanKey)), nil
}

func cleanKey(key string) (string, error) {
	if key == "" {
		return "", errors.New("empty storage key")
	}
	if filepath.IsAbs(key) || strings.Contains(key, "\x00") || strings.Contains(key, "\\") {
		return "", fmt.Errorf("invalid storage key %q", key)
	}

	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid storage key %q", key)
		}
	}

	cleaned := path.Clean(key)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("invalid storage key %q", key)
	}

	return cleaned, nil
}
