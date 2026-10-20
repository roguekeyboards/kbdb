package s3

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/rogueserenity/kbdb/internal/ctx"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/s3/mocks"
)

type OwnerImageStoreSuite struct {
	suite.Suite

	mockClient *mocks.MockS3API
	store      *OwnerImageStore
	ctx        context.Context
}

func TestOwnerImageStoreSuite(t *testing.T) {
	suite.Run(t, new(OwnerImageStoreSuite))
}

func (s *OwnerImageStoreSuite) SetupTest() {
	s.mockClient = mocks.NewMockS3API(s.T())
	s.store = &OwnerImageStore{client: s.mockClient, bucket: "images-bucket"}
	s.ctx = kbdbctx.WithUserID(s.T().Context(), "alice")
}

func (s *OwnerImageStoreSuite) TestDeletesEveryObjectUnderEachPrefix() {
	var listed []string
	s.mockClient.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			listed = append(listed, *in.Prefix)
			if *in.Prefix == "keyboards/alice/" {
				return &s3.ListObjectsV2Output{Contents: []types.Object{
					{Key: aws.String("keyboards/alice/k1/images/i1")},
					{Key: aws.String("keyboards/alice/k1/images/i2")},
				}}, nil
			}
			return &s3.ListObjectsV2Output{}, nil
		})
	s.mockClient.EXPECT().
		DeleteObjects(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectsInput) bool {
			return *in.Bucket == "images-bucket" && len(in.Delete.Objects) == 2 &&
				*in.Delete.Objects[0].Key == "keyboards/alice/k1/images/i1"
		})).
		Return(&s3.DeleteObjectsOutput{}, nil).Once()

	s.Require().NoError(s.store.DeleteAll(s.ctx))
	s.Equal(repository.OwnerImagePrefixes("alice"), listed)
}

func (s *OwnerImageStoreSuite) TestPerKeyErrors_Error() {
	s.mockClient.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).
		Return(&s3.ListObjectsV2Output{Contents: []types.Object{{Key: aws.String("builds/alice/b1/images/i1")}}}, nil).Once()
	s.mockClient.EXPECT().DeleteObjects(mock.Anything, mock.Anything).
		Return(&s3.DeleteObjectsOutput{Errors: []types.Error{{Key: aws.String("builds/alice/b1/images/i1"), Message: aws.String("AccessDenied")}}}, nil)

	s.ErrorContains(s.store.DeleteAll(s.ctx), "AccessDenied")
}

func (s *OwnerImageStoreSuite) TestListFails_Error() {
	s.mockClient.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("s3 down"))

	s.Require().Error(s.store.DeleteAll(s.ctx))
}

func (s *OwnerImageStoreSuite) TestNoCaller_ErrNoUserID() {
	s.ErrorIs(s.store.DeleteAll(s.T().Context()), repository.ErrNoUserID)
}
