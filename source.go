package main

import (
	"errors"
	"io"
	"sync"
)

type SourceImage struct {
	Data []byte
	io.Closer
}

var sourcePool sync.Pool

func initSourcePool() {
	sourcePool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 1024)
		},
	}
}

func NewSourceImage() SourceImage {
	return SourceImage{
		Data: sourcePool.Get().([]byte),
	}
}

func (si *SourceImage) Close() {
	//si.Reset()
	if cap(si.Data) < 3*1024*1024 {
		sourcePool.Put(si.Data)
	}
}

func LoadFromStorage(path string) (SourceImage, error) {
	si := SourceImage{
		Data: sourcePool.Get().([]byte),
	}

	err := storage.GetFile(path, &si)
	if err != nil {
		return si, err
	}

	if len(si.Data) == 0 {
		return si, errors.New("empty input file")
	}

	return si, nil
}

func (si *SourceImage) ReadFrom(r io.Reader) (n int64, err error) {
	si.Reset()
	for {
		if len(si.Data) == cap(si.Data) {
			// Add more capacity (let append pick how much).
			si.Data = append(si.Data, 0)[:len(si.Data)]
		}
		readSize, err := r.Read(si.Data[len(si.Data):cap(si.Data)])
		n += int64(readSize)

		si.Data = si.Data[:len(si.Data)+readSize]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return n, err
		}
	}
}

func (si *SourceImage) Reset() {
	si.Data = si.Data[:0]
}
