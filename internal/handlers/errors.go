package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/roguekeyboards/kbdb/internal/log"
	"github.com/roguekeyboards/kbdb/internal/problem"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/uploadlimit"
)

// handleMutationError is the standard error tail for a mutating repository
// call: repository.ErrNotFound -> 404, repository.ErrImageLimitReached ->
// 409, any other non-nil error -> 500 (logged as an error). logFields are passed through to the log call for correlation
// (e.g. log.SwitchID, id). Returns true if err was non-nil and a response
// was written - callers should return immediately when true.
func handleMutationError(w http.ResponseWriter, r *http.Request, err error, logFields ...any) bool {
	if errors.Is(err, repository.ErrNotFound) {
		problem.NotFound(w, "resource not found")
		return true
	}
	if errors.Is(err, repository.ErrImageLimitReached) {
		problem.Conflict(w, fmt.Sprintf("the item already has %d images, the maximum; delete one first", repository.MaxImagesPerItem))
		return true
	}
	if err != nil {
		log.FromContext(r.Context()).Error("mutation failed", append([]any{log.Error, err}, logFields...)...)
		problem.Internal(w, "an internal error occurred")
		return true
	}
	return false
}

// handleTransactionError is handleMutationError for the repository calls
// that write a transaction and can return repository.ErrMutationConflict,
// which it maps to 409 (logged as a warning).
func handleTransactionError(w http.ResponseWriter, r *http.Request, err error, logFields ...any) bool {
	if errors.Is(err, repository.ErrMutationConflict) {
		log.FromContext(r.Context()).Warn("mutation conflict", logFields...)
		problem.Conflict(w, "the resource is being modified concurrently, please retry")
		return true
	}
	return handleMutationError(w, r, err, logFields...)
}

// handleClearImageError is handleMutationError's counterpart for the
// image-pointer clear that follows a successful S3 object delete in a
// single-image delete handler. The S3 object is already gone by this
// point, so repository.ErrNotFound (the entity was deleted concurrently)
// is the documented idempotent-success state, not a 404: it's swallowed
// rather than routed through handleMutationError. Other errors still go
// through handleMutationError unchanged. Returns
// true if a response was written - callers should return immediately when
// true.
func handleClearImageError(w http.ResponseWriter, r *http.Request, err error, logFields ...any) bool {
	if errors.Is(err, repository.ErrNotFound) {
		return false
	}
	return handleMutationError(w, r, err, logFields...)
}

// reserveUpload counts one upload against the daily limit before an upload
// URL is presigned: repository.ErrUploadLimitReached -> 429, any other
// error -> 500. Returns true if a response was written - callers should
// return immediately when true.
func reserveUpload(w http.ResponseWriter, r *http.Request, uploads *uploadlimit.Limiter, logFields ...any) bool {
	err := uploads.Reserve(r.Context())
	if errors.Is(err, repository.ErrUploadLimitReached) {
		log.FromContext(r.Context()).Warn("daily upload limit reached", logFields...)
		problem.TooManyRequests(w, uploadsUnavailable)
		return true
	}
	if err != nil {
		log.FromContext(r.Context()).Error("reserving upload", append([]any{log.Error, err}, logFields...)...)
		problem.Internal(w, "an internal error occurred")
		return true
	}
	return false
}

const uploadsUnavailable = "image uploads aren't available at the moment"
