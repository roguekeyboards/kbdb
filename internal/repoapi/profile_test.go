package repoapi

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/handlers/api"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/mocks"
)

type ProfileMapperSuite struct {
	suite.Suite
}

func TestProfileMapperSuite(t *testing.T) {
	suite.Run(t, new(ProfileMapperSuite))
}

func profileImageKeyPtr(s string) *repository.ProfileImageKey {
	k := repository.ProfileImageKey(s)
	return &k
}

func (s *ProfileMapperSuite) TestProfileToAPI_FullProfile_NoAvatar() {
	p := repository.Profile{
		OwnerID:         "user-alice",
		Username:        "alice",
		Discoverable:    true,
		DiscordUsername: strPtr("alice_kb"),
		Bio:             strPtr("keebs"),
		Links: []repository.ProfileLink{
			{Name: "Twitch", URL: "https://twitch.tv/alice"},
			{Name: "Insta", URL: "https://instagram.com/alice"},
		},
	}

	pr := Profile{Images: mocks.NewMockProfileImageStore(s.T())}
	out, err := pr.ToAPI(s.T().Context(), p, true)

	s.Require().NoError(err)
	s.Equal("alice", out.Username)
	s.Equal("user-alice", out.UserId)
	s.True(out.Discoverable)
	s.Require().NotNil(out.DiscordUsername)
	s.Equal("alice_kb", *out.DiscordUsername)
	s.Require().NotNil(out.Bio)
	s.Equal("keebs", *out.Bio)
	s.Require().NotNil(out.Links)
	s.Len(*out.Links, 2)
	s.Equal(api.ProfileLink{Name: "Twitch", Url: "https://twitch.tv/alice"}, (*out.Links)[0])
	s.Nil(out.Avatar)
}

func (s *ProfileMapperSuite) TestProfileToAPI_ExposesSubjectAsUserID() {
	pr := Profile{Images: mocks.NewMockProfileImageStore(s.T())}
	out, err := pr.ToAPI(s.T().Context(), repository.Profile{
		OwnerID:  "user-alice",
		Username: "alice",
	}, true)

	s.Require().NoError(err)
	s.Equal("alice", out.Username)
	s.Equal("user-alice", out.UserId)
}

func (s *ProfileMapperSuite) TestProfileToAPI_SignsAvatar() {
	images := mocks.NewMockProfileImageStore(s.T())
	images.EXPECT().
		SignGet(mock.Anything, repository.ProfileImageKey("profiles/user-alice/avatar/up1")).
		Return("https://example.com/avatar", signedExpiry(), nil)
	repo := mocks.NewMockProfileRepository(s.T())
	repo.EXPECT().
		SetImageGetCache(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).Maybe()

	p := repository.Profile{
		Username:   "alice",
		AvatarPath: profileImageKeyPtr("profiles/user-alice/avatar/up1"),
	}

	pr := Profile{Images: images, Repo: repo}
	out, err := pr.ToAPI(s.T().Context(), p, true)

	s.Require().NoError(err)
	s.Require().NotNil(out.Avatar)
	s.Equal("https://example.com/avatar", *out.Avatar.Url)
}

func (s *ProfileMapperSuite) TestProfileToAPI_SignError_Propagates() {
	images := mocks.NewMockProfileImageStore(s.T())
	images.EXPECT().SignGet(mock.Anything, mock.Anything).Return("", time.Time{}, errors.New("s3 down"))

	pr := Profile{Images: images}
	_, err := pr.ToAPI(s.T().Context(), repository.Profile{
		Username:   "alice",
		AvatarPath: profileImageKeyPtr("profiles/user-alice/avatar/up1"),
	}, true)

	s.Require().Error(err)
}

func (s *ProfileMapperSuite) TestProfileToAPI_EmptyLinks_OmittedNotEmptySlice() {
	pr := Profile{Images: mocks.NewMockProfileImageStore(s.T())}
	out, err := pr.ToAPI(s.T().Context(), repository.Profile{Username: "alice"}, true)

	s.Require().NoError(err)
	s.Nil(out.Links)
}

func (s *ProfileMapperSuite) TestProfileToRepo_MapsBodyFields_NotAvatarOrDerived() {
	in := api.ProfileInput{
		Username:        "alice",
		Discoverable:    boolPtr(true),
		DiscordUsername: strPtr("alice_kb"),
		Bio:             strPtr("keebs"),
		Links:           &[]api.ProfileLink{{Name: "Twitch", Url: "https://twitch.tv/alice"}},
	}

	p := Profile{}.ToRepo(in)

	s.Equal("alice", p.Username)
	s.True(p.Discoverable)
	s.Require().NotNil(p.DiscordUsername)
	s.Equal("alice_kb", *p.DiscordUsername)
	s.Require().Len(p.Links, 1)
	s.Equal(repository.ProfileLink{Name: "Twitch", URL: "https://twitch.tv/alice"}, p.Links[0])
	s.Empty(p.OwnerID)
	s.Nil(p.AvatarPath)
	s.Nil(p.DiscoverablePK)
}

func (s *ProfileMapperSuite) TestProfileToRepo_DiscoverableOmitted_DefaultsFalse() {
	p := Profile{}.ToRepo(api.ProfileInput{Username: "alice"})

	s.False(p.Discoverable)
}

func (s *ProfileMapperSuite) TestProfileToRepo_MapsPreferences() {
	in := api.ProfileInput{
		Username: "alice",
		Preferences: &api.ProfilePreferences{
			Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true,
		},
	}

	p := Profile{}.ToRepo(in)

	s.Equal(repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true}, p.Preferences)
}

func (s *ProfileMapperSuite) TestProfileToRepo_PreferencesOmitted_DefaultsApplied() {
	p := Profile{}.ToRepo(api.ProfileInput{Username: "alice"})

	s.Equal(repository.DefaultProfilePreferences(), p.Preferences)
}

func (s *ProfileMapperSuite) TestProfileToAPI_MapsPreferences() {
	p := repository.Profile{
		Username:    "alice",
		Preferences: repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true},
	}

	pr := Profile{Images: mocks.NewMockProfileImageStore(s.T())}
	out, err := pr.ToAPI(s.T().Context(), p, true)

	s.Require().NoError(err)
	s.Require().NotNil(out.Preferences)
	s.Equal(api.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true}, *out.Preferences)
}

func (s *ProfileMapperSuite) TestProfileToAPI_NonOwner_OmitsPreferences() {
	p := repository.Profile{
		Username:    "alice",
		Preferences: repository.ProfilePreferences{Currency: "EUR", ShowPriceToOthers: true},
	}

	pr := Profile{Images: mocks.NewMockProfileImageStore(s.T())}
	out, err := pr.ToAPI(s.T().Context(), p, false)

	s.Require().NoError(err)
	s.Nil(out.Preferences)
}
