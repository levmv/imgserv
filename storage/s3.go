package storage

import "C"
import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/levmv/imgserv/config"
)

var NotFoundError = errors.New("file not found")

type S3Storage struct {
	Bucket   string
	Endpoint string
	client   *s3.Client
}

func NewS3Storage(storageConf config.StorageConf) (S3Storage, error) {

	f := S3Storage{
		Bucket:   storageConf.Bucket,
		Endpoint: storageConf.Endpoint,
	}
	ctx := context.TODO()

	endpoint := storageConf.Endpoint
	if endpoint == "" && storageConf.Region == "ru-central1" {
		// Existing configurations select Yandex by region alone.
		endpoint = "https://storage.yandexcloud.net"
	}

	sharedConfig, err := awsconfig.LoadSharedConfigProfile(ctx, "default", func(o *awsconfig.LoadSharedConfigOptions) {
		o.ConfigFiles = []string{}
		o.CredentialsFiles = []string{storageConf.Credentials}
	})
	if err != nil {
		return f, fmt.Errorf("failed to load S3 credentials from %s: %w", storageConf.Credentials, err)
	}
	if !sharedConfig.Credentials.HasKeys() {
		return f, fmt.Errorf("failed to load S3 credentials from %s: default profile must contain aws_access_key_id and aws_secret_access_key", storageConf.Credentials)
	}

	conf, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: sharedConfig.Credentials,
		}),
		awsconfig.WithRegion(storageConf.Region),
	)
	if err != nil {
		return f, err
	}

	f.client = s3.NewFromConfig(conf, func(options *s3.Options) {
		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
		// Dotted bucket names cannot use the storage wildcard certificate.
		options.UsePathStyle = strings.Contains(storageConf.Bucket, ".")
	})
	log.Printf("Configured S3 storage bucket=%q region=%q endpoint=%q credentials=%q credentials_source=%q", storageConf.Bucket, storageConf.Region, endpoint, storageConf.Credentials, sharedConfig.Credentials.Source)

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
