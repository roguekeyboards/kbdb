package repoapi

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository"
)

type ResolveImageURLSuite struct {
	suite.Suite
}

func TestResolveImageURLSuite(t *testing.T) {
	suite.Run(t, new(ResolveImageURLSuite))
}

func (s *ResolveImageURLSuite) TestCachedFromCurrentWindow_ReusedWithoutSigningOrWritingBack() {
	cachedURL := "https://example.com/cached"
	expiresAt := signedExpiry()

	mintCalled := false
	writeBackCalled := false

	url, err := resolveImageURL(&cachedURL, &expiresAt,
		func() (string, time.Time, error) {
			mintCalled = true
			return "https://example.com/fresh", signedExpiry(), nil
		},
		func(string, time.Time) error {
			writeBackCalled = true
			return nil
		},
	)
	s.Require().NoError(err)

	s.Equal(cachedURL, url)
	s.False(mintCalled)
	s.False(writeBackCalled)
}

func (s *ResolveImageURLSuite) TestCachedFromPreviousWindow_Resigned() {
	cachedURL := "https://example.com/cached"
	expiresAt := signedExpiry().Add(-repository.ImageURLWindow)

	url, err := resolveImageURL(&cachedURL, &expiresAt,
		func() (string, time.Time, error) { return "https://example.com/fresh", signedExpiry(), nil },
		func(string, time.Time) error { return nil },
	)
	s.Require().NoError(err)

	s.Equal("https://example.com/fresh", url)
}

func (s *ResolveImageURLSuite) TestCacheMiss_NoCachedURL_MintsAndWritesBack() {
	mintCalled := false
	var writtenURL string
	var writtenExpiresAt time.Time

	mintedExpiresAt := time.Now().Add(37 * time.Minute)

	url, err := resolveImageURL(nil, nil,
		func() (string, time.Time, error) {
			mintCalled = true
			return "https://example.com/fresh", mintedExpiresAt, nil
		},
		func(u string, e time.Time) error {
			writtenURL = u
			writtenExpiresAt = e
			return nil
		},
	)
	s.Require().NoError(err)

	s.True(mintCalled)
	s.Equal("https://example.com/fresh", url)
	s.Equal("https://example.com/fresh", writtenURL)
	s.Equal(mintedExpiresAt, writtenExpiresAt,
		"the cached expiry must be the one the signer reported, not one computed here")
}

func (s *ResolveImageURLSuite) TestMintError_Propagates() {
	_, err := resolveImageURL(nil, nil,
		func() (string, time.Time, error) { return "", time.Time{}, errors.New("s3: access denied") },
		func(string, time.Time) error { return nil },
	)

	s.Require().Error(err)
}

func (s *ResolveImageURLSuite) TestWriteBackError_IsNonFatal_StillReturnsFreshURL() {
	url, err := resolveImageURL(nil, nil,
		func() (string, time.Time, error) { return "https://example.com/fresh", time.Now().Add(time.Hour), nil },
		func(string, time.Time) error { return errors.New("conditional check failed") },
	)
	s.Require().NoError(err)

	s.Equal("https://example.com/fresh", url)
}

func signedExpiry() time.Time {
	return time.Now().Truncate(repository.ImageURLWindow).Add(repository.ImageURLLifetime)
}
