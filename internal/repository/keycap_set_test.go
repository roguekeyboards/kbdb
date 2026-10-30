package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

type NewKeycapKitImageKeySuite struct {
	suite.Suite
}

func TestNewKeycapKitImageKeySuite(t *testing.T) {
	suite.Run(t, new(NewKeycapKitImageKeySuite))
}

func (s *NewKeycapKitImageKeySuite) TestSucceeds() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")

	key, err := repository.NewKeycapKitImageKey(ctx, "ks1", "kit1", "up1")

	s.Require().NoError(err)
	s.Equal(repository.KeycapKitImageKey("keycap-sets/alice/ks1/kits/kit1/image/up1"), key)
}

func (s *NewKeycapKitImageKeySuite) TestNoUserIDInContext_ReturnsError() {
	key, err := repository.NewKeycapKitImageKey(context.Background(), "ks1", "kit1", "up1")

	s.Require().ErrorIs(err, repository.ErrNoUserID)
	s.Empty(key)
}

type AggregateOrderStatusSuite struct {
	suite.Suite
}

func TestAggregateOrderStatusSuite(t *testing.T) {
	suite.Run(t, new(AggregateOrderStatusSuite))
}

func kitWithStatus(status string) repository.KeycapKit {
	s := status
	return repository.KeycapKit{Purchase: repository.KeycapKitPurchase{OrderStatus: &s}}
}

func (s *AggregateOrderStatusSuite) TestNoKits_ReturnsNil() {
	s.Nil(repository.AggregateOrderStatus(nil))
}

func (s *AggregateOrderStatusSuite) TestNoKitHasStatusSet_ReturnsNil() {
	kits := map[string]repository.KeycapKit{"a": {}, "b": {}}

	s.Nil(repository.AggregateOrderStatus(kits))
}

func (s *AggregateOrderStatusSuite) TestAllKitsSameStatus_ReturnsThatStatus() {
	kits := map[string]repository.KeycapKit{"a": kitWithStatus("Shipped"), "b": kitWithStatus("Shipped")}

	got := repository.AggregateOrderStatus(kits)

	s.Require().NotNil(got)
	s.Equal("Shipped", *got)
}

func (s *AggregateOrderStatusSuite) TestMixedStatuses_ReturnsLeastProgressed() {
	kits := map[string]repository.KeycapKit{"a": kitWithStatus("Delivered"), "b": kitWithStatus("Ordered")}

	got := repository.AggregateOrderStatus(kits)

	s.Require().NotNil(got)
	s.Equal("Ordered", *got)
}

func (s *AggregateOrderStatusSuite) TestCancelledKitAmongOthers_IsExcludedFromComparison() {
	kits := map[string]repository.KeycapKit{"a": kitWithStatus("Delivered"), "b": kitWithStatus("Cancelled")}

	got := repository.AggregateOrderStatus(kits)

	s.Require().NotNil(got)
	s.Equal("Delivered", *got)
}

func (s *AggregateOrderStatusSuite) TestAllKitsCancelled_ReturnsCancelled() {
	kits := map[string]repository.KeycapKit{"a": kitWithStatus("Cancelled"), "b": kitWithStatus("Cancelled")}

	got := repository.AggregateOrderStatus(kits)

	s.Require().NotNil(got)
	s.Equal("Cancelled", *got)
}

func (s *AggregateOrderStatusSuite) TestUnrecognizedStatus_ReturnsNil() {
	kits := map[string]repository.KeycapKit{"a": kitWithStatus("SomethingUnexpected")}

	s.Nil(repository.AggregateOrderStatus(kits))
}

type KeycapSetTotalCostSuite struct {
	suite.Suite
}

func TestKeycapSetTotalCostSuite(t *testing.T) {
	suite.Run(t, new(KeycapSetTotalCostSuite))
}

func pricedKit(price *float64) repository.KeycapKit {
	return repository.KeycapKit{Purchase: repository.KeycapKitPurchase{Price: price}}
}

func (s *KeycapSetTotalCostSuite) TestSumsKnownPricesSkippingUnpriced() {
	set := repository.KeycapSet{Kits: map[string]repository.KeycapKit{
		"a": pricedKit(new(120.0)),
		"b": pricedKit(new(35.5)),
		"c": pricedKit(nil),
	}}

	s.Equal(new(155.5), set.TotalCost())
}

func (s *KeycapSetTotalCostSuite) TestRoundsToCents() {
	set := repository.KeycapSet{Kits: map[string]repository.KeycapKit{
		"a": pricedKit(new(0.1)),
		"b": pricedKit(new(0.2)),
		"c": pricedKit(new(0.3)),
	}}

	s.Equal(new(0.6), set.TotalCost())
}

func (s *KeycapSetTotalCostSuite) TestNoPricedKits_Nil() {
	set := repository.KeycapSet{Kits: map[string]repository.KeycapKit{"a": pricedKit(nil)}}

	s.Nil(set.TotalCost())
	s.Nil(repository.KeycapSet{}.TotalCost())
}
