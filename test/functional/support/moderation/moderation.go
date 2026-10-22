// Package moderation helps specs observe the moderation function: canned
// Rekognition responses for floci, and the tags it leaves on objects.
package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
)

// Label is one moderation label in a canned Rekognition response.
type Label struct {
	Name          string
	ParentName    string
	TaxonomyLevel int
	Confidence    float64
}

// probeBucket holds the valid JPEG MockLabels mocks along with the spec's
// key, so it can tell when floci has picked up the mock file before the
// spec's own object exists: floci refuses an S3Object that doesn't exist
// before it looks at the mocks. It's a bucket of its own so the moderation
// function never sees the probe.
const probeBucket = "kbdb-functional-mock-probe"

// MockLabels has floci's Rekognition return labels for the object at key,
// by replacing the AI mock config file. Only one spec may use it, since
// each call replaces every earlier response.
func MockLabels(ctx context.Context, key string, labels ...Label) error {
	// A new probe each time, so a mock file left by an earlier run can't
	// answer for it.
	probeKey := uuid.NewString() + ".jpg"
	response := map[string]any{
		"DetectModerationLabels": map[string]any{
			"ModerationLabels":       labels,
			"ModerationModelVersion": "7.0",
		},
	}
	config := map[string]any{
		"rekognition": map[string]any{
			support.ImagesBucketName() + "/" + key: response,
			probeBucket + "/" + probeKey:           response,
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	cfg, err := awsConfig(ctx)
	if err != nil {
		return err
	}
	if err := putProbe(ctx, cfg, probeKey); err != nil {
		return err
	}
	// floci reads it as its own user, not ours.
	if err := os.WriteFile(support.AIMockConfigPath(), data, 0o644); err != nil { //nolint:gosec // canned test responses, nothing secret
		return err
	}
	return awaitMock(ctx, cfg, probeKey)
}

// putProbe uploads a probe object, creating its bucket the first time.
func putProbe(ctx context.Context, cfg aws.Config, probeKey string) error {
	client := s3Client(cfg)
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(probeBucket),
		CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(cfg.Region),
		},
	})
	if err != nil && !bucketExists(err) {
		return fmt.Errorf("creating the mock probe bucket: %w", err)
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(probeBucket),
		Key:         aws.String(probeKey),
		Body:        bytes.NewReader(api.TestJPEG),
		ContentType: aws.String("image/jpeg"),
	})
	if err != nil {
		return fmt.Errorf("uploading the mock probe: %w", err)
	}
	return nil
}

func bucketExists(err error) bool {
	if _, ok := errors.AsType[*s3types.BucketAlreadyOwnedByYou](err); ok {
		return true
	}
	_, ok := errors.AsType[*s3types.BucketAlreadyExists](err)
	return ok
}

// awaitMock waits until floci answers for probeKey with the canned
// labels. floci re-reads the file when its modification time changes, but
// through Docker Desktop's file sharing the change can take a moment to
// show.
func awaitMock(ctx context.Context, cfg aws.Config, probeKey string) error {
	endpoint := support.S3EndpointURL()
	client := rekognition.NewFromConfig(cfg, func(o *rekognition.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	input := &rekognition.DetectModerationLabelsInput{Image: &rektypes.Image{S3Object: &rektypes.S3Object{
		Bucket: aws.String(probeBucket),
		Name:   aws.String(probeKey),
	}}}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		out, err := client.DetectModerationLabels(ctx, input)
		if err == nil && len(out.ModerationLabels) > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("floci didn't pick up the canned labels: %w", err)
			}
			return fmt.Errorf("floci didn't pick up the canned labels: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// awsConfig is the SDK configuration for talking to floci.
func awsConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, err
	}
	if support.S3EndpointURL() != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
	}
	return cfg, nil
}

// ObjectTags returns the tags on the object at key in the images bucket.
func ObjectTags(ctx context.Context, key string) (map[string]string, error) {
	awsCfg, err := awsConfig(ctx)
	if err != nil {
		return nil, err
	}
	out, err := s3Client(awsCfg).GetObjectTagging(ctx, &s3.GetObjectTaggingInput{
		Bucket: aws.String(support.ImagesBucketName()),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string, len(out.TagSet))
	for _, t := range out.TagSet {
		tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return tags, nil
}

func s3Client(cfg aws.Config) *s3.Client {
	endpoint := support.S3EndpointURL()
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})
}
