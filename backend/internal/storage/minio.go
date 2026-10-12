package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// ErrExists is returned by PutNew when the key is already taken.
var ErrExists = errors.New("object already exists")

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Size   int64
	SHA256 string // from the sha256 metadata written by PutNew; "" for older objects
}

// metaSHA256 is the user-metadata key holding the content hash.
const metaSHA256 = "Sha256"

// Stat returns an object's size and content hash; ok is false when the
// object does not exist.
func (c *Client) Stat(ctx context.Context, bucket, key string) (ObjectInfo, bool, error) {
	info, err := c.mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return ObjectInfo{}, false, nil
		}
		return ObjectInfo{}, false, fmt.Errorf("StatObject(%s/%s): %w", bucket, key, err)
	}
	out := ObjectInfo{Size: info.Size}
	for k, v := range info.UserMetadata {
		if strings.EqualFold(k, metaSHA256) {
			out.SHA256 = v
		}
	}
	return out, true, nil
}

// PutNew stores data under key only if the key is still free (write-once
// evidence, BE-S4): the PUT is conditional (If-None-Match: *), so a racing
// writer gets ErrExists. sha256 is stored as object metadata.
func (c *Client) PutNew(ctx context.Context, bucket, key string, data []byte, contentType, sha256 string) error {
	opts := minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{metaSHA256: sha256}}
	opts.SetMatchETagExcept("*")
	_, err := c.mc.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), opts)
	if err != nil {
		if r := minio.ToErrorResponse(err); r.StatusCode == http.StatusPreconditionFailed || r.Code == "PreconditionFailed" {
			return ErrExists
		}
		return fmt.Errorf("PutObject(%s/%s): %w", bucket, key, err)
	}
	return nil
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

// ErrTooLarge is returned by Get when an object exceeds the size limit.
var ErrTooLarge = errors.New("object too large")

// Get reads an object of at most max bytes; ok is false when it does not
// exist.
func (c *Client) Get(ctx context.Context, bucket, key string, max int64) ([]byte, bool, error) {
	obj, err := c.mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("GetObject(%s/%s): %w", bucket, key, err)
	}
	defer obj.Close()
	data, err := io.ReadAll(io.LimitReader(obj, max+1))
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("GetObject(%s/%s): %w", bucket, key, err)
	}
	if int64(len(data)) > max {
		return nil, true, ErrTooLarge
	}
	return data, true, nil
}

// RemoveAllVersions permanently deletes an object including every version
// and delete marker (retention, BE-S9: a plain delete in a versioned bucket
// would only hide the data). Missing objects are not an error.
func (c *Client) RemoveAllVersions(ctx context.Context, bucket, key string) error {
	found := false
	for obj := range c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: key, WithVersions: true}) {
		if obj.Err != nil {
			return fmt.Errorf("ListObjectVersions(%s/%s): %w", bucket, key, obj.Err)
		}
		if obj.Key != key {
			continue
		}
		found = true
		if err := c.mc.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{VersionID: obj.VersionID}); err != nil {
			return fmt.Errorf("RemoveObject(%s/%s@%s): %w", bucket, key, obj.VersionID, err)
		}
	}
	if !found {
		// Unversioned bucket or listing without versions: plain delete.
		if err := c.mc.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil &&
			minio.ToErrorResponse(err).Code != "NoSuchKey" {
			return fmt.Errorf("RemoveObject(%s/%s): %w", bucket, key, err)
		}
	}
	return nil
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
