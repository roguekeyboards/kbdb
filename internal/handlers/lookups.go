package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/roguekeyboards/kbdb/internal/lookup"
	"github.com/roguekeyboards/kbdb/internal/problem"
	"github.com/roguekeyboards/kbdb/internal/repoapi"
)

// ListLookups reads no path values and requires no auth. Returns category
// names only, not their values (see [GetLookup] for that).
func ListLookups(w http.ResponseWriter, r *http.Request) {
	names := lookup.ListCategoryNames(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(names)
}

// GetLookup reads the {category} path value and requires no auth.
func GetLookup(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")

	l, ok := lookup.GetCategory(r.Context(), lookup.Category(category))
	if !ok {
		problem.NotFound(w, "resource not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(repoapi.Lookup{}.ToAPI(l))
}
