package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type AuthSuite struct {
	suite.Suite
}

func TestAuthSuite(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

// unsignedJWT builds a "header.payload.sig" string with the given claims and a
// throwaway signature segment, enough for tokenSubject which never verifies.
func unsignedJWT(claims map[string]any) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]any{"alg": "none", "typ": "JWT"}) + "." + enc(claims) + ".sig"
}

func (s *AuthSuite) TestTokenSubject_ReadsSub() {
	tok := unsignedJWT(map[string]any{"sub": "user_abc123", "aud": "proj"})
	got, err := tokenSubject(tok)
	s.Require().NoError(err)
	s.Equal("user_abc123", got)
}

func (s *AuthSuite) TestTokenSubject_RejectsMalformed() {
	_, err := tokenSubject("not-a-jwt")
	s.Require().Error(err)
}

func (s *AuthSuite) TestTokenSubject_RejectsMissingSub() {
	tok := unsignedJWT(map[string]any{"aud": "proj"})
	_, err := tokenSubject(tok)
	s.Require().Error(err)
	s.ErrorContains(err, "sub")
}

func (s *AuthSuite) TestResolveToken_ExplicitTokenWins() {
	got, err := resolveToken("explicit-token", "")
	s.Require().NoError(err)
	s.Equal("explicit-token", got)
}

func (s *AuthSuite) TestResolveToken_NoTokenNoIssuer() {
	_, err := resolveToken("", "")
	s.Require().Error(err)
}

func (s *AuthSuite) TestResolveToken_UsesUnexpiredCache() {
	issuer := "https://auth.example.test"
	s.withConfigDir(func() {
		s.Require().NoError(saveCreds(issuer, cachedCreds{
			AccessToken: "cached-token",
			Expiry:      time.Now().Add(time.Hour),
		}))
		got, err := resolveToken("", issuer)
		s.Require().NoError(err)
		s.Equal("cached-token", got)
	})
}

func (s *AuthSuite) TestResolveToken_RejectsExpiredCache() {
	issuer := "https://auth.example.test"
	s.withConfigDir(func() {
		s.Require().NoError(saveCreds(issuer, cachedCreds{
			AccessToken: "old-token",
			Expiry:      time.Now().Add(-time.Minute),
		}))
		_, err := resolveToken("", issuer)
		s.Require().Error(err)
		s.ErrorContains(err, "expired")
	})
}

func (s *AuthSuite) TestCredsPath_DistinctPerIssuerHost() {
	s.withConfigDir(func() {
		a, err := credsPath("https://auth.one.test")
		s.Require().NoError(err)
		b, err := credsPath("https://auth.two.test")
		s.Require().NoError(err)
		s.NotEqual(a, b)
		s.Equal(filepath.Dir(a), filepath.Dir(b))
	})
}

// withConfigDir points os.UserConfigDir at a temp dir for the duration of fn.
func (s *AuthSuite) withConfigDir(fn func()) {
	tmp := s.T().TempDir()
	// os.UserConfigDir reads XDG_CONFIG_HOME on non-darwin; on darwin it uses
	// $HOME/Library/Application Support. Set both so the test is portable.
	s.T().Setenv("XDG_CONFIG_HOME", tmp)
	s.T().Setenv("HOME", tmp)
	if _, err := os.Stat(tmp); err != nil {
		s.Require().NoError(err)
	}
	fn()
}
