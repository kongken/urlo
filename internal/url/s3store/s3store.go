// Package s3store provides an S3-backed implementation of url.Store.
//
// Each short link is stored as a JSON object at:
//
//	{prefix}/{code}.json
//
// Concurrency notes: Create uses S3's If-None-Match: * conditional write
// to atomically reject duplicate codes. IncrVisit performs a best-effort
// read-modify-write and is NOT safe under heavy concurrent traffic for
// the same code; use a Redis counter or DynamoDB if exact counts matter.
package s3store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/kongken/urlo/internal/url"
)

// API is the subset of *s3.Client used by Store. *awss3.Client satisfies
// it; tests can pass a fake.
type API interface {
	PutObject(ctx context.Context, in *awss3.PutObjectInput, opts ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	GetObject(ctx context.Context, in *awss3.GetObjectInput, opts ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	HeadObject(ctx context.Context, in *awss3.HeadObjectInput, opts ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, in *awss3.DeleteObjectInput, opts ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
	ListObjectsV2(ctx context.Context, in *awss3.ListObjectsV2Input, opts ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
}

// Store implements url.Store on top of an S3 bucket.
type Store struct {
	client API
	bucket string
	prefix string
}

type Options struct {
	Client API
	Bucket string
	// Prefix is prepended to all object keys. Leading/trailing slashes are
	// trimmed. Use it to share a bucket with other data, e.g. "urlo/links"
	// or "envs/prod/urlo". Default: "links".
	Prefix string
}

func New(opts Options) (*Store, error) {
	if opts.Client == nil {
		return nil, errors.New("s3store: Client is required")
	}
	if opts.Bucket == "" {
		return nil, errors.New("s3store: Bucket is required")
	}
	prefix := strings.Trim(opts.Prefix, "/")
	if prefix == "" {
		prefix = "links"
	}
	return &Store{
		client: opts.Client,
		bucket: opts.Bucket,
		prefix: prefix,
	}, nil
}

func (s *Store) key(code string) string {
	return s.prefix + "/" + code + ".json"
}

// ownerKey returns the index pointer object key for (ownerID, code).
// The object body is empty; presence is the index.
func (s *Store) ownerKey(ownerID, code string) string {
	return s.prefix + "/owners/" + ownerID + "/" + code
}

func (s *Store) ownerPrefix(ownerID string) string {
	return s.prefix + "/owners/" + ownerID + "/"
}

func (s *Store) Create(ctx context.Context, r *url.Record) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key(r.Code)),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
		IfNoneMatch: aws.String("*"),
	})
	if err != nil {
		if isPreconditionFailed(err) {
			return url.ErrAlreadyExists
		}
		return err
	}
	if r.OwnerID != "" {
		// Best-effort owner index pointer.
		_, _ = s.client.PutObject(ctx, &awss3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s.ownerKey(r.OwnerID, r.Code)),
			Body:   bytes.NewReader(nil),
		})
	}
	return nil
}

func (s *Store) Get(ctx context.Context, code string) (*url.Record, error) {
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(code)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, url.ErrNotFound
		}
		return nil, err
	}
	defer out.Body.Close()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}
	var r url.Record
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) Delete(ctx context.Context, code string) error {
	// HEAD first so we can return ErrNotFound consistently — DeleteObject
	// is idempotent and returns success even if the key doesn't exist.
	_, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(code)),
	})
	if err != nil {
		if isNotFound(err) {
			return url.ErrNotFound
		}
		return err
	}
	// Read existing record to find owner pointer (best-effort).
	var ownerID string
	if r, gerr := s.Get(ctx, code); gerr == nil && r != nil {
		ownerID = r.OwnerID
	}
	_, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(code)),
	})
	if err != nil {
		return err
	}
	if ownerID != "" {
		_, _ = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s.ownerKey(ownerID, code)),
		})
	}
	return nil
}

func (s *Store) IncrVisit(ctx context.Context, code string) (*url.Record, error) {
	r, err := s.Get(ctx, code)
	if err != nil {
		return nil, err
	}
	r.VisitCount++

	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key(code)),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) ListByOwner(ctx context.Context, ownerID string) ([]*url.Record, error) {
	if ownerID == "" {
		return nil, nil
	}
	prefix := s.ownerPrefix(ownerID)
	var (
		out   []*url.Record
		token *string
	)
	for {
		page, err := s.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			code := strings.TrimPrefix(*obj.Key, prefix)
			if code == "" {
				continue
			}
			r, err := s.Get(ctx, code)
			if err != nil {
				if errors.Is(err, url.ErrNotFound) {
					continue
				}
				return nil, err
			}
			out = append(out, r)
		}
		if page.IsTruncated == nil || !*page.IsTruncated {
			break
		}
		token = page.NextContinuationToken
	}
	return out, nil
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

func isPreconditionFailed(err error) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "PreconditionFailed", "412":
			return true
		}
	}
	return false
}
