package accountdelete_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/accountdelete"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/mocks"
)

type DeleteSuite struct {
	suite.Suite

	images    *mocks.MockOwnerImageStore
	builds    *mocks.MockBuildRepository
	keyboards *mocks.MockKeyboardRepository
	switches  *mocks.MockSwitchRepository
	sets      *mocks.MockKeycapSetRepository
	profiles  *mocks.MockProfileRepository
	account   accountdelete.Account

	order []string
}

func TestDeleteSuite(t *testing.T) {
	suite.Run(t, new(DeleteSuite))
}

func (s *DeleteSuite) SetupTest() {
	s.images = mocks.NewMockOwnerImageStore(s.T())
	s.builds = mocks.NewMockBuildRepository(s.T())
	s.keyboards = mocks.NewMockKeyboardRepository(s.T())
	s.switches = mocks.NewMockSwitchRepository(s.T())
	s.sets = mocks.NewMockKeycapSetRepository(s.T())
	s.profiles = mocks.NewMockProfileRepository(s.T())
	s.account = accountdelete.Account{
		Images:     s.images,
		Builds:     s.builds,
		Keyboards:  s.keyboards,
		Switches:   s.switches,
		KeycapSets: s.sets,
		Profiles:   s.profiles,
	}
	s.order = nil
}

func (s *DeleteSuite) record(step string) func(context.Context) {
	return func(context.Context) { s.order = append(s.order, step) }
}

func (s *DeleteSuite) TestDeletesImagesFirstAndProfileLast() {
	s.images.EXPECT().DeleteAll(mock.Anything).Run(s.record("images")).Return(nil)
	s.builds.EXPECT().DeleteAll(mock.Anything).Run(s.record("builds")).Return(nil)
	s.keyboards.EXPECT().DeleteAll(mock.Anything).Run(s.record("keyboards")).Return(nil)
	s.switches.EXPECT().DeleteAll(mock.Anything).Run(s.record("switches")).Return(nil)
	s.sets.EXPECT().DeleteAll(mock.Anything).Run(s.record("keycap sets")).Return(nil)
	s.profiles.EXPECT().Delete(mock.Anything).Run(s.record("profile")).Return(nil)

	s.Require().NoError(s.account.Delete(s.T().Context()))

	s.Equal([]string{"images", "builds", "keyboards", "switches", "keycap sets", "profile"}, s.order)
}

func (s *DeleteSuite) TestImageDeleteFails_NoRecordDeleted() {
	s.images.EXPECT().DeleteAll(mock.Anything).Return(errors.New("s3 down"))

	s.Require().Error(s.account.Delete(s.T().Context()))
}

func (s *DeleteSuite) TestItemDeleteFails_ProfileKept() {
	s.images.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.builds.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.keyboards.EXPECT().DeleteAll(mock.Anything).Return(errors.New("dynamo down"))

	s.Require().Error(s.account.Delete(s.T().Context()))
}

func (s *DeleteSuite) TestProfileConflict_Matchable() {
	s.images.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.builds.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.keyboards.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.switches.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.sets.EXPECT().DeleteAll(mock.Anything).Return(nil)
	s.profiles.EXPECT().Delete(mock.Anything).Return(repository.ErrMutationConflict)

	s.ErrorIs(s.account.Delete(s.T().Context()), repository.ErrMutationConflict)
}
