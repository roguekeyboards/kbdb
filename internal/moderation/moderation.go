// Package moderation is the moderation function's logic: for each image
// uploaded to the images bucket, it asks Rekognition for moderation
// labels and marks the image approved or rejected. functions/moderate
// only wires it to Lambda.
package moderation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// reasonUnprocessable is the rejection reason for an image that can never
// be moderated, as opposed to one a label rejected.
const reasonUnprocessable = "unprocessable"

// s3API is the subset of the S3 client the moderator and Overrides use.
type s3API interface {
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObjectTagging(ctx context.Context, in *s3.PutObjectTaggingInput, opts ...func(*s3.Options)) (*s3.PutObjectTaggingOutput, error)
	DeleteObjectTagging(ctx context.Context, in *s3.DeleteObjectTaggingInput, opts ...func(*s3.Options)) (*s3.DeleteObjectTaggingOutput, error)
}

// rekognitionAPI is the subset of the Rekognition client the moderator
// uses.
type rekognitionAPI interface {
	DetectModerationLabels(ctx context.Context, in *rekognition.DetectModerationLabelsInput, opts ...func(*rekognition.Options)) (*rekognition.DetectModerationLabelsOutput, error)
}

// Moderator handles the images bucket's ObjectCreated events.
type Moderator struct {
	s3          s3API
	rekognition rekognitionAPI
	images      *Images
	calibration bool
}

// New returns a Moderator. calibration lowers the confidence Rekognition
// reports labels at to 0, so every label is logged; decisions still use
// the per-category thresholds.
func New(s3Client *s3.Client, rekognitionClient *rekognition.Client, images *Images, calibration bool) *Moderator {
	return &Moderator{s3: s3Client, rekognition: rekognitionClient, images: images, calibration: calibration}
}

// Handle moderates every object in event. An error means a temporary
// failure, so Lambda retries the whole event; images already decided are
// skipped on the retry.
func (m *Moderator) Handle(ctx context.Context, event events.S3Event) error {
	for _, rec := range event.Records {
		if err := m.moderate(ctx, rec.S3.Bucket.Name, rec.S3.Object.URLDecodedKey); err != nil {
			return err
		}
	}
	return nil
}

func (m *Moderator) moderate(ctx context.Context, bucket, key string) error {
	log := slog.With("key", key)

	ref, err := imagekey.Parse(key)
	if err != nil {
		log.Warn("ignoring an object that isn't an image", "error", err)
		return nil
	}

	// Read first: a deleted, replaced, duplicate or already-decided image
	// isn't sent to Rekognition again.
	status, current, err := m.images.Status(ctx, ref, key)
	if err != nil {
		return fmt.Errorf("reading the status of %q: %w", key, err)
	}
	if !current || status != repository.ImageStatusPending {
		log.Info("skipping an image that isn't pending", "current", current, "status", status)
		return nil
	}

	to, reason := repository.ImageStatusApproved, (*string)(nil)
	out, err := m.detect(ctx, bucket, key)
	switch {
	case errors.Is(err, errUnprocessable):
		to, reason = repository.ImageStatusRejected, aws.String(reasonUnprocessable)
		log.Info("moderated image", "decision", to, "reason", *reason, "error", err.Error())
	case err != nil:
		return err
	default:
		d := decide(out.ModerationLabels)
		if d.Rejected {
			to, reason = repository.ImageStatusRejected, aws.String(d.Label)
		}
		log.Info("moderated image",
			"decision", to,
			"reason", aws.ToString(reason),
			"labels", loggedLabels(out.ModerationLabels),
			"model_version", aws.ToString(out.ModerationModelVersion),
			"content_types", loggedContentTypes(out.ContentTypes))
	}

	// Tagged before the status write, so a failed write's retry finds the
	// image still pending and tags it again rather than leaving it
	// untagged.
	if to == repository.ImageStatusRejected {
		if err := tagRejected(ctx, m.s3, bucket, key); err != nil {
			return err
		}
	}

	ok, err := m.images.SetStatus(ctx, ref, key, repository.ImageStatusPending, to, reason)
	if err != nil {
		return fmt.Errorf("writing the status of %q: %w", key, err)
	}
	if !ok {
		log.Info("dropping the decision: the image changed while it was moderated")
	}
	return nil
}

// detect sends the image to Rekognition. JPEG and PNG go by reference;
// WebP, which Rekognition can't read, is re-encoded and sent as bytes.
func (m *Moderator) detect(ctx context.Context, bucket, key string) (*rekognition.DetectModerationLabelsOutput, error) {
	head, err := m.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return nil, fmt.Errorf("reading %q's metadata: %w", key, err)
	}

	var img rektypes.Image
	switch ct := aws.ToString(head.ContentType); ct {
	case "image/jpeg", "image/png":
		img.S3Object = &rektypes.S3Object{Bucket: &bucket, Name: &key}
	case "image/webp":
		data, err := m.read(ctx, bucket, key)
		if err != nil {
			return nil, err
		}
		if img.Bytes, err = webpToJPEG(data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: content type %q", errUnprocessable, ct)
	}

	out, err := m.rekognition.DetectModerationLabels(ctx, &rekognition.DetectModerationLabelsInput{
		Image:         &img,
		MinConfidence: aws.Float32(minConfidence(m.calibration)),
	})
	if err != nil {
		if permanent(err) {
			return nil, fmt.Errorf("%w: %w", errUnprocessable, err)
		}
		return nil, fmt.Errorf("detecting moderation labels for %q: %w", key, err)
	}
	return out, nil
}

func (m *Moderator) read(ctx context.Context, bucket, key string) ([]byte, error) {
	obj, err := m.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", key, err)
	}
	defer func() { _ = obj.Body.Close() }()
	data, err := io.ReadAll(obj.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", key, err)
	}
	return data, nil
}

// permanent reports whether Rekognition refused the image itself, so a
// retry can't succeed.
func permanent(err error) bool {
	var format *rektypes.InvalidImageFormatException
	var tooLarge *rektypes.ImageTooLargeException
	var invalid *rektypes.InvalidParameterException
	return errors.As(err, &format) || errors.As(err, &tooLarge) || errors.As(err, &invalid)
}

// tagRejected marks the object for the bucket's lifecycle rule, which
// expires rejected images.
func tagRejected(ctx context.Context, client s3API, bucket, key string) error {
	_, err := client.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{
		Bucket: &bucket,
		Key:    &key,
		Tagging: &s3types.Tagging{TagSet: []s3types.Tag{
			{Key: aws.String("moderation"), Value: aws.String("rejected")},
		}},
	})
	if err != nil {
		return fmt.Errorf("tagging %q rejected: %w", key, err)
	}
	return nil
}

type loggedLabel struct {
	Name       string  `json:"name"`
	Parent     string  `json:"parent,omitempty"`
	Level      int32   `json:"level"`
	Confidence float32 `json:"confidence"`
}

func loggedLabels(labels []rektypes.ModerationLabel) []loggedLabel {
	out := make([]loggedLabel, len(labels))
	for i, l := range labels {
		out[i] = loggedLabel{
			Name:       aws.ToString(l.Name),
			Parent:     aws.ToString(l.ParentName),
			Level:      aws.ToInt32(l.TaxonomyLevel),
			Confidence: aws.ToFloat32(l.Confidence),
		}
	}
	return out
}

type loggedContentType struct {
	Name       string  `json:"name"`
	Confidence float32 `json:"confidence"`
}

func loggedContentTypes(types []rektypes.ContentType) []loggedContentType {
	out := make([]loggedContentType, len(types))
	for i, t := range types {
		out[i] = loggedContentType{Name: aws.ToString(t.Name), Confidence: aws.ToFloat32(t.Confidence)}
	}
	return out
}
