package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// ErrNotCurrent is returned for a key no item's image slot holds any more.
var ErrNotCurrent = errors.New("no item holds this image")

// ErrImageGone is returned when approving an image whose file has
// already expired.
var ErrImageGone = errors.New("the image's file no longer exists")

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
	if _, err := imagekey.Parse(key); err != nil {
		return false, err
	}
	// Checked before the write: approving an expired file would leave the
	// item pointing at nothing.
	if _, err := o.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &o.bucket, Key: &key}); err != nil {
		if _, ok := errors.AsType[*s3types.NotFound](err); ok {
			return false, fmt.Errorf("%w: %q", ErrImageGone, key)
		}
		return false, fmt.Errorf("reading %q's metadata: %w", key, err)
	}
	already, err = o.override(ctx, key, repository.ImageStatusRejected, repository.ImageStatusApproved, nil)
	if err != nil {
		return false, err
	}
	return already, o.untagRejected(ctx, key)
}

// Reject takes down an approved image: it marks it rejected with reason.
// Unlike the moderation function's rejections, it isn't tagged to expire:
// the lifecycle rule counts from upload, so an older image would be
// deleted within a day, before the takedown could be reviewed. already
// reports an image already rejected, whose reason is left unchanged.
func (o *Overrides) Reject(ctx context.Context, key, reason string) (already bool, err error) {
	return o.override(ctx, key, repository.ImageStatusApproved, repository.ImageStatusRejected, &reason)
}

// untagRejected removes the moderation tag from key, keeping any others.
func (o *Overrides) untagRejected(ctx context.Context, key string) error {
	out, err := o.s3.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{Bucket: &o.bucket, Key: &key})
	if err != nil {
		return fmt.Errorf("reading %q's tags: %w", key, err)
	}
	kept := slices.DeleteFunc(out.TagSet, func(t s3types.Tag) bool { return aws.ToString(t.Key) == "moderation" })
	if len(kept) == len(out.TagSet) {
		return nil
	}
	if len(kept) == 0 {
		_, err = o.s3.DeleteObjectTagging(ctx, &s3.DeleteObjectTaggingInput{Bucket: &o.bucket, Key: &key})
	} else {
		_, err = o.s3.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{Bucket: &o.bucket, Key: &key, Tagging: &s3types.Tagging{TagSet: kept}})
	}
	if err != nil {
		return fmt.Errorf("removing the rejected tag from %q: %w", key, err)
	}
	return nil
}

// override moves the image at key from one status to the other. Approve
// writes the status before removing the tag, so a failed tag change is
// fixed by running it again, which finds the image already at to.
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
