package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// region is fixed so pre-signing never needs a bucket-location round trip
// (the public endpoint is usually not reachable from inside the stack).
const region = "us-east-1"

// Client wraps minio.Client with convenience methods for SecurePatrol.
type Client struct {
	mc     *minio.Client // internal endpoint, used for all I/O
	signer *minio.Client // public endpoint, used only to pre-sign URLs
}

// Options configures the storage client.
type Options struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	UseSSL         bool
	PublicUseSSL   bool
}

// New creates a MinIO client.
func New(o Options) (*Client, error) {
	creds := credentials.NewStaticV4(o.AccessKey, o.SecretKey, "")
	mc, err := minio.New(o.Endpoint, &minio.Options{Creds: creds, Secure: o.UseSSL, Region: region})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}
	signer := mc
	if o.PublicEndpoint != "" && (o.PublicEndpoint != o.Endpoint || o.PublicUseSSL != o.UseSSL) {
		signer, err = minio.New(o.PublicEndpoint, &minio.Options{Creds: creds, Secure: o.PublicUseSSL, Region: region})
		if err != nil {
			return nil, fmt.Errorf("minio.New(public): %w", err)
		}
	}
	return &Client{mc: mc, signer: signer}, nil
}

// EnsureBuckets creates each bucket if it does not already exist.
func (c *Client) EnsureBuckets(ctx context.Context, buckets []string) error {
	for _, bucket := range buckets {
		exists, err := c.mc.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("BucketExists(%s): %w", bucket, err)
		}
		if !exists {
			if err := c.mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
				return fmt.Errorf("MakeBucket(%s): %w", bucket, err)
			}
		}
	}
	return nil
}

// EnableVersioning turns on object versioning for a bucket (used for backups).
func (c *Client) EnableVersioning(ctx context.Context, bucket string) error {
	if err := c.mc.EnableVersioning(ctx, bucket); err != nil {
		return fmt.Errorf("EnableVersioning(%s): %w", bucket, err)
	}
	return nil
}

// Put stores an object. size may be -1 for streams of unknown length.
func (c *Client) Put(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (int64, error) {
	info, err := c.mc.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return 0, fmt.Errorf("PutObject(%s/%s): %w", bucket, key, err)
	}
	return info.Size, nil
}

// Exists reports whether an object exists.
func (c *Client) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := c.mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return false, nil
	}
	return false, fmt.Errorf("StatObject(%s/%s): %w", bucket, key, err)
}

// PresignGet returns a time-limited GET URL on the public endpoint.
func (c *Client) PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := c.signer.PresignedGetObject(ctx, bucket, key, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("PresignedGetObject(%s/%s): %w", bucket, key, err)
	}
	return u.String(), nil
}

// Ping checks that the object store answers.
func (c *Client) Ping(ctx context.Context, bucket string) error {
	_, err := c.mc.BucketExists(ctx, bucket)
	return err
}
