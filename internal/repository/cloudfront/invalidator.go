package cloudfront

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscloudfront "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/google/uuid"
)

type cloudfrontAPI interface {
	CreateInvalidation(ctx context.Context, params *awscloudfront.CreateInvalidationInput, optFns ...func(*awscloudfront.Options)) (*awscloudfront.CreateInvalidationOutput, error)
}

// Invalidator clears paths from the images distribution's edge caches.
type Invalidator struct {
	client         cloudfrontAPI
	distributionID string
}

// NewInvalidator returns an Invalidator for distributionID. With an empty
// distributionID, Invalidate does nothing.
func NewInvalidator(client *awscloudfront.Client, distributionID string) *Invalidator {
	return &Invalidator{client: client, distributionID: distributionID}
}

// Invalidate clears paths, each an object key with a leading "/" or a key
// prefix ending in "*".
func (i *Invalidator) Invalidate(ctx context.Context, paths ...string) error {
	if i.distributionID == "" || len(paths) == 0 {
		return nil
	}

	_, err := i.client.CreateInvalidation(ctx, &awscloudfront.CreateInvalidationInput{
		DistributionId: aws.String(i.distributionID),
		InvalidationBatch: &types.InvalidationBatch{
			CallerReference: aws.String(uuid.NewString()),
			Paths:           &types.Paths{Quantity: aws.Int32(int32(len(paths))), Items: paths}, //nolint:gosec // a handful of paths, never near int32
		},
	})
	if err != nil {
		return fmt.Errorf("invalidating %v on distribution %s: %w", paths, i.distributionID, err)
	}

	return nil
}
