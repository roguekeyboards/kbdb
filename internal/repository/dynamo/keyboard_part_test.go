package dynamo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/dynamo/mocks"
)

type KeyboardPartRepositorySuite struct {
	suite.Suite

	mockClient *mocks.MockDynamoAPI
	repo       *KeyboardRepository
}

func TestKeyboardPartRepositorySuite(t *testing.T) {
	suite.Run(t, new(KeyboardPartRepositorySuite))
}

func (s *KeyboardPartRepositorySuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
	s.repo = &KeyboardRepository{client: s.mockClient, tableName: "keyboard-table"}
}

func (s *KeyboardPartRepositorySuite) ctx() context.Context {
	return kbdbctx.WithUserID(s.T().Context(), "alice")
}

// names reports whether every want appears among an expression's names.
func names(in map[string]string, want ...string) bool {
	have := map[string]bool{}
	for _, n := range in {
		have[n] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}

func (s *KeyboardPartRepositorySuite) TestAddPlate_SetsEntryWithSeqUnderExistsAndNotExists() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return in.Key["id"].(*types.AttributeValueMemberS).Value == "kb1" &&
				names(in.ExpressionAttributeNames, "plates", "p1") &&
				strings.Contains(*in.ConditionExpression, "attribute_exists") &&
				strings.Contains(*in.ConditionExpression, "attribute_not_exists")
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	plate, err := s.repo.AddPlate(s.ctx(), "kb1", repository.KeyboardPlate{ID: "p1", Material: "PC"})

	s.Require().NoError(err)
	s.Equal("p1", plate.ID)
	s.Positive(plate.Seq, "a new plate gets a seq that sorts after existing ones")
}

func (s *KeyboardPartRepositorySuite) TestAddPlate_KeyboardMissing_ReturnsErrNotFound() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.GetItemInput) bool {
			return in.ConsistentRead != nil && *in.ConsistentRead
		})).
		Return(&dynamodb.GetItemOutput{}, nil)

	_, err := s.repo.AddPlate(s.ctx(), "kb1", repository.KeyboardPlate{ID: "p1", Material: "PC"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeyboardPartRepositorySuite) TestAddPCB_DuplicateID_ReturnsError() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "kb1"}}}, nil)

	_, err := s.repo.AddPCB(s.ctx(), "kb1", repository.KeyboardPCB{ID: "b1"})

	s.Require().ErrorIs(err, errDuplicateKeyboardPartID)
}

func (s *KeyboardPartRepositorySuite) TestAddPCB_EmptyID_ReturnsError() {
	_, err := s.repo.AddPCB(s.ctx(), "kb1", repository.KeyboardPCB{})

	s.Require().ErrorIs(err, errInvalidMapKey)
}

func (s *KeyboardPartRepositorySuite) TestDeletePlate_DottedID_IsOneKeyNotANestedField() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return names(in.ExpressionAttributeNames, "plates", "p1.material") &&
				!names(in.ExpressionAttributeNames, "material")
		})).
		Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "kb1"}}}, nil)

	s.Require().NoError(s.repo.DeletePlate(s.ctx(), "kb1", "p1.material"))
}

func (s *KeyboardPartRepositorySuite) TestUpdatePlate_DottedID_IsOneKeyNotANestedField() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return names(in.ExpressionAttributeNames, "plates", "p1.color", "material")
		})).
		Return(nil, &types.ConditionalCheckFailedException{})

	_, err := s.repo.UpdatePlate(s.ctx(), "kb1", repository.KeyboardPlate{ID: "p1.color", Material: "AL"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeyboardPartRepositorySuite) TestBracketedID_NeverReachesDynamo() {
	_, err := s.repo.UpdatePCB(s.ctx(), "kb1", repository.KeyboardPCB{ID: "b1[0]"})
	s.Require().ErrorIs(err, repository.ErrNotFound)

	s.Require().NoError(s.repo.DeletePCB(s.ctx(), "kb1", "b1[0]"), "no stored PCB can have that id, so it's already gone")
}

func (s *KeyboardPartRepositorySuite) TestUpdatePlate_NamesFieldsButNotSeqAndReturnsStoredPlate() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return names(in.ExpressionAttributeNames, "plates", "p1", "material", "purchase", "color", "thickness") &&
				!names(in.ExpressionAttributeNames, "seq") &&
				in.ReturnValues == types.ReturnValueAllNew
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "kb1"},
			"plates": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"p1": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"id":       &types.AttributeValueMemberS{Value: "p1"},
					"material": &types.AttributeValueMemberS{Value: "AL"},
					"seq":      &types.AttributeValueMemberN{Value: "3"},
				}},
			}},
		}}, nil)

	plate, err := s.repo.UpdatePlate(s.ctx(), "kb1", repository.KeyboardPlate{ID: "p1", Material: "AL"})

	s.Require().NoError(err)
	s.Equal("AL", plate.Material)
	s.Equal(3, plate.Seq)
}

func (s *KeyboardPartRepositorySuite) TestUpdatePCB_Missing_ReturnsErrNotFound() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{})

	_, err := s.repo.UpdatePCB(s.ctx(), "kb1", repository.KeyboardPCB{ID: "b1"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeyboardPartRepositorySuite) TestDeletePlate_Removes() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.UpdateExpression, "REMOVE") && names(in.ExpressionAttributeNames, "plates", "p1")
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	s.Require().NoError(s.repo.DeletePlate(s.ctx(), "kb1", "p1"))
}

func (s *KeyboardPartRepositorySuite) TestDeletePCB_AlreadyGone_IsIdempotent() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "kb1"}}}, nil)

	s.Require().NoError(s.repo.DeletePCB(s.ctx(), "kb1", "b1"))
}

func (s *KeyboardPartRepositorySuite) TestDeletePCB_KeyboardMissing_ReturnsErrNotFound() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)

	s.Require().ErrorIs(s.repo.DeletePCB(s.ctx(), "kb1", "b1"), repository.ErrNotFound)
}

func (s *KeyboardPartRepositorySuite) TestDeletePlate_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(nil, errors.New("throttled"))

	err := s.repo.DeletePlate(s.ctx(), "kb1", "p1")

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrNotFound)
}

func (s *KeyboardPartRepositorySuite) TestNoUserIDInContext_ReturnsError() {
	_, err := s.repo.AddPlate(s.T().Context(), "kb1", repository.KeyboardPlate{ID: "p1"})
	s.Require().ErrorIs(err, repository.ErrNoUserID)
}
