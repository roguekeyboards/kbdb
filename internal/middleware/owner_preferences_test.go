package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/ownerprefs"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

type OwnerPreferencesSuite struct {
	suite.Suite
}

func TestOwnerPreferencesSuite(t *testing.T) {
	suite.Run(t, new(OwnerPreferencesSuite))
}

func (s *OwnerPreferencesSuite) newRequest() *http.Request {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/v1/users/alice/switches", nil)
	req.SetPathValue("userId", "alice")
	return req
}

func (s *OwnerPreferencesSuite) TestLoadsPathOwnersPreferences() {
	reader := mocks.NewMockPreferencesReader(s.T())
	want := repository.ProfilePreferences{Currency: "EUR"}
	reader.EXPECT().GetPreferences(mock.Anything, "alice").Return(want, nil)

	var got repository.ProfilePreferences
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		got, err = ownerprefs.Get(r.Context())
		s.NoError(err)
	})

	OwnerPreferences(reader)(next).ServeHTTP(httptest.NewRecorder(), s.newRequest())

	s.Equal(want, got)
}

func (s *OwnerPreferencesSuite) TestLookupFails_Returns500WithoutCallingNext() {
	reader := mocks.NewMockPreferencesReader(s.T())
	reader.EXPECT().GetPreferences(mock.Anything, "alice").Return(repository.ProfilePreferences{}, errors.New("boom"))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Fail("next must not run when the lookup fails")
	})

	rec := httptest.NewRecorder()
	OwnerPreferences(reader)(next).ServeHTTP(rec, s.newRequest())

	s.Equal(http.StatusInternalServerError, rec.Code)
	s.Equal("application/problem+json", rec.Header().Get("Content-Type"))
}
