package repoapi

import (
	"context"
	"fmt"

	"github.com/rogueserenity/kbdb/internal/handlers/api"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// Profile maps repository.Profile to and from its wire representations.
type Profile struct {
	Images repository.ProfileImageStore
	Repo   repository.ProfileRepository
}

// ToAPI maps a repository.Profile to its wire shape, signing the avatar's
// URL if set and the caller may see it (see [repository.ImageStatus]).
// Errors only if signing fails.
func (p Profile) ToAPI(ctx context.Context, prof repository.Profile, isOwner bool) (api.Profile, error) {
	out := api.Profile{
		Username:        prof.Username,
		UserId:          prof.OwnerID,
		Discoverable:    prof.Discoverable,
		DiscordUsername: prof.DiscordUsername,
		Bio:             prof.Bio,
		Links:           p.linksToAPI(prof.Links),
	}
	if isOwner {
		prefs := p.preferencesToAPI(prof.Preferences)
		out.Preferences = &prefs
	}

	if prof.AvatarPath != nil && prof.AvatarStatus.VisibleTo(isOwner) {
		out.Avatar = &api.ProfileImage{}
		out.Avatar.Status, out.Avatar.RejectionReason = imageStatusToAPI(prof.AvatarStatus, prof.AvatarRejectionReason, isOwner)
		if prof.AvatarStatus.Downloadable() {
			url, _, err := p.Images.SignGet(ctx, *prof.AvatarPath)
			if err != nil {
				return api.Profile{}, fmt.Errorf("signing profile avatar: %w", err)
			}
			out.Avatar.Url = &url
		}
	}

	return out, nil
}

// ToRepo maps a ProfileInput to a repository.Profile. OwnerID, AvatarPath,
// and the GSI discriminators are set downstream, not here.
func (p Profile) ToRepo(in api.ProfileInput) repository.Profile {
	prof := repository.Profile{
		Username:        in.Username,
		DiscordUsername: in.DiscordUsername,
		Bio:             in.Bio,
		Links:           p.linksToRepo(in.Links),
		Preferences:     repository.DefaultProfilePreferences(),
	}
	if in.Discoverable != nil {
		prof.Discoverable = *in.Discoverable
	}
	if in.Preferences != nil {
		prof.Preferences = p.preferencesToRepo(*in.Preferences)
	}

	return prof
}

func (p Profile) preferencesToAPI(prefs repository.ProfilePreferences) api.ProfilePreferences {
	return api.ProfilePreferences{
		Currency:          prefs.Currency,
		ShowPriceToMe:     prefs.ShowPriceToMe,
		ShowPriceToOthers: prefs.ShowPriceToOthers,
	}
}

func (p Profile) preferencesToRepo(in api.ProfilePreferences) repository.ProfilePreferences {
	return repository.ProfilePreferences{
		Currency:          in.Currency,
		ShowPriceToMe:     in.ShowPriceToMe,
		ShowPriceToOthers: in.ShowPriceToOthers,
	}
}

func (p Profile) linksToAPI(links []repository.ProfileLink) *[]api.ProfileLink {
	if len(links) == 0 {
		return nil
	}

	out := make([]api.ProfileLink, len(links))
	for i, l := range links {
		out[i] = api.ProfileLink{Name: l.Name, Url: l.URL}
	}

	return &out
}

func (p Profile) linksToRepo(links *[]api.ProfileLink) []repository.ProfileLink {
	if links == nil || len(*links) == 0 {
		return nil
	}

	out := make([]repository.ProfileLink, len(*links))
	for i, l := range *links {
		out[i] = repository.ProfileLink{Name: l.Name, URL: l.Url}
	}

	return out
}
