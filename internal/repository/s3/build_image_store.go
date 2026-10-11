package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// BuildImageStore is the S3-backed repository.BuildImageStore.
type BuildImageStore struct {
	client  s3API
	presign s3PresignAPI
	bucket  string
	creds   credentialsProvider
}

var _ repository.BuildImageStore = (*BuildImageStore)(nil)

// NewBuildImageStore returns a BuildImageStore backed by client.
func NewBuildImageStore(client *s3.Client, presign *s3.PresignClient, bucket string, creds aws.CredentialsProvider) *BuildImageStore {
	return &BuildImageStore{
		client:  client,
		presign: presign,
		bucket:  bucket,
		creds:   creds,
	}
}

// PresignGetBuildImage implements repository.BuildImageStore.
func (s *BuildImageStore) PresignGetBuildImage(ctx context.Context, key repository.BuildImageKey) (url string, expiresAt time.Time, err error) {
	return presignGet(ctx, s.creds, s.presign, s.bucket, string(key))
}

// PresignPutBuildImage implements repository.BuildImageStore.
func (s *BuildImageStore) PresignPutBuildImage(ctx context.Context, key repository.BuildImageKey, contentType string, size int64) (string, error) {
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

// DeleteBuildImage implements repository.BuildImageStore.
func (s *BuildImageStore) DeleteBuildImage(ctx context.Context, key repository.BuildImageKey) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(string(key)),
	})
	if err != nil {
		return fmt.Errorf("deleting s3://%s/%s: %w", s.bucket, key, err)
	}

	return nil
}
