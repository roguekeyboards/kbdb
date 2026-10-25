package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// KeyboardImageStore is the S3-backed repository.KeyboardImageStore.
type KeyboardImageStore struct {
	client  s3API
	presign s3PresignAPI
	bucket  string
	get     *GetSigner
}

var _ repository.KeyboardImageStore = (*KeyboardImageStore)(nil)

// NewKeyboardImageStore returns a KeyboardImageStore backed by client.
func NewKeyboardImageStore(client *s3.Client, presign *s3.PresignClient, bucket string, get *GetSigner) *KeyboardImageStore {
	return &KeyboardImageStore{
		client:  client,
		presign: presign,
		bucket:  bucket,
		get:     get,
	}
}

// SignGetKeyboardImage implements repository.KeyboardImageStore.
func (s *KeyboardImageStore) SignGetKeyboardImage(_ context.Context, key repository.KeyboardImageKey) (url string, expiresAt time.Time, err error) {
	return s.get.sign(string(key))
}

// PresignPutKeyboardImage implements repository.KeyboardImageStore.
func (s *KeyboardImageStore) PresignPutKeyboardImage(ctx context.Context, key repository.KeyboardImageKey, contentType string, size int64) (string, error) {
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

// DeleteKeyboardImage implements repository.KeyboardImageStore.
func (s *KeyboardImageStore) DeleteKeyboardImage(ctx context.Context, key repository.KeyboardImageKey) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(string(key)),
	})
	if err != nil {
		return fmt.Errorf("deleting s3://%s/%s: %w", s.bucket, key, err)
	}

	return nil
}
