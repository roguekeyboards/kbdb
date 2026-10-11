package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// KeycapKitImageStore is the S3-backed repository.KeycapKitImageStore.
type KeycapKitImageStore struct {
	client  s3API
	presign s3PresignAPI
	bucket  string
	creds   credentialsProvider
}

var _ repository.KeycapKitImageStore = (*KeycapKitImageStore)(nil)

// NewKeycapKitImageStore returns a KeycapKitImageStore backed by client.
func NewKeycapKitImageStore(client *s3.Client, presign *s3.PresignClient, bucket string, creds aws.CredentialsProvider) *KeycapKitImageStore {
	return &KeycapKitImageStore{
		client:  client,
		presign: presign,
		bucket:  bucket,
		creds:   creds,
	}
}

// PresignGet implements repository.KeycapKitImageStore.
func (s *KeycapKitImageStore) PresignGet(ctx context.Context, key repository.KeycapKitImageKey) (url string, expiresAt time.Time, err error) {
	return presignGet(ctx, s.creds, s.presign, s.bucket, string(key))
}

// PresignPut implements repository.KeycapKitImageStore.
func (s *KeycapKitImageStore) PresignPut(ctx context.Context, key repository.KeycapKitImageKey, contentType string, size int64) (string, error) {
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

// Delete implements repository.KeycapKitImageStore.
func (s *KeycapKitImageStore) Delete(ctx context.Context, key repository.KeycapKitImageKey) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(string(key)),
	})
	if err != nil {
		return fmt.Errorf("deleting s3://%s/%s: %w", s.bucket, key, err)
	}

	return nil
}
