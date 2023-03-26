package storage

import "C"
import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"io"
	"log"
)

var NotFoundError = errors.New("file not found")

type S3Storage struct {
	Bucket string
	client *s3.Client
}

func NewS3Storage(b string, credentials string) (S3Storage, error) {

	f := S3Storage{
		Bucket: b,
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
		[]string{credentials},
	), config.WithRegion("ru-central1"),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		log.Fatal(err)
		return f, err
	}

	f.client = s3.NewFromConfig(conf)

	return f, nil
}

func (f *S3Storage) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	r, err := f.client.GetObject(ctx, &s3.GetObjectInput{
		Key:    aws.String(path),
		Bucket: aws.String(f.Bucket),
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

func (f *S3Storage) Save(path string, file io.Reader) error {
	_, err := f.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(f.Bucket),
		Key:    aws.String(path),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("couldn't upload file %v to %v: %w", path, f.Bucket, err)
	}
	return nil
}

func (f *S3Storage) Delete(path string) error {
	_, err := f.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(f.Bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		var er *types.NoSuchKey
		if errors.As(err, &er) {
			return NotFoundError
		}
		return fmt.Errorf("couldn't delete file %s from %s: %w", path, f.Bucket, err)
	}
	return nil
}
