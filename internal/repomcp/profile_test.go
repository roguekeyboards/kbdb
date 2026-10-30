package repomcp

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/mcp/schema"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

type ProfileMapperSuite struct {
	suite.Suite
}

func TestProfileMapperSuite(t *testing.T) {
	suite.Run(t, new(ProfileMapperSuite))
}

func (s *ProfileMapperSuite) TestProfileToMCP_FullProfile() {
	discord := "alice_kb"
	bio := "keebs"
	p := repository.Profile{
		OwnerID:         "user-alice",
		Username:        "alice",
		Discoverable:    true,
		DiscordUsername: &discord,
		Bio:             &bio,
		Links: []repository.ProfileLink{
			{Name: "Twitch", URL: "https://twitch.tv/alice"},
		},
	}

	out := Profile{}.ToMCP(p, true)

	s.Equal("alice", out.Username)
	s.Equal("user-alice", out.UserID)
	s.True(out.Discoverable)
	s.Require().NotNil(out.DiscordUsername)
	s.Equal("alice_kb", *out.DiscordUsername)
	s.Require().NotNil(out.Bio)
	s.Equal("keebs", *out.Bio)
	s.Require().Len(out.Links, 1)
	s.Equal("Twitch", out.Links[0].Name)
	s.Equal("https://twitch.tv/alice", out.Links[0].URL)
	s.False(out.HasAvatar)
}

func (s *ProfileMapperSuite) TestProfileToMCP_AvatarReportedAsBool() {
	key := repository.ProfileImageKey("profiles/user-alice/avatar/up1")
	out := Profile{}.ToMCP(repository.Profile{Username: "alice", AvatarPath: &key}, true)

	s.True(out.HasAvatar)
}

func (s *ProfileMapperSuite) TestProfileToMCP_EmptyLinks_Nil() {
	out := Profile{}.ToMCP(repository.Profile{Username: "alice"}, true)

	s.Nil(out.Links)
}

func (s *ProfileMapperSuite) TestProfileFromMCP_MapsWritableFields() {
	discord := "alice_kb"
	bio := "keebs"
	p := Profile{}.FromMCP(schema.ProfileInput{
		Username:        "alice",
		Discoverable:    true,
		DiscordUsername: &discord,
		Bio:             &bio,
		Links: []schema.ProfileLink{
			{Name: "Twitch", URL: "https://twitch.tv/alice"},
		},
	})

	s.Equal("alice", p.Username)
	s.True(p.Discoverable)
	s.Require().NotNil(p.DiscordUsername)
	s.Equal("alice_kb", *p.DiscordUsername)
	s.Require().NotNil(p.Bio)
	s.Equal("keebs", *p.Bio)
	s.Require().Len(p.Links, 1)
	s.Equal("Twitch", p.Links[0].Name)
	s.Equal("https://twitch.tv/alice", p.Links[0].URL)
}

func (s *ProfileMapperSuite) TestProfileFromMCP_ServerOwnedFieldsUnset() {
	p := Profile{}.FromMCP(schema.ProfileInput{Username: "alice"})

	s.Empty(p.OwnerID)
	s.Nil(p.AvatarPath)
	s.Nil(p.DiscoverablePK)
	s.Nil(p.Links)
}

func (s *ProfileMapperSuite) TestProfileFromMCP_MapsPreferences() {
	p := Profile{}.FromMCP(schema.ProfileInput{
		Username:    "alice",
		Preferences: &schema.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true},
	})

	s.Equal(repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true}, p.Preferences)
}

func (s *ProfileMapperSuite) TestProfileFromMCP_PreferencesOmitted_DefaultsApplied() {
	p := Profile{}.FromMCP(schema.ProfileInput{Username: "alice"})

	s.Equal(repository.DefaultProfilePreferences(), p.Preferences)
}

func (s *ProfileMapperSuite) TestProfileToMCP_MapsPreferences() {
	p := repository.Profile{
		Username:    "alice",
		Preferences: repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true},
	}

	out := Profile{}.ToMCP(p, true)

	s.Require().NotNil(out.Preferences)
	s.Equal(schema.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true}, *out.Preferences)
}

func (s *ProfileMapperSuite) TestProfileToMCP_NonOwner_OmitsPreferences() {
	out := Profile{}.ToMCP(repository.Profile{
		Username:    "alice",
		Preferences: repository.ProfilePreferences{Currency: "EUR", ShowPriceToOthers: true},
	}, false)

	s.Nil(out.Preferences)
}
