package lookup_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/lookup"
	"github.com/rogueserenity/kbdb/internal/repository"
)

type ValidateKeycapSetSuite struct {
	suite.Suite
}

func TestValidateKeycapSetSuite(t *testing.T) {
	suite.Run(t, new(ValidateKeycapSetSuite))
}

func (s *ValidateKeycapSetSuite) TestAllFieldsUnset_SkipsValidation() {
	ks := repository.KeycapSet{}

	errs := lookup.ValidateKeycapSet(s.T().Context(), ks)
	s.Empty(errs)
}

func (s *ValidateKeycapSetSuite) TestInvalidProfile_ReturnsFieldError() {
	profile := "NotAProfile"
	ks := repository.KeycapSet{Profile: &profile}

	errs := lookup.ValidateKeycapSet(s.T().Context(), ks)
	s.Equal([]lookup.FieldError{
		{Field: "profile", Value: "NotAProfile", Reason: `"NotAProfile" is not an approved keycap_profile value`},
	}, errs)
}

type ValidateKeycapKitSuite struct {
	suite.Suite
}

func TestValidateKeycapKitSuite(t *testing.T) {
	suite.Run(t, new(ValidateKeycapKitSuite))
}

func (s *ValidateKeycapKitSuite) TestAllFieldsUnset_SkipsValidation() {
	k := repository.KeycapKit{}

	errs := lookup.ValidateKeycapKit(s.T().Context(), k)
	s.Empty(errs)
}

func (s *ValidateKeycapKitSuite) TestValidFields_ReturnsNoErrors() {
	vendor := "Amazon"
	status := "Delivered"
	k := repository.KeycapKit{Purchase: repository.KeycapKitPurchase{Vendor: &vendor, OrderStatus: &status}}

	errs := lookup.ValidateKeycapKit(s.T().Context(), k)
	s.Empty(errs)
}

func (s *ValidateKeycapKitSuite) TestInvalidVendor_ReturnsFieldError() {
	vendor := "NotARealVendor"
	k := repository.KeycapKit{Purchase: repository.KeycapKitPurchase{Vendor: &vendor}}

	errs := lookup.ValidateKeycapKit(s.T().Context(), k)
	s.Equal([]lookup.FieldError{
		{Field: "purchase.vendor", Value: "NotARealVendor", Reason: `"NotARealVendor" is not an approved vendor value`},
	}, errs)
}

func (s *ValidateKeycapKitSuite) TestInvalidOrderStatus_ReturnsFieldError() {
	status := "Bogus"
	k := repository.KeycapKit{Purchase: repository.KeycapKitPurchase{OrderStatus: &status}}

	errs := lookup.ValidateKeycapKit(s.T().Context(), k)
	s.Equal([]lookup.FieldError{
		{Field: "purchase.order_status", Value: "Bogus", Reason: `"Bogus" is not an approved order_status value`},
	}, errs)
}
