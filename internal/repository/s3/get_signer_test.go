package s3

import (
	"crypto/rand"
	"crypto/rsa"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository/s3/mocks"
)

var (
	testGetNow    = time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC)
	testGetExpiry = time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
)

func testGetSigner(signer urlSigner) *GetSigner {
	return &GetSigner{
		baseURL: "https://images.example.com",
		signer:  signer,
		now:     func() time.Time { return testGetNow },
	}
}

type GetSignerSuite struct {
	suite.Suite
}

func TestGetSignerSuite(t *testing.T) {
	suite.Run(t, new(GetSignerSuite))
}

func (s *GetSignerSuite) TestExpiry_AlignsToUTCWindows() {
	newYork, err := time.LoadLocation("America/New_York")
	s.Require().NoError(err)

	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"window start", time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)},
		{"window end", time.Date(2026, 10, 25, 11, 59, 59, 0, time.UTC), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)},
		{"next window", time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC), time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)},
		{"non-UTC clock", time.Date(2026, 10, 25, 9, 30, 0, 0, newYork), time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			signer := mocks.NewMockURLSigner(s.T())
			signer.EXPECT().
				Sign(mock.Anything, mock.MatchedBy(tc.want.Equal)).
				Return("https://images.example.com/signed", nil)
			g := testGetSigner(signer)
			g.now = func() time.Time { return tc.now }

			_, expiresAt, err := g.sign("k")

			s.Require().NoError(err)
			s.True(tc.want.Equal(expiresAt), "got %s, want %s", expiresAt, tc.want)
		})
	}
}

func (s *GetSignerSuite) TestSign_TrimsTrailingSlashFromBaseURL() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)
	g := NewGetSigner("https://images.example.com/", "KEYID", key)

	signed, _, err := g.sign("switches/alice/sw1/image/up1")

	s.Require().NoError(err)
	u, err := url.Parse(signed)
	s.Require().NoError(err)
	s.Equal("/switches/alice/sw1/image/up1", u.Path)
}

func (s *GetSignerSuite) TestSign_RealSigner_SHA256AndDeterministic() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)
	g := NewGetSigner("https://images.example.com", "KEYID", key)
	g.now = func() time.Time { return testGetNow }

	first, expiresAt, err := g.sign("k")
	s.Require().NoError(err)
	second, _, err := g.sign("k")
	s.Require().NoError(err)

	s.Equal(first, second, "URLs signed within one window must be identical")
	u, err := url.Parse(first)
	s.Require().NoError(err)
	q := u.Query()
	s.Equal("SHA256", q.Get("Hash-Algorithm"))
	s.Equal("KEYID", q.Get("Key-Pair-Id"))
	s.Equal(strconv.FormatInt(expiresAt.Unix(), 10), q.Get("Expires"))
	s.NotEmpty(q.Get("Signature"))
}
