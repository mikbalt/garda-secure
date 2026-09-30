package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps minio.Client with convenience methods for SecurePatrol.
type Client struct {
	mc *minio.Client
}

// New creates a connected MinIO client.
func New(endpoint, accessKey, secretKey string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}
	return &Client{mc: mc}, nil
}

// EnsureBuckets creates each bucket if it does not already exist.
func (c *Client) EnsureBuckets(ctx context.Context, buckets []string) error {
	for _, bucket := range buckets {
		exists, err := c.mc.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("BucketExists(%s): %w", bucket, err)
		}
		if !exists {
			if err := c.mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("MakeBucket(%s): %w", bucket, err)
			}
		}
	}
	return nil
}

// UploadPhoto stores a binary object under the given key in the specified bucket.
func (c *Client) UploadPhoto(
	ctx context.Context,
	bucket, objectKey string,
	reader io.Reader,
	size int64,
	contentType string,
) error {
	_, err := c.mc.PutObject(ctx, bucket, objectKey, reader, size,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("PutObject(%s/%s): %w", bucket, objectKey, err)
	}
	return nil
}

// GetPresignedURL returns a time-limited pre-signed GET URL for an object.
func (c *Client) GetPresignedURL(
	ctx context.Context,
	bucket, objectKey string,
	expiry time.Duration,
) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, bucket, objectKey, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("PresignedGetObject(%s/%s): %w", bucket, objectKey, err)
	}
	return u.String(), nil
}
