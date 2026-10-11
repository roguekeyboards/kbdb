// Package uploadlimit caps how many image upload URLs are issued per UTC
// day across every user, shared by internal/handlers and internal/mcp.
// Each issued URL can trigger one paid moderation call, so the cap bounds
// that cost.
package uploadlimit

import (
	"context"
	"time"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// Limiter reserves one upload against the day's limit.
type Limiter struct {
	counter repository.UploadCounter
	limit   int64
	now     func() time.Time
}

// New returns a Limiter allowing limit uploads per UTC day.
func New(counter repository.UploadCounter, limit int64) *Limiter {
	return &Limiter{counter: counter, limit: limit, now: time.Now}
}

// Reserve counts one upload against today. It returns
// [repository.ErrUploadLimitReached] once today's limit has been reached;
// call it before presigning, so a refused request issues no URL.
func (l *Limiter) Reserve(ctx context.Context) error {
	day := l.now().UTC().Truncate(24 * time.Hour)
	return l.counter.Increment(ctx, day.Format(time.DateOnly), l.limit, day.Add(48*time.Hour))
}
