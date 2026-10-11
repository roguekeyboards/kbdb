// Package moderation helps specs observe the moderation function: canned
// Rekognition responses for floci, and the tags it leaves on objects.
package moderation

import (
	"context"
	"encoding/json"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
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
func MockLabels(key string, labels ...Label) error {
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
	return os.WriteFile(support.AIMockConfigPath(), data, 0o644) //nolint:gosec // canned test responses, nothing secret
}

// ObjectTags returns the tags on the object at key in the images bucket.
func ObjectTags(ctx context.Context, key string) (map[string]string, error) {
	endpoint := support.S3EndpointURL()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-2"))
	if err != nil {
		return nil, err
	}
	if endpoint != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
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
