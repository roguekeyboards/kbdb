// Package cdn lets specs check what the images distribution was asked to
// invalidate.
package cdn

import (
	"context"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// Invalidated reports whether any of the distribution's invalidations
// covers path.
func Invalidated(ctx context.Context, path string) (bool, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return false, err
	}
	endpoint := support.AWSEndpointURL()
	if endpoint != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
	}
	client := cloudfront.NewFromConfig(cfg, func(o *cloudfront.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	id := aws.String(support.ImagesDistributionID())
	var marker *string
	for {
		list, err := client.ListInvalidations(ctx, &cloudfront.ListInvalidationsInput{DistributionId: id, Marker: marker})
		if err != nil {
			return false, err
		}
		for _, summary := range list.InvalidationList.Items {
			inv, err := client.GetInvalidation(ctx, &cloudfront.GetInvalidationInput{DistributionId: id, Id: summary.Id})
			if err != nil {
				return false, err
			}
			if slices.Contains(inv.Invalidation.InvalidationBatch.Paths.Items, path) {
				return true, nil
			}
		}
		if !aws.ToBool(list.InvalidationList.IsTruncated) {
			return false, nil
		}
		marker = list.InvalidationList.NextMarker
	}
}
