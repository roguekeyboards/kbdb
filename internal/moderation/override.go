package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// ErrNotCurrent is returned for a key no item's image slot holds any more.
var ErrNotCurrent = errors.New("no item holds this image")

// Overrides changes moderation decisions by hand, for the operator CLI.
type Overrides struct {
	s3     s3API
	images *Images
	bucket string
}

// NewOverrides returns Overrides for the images in bucket.
func NewOverrides(s3Client *s3.Client, images *Images, bucket string) *Overrides {
	return &Overrides{s3: s3Client, images: images, bucket: bucket}
}

// Approve marks a rejected image approved and removes the tag that would
// expire it. Approving an image already approved only removes the tag, so
// a run that failed partway can be repeated; already reports that case.
func (o *Overrides) Approve(ctx context.Context, key string) (already bool, err error) {
	already, err = o.override(ctx, key, repository.ImageStatusRejected, repository.ImageStatusApproved, nil)
	if err != nil {
		return false, err
	}
	if _, err := o.s3.DeleteObjectTagging(ctx, &s3.DeleteObjectTaggingInput{Bucket: &o.bucket, Key: &key}); err != nil {
		return false, fmt.Errorf("removing the rejected tag from %q: %w", key, err)
	}
	return already, nil
}

// Reject takes down an approved image: it marks it rejected with reason
// and tags it to expire like any other rejection. Rejecting an image
// already rejected only tags it, keeping its reason; already reports that
// case.
func (o *Overrides) Reject(ctx context.Context, key, reason string) (already bool, err error) {
	already, err = o.override(ctx, key, repository.ImageStatusApproved, repository.ImageStatusRejected, &reason)
	if err != nil {
		return false, err
	}
	return already, tagRejected(ctx, o.s3, o.bucket, key)
}

// override moves the image at key from one status to the other. The
// status is written before the tag changes, so a failed tag change is
// fixed by running the override again, which finds the image already at
// to.
func (o *Overrides) override(ctx context.Context, key string, from, to repository.ImageStatus, reason *string) (already bool, err error) {
	ref, err := imagekey.Parse(key)
	if err != nil {
		return false, err
	}
	ok, err := o.images.SetStatus(ctx, ref, key, from, to, reason)
	if err != nil {
		return false, fmt.Errorf("writing the status of %q: %w", key, err)
	}
	if ok {
		return false, nil
	}

	status, current, err := o.images.Status(ctx, ref, key)
	switch {
	case err != nil:
		return false, fmt.Errorf("reading the status of %q: %w", key, err)
	case !current:
		return false, fmt.Errorf("%w: %q", ErrNotCurrent, key)
	case status != to:
		return false, fmt.Errorf("%q is %s, not %s", key, status, from)
	}
	return true, nil
}
