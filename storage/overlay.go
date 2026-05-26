package storage

import (
	"context"
	"errors"
	"io"

	"github.com/levmv/imgserv/config"
)

type Overlay struct {
	local  ImageStorage
	remote ImageStorage
}

func NewOverlay(local ImageStorage, remote ImageStorage) *Overlay {
	return &Overlay{
		local:  local,
		remote: remote,
	}
}

func NewLocalOverlay(conf config.StorageConf) (*Overlay, error) {
	local, err := NewLocal(conf.LocalPath)
	if err != nil {
		return nil, err
	}

	remote, err := NewCached(conf)
	if err != nil {
		return nil, err
	}

	return NewOverlay(local, remote), nil
}

func (st *Overlay) NewImage() SourceImage {
	return st.local.NewImage()
}

func (st *Overlay) LoadImage(ctx context.Context, key string) (SourceImage, error) {
	localImg, err := st.local.LoadImage(ctx, key)
	if err == nil {
		return localImg, nil
	}
	if !errors.Is(err, NotFoundError) {
		return localImg, err
	}
	if localImg.pool != nil {
		localImg.Close()
	}

	return st.remote.LoadImage(ctx, key)
}

func (st *Overlay) Upload(key string, contents []byte) error {
	return st.local.Upload(key, contents)
}

func (st *Overlay) UploadFile(key string, r io.Reader) error {
	return st.local.UploadFile(key, r)
}

func (st *Overlay) Delete(key string) error {
	err := st.local.Delete(key)
	if errors.Is(err, NotFoundError) {
		return nil
	}
	return err
}
