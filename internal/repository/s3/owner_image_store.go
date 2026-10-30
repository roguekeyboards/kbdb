package s3

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/log"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/cloudfront"
)

// OwnerImageStore is the S3-backed repository.OwnerImageStore.
type OwnerImageStore struct {
	client s3API
	bucket string
	cdn    invalidator
}

var _ repository.OwnerImageStore = (*OwnerImageStore)(nil)

// NewOwnerImageStore returns an OwnerImageStore backed by client.
func NewOwnerImageStore(client *s3.Client, bucket string, cdn *cloudfront.Invalidator) *OwnerImageStore {
	return &OwnerImageStore{client: client, bucket: bucket, cdn: cdn}
}

// DeleteAll implements repository.OwnerImageStore. A list page holds at
// most 1,000 keys, DeleteObjects' cap, so each page is one delete request.
// Each prefix that had objects is then invalidated as a wildcard.
func (s *OwnerImageStore) DeleteAll(ctx context.Context) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("deleting all images: %w", repository.ErrNoUserID)
	}

	var deleted []string
	for _, prefix := range repository.OwnerImagePrefixes(ownerID) {
		pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
			Bucket: aws.String(s.bucket),
			Prefix: aws.String(prefix),
		})
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("listing s3://%s/%s: %w", s.bucket, prefix, err)
			}
			if len(page.Contents) == 0 {
				continue
			}
			if err := s.deleteObjects(ctx, page.Contents); err != nil {
				return fmt.Errorf("deleting under s3://%s/%s: %w", s.bucket, prefix, err)
			}
			if !slices.Contains(deleted, "/"+prefix+"*") {
				deleted = append(deleted, "/"+prefix+"*")
			}
		}
	}

	if len(deleted) == 0 {
		return nil
	}
	if err := s.cdn.Invalidate(ctx, deleted...); err != nil {
		log.FromContext(ctx).Error("invalidating deleted images", log.Error, err, log.OwnerID, ownerID)
	}

	return nil
}

func (s *OwnerImageStore) deleteObjects(ctx context.Context, objects []types.Object) error {
	ids := make([]types.ObjectIdentifier, 0, len(objects))
	for _, o := range objects {
		ids = append(ids, types.ObjectIdentifier{Key: o.Key})
	}
	out, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
	})
	if err != nil {
		return err
	}
	// DeleteObjects succeeds as a request even when some keys fail.
	errs := make([]error, 0, len(out.Errors))
	for _, e := range out.Errors {
		errs = append(errs, fmt.Errorf("%s: %s", aws.ToString(e.Key), aws.ToString(e.Message)))
	}
	return errors.Join(errs...)
}
