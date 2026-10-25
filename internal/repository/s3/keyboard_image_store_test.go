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

type KeyboardImageStoreSuite struct {
	suite.Suite

	mockClient  *mocks.MockS3API
	mockPresign *mocks.MockS3PresignAPI
	mockSigner  *mocks.MockURLSigner
	store       *KeyboardImageStore
}

func TestKeyboardImageStoreSuite(t *testing.T) {
	suite.Run(t, new(KeyboardImageStoreSuite))
}

func (s *KeyboardImageStoreSuite) SetupTest() {
	s.mockClient = mocks.NewMockS3API(s.T())
	s.mockPresign = mocks.NewMockS3PresignAPI(s.T())
	s.mockSigner = mocks.NewMockURLSigner(s.T())
	s.store = &KeyboardImageStore{
		client:  s.mockClient,
		presign: s.mockPresign,
		bucket:  "images-bucket",
		get:     testGetSigner(s.mockSigner),
	}
}

func (s *KeyboardImageStoreSuite) TestPresignGetKeyboardImage_SignsKeyOnImagesDomain() {
	s.mockSigner.EXPECT().
		Sign("https://images.example.com/keyboards/alice/kb1/images/img1", testGetExpiry).
		Return("https://images.example.com/signed", nil)

	url, expiresAt, err := s.store.PresignGetKeyboardImage(s.T().Context(), "keyboards/alice/kb1/images/img1")

	s.Require().NoError(err)
	s.Equal("https://images.example.com/signed", url)
	s.Equal(testGetExpiry, expiresAt)
}

func (s *KeyboardImageStoreSuite) TestPresignGetKeyboardImage_SignerError_Propagates() {
	s.mockSigner.EXPECT().
		Sign(mock.Anything, mock.Anything).
		Return("", errors.New("crypto/rsa: verification error"))

	url, _, err := s.store.PresignGetKeyboardImage(s.T().Context(), "keyboards/alice/kb1/images/img1")

	s.Require().ErrorContains(err, "crypto/rsa: verification error")
	s.Empty(url)
}

func (s *KeyboardImageStoreSuite) TestPresignPutKeyboardImage_Succeeds() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "keyboards/alice/kb1/images/img1" && *in.ContentType == "image/png" &&
				*in.ContentLength == 1234 && *in.IfNoneMatch == "*"
		})).
		Return(&v4.PresignedHTTPRequest{URL: "https://example.com/presigned-put"}, nil)

	url, err := s.store.PresignPutKeyboardImage(s.T().Context(), "keyboards/alice/kb1/images/img1", "image/png", 1234)

	s.Require().NoError(err)
	s.Equal("https://example.com/presigned-put", url)
}

func (s *KeyboardImageStoreSuite) TestPresignPutKeyboardImage_SDKError_Propagates() {
	s.mockPresign.EXPECT().
		PresignPutObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	url, err := s.store.PresignPutKeyboardImage(s.T().Context(), "keyboards/alice/kb1/images/img1", "image/png", 1234)

	s.Require().ErrorContains(err, "s3: access denied")
	s.Empty(url)
}

func (s *KeyboardImageStoreSuite) TestDeleteKeyboardImage_Succeeds() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectInput) bool {
			return *in.Bucket == "images-bucket" && *in.Key == "keyboards/alice/kb1/images/img1"
		})).
		Return(&s3.DeleteObjectOutput{}, nil)

	err := s.store.DeleteKeyboardImage(s.T().Context(), "keyboards/alice/kb1/images/img1")

	s.Require().NoError(err)
}

func (s *KeyboardImageStoreSuite) TestDeleteKeyboardImage_SDKError_Propagates() {
	s.mockClient.EXPECT().
		DeleteObject(mock.Anything, mock.Anything).
		Return(nil, errors.New("s3: access denied"))

	err := s.store.DeleteKeyboardImage(s.T().Context(), "keyboards/alice/kb1/images/img1")

	s.Require().ErrorContains(err, "s3: access denied")
}
