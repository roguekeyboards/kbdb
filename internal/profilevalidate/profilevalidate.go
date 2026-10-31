// Package profilevalidate holds the field-level rules for a profile's
// writable body, shared by the REST handlers and the MCP tools so the rules
// can't drift.
package profilevalidate

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// FieldError reports that Name (a JSON pointer-ish field path, e.g.
// "username" or "links[0].url") failed validation for the stated Reason.
type FieldError struct {
	Name   string
	Reason string
}

const (
	maxLinks    = 5
	maxLinkName = 32
	maxBio      = 500
)

// usernamePattern mirrors ProfileInput.username in api/openapi.yaml: the
// Discord-handle rule plus hyphen and a 3-char minimum, so every valid
// Discord username is also a valid kbdb username. RE2 has no lookahead, so
// the no-consecutive-periods rule is a separate strings.Contains check.
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,30}[a-z0-9]$`)

func validUsername(s string) bool {
	return usernamePattern.MatchString(s) && !strings.Contains(s, "..")
}

var discordUsernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.]{0,30}[a-z0-9]$`)

func validDiscordUsername(s string) bool {
	return discordUsernamePattern.MatchString(s) && !strings.Contains(s, "..")
}

// currencyPattern checks shape only (3 uppercase letters) - kbdb doesn't
// maintain or validate against the actual ISO 4217 code list.
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func validCurrency(s string) bool {
	return currencyPattern.MatchString(s)
}

// Normalize coerces a blank (empty or all-whitespace) DiscordUsername to nil
// in place, treating it as not provided rather than a validation failure,
// and uppercases Preferences.Currency. Callers must run this before
// Validate and before passing p to a repository.ProfileRepository, so a
// blank handle never reaches the DynamoDB directory GSIs.
func Normalize(p *repository.Profile) {
	if p.DiscordUsername != nil && strings.TrimSpace(*p.DiscordUsername) == "" {
		p.DiscordUsername = nil
	}
	p.Preferences.Currency = strings.ToUpper(strings.TrimSpace(p.Preferences.Currency))
}

// Validate returns every field-level violation in p's writable body, or nil
// if valid. Unset optional fields are not violations.
func Validate(p repository.Profile) []FieldError {
	var errs []FieldError

	if !validUsername(p.Username) {
		errs = append(errs, FieldError{
			Name:   "username",
			Reason: "must be 3-32 characters of lowercase letters, digits, hyphen, period, or underscore, not starting or ending with a period, hyphen, or underscore, with no consecutive periods",
		})
	}

	if p.DiscordUsername != nil && !validDiscordUsername(*p.DiscordUsername) {
		errs = append(errs, FieldError{
			Name:   "discord_username",
			Reason: "must be 2-32 characters of lowercase letters, digits, period, or underscore, not starting or ending with a period or underscore, with no consecutive periods",
		})
	}

	if p.Bio != nil && utf8.RuneCountInString(*p.Bio) > maxBio {
		errs = append(errs, FieldError{
			Name:   "bio",
			Reason: "must be at most 500 characters",
		})
	}

	if len(p.Links) > maxLinks {
		errs = append(errs, FieldError{
			Name:   "links",
			Reason: "must have at most 5 entries",
		})
	}
	errs = append(errs, validateLinks(p.Links)...)

	if !validCurrency(p.Preferences.Currency) {
		errs = append(errs, FieldError{
			Name:   "preferences.currency",
			Reason: "must be a 3-letter uppercase ISO 4217 currency code",
		})
	}

	return errs
}

func validateLinks(links []repository.ProfileLink) []FieldError {
	var errs []FieldError

	for i, l := range links {
		if strings.TrimSpace(l.Name) == "" {
			errs = append(errs, FieldError{
				Name:   linkField(i, "name"),
				Reason: "must not be blank",
			})
		} else if utf8.RuneCountInString(l.Name) > maxLinkName {
			errs = append(errs, FieldError{
				Name:   linkField(i, "name"),
				Reason: "must be at most 32 characters",
			})
		}

		if reason := badLinkURL(l.URL); reason != "" {
			errs = append(errs, FieldError{Name: linkField(i, "url"), Reason: reason})
		}
	}

	return errs
}

// badLinkURL returns why u is not an acceptable link URL (must parse, be
// https, have a host), or "" if it's fine.
func badLinkURL(u string) string {
	if strings.TrimSpace(u) == "" {
		return "must not be blank"
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return "must be a valid URL"
	}
	if parsed.Scheme != "https" {
		return "must use the https scheme"
	}
	if parsed.Host == "" {
		return "must have a host"
	}

	return ""
}

func linkField(i int, sub string) string {
	return "links[" + strconv.Itoa(i) + "]." + sub
}
