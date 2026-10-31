package mcp

import (
	"errors"
	"strings"

	"github.com/roguekeyboards/mykeebs-api/internal/lookup"
)

// lookupErrors joins fieldErrs into one tool error, or returns nil.
func lookupErrors(fieldErrs []lookup.FieldError) error {
	if len(fieldErrs) == 0 {
		return nil
	}

	reasons := make([]string, len(fieldErrs))
	for i, fe := range fieldErrs {
		reasons[i] = fe.Field + ": " + fe.Reason
	}
	return errors.New(strings.Join(reasons, "; "))
}
