package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/auth/mocks"
)

type VerifyTokenSuite struct {
	suite.Suite

	mockVerifier *mocks.MockTokenVerifier
	verifier     *Verifier
}

func TestVerifyTokenSuite(t *testing.T) {
	suite.Run(t, new(VerifyTokenSuite))
}

func (s *VerifyTokenSuite) SetupTest() {
	s.mockVerifier = mocks.NewMockTokenVerifier(s.T())
	s.verifier = &Verifier{verifier: s.mockVerifier, audience: "expected-audience"}
}

func (s *VerifyTokenSuite) TestValidToken_AudienceMatches_Succeeds() {
	expiry := time.Now().Add(time.Hour)
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "raw-token").
		Return(&oidc.IDToken{Subject: "user-123", Expiry: expiry, Audience: []string{"expected-audience"}}, nil)

	claims, err := s.verifier.VerifyToken(s.T().Context(), "raw-token")

	s.Require().NoError(err)
	s.Equal("user-123", claims.Subject)
	s.Equal(expiry, claims.Expiry)
}

func (s *VerifyTokenSuite) TestValidToken_AudienceMismatches_Rejected() {
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "raw-token").
		Return(&oidc.IDToken{Subject: "user-123", Audience: []string{"someone-else"}}, nil)

	claims, err := s.verifier.VerifyToken(s.T().Context(), "raw-token")

	s.Require().Error(err)
	s.Nil(claims)
}

func (s *VerifyTokenSuite) TestInvalidToken_Rejected() {
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "bad-token").
		Return(nil, errors.New("oidc: token is expired"))

	claims, err := s.verifier.VerifyToken(s.T().Context(), "bad-token")

	s.Require().Error(err)
	s.Nil(claims)
}

// checkAudience is exercised directly (not through VerifyToken) so its
// multi-value aud-containment behavior can be tested without a real
// *oidc.IDToken (whose Claims() only works after a genuine Verify() call
// populates its unexported raw payload).
type fakeToken struct {
	aud []string
	sub string
	exp time.Time
}

func (f fakeToken) audience() []string { return f.aud }
func (f fakeToken) subject() string    { return f.sub }
func (f fakeToken) expiry() time.Time  { return f.exp }

func (s *VerifyTokenSuite) TestCheckAudience_AudContainsExpected_Succeeds() {
	err := s.verifier.checkAudience(fakeToken{aud: []string{"expected-audience"}})

	s.NoError(err)
}

// Some IdPs carry aud as a multi-value array (e.g. [project_id, ...]),
// not just the expected value alone - this must still succeed.
func (s *VerifyTokenSuite) TestCheckAudience_AudIsMultiValueContainingExpected_Succeeds() {
	err := s.verifier.checkAudience(fakeToken{aud: []string{"someone-else", "expected-audience"}})

	s.NoError(err)
}

func (s *VerifyTokenSuite) TestCheckAudience_AudDoesNotContainExpected_Rejected() {
	err := s.verifier.checkAudience(fakeToken{aud: []string{"someone-else"}})

	s.Error(err)
}

func (s *VerifyTokenSuite) TestCheckAudience_AudEmpty_Rejected() {
	err := s.verifier.checkAudience(fakeToken{})

	s.Error(err)
}
