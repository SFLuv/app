// Package blobstore keeps image bytes in S3-compatible object storage (a
// Railway bucket) instead of Postgres BYTEA columns.
//
// The move is gradual and reversible. Rows gain a nullable object_key; a
// background job copies existing bytes into the bucket and sets the key only
// after reading the object back and checking it. Reads prefer the bucket when
// a row has a key and fall back to the Postgres bytes, which are kept until a
// later, separate cleanup. Unsetting the BLOB_* variables turns the bucket off
// entirely and every image is served from Postgres again.
//
// Keys are append-only: every copy gets a fresh key and objects are never
// overwritten, so a bug cannot silently replace an existing image.
package blobstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

type s3Store struct {
	client *minio.Client
	bucket string
}

// FromEnv returns the configured bucket, or nil when BLOB_BUCKET is unset —
// nil means "bucket off": callers keep reading and writing Postgres.
//
// On Railway, set these on the backend as references to the bucket service:
// BLOB_BUCKET=${{Bucket.BUCKET}}, BLOB_ENDPOINT=${{Bucket.ENDPOINT}},
// BLOB_ACCESS_KEY_ID=${{Bucket.ACCESS_KEY_ID}},
// BLOB_SECRET_ACCESS_KEY=${{Bucket.SECRET_ACCESS_KEY}},
// BLOB_REGION=${{Bucket.REGION}}.
func FromEnv() (Store, error) {
	bucket := strings.TrimSpace(os.Getenv("BLOB_BUCKET"))
	if bucket == "" {
		return nil, nil
	}
	rawEndpoint := strings.TrimSpace(os.Getenv("BLOB_ENDPOINT"))
	if rawEndpoint == "" {
		return nil, fmt.Errorf("BLOB_BUCKET is set but BLOB_ENDPOINT is not")
	}
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil || endpoint.Host == "" {
		return nil, fmt.Errorf("BLOB_ENDPOINT %q is not a URL", rawEndpoint)
	}
	region := strings.TrimSpace(os.Getenv("BLOB_REGION"))
	if region == "" {
		region = "auto"
	}
	// Older Railway buckets only answer path-style URLs.
	lookup := minio.BucketLookupAuto
	if strings.TrimSpace(os.Getenv("BLOB_PATH_STYLE")) == "true" {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds: credentials.NewStaticV4(
			strings.TrimSpace(os.Getenv("BLOB_ACCESS_KEY_ID")),
			strings.TrimSpace(os.Getenv("BLOB_SECRET_ACCESS_KEY")),
			"",
		),
		Secure:       endpoint.Scheme != "http",
		Region:       region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, err
	}
	return &s3Store{client: client, bucket: bucket}, nil
}

func (s *s3Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *s3Store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

// NewKey returns a fresh, never-reused key under prefix, e.g.
// "event_photos/<row id>/<random>".
func NewKey(prefix, rowID string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s/%s/%s", prefix, rowID, hex.EncodeToString(b[:]))
}
