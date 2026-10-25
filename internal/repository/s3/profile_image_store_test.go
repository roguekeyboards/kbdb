package s3

import (
	"errors"
	"testing"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository/s3/mocks"
)

type ProfileImageStoreSuite struct {
	suite.Suite

	mockClient  *mocks.MockS3API
	mockPresign *mocks.MockS3PresignAPI
	mockSigner  *mocks.MockURLSigner
	store       *ProfileImageStore
}

func TestProfileImageStoreSuite(t *testing.T) {
	suite.Run(t, new(ProfileImageStoreSuite))
}

func (s *ProfileImageStoreSuite) SetupTest() {
	s.mockClient = mocks.NewMockS3API(s.T())
	s.mockPresign = mocks.NewMockS3PresignAPI(s.T())
	s.mockSigner = mocks.NewMockURLSigner(s.T())
	s.store = &ProfileImageStore{
		client:  s.mockClient,
		presign: s.mockPresign,
		bucket:  "images-bucket",
		get:     testGetSigner(s.mockSigner),
	}
}

func (s *ProfileImageStoreSuite) TestPresignGet_SignsKeyOnImagesDomain() {
	s.mockSigner.EXPECT().
		Sign("https://images.example.com/profiles/user-alice/avatar/up1", testGetExpiry).
		Return("https://images.example.com/signed", nil)

	url, expiresAt, err := s.store.PresignGet(s.T().Context(), "profiles/user-alice/avatar/up1")

	s.Require().NoError(err)
	s.Equal("https://images.example.com/signed", url)
	s.Equal(testGetExpiry, expiresAt)
}

func (s *ProfileImageStoreSuite) TestPresignGet_SignerError_Propagates() {
	s.mockSigner.EXPECT().
		Sign(mock.Anything, mock.Anything).
		Return("", errors.New("crypto/rsa: verification error"))

	url, _, err := s.store.PresignGet(s.T().Context(), "profiles/user-alice/avatar/up1")

	s.Require().ErrorContains(err, "crypto/rsa: verification error")
	s.Empty(url)
}

func (s *ProfileImageStoreSuite) TestPresignPut_Succeeds() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectInput) bool {
			return *in.Bucket == "images-bucket" &&
				*in.Key == "profiles/user-alice/avatar/up1" &&
				*in.ContentType == "image/png" &&
				*in.ContentLength == 1234 && *in.IfNoneMatch == "*"
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-put"}, nil)

	url, err := s.store.PresignPut(s.T().Context(), "profiles/user-alice/avatar/up1", "image/png", 1234)

	s.Require().NoError(err)
	s.Equal("https://example.com/presigned-put", url)
}

func (s *ProfileImageStoreSuite) TestDelete_Succeeds() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "profiles/user-alice/avatar/up1"
		})).
		Return(&s3.DeleteObjectOutput{}, nil)

	s.Require().NoError(s.store.Delete(s.T().Context(), "profiles/user-alice/avatar/up1"))
}

func (s *ProfileImageStoreSuite) TestDelete_SDKError_Propagates() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: boom"))

	s.Require().ErrorContains(s.store.Delete(s.T().Context(), "profiles/user-alice/avatar/up1"), "s3: boom")
}
