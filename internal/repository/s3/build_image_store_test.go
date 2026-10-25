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

type BuildImageStoreSuite struct {
	suite.Suite

	mockClient  *mocks.MockS3API
	mockPresign *mocks.MockS3PresignAPI
	mockSigner  *mocks.MockGetSigner
	mockCDN     *mocks.MockInvalidator
	store       *BuildImageStore
}

func TestBuildImageStoreSuite(t *testing.T) {
	suite.Run(t, new(BuildImageStoreSuite))
}

func (s *BuildImageStoreSuite) SetupTest() {
	s.mockClient = mocks.NewMockS3API(s.T())
	s.mockPresign = mocks.NewMockS3PresignAPI(s.T())
	s.mockSigner = mocks.NewMockGetSigner(s.T())
	s.mockCDN = mocks.NewMockInvalidator(s.T())
	s.store = &BuildImageStore{
		client:  s.mockClient,
		presign: s.mockPresign,
		bucket:  "images-bucket",
		get:     s.mockSigner,
		cdn:     s.mockCDN,
	}
}

func (s *BuildImageStoreSuite) TestSignGetBuildImage_SignsKeyOnImagesDomain() {
	expiry := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	s.mockSigner.EXPECT().Sign("builds/alice/b1/images/img1").Return("https://images.example.com/signed", expiry, nil)

	url, expiresAt, err := s.store.SignGetBuildImage(s.T().Context(), "builds/alice/b1/images/img1")

	s.Require().NoError(err)
	s.Equal("https://images.example.com/signed", url)
	s.Equal(expiry, expiresAt)
}

func (s *BuildImageStoreSuite) TestSignGetBuildImage_SignerError_Propagates() {
	s.mockSigner.EXPECT().Sign(mock.Anything).Return("", time.Time{}, errors.New("crypto/rsa: verification error"))

	url, _, err := s.store.SignGetBuildImage(s.T().Context(), "builds/alice/b1/images/img1")

	s.Require().ErrorContains(err, "crypto/rsa: verification error")
	s.Empty(url)
}

func (s *BuildImageStoreSuite) TestPresignPutBuildImage_Succeeds() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "builds/alice/b1/images/img1" && *in.ContentType == "image/png" &&
				*in.ContentLength == 1234 && *in.IfNoneMatch == "*"
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-put"}, nil)

	url, err := s.store.PresignPutBuildImage(s.T().Context(), "builds/alice/b1/images/img1", "image/png", 1234)

	s.Require().NoError(err)
	s.Equal("https://example.com/presigned-put", url)
}

func (s *BuildImageStoreSuite) TestPresignPutBuildImage_SDKError_Propagates() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	url, err := s.store.PresignPutBuildImage(s.T().Context(), "builds/alice/b1/images/img1", "image/png", 1234)

	s.Require().ErrorContains(err, "s3: access denied")
	s.Empty(url)
}

func (s *BuildImageStoreSuite) TestDeleteBuildImage_Succeeds() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "builds/alice/b1/images/img1"
		})).
		Return(&s3.DeleteObjectOutput{}, nil)

	s.mockCDN.EXPECT().Invalidate(mock.Anything, []string{"/builds/alice/b1/images/img1"}).Return(nil)

	err := s.store.DeleteBuildImage(s.T().Context(), "builds/alice/b1/images/img1")

	s.Require().NoError(err)
}

func (s *BuildImageStoreSuite) TestDeleteBuildImage_InvalidationFails_StillSucceeds() {
	s.mockClient.EXPECT().DeleteObject(mock.Anything, mock.Anything).Return(&s3.DeleteObjectOutput{}, nil)
	s.mockCDN.EXPECT().Invalidate(mock.Anything, mock.Anything).Return(errors.New("cloudfront: throttled"))

	err := s.store.DeleteBuildImage(s.T().Context(), "builds/alice/b1/images/img1")

	s.Require().NoError(err)
}

func (s *BuildImageStoreSuite) TestDeleteBuildImage_SDKError_Propagates() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	err := s.store.DeleteBuildImage(s.T().Context(), "builds/alice/b1/images/img1")

	s.Require().ErrorContains(err, "s3: access denied")
}
