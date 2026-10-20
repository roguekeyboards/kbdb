package dynamo

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/rogueserenity/kbdb/internal/ctx"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/dynamo/mocks"
)

type DeleteOwnerItemsSuite struct {
	suite.Suite

	mockClient *mocks.MockDynamoAPI
}

func TestDeleteOwnerItemsSuite(t *testing.T) {
	suite.Run(t, new(DeleteOwnerItemsSuite))
}

func (s *DeleteOwnerItemsSuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
}

func ownerKeys(n int) []map[string]types.AttributeValue {
	keys := make([]map[string]types.AttributeValue, n)
	for i := range keys {
		keys[i] = map[string]types.AttributeValue{
			"user_id": &types.AttributeValueMemberS{Value: "alice"},
			"id":      &types.AttributeValueMemberS{Value: string(rune('a' + i%26))},
		}
	}
	return keys
}

func (s *DeleteOwnerItemsSuite) TestDeletesEveryPageInBatchesOf25() {
	lastKey := map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: "alice"}}
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return *in.TableName == "t" && in.ExclusiveStartKey == nil &&
				in.ExpressionAttributeValues[":u"].(*types.AttributeValueMemberS).Value == "alice"
		})).
		Return(&dynamodb.QueryOutput{Items: ownerKeys(30), LastEvaluatedKey: lastKey}, nil)
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool { return in.ExclusiveStartKey != nil })).
		Return(&dynamodb.QueryOutput{Items: ownerKeys(1)}, nil)

	var batchSizes []int
	s.mockClient.EXPECT().
		BatchWriteItem(mock.Anything, mock.Anything).
		Run(func(_ context.Context, in *dynamodb.BatchWriteItemInput, _ ...func(*dynamodb.Options)) {
			batchSizes = append(batchSizes, len(in.RequestItems["t"]))
		}).
		Return(&dynamodb.BatchWriteItemOutput{}, nil)

	err := deleteOwnerItems(kbdbctx.WithUserID(s.T().Context(), "alice"), s.mockClient, "t")

	s.Require().NoError(err)
	s.Equal([]int{25, 5, 1}, batchSizes)
}

func (s *DeleteOwnerItemsSuite) TestResendsUnprocessed() {
	s.mockClient.EXPECT().Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{Items: ownerKeys(2)}, nil)
	unprocessed := map[string][]types.WriteRequest{"t": {{DeleteRequest: &types.DeleteRequest{Key: ownerKeys(1)[0]}}}}
	s.mockClient.EXPECT().
		BatchWriteItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.BatchWriteItemInput) bool { return len(in.RequestItems["t"]) == 2 })).
		Return(&dynamodb.BatchWriteItemOutput{UnprocessedItems: unprocessed}, nil).Once()
	s.mockClient.EXPECT().
		BatchWriteItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.BatchWriteItemInput) bool { return len(in.RequestItems["t"]) == 1 })).
		Return(&dynamodb.BatchWriteItemOutput{}, nil).Once()

	s.Require().NoError(deleteOwnerItems(kbdbctx.WithUserID(s.T().Context(), "alice"), s.mockClient, "t"))
}

func (s *DeleteOwnerItemsSuite) TestQueryFails_Error() {
	s.mockClient.EXPECT().Query(mock.Anything, mock.Anything).Return(nil, errors.New("dynamo down"))

	s.Require().Error(deleteOwnerItems(kbdbctx.WithUserID(s.T().Context(), "alice"), s.mockClient, "t"))
}

func (s *DeleteOwnerItemsSuite) TestNoCaller_ErrNoUserID() {
	s.ErrorIs(deleteOwnerItems(s.T().Context(), s.mockClient, "t"), repository.ErrNoUserID)
}
