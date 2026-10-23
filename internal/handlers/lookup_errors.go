package handlers

import (
	"net/http"

	"github.com/rogueserenity/kbdb/internal/lookup"
	"github.com/rogueserenity/kbdb/internal/problem"
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
