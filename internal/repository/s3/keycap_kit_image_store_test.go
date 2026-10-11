package s3

import (
	"errors"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository/s3/mocks"
)

type KeycapKitImageStoreSuite struct {
	suite.Suite

	mockClient  *mocks.MockS3API
	mockPresign *mocks.MockS3PresignAPI
	store       *KeycapKitImageStore
}

func TestKeycapKitImageStoreSuite(t *testing.T) {
	suite.Run(t, new(KeycapKitImageStoreSuite))
}

func (s *KeycapKitImageStoreSuite) SetupTest() {
	s.mockClient = mocks.NewMockS3API(s.T())
	s.mockPresign = mocks.NewMockS3PresignAPI(s.T())
	s.store = &KeycapKitImageStore{
		client:  s.mockClient,
		presign: s.mockPresign,
		bucket:  "images-bucket",
		creds:   expiringCredentials(time.Hour),
	}
}

func (s *KeycapKitImageStoreSuite) TestPresignGet_Succeeds() {
	s.mockPresign.EXPECT().
		PresignGetObject(mock.Anything, mock.MatchedBy(func(in *s3.GetObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "keycap-sets/alice/ks1/kits/kit1/image/up1"
		}), mock.Anything).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-get"}, nil)

	before := time.Now()

	url, expiresAt, err := s.store.PresignGet(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().NoError(err)
	s.Equal("https://example.com/presigned-get", url)
	s.WithinDuration(before.Add(time.Hour-presignMargin), expiresAt, 5*time.Second,
		"expiresAt must track the signing credentials lifetime, less the safety margin")
}

func (s *KeycapKitImageStoreSuite) TestPresignGet_CapsExpiryToCredentialLifetime() {
	s.store.creds = expiringCredentials(10 * time.Minute)

	s.mockPresign.EXPECT().
		PresignGetObject(mock.Anything, mock.Anything, mock.MatchedBy(func(optFns []func(*s3.PresignOptions)) bool {
			var opts s3.PresignOptions
			for _, fn := range optFns {
				fn(&opts)
			}

			want := 10*time.Minute - presignMargin

			return opts.Expires > want-2*time.Second && opts.Expires <= want
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-get"}, nil)

	_, _, err := s.store.PresignGet(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().NoError(err)
}

func (s *KeycapKitImageStoreSuite) TestPresignGet_StaticCredentials_UsesFixedTTL() {
	s.store.creds = staticCredentials()

	s.mockPresign.EXPECT().
		PresignGetObject(mock.Anything, mock.Anything, mock.MatchedBy(func(optFns []func(*s3.PresignOptions)) bool {
			var opts s3.PresignOptions
			for _, fn := range optFns {
				fn(&opts)
			}

			return opts.Expires == staticCredentialsTTL
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-get"}, nil)

	_, _, err := s.store.PresignGet(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().NoError(err)
}

func (s *KeycapKitImageStoreSuite) TestPresignGet_CredentialsInsideMargin_StillSignsForWhatIsLeft() {
	window := presignMargin / 2
	s.store.creds = expiringCredentials(window)

	s.mockPresign.EXPECT().
		PresignGetObject(mock.Anything, mock.Anything, mock.MatchedBy(func(optFns []func(*s3.PresignOptions)) bool {
			var opts s3.PresignOptions
			for _, fn := range optFns {
				fn(&opts)
			}

			return opts.Expires == window/2
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-get"}, nil)

	url, expiresAt, err := s.store.PresignGet(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().NoError(err)
	s.Equal("https://example.com/presigned-get", url)
	s.WithinDuration(time.Now().Add(window/2), expiresAt, 2*time.Second)
}

func (s *KeycapKitImageStoreSuite) TestPresignGet_CredentialsAlreadyExpired_Fails() {
	s.store.creds = expiringCredentials(-time.Minute)

	_, _, err := s.store.PresignGet(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().ErrorContains(err, "signing credentials expired")
}

func (s *KeycapKitImageStoreSuite) TestPresignGet_SDKError_Propagates() {
	s.mockPresign.EXPECT().
		PresignGetObject(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	url, _, err := s.store.PresignGet(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().ErrorContains(err, "s3: access denied")
	s.Empty(url)
}

func (s *KeycapKitImageStoreSuite) TestPresignPut_Succeeds() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "keycap-sets/alice/ks1/kits/kit1/image/up1" && *in.ContentType == "image/png" &&
				*in.ContentLength == 1234 && *in.IfNoneMatch == "*"
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-put"}, nil)

	url, err := s.store.PresignPut(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1", "image/png", 1234)

	s.Require().NoError(err)
	s.Equal("https://example.com/presigned-put", url)
}

func (s *KeycapKitImageStoreSuite) TestPresignPut_SDKError_Propagates() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	url, err := s.store.PresignPut(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1", "image/png", 1234)

	s.Require().ErrorContains(err, "s3: access denied")
	s.Empty(url)
}

func (s *KeycapKitImageStoreSuite) TestDelete_Succeeds() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "keycap-sets/alice/ks1/kits/kit1/image/up1"
		})).
		Return(&s3.DeleteObjectOutput{}, nil)

	err := s.store.Delete(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().NoError(err)
}

func (s *KeycapKitImageStoreSuite) TestDelete_SDKError_Propagates() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	err := s.store.Delete(s.T().Context(), "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().ErrorContains(err, "s3: access denied")
}
