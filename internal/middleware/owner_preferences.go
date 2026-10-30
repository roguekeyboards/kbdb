package middleware

import (
	"net/http"

	"github.com/roguekeyboards/kbdb/internal/log"
	"github.com/roguekeyboards/kbdb/internal/ownerprefs"
	"github.com/roguekeyboards/kbdb/internal/problem"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

// OwnerPreferences puts the {userId} path segment's owner preferences on the
// request context, for [ownerprefs.Get]. It must wrap a route whose pattern
// includes {userId}.
func OwnerPreferences(reader repository.PreferencesReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ownerID := r.PathValue("userId")
			prefs, err := reader.GetPreferences(r.Context(), ownerID)
			if err != nil {
				log.FromContext(r.Context()).Error("getting owner preferences", log.Error, err, log.OwnerID, ownerID)
				problem.Internal(w, "failed to get owner preferences")
				return
			}

			next.ServeHTTP(w, r.WithContext(ownerprefs.WithPreferences(r.Context(), prefs)))
		})
	}
}
