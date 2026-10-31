package handlers

import (
	"net/http"

	"github.com/roguekeyboards/mykeebs-api/internal/lookup"
	"github.com/roguekeyboards/mykeebs-api/internal/problem"
)

// writeLookupErrors writes a 400 listing fieldErrs, if any.
func writeLookupErrors(w http.ResponseWriter, fieldErrs []lookup.FieldError) (ok bool) {
	if len(fieldErrs) == 0 {
		return true
	}

	invalidParams := make([]problem.InvalidParam, len(fieldErrs))
	for i, fe := range fieldErrs {
		invalidParams[i] = problem.InvalidParam{Name: fe.Field, Reason: fe.Reason}
	}
	problem.ValidationFailed(w, "one or more fields are not approved lookup values", invalidParams)
	return false
}
