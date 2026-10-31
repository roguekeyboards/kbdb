package dynamo

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
	"github.com/roguekeyboards/mykeebs-api/internal/repository/dynamo/mocks"
)

type UploadCounterSuite struct {
	suite.Suite
	mockClient *mocks.MockDynamoAPI
	counter    *UploadCounter
}

func TestUploadCounterSuite(t *testing.T) {
	suite.Run(t, new(UploadCounterSuite))
}

func (s *UploadCounterSuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
	s.counter = &UploadCounter{client: s.mockClient, tableName: "upload-counter-table"}
}

var uploadCounterExpiresAt = time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)

func (s *UploadCounterSuite) TestIncrement_Succeeds() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			day, ok := in.Key["day"].(*types.AttributeValueMemberS)
			if !ok || *in.TableName != "upload-counter-table" || day.Value != "2026-10-11" || in.ConditionExpression == nil {
				return false
			}
			var sawLimit, sawExpiry bool
			for _, v := range in.ExpressionAttributeValues {
				if n, ok := v.(*types.AttributeValueMemberN); ok {
					sawLimit = sawLimit || n.Value == "500"
					sawExpiry = sawExpiry || n.Value == "1791849600"
				}
			}
			return sawLimit && sawExpiry
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	err := s.counter.Increment(s.T().Context(), "2026-10-11", 500, uploadCounterExpiresAt)

	s.Require().NoError(err)
}

func (s *UploadCounterSuite) TestIncrement_ConditionFailed_ReturnsLimitReached() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	err := s.counter.Increment(s.T().Context(), "2026-10-11", 500, uploadCounterExpiresAt)

	s.Require().ErrorIs(err, repository.ErrUploadLimitReached)
}

func (s *UploadCounterSuite) TestIncrement_ClientError_IsWrapped() {
	boom := errors.New("boom")
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, boom)

	err := s.counter.Increment(s.T().Context(), "2026-10-11", 500, uploadCounterExpiresAt)

	s.Require().ErrorIs(err, boom)
	s.NotErrorIs(err, repository.ErrUploadLimitReached)
}
