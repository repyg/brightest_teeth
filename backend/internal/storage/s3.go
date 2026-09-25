package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/repyg/brightest_teeth/backend/internal/service"
)

type S3 struct {
	Client *minio.Client
	Bucket string
}

func NewS3(endpoint, region, bucket, accessKey, secretKey string) (*S3, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid S3_ENDPOINT")
	}
	client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: u.Scheme == "https", Region: region, BucketLookup: minio.BucketLookupPath})
	if err != nil {
		return nil, err
	}
	return &S3{client, bucket}, nil
}

func (s *S3) Ready(ctx context.Context) error {
	exists, err := s.Client.BucketExists(ctx, s.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("S3 bucket does not exist")
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key, kind string, data []byte) error {
	_, err := s.Client.PutObject(ctx, s.Bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: kind})
	return err
}

func (s *S3) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	object, err := s.Client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, service.MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > service.MaxImageBytes {
		return nil, fmt.Errorf("stored image exceeds limit")
	}
	return data, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.Client.RemoveObject(ctx, s.Bucket, key, minio.RemoveObjectOptions{})
}
