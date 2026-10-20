package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3API interface {
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type s3PresignAPI interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

type credentialsProvider interface {
	Retrieve(ctx context.Context) (aws.Credentials, error)
}

const (
	// Absorbs clock skew against S3 and refresh in flight.
	presignMargin = 60 * time.Second

	// SigV4 allows 7 days; shorter so a leaked URL dies within a day.
	staticCredentialsTTL = 24 * time.Hour
)

// presignGet signs a GET for key and reports when that URL stops working.
//
// A presigned URL cannot outlive the credentials that signed it - S3 answers
// ExpiredToken once the embedded session token lapses, whatever X-Amz-Expires
// claims - so the lifetime comes from the credentials rather than config, and
// those exact credentials are pinned for the signature. Without pinning the
// SDK resolves them again internally and may sign with a refreshed set,
// outliving the expiry reported here.
func presignGet(
	ctx context.Context,
	provider credentialsProvider,
	presign s3PresignAPI,
	bucket, key string,
) (string, time.Time, error) {
	creds, err := provider.Retrieve(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("retrieving credentials to presign GET s3://%s/%s: %w", bucket, key, err)
	}

	ttl := staticCredentialsTTL
	window := ttl
	if creds.CanExpire {
		window = time.Until(creds.Expires)
		if window <= 0 {
			return "", time.Time{}, fmt.Errorf(
				"cannot presign GET s3://%s/%s: signing credentials expired %s ago",
				bucket, key, time.Since(creds.Expires).Round(time.Second))
		}

		// Credentials inside the margin haven't expired, so still sign - half
		// the window rather than a refusal, which also keeps the TTL rising
		// with the window instead of collapsing either side of the margin.
		ttl = max(window-presignMargin, window/2)
	}

	// Whole seconds, matching the X-Amz-Expires the signature carries, and
	// never rounded past the window - the URL must not outlive the
	// credentials behind it. Sub-second windows floor to a second, which is
	// the shortest URL S3 will accept; they're already too short to use.
	if ttl = min(ttl.Round(time.Second), window); ttl < time.Second {
		ttl = time.Second
	}

	expiresAt := time.Now().Add(ttl)

	req, err := presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
		o.ClientOptions = append(o.ClientOptions, func(co *s3.Options) {
			co.Credentials = credentials.StaticCredentialsProvider{Value: creds}
		})
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presigning GET s3://%s/%s: %w", bucket, key, err)
	}

	return req.URL, expiresAt, nil
}
