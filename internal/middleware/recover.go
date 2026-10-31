package middleware

import (
	"errors"
	"net/http"
	"runtime/debug"

	"github.com/roguekeyboards/mykeebs-api/internal/log"
	"github.com/roguekeyboards/mykeebs-api/internal/problem"
)

// Recover must stay wrapped inside Logging (see router.go), not the other
// way around, or a recovered panic will skip the request-finished log.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}

			log.FromContext(r.Context()).Error("panic recovered", "panic", rec, "stack", string(debug.Stack()))

			// A response already in flight can't be replaced - writing
			// problem.Internal here would append a second, malformed body
			// after whatever was already sent.
			if !headerWritten(w) {
				problem.Internal(w, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
