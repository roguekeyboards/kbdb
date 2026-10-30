package lookup

import (
	"context"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

// ValidateSwitch returns every field on sw that isn't an approved value for
// its lookup category. An unset field is skipped, not treated as invalid.
func ValidateSwitch(ctx context.Context, sw repository.Switch) []FieldError {
	var checks []fieldCheck
	add := func(field string, value *string, category Category) {
		if value == nil {
			return
		}
		checks = append(checks, fieldCheck{Field: field, Value: *value, Category: category})
	}

	add("type", &sw.Type, CategorySwitchType)
	add("material.top_housing", sw.Material.TopHousing, CategorySwitchMaterial)
	add("material.bottom_housing", sw.Material.BottomHousing, CategorySwitchMaterial)
	add("material.stem", sw.Material.Stem, CategorySwitchMaterial)
	add("spring.material", sw.Spring.Material, CategorySwitchSpringMaterial)
	add("purchase.vendor", sw.Purchase.Vendor, CategoryVendor)
	add("purchase.order_status", sw.Purchase.OrderStatus, CategoryOrderStatus)

	return validateFields(ctx, checks)
}
