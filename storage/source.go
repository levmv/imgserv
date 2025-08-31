package storage

import (
	"io"
	"sync"
)

type SourceImage struct {
	Data []byte
	pool *sync.Pool
	io.Closer
}

func (si *SourceImage) Close() {
	//si.Reset()
	if cap(si.Data) < 3*1024*1024 {
		si.pool.Put(si.Data)
	}
}

//func (si *SourceImage) Read(b []byte) (n int, err error) {
//	b = si.Data[:len(b)]
//	return len(si.Data), nil
//}

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
