package repository

import (
	"context"
	"errors"
	"time"
)

// ErrUploadLimitReached is returned by [UploadCounter.Increment] when the
// day's count has already reached its limit.
var ErrUploadLimitReached = errors.New("daily upload limit reached")

// UploadCounter counts upload URLs issued per day, across every user.
type UploadCounter interface {
	// Increment adds one to day's count unless it has already reached
	// limit, in which case it returns [ErrUploadLimitReached] and changes
	// nothing. expiresAt is when day's count may be deleted.
	Increment(ctx context.Context, day string, limit int64, expiresAt time.Time) error
}
