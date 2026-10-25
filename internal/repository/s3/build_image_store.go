package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/log"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/cloudfront"
)

// BuildImageStore is the S3-backed repository.BuildImageStore.
type BuildImageStore struct {
	client  s3API
	presign s3PresignAPI
	bucket  string
	get     getSigner
	cdn     invalidator
}

var _ repository.BuildImageStore = (*BuildImageStore)(nil)

// NewBuildImageStore returns a BuildImageStore backed by client.
func NewBuildImageStore(client *s3.Client, presign *s3.PresignClient, bucket string, get *cloudfront.GetSigner, cdn *cloudfront.Invalidator) *BuildImageStore {
	return &BuildImageStore{
		client:  client,
		presign: presign,
		bucket:  bucket,
		get:     get,
		cdn:     cdn,
	}
}

// SignGetBuildImage implements repository.BuildImageStore.
func (s *BuildImageStore) SignGetBuildImage(_ context.Context, key repository.BuildImageKey) (url string, expiresAt time.Time, err error) {
	return s.get.Sign(string(key))
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

	if err := s.cdn.Invalidate(ctx, "/"+string(key)); err != nil {
		log.FromContext(ctx).Error("invalidating a deleted image", log.Error, err)
	}

	return nil
}
