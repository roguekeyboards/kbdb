package repomcp

import (
	"github.com/roguekeyboards/kbdb/internal/mcp/schema"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

// Profile maps repository.Profile to and from its MCP tool shape. It has
// no dependencies - unlike repoapi.Profile, this never signs the
// avatar; ToMCP reports only HasAvatar.
type Profile struct{}

// ToMCP maps a repository.Profile to its MCP tool shape: avatar as a bool.
func (p Profile) ToMCP(prof repository.Profile, isOwner bool) schema.Profile {
	out := schema.Profile{
		Username:        prof.Username,
		UserID:          prof.OwnerID,
		Discoverable:    prof.Discoverable,
		DiscordUsername: prof.DiscordUsername,
		Bio:             prof.Bio,
		Links:           p.linksToMCP(prof.Links),
		HasAvatar:       prof.AvatarPath != nil && prof.AvatarStatus.VisibleTo(isOwner),
	}
	out.AvatarStatus, out.AvatarRejectionReason = imageStatusToMCP(prof.AvatarPath != nil, prof.AvatarStatus, prof.AvatarRejectionReason, isOwner)
	if isOwner {
		prefs := p.preferencesToMCP(prof.Preferences)
		out.Preferences = &prefs
	}

	return out
}

// ToMCPSummary maps a repository.Profile to a list_profiles row - no bio
// or links, avatar as a bool.
func (p Profile) ToMCPSummary(prof repository.Profile, isOwner bool) schema.ProfileSummary {
	return schema.ProfileSummary{
		Username:        prof.Username,
		UserID:          prof.OwnerID,
		DiscordUsername: prof.DiscordUsername,
		HasAvatar:       prof.AvatarPath != nil && prof.AvatarStatus.VisibleTo(isOwner),
	}
}

// FromMCP maps a create_profile / update_profile tool input to a
// repository.Profile. OwnerID, AvatarPath, and the GSI discriminators
// are set downstream, not here.
func (p Profile) FromMCP(in schema.ProfileInput) repository.Profile {
	prefs := repository.DefaultProfilePreferences()
	if in.Preferences != nil {
		prefs = p.preferencesFromMCP(*in.Preferences)
	}

	return repository.Profile{
		Username:        in.Username,
		Discoverable:    in.Discoverable,
		DiscordUsername: in.DiscordUsername,
		Bio:             in.Bio,
		Links:           p.linksFromMCP(in.Links),
		Preferences:     prefs,
	}
}

func (p Profile) preferencesToMCP(prefs repository.ProfilePreferences) schema.ProfilePreferences {
	return schema.ProfilePreferences{
		Currency:          prefs.Currency,
		ShowPriceToMe:     prefs.ShowPriceToMe,
		ShowPriceToOthers: prefs.ShowPriceToOthers,
	}
}

func (p Profile) preferencesFromMCP(in schema.ProfilePreferences) repository.ProfilePreferences {
	return repository.ProfilePreferences{
		Currency:          in.Currency,
		ShowPriceToMe:     in.ShowPriceToMe,
		ShowPriceToOthers: in.ShowPriceToOthers,
	}
}

func (p Profile) linksFromMCP(links []schema.ProfileLink) []repository.ProfileLink {
	if len(links) == 0 {
		return nil
	}

	out := make([]repository.ProfileLink, len(links))
	for i, l := range links {
		out[i] = repository.ProfileLink{Name: l.Name, URL: l.URL}
	}

	return out
}

func (p Profile) linksToMCP(links []repository.ProfileLink) []schema.ProfileLink {
	if len(links) == 0 {
		return nil
	}

	out := make([]schema.ProfileLink, len(links))
	for i, l := range links {
		out[i] = schema.ProfileLink{Name: l.Name, URL: l.URL}
	}

	return out
}
