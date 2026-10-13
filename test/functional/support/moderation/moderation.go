// Package moderation helps specs observe the moderation function: canned
// Rekognition responses for floci, and the tags it leaves on objects.
package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// Label is one moderation label in a canned Rekognition response.
type Label struct {
	Name          string
	ParentName    string
	TaxonomyLevel int
	Confidence    float64
}

// MockLabels has floci's Rekognition return labels for the object at key,
// by replacing the AI mock config file. Only one spec may use it, since
// each call replaces every earlier response.
func MockLabels(ctx context.Context, key string, labels ...Label) error {
	config := map[string]any{
		"rekognition": map[string]any{
			support.ImagesBucketName() + "/" + key: map[string]any{
				"DetectModerationLabels": map[string]any{
					"ModerationLabels":       labels,
					"ModerationModelVersion": "7.0",
				},
			},
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	// floci reads it as its own user, not ours.
	if err := os.WriteFile(support.AIMockConfigPath(), data, 0o644); err != nil { //nolint:gosec // canned test responses, nothing secret
		return err
	}
	return awaitMock(ctx, key)
}

// awaitMock waits until floci answers for key with the canned labels.
// floci re-reads the file when its modification time changes, but through
// Docker Desktop's file sharing the change can take a moment to show.
func awaitMock(ctx context.Context, key string) error {
	cfg, err := awsConfig(ctx)
	if err != nil {
		return err
	}
	endpoint := support.S3EndpointURL()
	client := rekognition.NewFromConfig(cfg, func(o *rekognition.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	input := &rekognition.DetectModerationLabelsInput{Image: &rektypes.Image{S3Object: &rektypes.S3Object{
		Bucket: aws.String(support.ImagesBucketName()),
		Name:   aws.String(key),
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
				return fmt.Errorf("floci didn't pick up the canned labels for %q: %w", key, err)
			}
			return fmt.Errorf("floci didn't pick up the canned labels for %q: %w", key, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// awsConfig is the SDK configuration for talking to floci.
func awsConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-2"))
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
	endpoint := support.S3EndpointURL()
	awsCfg, err := awsConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})

	out, err := client.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{
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
