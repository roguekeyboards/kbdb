package lookup

import (
	"context"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// ValidateKeycapSet returns every field on ks that isn't an approved value
// for its lookup category. An unset field is skipped, not treated as
// invalid.
func ValidateKeycapSet(ctx context.Context, ks repository.KeycapSet) []FieldError {
	var checks []fieldCheck
	add := func(field string, value *string, category Category) {
		if value == nil {
			return
		}
		checks = append(checks, fieldCheck{Field: field, Value: *value, Category: category})
	}

	add("profile", ks.Profile, CategoryKeycapProfile)
	add("material", ks.Material, CategoryKeycapMaterial)

	return validateFields(ctx, checks)
}

// ValidateKeycapKit returns every field on k that isn't an approved value
// for its lookup category. An unset field is skipped, not treated as
// invalid. Mirrors [ValidateSwitch]/[ValidateKeyboard]'s purchase.vendor and
// purchase.order_status checks - a kit's purchase is structurally the same
// shape as a keyboard's or switch's.
func ValidateKeycapKit(ctx context.Context, k repository.KeycapKit) []FieldError {
	var checks []fieldCheck
	add := func(field string, value *string, category Category) {
		if value == nil {
			return
		}
		checks = append(checks, fieldCheck{Field: field, Value: *value, Category: category})
	}

	add("purchase.vendor", k.Purchase.Vendor, CategoryVendor)
	add("purchase.order_status", k.Purchase.OrderStatus, CategoryOrderStatus)

	return validateFields(ctx, checks)
}
