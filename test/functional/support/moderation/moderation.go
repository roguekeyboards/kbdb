// Package moderation helps specs observe the moderation function: the tags
// it leaves on objects, and the operator CLI's takedown.
package moderation

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/roguekeyboards/kbdb/test/functional/support"
)

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

// Reject takes down the image at key by running the operator CLI's reject
// against the stack under test, as an operator would.
func Reject(ctx context.Context, key, reason string) error {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	args := []string{"run", "./cmd/kbdb-moderation", "--stack", support.StackName()}
	if region != "" {
		args = append(args, "--region", region)
	}
	args = append(args, "reject", key, reason)
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // test-controlled arguments
	cmd.Dir = support.RepoRoot()
	cmd.Env = os.Environ()
	if endpoint := support.AWSEndpointURL(); endpoint != "" {
		cmd.Env = append(cmd.Env, "AWS_ENDPOINT_URL="+endpoint)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("kbdb-moderation reject: %w: %s", err, out)
	}
	return nil
}
