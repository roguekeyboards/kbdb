package moderation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// ErrNotCurrent is returned for a key no item's image slot holds any more.
var ErrNotCurrent = errors.New("no item holds this image")

// ErrImageGone is returned when approving an image whose file has
// expired, whether S3 has deleted it yet or not.
var ErrImageGone = errors.New("the image's file has expired")

// Overrides changes moderation decisions by hand, for the operator CLI.
type Overrides struct {
	s3     s3API
	images *Images
	bucket string
	now    func() time.Time
}

// NewOverrides returns Overrides for the images in bucket.
func NewOverrides(s3Client *s3.Client, images *Images, bucket string) *Overrides {
	return &Overrides{s3: s3Client, images: images, bucket: bucket, now: time.Now}
}

// Approve marks a rejected image approved and removes the tag that would
// expire it. already reports an image that was already approved, and
// untagged whether a tag was removed.
//
// The status is read first, so a key no item holds is reported as such
// rather than as an expired file, and the tag stays on a file that isn't
// a current rejected or approved image. The tag then goes before the status
// write, the mirror of the moderation function's order: if the write
// fails, the image is left rejected but no longer expiring, rather than
// approved with its file about to be deleted.
func (o *Overrides) Approve(ctx context.Context, key string) (already, untagged bool, err error) {
	ref, err := imagekey.Parse(key)
	if err != nil {
		return false, false, err
	}
	from, to := repository.ImageStatusRejected, repository.ImageStatusApproved
	status, err := o.currentStatus(ctx, ref, key, from, to)
	if err != nil {
		return false, false, err
	}
	if err := o.checkNotExpired(ctx, key); err != nil {
		return false, false, err
	}
	if untagged, err = o.untagRejected(ctx, key); err != nil {
		return false, false, err
	}
	if status == to {
		return true, untagged, nil
	}
	already, err = o.override(ctx, ref, key, from, to, nil)
	return already, untagged, err
}

// checkNotExpired fails when key's file is gone or past the expiry date S3
// reports for it: the lifecycle rule deletes in the background, so an
// expired file can still exist for up to a day, and removing its tag then
// doesn't save it.
func (o *Overrides) checkNotExpired(ctx context.Context, key string) error {
	head, err := o.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &o.bucket, Key: &key})
	if err != nil {
		if _, ok := errors.AsType[*s3types.NotFound](err); ok {
			return fmt.Errorf("%w: %q", ErrImageGone, key)
		}
		return fmt.Errorf("reading %q's metadata: %w", key, err)
	}
	if expiry, ok := expiryDate(aws.ToString(head.Expiration)); ok && !expiry.After(o.now()) {
		return fmt.Errorf("%w on %s: %q", ErrImageGone, expiry.Format(time.DateOnly), key)
	}
	return nil
}

// expiryDate reads the expiry-date out of an x-amz-expiration header, e.g.
// expiry-date="Fri, 23 Dec 2012 00:00:00 GMT", rule-id="expire-rejected-images".
func expiryDate(header string) (time.Time, bool) {
	m := expiryDatePattern.FindStringSubmatch(header)
	if m == nil {
		return time.Time{}, false
	}
	t, err := http.ParseTime(m[1])
	return t, err == nil
}

var expiryDatePattern = regexp.MustCompile(`expiry-date="([^"]+)"`)

// Reject takes down an approved image: it marks it rejected with reason
// and tags it to expire like any other rejection, keeping any other tags.
// The lifecycle rule counts from upload, so an older image is deleted at
// its next run: a takedown is a decision already reviewed. already reports
// an image already rejected, whose reason is left unchanged, and gone a
// file already deleted, which needs no tag.
//
// The status is written before the tag, so a pending image is never
// tagged; a failed tag is fixed by running Reject again.
func (o *Overrides) Reject(ctx context.Context, key, reason string) (already, gone bool, err error) {
	if strings.TrimSpace(reason) == "" {
		return false, false, errors.New("a rejection reason can't be blank")
	}
	ref, err := imagekey.Parse(key)
	if err != nil {
		return false, false, err
	}
	already, err = o.override(ctx, ref, key, repository.ImageStatusApproved, repository.ImageStatusRejected, &reason)
	if err != nil {
		return false, false, err
	}
	gone, err = o.addRejectedTag(ctx, key)
	return already, gone, err
}

// addRejectedTag adds the rejected tag to key's tags. gone reports that
// the file no longer exists.
func (o *Overrides) addRejectedTag(ctx context.Context, key string) (gone bool, err error) {
	out, err := o.s3.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{Bucket: &o.bucket, Key: &key})
	if isNoSuchKey(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %q's tags: %w", key, err)
	}
	tags := slices.DeleteFunc(slices.Clone(out.TagSet), func(t s3types.Tag) bool { return aws.ToString(t.Key) == rejectedTagKey })
	tags = append(tags, s3types.Tag{Key: aws.String(rejectedTagKey), Value: aws.String(rejectedTagValue)})
	_, err = o.s3.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{Bucket: &o.bucket, Key: &key, Tagging: &s3types.Tagging{TagSet: tags}})
	if isNoSuchKey(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("tagging %q rejected: %w", key, err)
	}
	return false, nil
}

// isNoSuchKey reports S3's NoSuchKey, which the object tagging calls
// return as a plain API error rather than *s3types.NoSuchKey.
func isNoSuchKey(err error) bool {
	apiErr, ok := errors.AsType[smithy.APIError](err)
	return ok && apiErr.ErrorCode() == "NoSuchKey"
}

// untagRejected removes the rejected tag from key, keeping any others.
// untagged is false when there was none. A file the lifecycle rule
// deleted since checkNotExpired is ErrImageGone.
func (o *Overrides) untagRejected(ctx context.Context, key string) (untagged bool, err error) {
	out, err := o.s3.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{Bucket: &o.bucket, Key: &key})
	if isNoSuchKey(err) {
		return false, fmt.Errorf("%w: %q", ErrImageGone, key)
	}
	if err != nil {
		return false, fmt.Errorf("reading %q's tags: %w", key, err)
	}
	kept := slices.DeleteFunc(slices.Clone(out.TagSet), func(t s3types.Tag) bool { return aws.ToString(t.Key) == rejectedTagKey })
	if len(kept) == len(out.TagSet) {
		return false, nil
	}
	if len(kept) == 0 {
		_, err = o.s3.DeleteObjectTagging(ctx, &s3.DeleteObjectTaggingInput{Bucket: &o.bucket, Key: &key})
	} else {
		_, err = o.s3.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{Bucket: &o.bucket, Key: &key, Tagging: &s3types.Tagging{TagSet: kept}})
	}
	if isNoSuchKey(err) {
		return false, fmt.Errorf("%w: %q", ErrImageGone, key)
	}
	if err != nil {
		return false, fmt.Errorf("removing the rejected tag from %q: %w", key, err)
	}
	return true, nil
}

// override moves the image at key from one status to the other. already
// reports an image already at to, so a rerun after a partial failure
// succeeds.
func (o *Overrides) override(ctx context.Context, ref imagekey.Ref, key string, from, to repository.ImageStatus, reason *string) (already bool, err error) {
	ok, err := o.images.SetStatus(ctx, ref, key, from, to, reason)
	if err != nil {
		return false, fmt.Errorf("writing the status of %q: %w", key, err)
	}
	if ok {
		return false, nil
	}

	status, err := o.currentStatus(ctx, ref, key, from, to)
	if err != nil {
		return false, err
	}
	if status != to {
		return false, fmt.Errorf("%q changed while it was being updated; run this again", key)
	}
	return true, nil
}

// currentStatus returns the status of the image at key, failing unless an
// item still holds it and it's at from or already at to.
func (o *Overrides) currentStatus(ctx context.Context, ref imagekey.Ref, key string, from, to repository.ImageStatus) (repository.ImageStatus, error) {
	status, current, err := o.images.Status(ctx, ref, key)
	switch {
	case err != nil:
		return "", fmt.Errorf("reading the status of %q: %w", key, err)
	case !current:
		return "", fmt.Errorf("%w: %q", ErrNotCurrent, key)
	case status != from && status != to:
		return "", fmt.Errorf("%q is %s, not %s", key, status, from)
	}
	return status, nil
}
