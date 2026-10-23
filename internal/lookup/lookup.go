package lookup

import (
	"context"
	"fmt"
	"slices"
)

// fieldCheck is one entity field to validate against a lookup category.
type fieldCheck struct {
	Field    string
	Value    string
	Category Category
}

// FieldError reports that Field's Value was rejected, and why.
type FieldError struct {
	Field  string
	Value  string
	Reason string
}

func notApproved(field, value string, category Category) FieldError {
	return FieldError{Field: field, Value: value, Reason: fmt.Sprintf("%q is not an approved %s value", value, category)}
}

// ValidateImageContentType returns a FieldError if contentType isn't an
// approved CategoryImageContentType value.
func ValidateImageContentType(ctx context.Context, contentType string) []FieldError {
	return validateFields(ctx, []fieldCheck{
		{Field: "content_type", Value: contentType, Category: CategoryImageContentType},
	})
}

// validateFields fetches each distinct category at most once and returns
// every invalid field, not just the first.
func validateFields(ctx context.Context, checks []fieldCheck) []FieldError {
	values := make(map[Category][]string, len(checks))

	for _, c := range checks {
		if _, ok := values[c.Category]; ok {
			continue
		}

		l, ok := GetCategory(ctx, c.Category)
		if !ok {
			values[c.Category] = nil
			continue
		}

		values[c.Category] = l.Strings()
	}

	var errs []FieldError
	for _, c := range checks {
		if !slices.Contains(values[c.Category], c.Value) {
			errs = append(errs, notApproved(c.Field, c.Value, c.Category))
		}
	}

	return errs
}
