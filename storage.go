package main

import "C"
import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	time2 "time"
)

var NotFoundError = errors.New("file not found")

type Storage interface {
	GetFile(string, *SourceImage) error
}

type FileStorage struct {
	basePath string
}

func NewFileStorage(basePath string) (Storage, error) {
	return FileStorage{
		basePath: basePath,
	}, nil
}

func (f FileStorage) GetFile(filename string, s *SourceImage) error {
	r, err := os.Open(filename)

	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NotFoundError
		}
		return err
	}

	s.ReadFrom(r)

	return nil
}

type S3Storage struct {
	client *s3.Client
	bucket string
}

func NewS3Storage(bucket string, cachePath string) (Storage, error) {

	f := S3Storage{
		bucket: bucket,
	}

	if err := initCache(cachePath); err != nil {
		return nil, err
	}

	// Creating a custom endpoint resolver for returning correct URL for S3 storage in the ru-central1 region
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID && region == "ru-central1" {
			return aws.Endpoint{
				PartitionID:   "yc",
				URL:           "https://storage.yandexcloud.net",
				SigningRegion: "ru-central1",
			}, nil
		}
		return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
	})

	conf, err := config.LoadDefaultConfig(context.TODO(), config.WithSharedCredentialsFiles(
		[]string{cfg.S3.Credentials},
	), config.WithRegion(cfg.S3.Region),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	f.client = s3.NewFromConfig(conf)

	return f, nil
}

func (f S3Storage) GetFile(filename string, s *SourceImage) error {

	r, err := getCached(filename)
	if err == nil {
		_, err = s.ReadFrom(r)
		return err
	}
	if !errors.Is(err, NotCached) {
		return err
	}

	r, err = f.realGetFile(filename)
	if err != nil {
		if errors.Is(err, NotFoundError) {
			cacheFile(filename, &SourceImage{Data: []byte("404")})
		}
		return err
	}

	_, err = s.ReadFrom(r)
	if err != nil {
		return err
	}

	err = cacheFile(filename, s)

	return err
}

func (f S3Storage) realGetFile(filename string) (io.Reader, error) {
	r, err := f.client.GetObject(context.TODO(), &s3.GetObjectInput{
		Key:    aws.String(filename),
		Bucket: aws.String(f.bucket),
	})
	if err != nil {
		var er *types.NoSuchKey
		if errors.As(err, &er) {
			return nil, NotFoundError
		}
		return nil, err
	}

	return r.Body, err
}

var basePath string

var NotCached = errors.New("not cached")

func initCache(base string) error {
	var err error
	basePath, err = filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("incorrect cachePath %s (%w)", base, err)
	}
	_, err = os.Stat(basePath)

	if err != nil {
		return fmt.Errorf("can't access cache directory: %s (%w)", base, err)
	}

	return nil
}

func getCached(path string) (io.Reader, error) {
	r, err := os.Open(hashName(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, NotCached
		}
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

func cacheFile(path string, s *SourceImage) error {
	path = hashName(path)
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
	_, err = tempFile.Write(s.Data)
	if err != nil {
		return err
	}

	err = tempFile.Sync()
	if err != nil {
		return err
	}

	if err = os.Rename(tempFile.Name(), path); err != nil {
		return err
	}

	return nil
}

func hashName(path string) string {
	hash := md5.Sum([]byte(path))
	hashed := hex.EncodeToString(hash[:])
	prefix := hashed[:2]

	return basePath + "/" + prefix + "/" + hashed
}
