package lookup

import (
	"context"
	"fmt"
	"slices"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

// ValidateKeyboard returns every field on kb that isn't an approved value
// for its lookup category. An unset field is skipped, not treated as
// invalid.
//
// An already-invalid size is never cross-checked against Layout's approved
// sizes: that check would always fail too, reporting a second, misleading
// error blaming a perfectly valid layout instead of the real problem (size
// itself).
func ValidateKeyboard(ctx context.Context, kb repository.Keyboard) []FieldError {
	var checks []fieldCheck
	add := func(field string, value *string, category Category) {
		if value == nil {
			return
		}
		checks = append(checks, fieldCheck{Field: field, Value: *value, Category: category})
	}

	add("size", kb.Size, CategoryKeyboardSize)
	add("design.top_case.material", kb.Design.TopCase.Material, CategoryKeyboardCaseMaterial)
	add("design.bottom_case.material", kb.Design.BottomCase.Material, CategoryKeyboardCaseMaterial)
	add("design.weight.material", kb.Design.Weight.Material, CategoryKeyboardWeightMaterial)
	add("purchase.vendor", kb.Purchase.Vendor, CategoryVendor)
	add("purchase.order_status", kb.Purchase.OrderStatus, CategoryOrderStatus)

	fieldErrs := validateFields(ctx, checks)

	if kb.Layout == nil {
		return fieldErrs
	}

	sizeInvalid := slices.ContainsFunc(fieldErrs, func(fe FieldError) bool { return fe.Field == "size" })
	size := kb.Size
	if sizeInvalid {
		size = nil
	}

	if layoutErr := validateKeyboardLayout(ctx, size, *kb.Layout); layoutErr != nil {
		fieldErrs = append(fieldErrs, *layoutErr)
	}

	return fieldErrs
}

// validateKeyboardLayout also cross-checks layout's sizes against
// CategoryKeyboardSize, so a keyboard can't pass its layout-vs-size check
// against a size that was never itself approved.
func validateKeyboardLayout(ctx context.Context, size *string, layout string) *FieldError {
	category := CategoryKeyboardLayout

	l, ok := GetCategory(ctx, category)
	if !ok {
		fe := notApproved("layout", layout, category)
		return &fe
	}

	values := l.LayoutValues()

	idx := slices.IndexFunc(values, func(v LayoutValue) bool { return v.Name == layout })
	if idx == -1 {
		fe := notApproved("layout", layout, category)
		return &fe
	}

	if size != nil && !slices.Contains(values[idx].Sizes, *size) {
		return &FieldError{Field: "layout", Value: layout, Reason: fmt.Sprintf("%q is not a valid layout for size %q", layout, *size)}
	}

	return nil
}

// ValidateKeyboardPlate returns every field on p that isn't an approved
// value for its lookup category. An unset field is skipped.
func ValidateKeyboardPlate(ctx context.Context, p repository.KeyboardPlate) []FieldError {
	checks := []fieldCheck{{Field: "material", Value: p.Material, Category: CategoryKeyboardPlateMaterial}}
	checks = appendPartPurchaseChecks(checks, p.Purchase)

	return validateFields(ctx, checks)
}

// ValidateKeyboardPCB returns every field on p that isn't an approved value
// for its lookup category. An unset field is skipped.
func ValidateKeyboardPCB(ctx context.Context, p repository.KeyboardPCB) []FieldError {
	var checks []fieldCheck
	for _, c := range []struct {
		field    string
		value    *string
		category Category
	}{
		{"firmware", p.Firmware, CategoryKeyboardPCBFirmware},
		{"assembly", p.Assembly, CategoryKeyboardPCBAssemblyType},
		{"connectivity", p.Connectivity, CategoryKeyboardPCBConnectivityType},
	} {
		if c.value != nil {
			checks = append(checks, fieldCheck{Field: c.field, Value: *c.value, Category: c.category})
		}
	}
	checks = appendPartPurchaseChecks(checks, p.Purchase)

	return validateFields(ctx, checks)
}

func appendPartPurchaseChecks(checks []fieldCheck, p repository.KeyboardPurchase) []fieldCheck {
	if p.Vendor != nil {
		checks = append(checks, fieldCheck{Field: "purchase.vendor", Value: *p.Vendor, Category: CategoryVendor})
	}
	if p.OrderStatus != nil {
		checks = append(checks, fieldCheck{Field: "purchase.order_status", Value: *p.OrderStatus, Category: CategoryOrderStatus})
	}
	return checks
}
