package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/roguekeyboards/kbdb/internal/log"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/cloudfront"
)

// KeyboardImageStore is the S3-backed repository.KeyboardImageStore.
type KeyboardImageStore struct {
	client  s3API
	presign s3PresignAPI
	bucket  string
	get     getSigner
	cdn     invalidator
}

var _ repository.KeyboardImageStore = (*KeyboardImageStore)(nil)

// NewKeyboardImageStore returns a KeyboardImageStore backed by client.
func NewKeyboardImageStore(client *s3.Client, presign *s3.PresignClient, bucket string, get *cloudfront.GetSigner, cdn *cloudfront.Invalidator) *KeyboardImageStore {
	return &KeyboardImageStore{
		client:  client,
		presign: presign,
		bucket:  bucket,
		get:     get,
		cdn:     cdn,
	}
}

// SignGetKeyboardImage implements repository.KeyboardImageStore.
func (s *KeyboardImageStore) SignGetKeyboardImage(_ context.Context, key repository.KeyboardImageKey) (url string, err error) {
	return s.get.Sign(string(key))
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

	if err := s.cdn.Invalidate(ctx, "/"+string(key)); err != nil {
		log.FromContext(ctx).Error("invalidating a deleted image", log.Error, err)
	}

	return nil
}
