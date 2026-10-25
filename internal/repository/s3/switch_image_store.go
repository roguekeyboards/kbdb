package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// SwitchImageStore is the S3-backed repository.SwitchImageStore.
type SwitchImageStore struct {
	client  s3API
	presign s3PresignAPI
	bucket  string
	get     *GetSigner
}

var _ repository.SwitchImageStore = (*SwitchImageStore)(nil)

// NewSwitchImageStore returns a SwitchImageStore backed by client.
func NewSwitchImageStore(client *s3.Client, presign *s3.PresignClient, bucket string, get *GetSigner) *SwitchImageStore {
	return &SwitchImageStore{
		client:  client,
		presign: presign,
		bucket:  bucket,
		get:     get,
	}
}

// SignGet implements repository.SwitchImageStore.
func (s *SwitchImageStore) SignGet(_ context.Context, key repository.SwitchImageKey) (url string, expiresAt time.Time, err error) {
	return s.get.sign(string(key))
}

// PresignPut implements repository.SwitchImageStore.
func (s *SwitchImageStore) PresignPut(ctx context.Context, key repository.SwitchImageKey, contentType string, size int64) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(string(key)),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
		IfNoneMatch:   aws.String("*"),
	})
	if err != nil {
		return "", fmt.Errorf("presigning PUT s3://%s/%s (content-type %q, %d bytes): %w", s.bucket, key, contentType, size, err)
	}

	return req.URL, nil
}

// Delete implements repository.SwitchImageStore.
func (s *SwitchImageStore) Delete(ctx context.Context, key repository.SwitchImageKey) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(string(key)),
	})
	if err != nil {
		return fmt.Errorf("deleting s3://%s/%s: %w", s.bucket, key, err)
	}

	return nil
}
