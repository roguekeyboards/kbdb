package dynamo

import (
	"errors"
	"math"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository/dynamo/mocks"
)

type QueryLimitSuite struct {
	suite.Suite
}

func TestQueryLimitSuite(t *testing.T) {
	suite.Run(t, new(QueryLimitSuite))
}

func (s *QueryLimitSuite) TestClampsToRange() {
	cases := []struct {
		name string
		in   int
		want int32
	}{
		{"zero -> 1", 0, 1},
		{"negative -> 1", -5, 1},
		{"in range passes through", 20, 20},
		{"max int32 passes through", math.MaxInt32, math.MaxInt32},
		{"above max int32 -> max int32", math.MaxInt32 + 1, math.MaxInt32},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			got := queryLimit(c.in)
			s.Require().NotNil(got)
			s.Equal(c.want, *got)
		})
	}
}

type QueryFullPageSuite struct {
	suite.Suite

	mockClient *mocks.MockDynamoAPI
}

func TestQueryFullPageSuite(t *testing.T) {
	suite.Run(t, new(QueryFullPageSuite))
}

func (s *QueryFullPageSuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
}

func item(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"user_id": &types.AttributeValueMemberS{Value: "alice"},
		"id":      &types.AttributeValueMemberS{Value: id},
	}
}

func startsAfter(id string) func(*dynamodb.QueryInput) bool {
	return func(in *dynamodb.QueryInput) bool {
		key, ok := in.ExclusiveStartKey["id"].(*types.AttributeValueMemberS)
		return ok && key.Value == id
	}
}

func ids(items []map[string]types.AttributeValue) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["id"].(*types.AttributeValueMemberS).Value
	}
	return out
}

func (s *QueryFullPageSuite) TestShortFilteredPages_KeepsQueryingUntilFull() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool { return len(in.ExclusiveStartKey) == 0 && *in.Limit == 3 })).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item("a")}, LastEvaluatedKey: item("c")}, nil).Once()
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(startsAfter("c"))).
		Return(&dynamodb.QueryOutput{LastEvaluatedKey: item("f")}, nil).Once()
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(startsAfter("f"))).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item("g"), item("h")}, LastEvaluatedKey: item("i")}, nil).Once()

	items, lastKey, err := queryFullPage(s.T().Context(), s.mockClient, &dynamodb.QueryInput{}, 3)

	s.Require().NoError(err)
	s.Equal([]string{"a", "g", "h"}, ids(items))
	s.Equal(item("i"), lastKey)
}

func (s *QueryFullPageSuite) TestMoreMatchesThanLimit_ResumesAfterLastKept() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool { return len(in.ExclusiveStartKey) == 0 })).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item("a")}, LastEvaluatedKey: item("b")}, nil).Once()
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(startsAfter("b"))).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item("c"), item("d")}, LastEvaluatedKey: item("d")}, nil).Once()

	items, lastKey, err := queryFullPage(s.T().Context(), s.mockClient, &dynamodb.QueryInput{}, 2)

	s.Require().NoError(err)
	s.Equal([]string{"a", "c"}, ids(items))
	s.Equal(item("c"), lastKey)
}

func (s *QueryFullPageSuite) TestRangeExhausted_ReturnsShortPageWithNoKey() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item("a")}}, nil).Once()

	items, lastKey, err := queryFullPage(s.T().Context(), s.mockClient, &dynamodb.QueryInput{}, 20)

	s.Require().NoError(err)
	s.Equal([]string{"a"}, ids(items))
	s.Empty(lastKey)
}

func (s *QueryFullPageSuite) TestQueryError_Propagates() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	_, _, err := queryFullPage(s.T().Context(), s.mockClient, &dynamodb.QueryInput{}, 20)

	s.Require().Error(err)
}
