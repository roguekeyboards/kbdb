package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

type NewProfileImageKeySuite struct {
	suite.Suite
}

func TestNewProfileImageKeySuite(t *testing.T) {
	suite.Run(t, new(NewProfileImageKeySuite))
}

func (s *NewProfileImageKeySuite) TestSucceeds() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")

	key, err := repository.NewProfileImageKey(ctx, "up1")

	s.Require().NoError(err)
	s.Equal(repository.ProfileImageKey("profiles/user-alice/avatar/up1"), key)
}

func (s *NewProfileImageKeySuite) TestNoUserIDInContext_ReturnsError() {
	key, err := repository.NewProfileImageKey(context.Background(), "up1")

	s.Require().ErrorIs(err, repository.ErrNoUserID)
	s.Empty(key)
}

type ProfilePreferencesSuite struct {
	suite.Suite
}

func TestProfilePreferencesSuite(t *testing.T) {
	suite.Run(t, new(ProfilePreferencesSuite))
}

func (s *ProfilePreferencesSuite) TestShowPriceSingle_Owner_AlwaysTrue() {
	s.True(repository.ProfilePreferences{ShowPriceToOthers: false}.ShowPriceSingle(true))
	s.True(repository.ProfilePreferences{ShowPriceToOthers: true}.ShowPriceSingle(true))
}

func (s *ProfilePreferencesSuite) TestShowPriceSingle_NonOwner_FollowsShowPriceToOthers() {
	s.False(repository.ProfilePreferences{ShowPriceToOthers: false}.ShowPriceSingle(false))
	s.True(repository.ProfilePreferences{ShowPriceToOthers: true}.ShowPriceSingle(false))
}

func (s *ProfilePreferencesSuite) TestShowPriceSummary_Owner_FollowsShowPriceToMe() {
	s.True(repository.ProfilePreferences{ShowPriceToMe: true}.ShowPriceSummary(true))
	s.False(repository.ProfilePreferences{ShowPriceToMe: false}.ShowPriceSummary(true))
}

func (s *ProfilePreferencesSuite) TestShowPriceSummary_NonOwner_FollowsShowPriceToOthers() {
	s.True(repository.ProfilePreferences{ShowPriceToOthers: true}.ShowPriceSummary(false))
	s.False(repository.ProfilePreferences{ShowPriceToOthers: false}.ShowPriceSummary(false))
}

func (s *ProfilePreferencesSuite) TestCurrencyFor_PriceSet_ReturnsCurrency() {
	price := 120.0

	got := repository.ProfilePreferences{Currency: "EUR"}.CurrencyFor(&price)

	s.Require().NotNil(got)
	s.Equal("EUR", *got)
}

func (s *ProfilePreferencesSuite) TestCurrencyFor_NoPrice_ReturnsNil() {
	s.Nil(repository.ProfilePreferences{Currency: "EUR"}.CurrencyFor(nil))
}
