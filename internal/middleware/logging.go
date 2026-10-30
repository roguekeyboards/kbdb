package middleware

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	logpkg "github.com/roguekeyboards/kbdb/internal/log"
)

// Logging generates a request ID, builds a request-scoped slog.Logger, and
// logs once at request start and once at request end (status/duration).
// Per-internal-call logging is deliberately not done here to avoid noise.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		l := logpkg.WithRequestID(logpkg.New(), requestID)
		ctx := logpkg.WithLogger(r.Context(), l)
		r = r.WithContext(ctx)

		l.Info("request started", "method", r.Method, "path", r.URL.Path)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		l.Info("request finished",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.written {
		return
	}
	r.written = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// headerWritten reports whether w has already sent a response header -
// Recover uses this to avoid writing a second, corrupt response body after
// a panic that occurs partway through an already-started response. Only
// meaningful when w is the *statusRecorder Logging installs; any other
// http.ResponseWriter (e.g. in a unit test) reports false, matching
// net/http's own default assumption that nothing has been written yet.
func headerWritten(w http.ResponseWriter) bool {
	rec, ok := w.(*statusRecorder)
	return ok && rec.written
}
