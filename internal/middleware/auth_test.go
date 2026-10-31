package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/roguekeyboards/mykeebs-api/internal/auth"
	"github.com/roguekeyboards/mykeebs-api/internal/auth/mocks"
	ctxpkg "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/middleware"
)

type OptionalAuthSuite struct {
	suite.Suite

	mockVerifier *mocks.MockTokenVerifier
	verifier     *auth.Verifier
}

func TestOptionalAuthSuite(t *testing.T) {
	suite.Run(t, new(OptionalAuthSuite))
}

func (s *OptionalAuthSuite) SetupTest() {
	s.mockVerifier = mocks.NewMockTokenVerifier(s.T())
	s.verifier = auth.NewVerifierForTesting(s.mockVerifier, "expected-audience")
}

func (s *OptionalAuthSuite) TestNoTokenProceedsAnonymously() {
	// mockVerifier has no EXPECT() set up - if OptionalAuth called
	// Verify at all in this case, the mock would fail the test on its
	// own (mockery's generated mocks fail on unexpected calls by
	// default), so this doubles as an assertion that VerifyToken is
	// never invoked for a request with no token.
	var calledWithUserID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		calledWithUserID, _ = ctxpkg.UserID(r.Context())
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	middleware.OptionalAuth(s.verifier)(next).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Empty(calledWithUserID)
}

func (s *OptionalAuthSuite) TestInvalidTokenIsRejected() {
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "not-a-real-token").
		Return(nil, errors.New("oidc: signature verification failed"))
	nextCalled := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		nextCalled = true
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	middleware.OptionalAuth(s.verifier)(next).ServeHTTP(rec, req)

	s.Equal(http.StatusUnauthorized, rec.Code)
	s.False(nextCalled)
}

func (s *OptionalAuthSuite) TestValidTokenProceedsAuthenticated() {
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "a-valid-token").
		Return(&oidc.IDToken{Subject: "user-123", Audience: []string{"expected-audience"}}, nil)
	var calledWithUserID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		calledWithUserID, _ = ctxpkg.UserID(r.Context())
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer a-valid-token")
	rec := httptest.NewRecorder()
	middleware.OptionalAuth(s.verifier)(next).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("user-123", calledWithUserID)
}

type RequireAuthSuite struct {
	suite.Suite

	mockVerifier *mocks.MockTokenVerifier
	verifier     *auth.Verifier
}

func TestRequireAuthSuite(t *testing.T) {
	suite.Run(t, new(RequireAuthSuite))
}

func (s *RequireAuthSuite) SetupTest() {
	s.mockVerifier = mocks.NewMockTokenVerifier(s.T())
	s.verifier = auth.NewVerifierForTesting(s.mockVerifier, "expected-audience")
}

func (s *RequireAuthSuite) TestNoTokenIsRejectedWithMetadataURL() {
	// mockVerifier has no EXPECT() set up - a missing token must be
	// rejected before ever calling Verify, same as OptionalAuth's
	// equivalent case.
	nextCalled := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		nextCalled = true
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/mcp", nil)
	req.Host = "api.jay.mykeebs.dev"
	rec := httptest.NewRecorder()
	middleware.RequireAuth(s.verifier, "/.well-known/oauth-protected-resource/mcp")(next).ServeHTTP(rec, req)

	s.Equal(http.StatusUnauthorized, rec.Code)
	s.False(nextCalled)
	s.Equal(`Bearer resource_metadata="https://api.jay.mykeebs.dev/.well-known/oauth-protected-resource/mcp"`,
		rec.Header().Get("WWW-Authenticate"))
}

func (s *RequireAuthSuite) TestInvalidTokenIsRejectedWithMetadataURL() {
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "not-a-real-token").
		Return(nil, errors.New("oidc: signature verification failed"))
	nextCalled := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		nextCalled = true
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/mcp", nil)
	req.Host = "api.jay.mykeebs.dev"
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	middleware.RequireAuth(s.verifier, "/.well-known/oauth-protected-resource/mcp")(next).ServeHTTP(rec, req)

	s.Equal(http.StatusUnauthorized, rec.Code)
	s.False(nextCalled)
	s.Equal(`Bearer resource_metadata="https://api.jay.mykeebs.dev/.well-known/oauth-protected-resource/mcp"`,
		rec.Header().Get("WWW-Authenticate"))
}

func (s *RequireAuthSuite) TestValidTokenProceedsAuthenticated() {
	s.mockVerifier.EXPECT().
		Verify(mock.Anything, "a-valid-token").
		Return(&oidc.IDToken{Subject: "user-123", Audience: []string{"expected-audience"}}, nil)
	var calledWithUserID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		calledWithUserID, _ = ctxpkg.UserID(r.Context())
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer a-valid-token")
	rec := httptest.NewRecorder()
	middleware.RequireAuth(s.verifier, "/.well-known/oauth-protected-resource/mcp")(next).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("user-123", calledWithUserID)
	s.Empty(rec.Header().Get("WWW-Authenticate"))
}
