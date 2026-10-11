package dynamo

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/rogueserenity/kbdb/internal/ctx"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/dynamo/mocks"
)

type KeycapSetRepositorySuite struct {
	suite.Suite

	mockClient *mocks.MockDynamoAPI
	repo       *KeycapSetRepository
}

func TestKeycapSetRepositorySuite(t *testing.T) {
	suite.Run(t, new(KeycapSetRepositorySuite))
}

func (s *KeycapSetRepositorySuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
	s.repo = &KeycapSetRepository{client: s.mockClient, tableName: "keycap-set-table"}
}

func (s *KeycapSetRepositorySuite) TestList_Succeeds() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return len(in.ExclusiveStartKey) == 0 && *in.Limit == 20
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"user_id": &types.AttributeValueMemberS{Value: "alice"},
					"id":      &types.AttributeValueMemberS{Value: "ks1"},
					"brand":   &types.AttributeValueMemberS{Value: "GMK"},
				},
			},
		}, nil)

	sets, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().NoError(err)
	s.Empty(next)
	s.Require().Len(sets, 1)
	s.Equal("ks1", sets[0].ID)
	s.Equal("GMK", sets[0].Brand)
}

func (s *KeycapSetRepositorySuite) TestList_EmptyVisibilities_ReturnsEmptySliceWithoutQuerying() {
	// No EXPECT() on s.mockClient.Query - an empty visibilities slice must
	// short-circuit before building a Query, since expression.In(...)
	// requires at least one value and would otherwise panic.
	sets, next, err := s.repo.List(s.T().Context(), "alice", nil, 20, "")

	s.Require().NoError(err)
	s.NotNil(sets)
	s.Empty(sets)
	s.Empty(next)
}

func (s *KeycapSetRepositorySuite) TestList_EmptyResult_ReturnsEmptySliceNotNil() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)

	sets, _, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().NoError(err)
	s.NotNil(sets)
	s.Empty(sets)
}

func (s *KeycapSetRepositorySuite) TestList_ReturnsEncodedCursor_WhenMorePagesExist() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id": &types.AttributeValueMemberS{Value: "alice"},
				"id":      &types.AttributeValueMemberS{Value: "ks1"},
			},
		}, nil)

	_, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().NoError(err)
	s.NotEmpty(next)
}

func (s *KeycapSetRepositorySuite) TestList_DecodesCursor_IntoExclusiveStartKey() {
	// Round-trip: encode a key via a first call, then confirm the second
	// call's ExclusiveStartKey matches what was encoded.
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return len(in.ExclusiveStartKey) == 0
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id": &types.AttributeValueMemberS{Value: "alice"},
				"id":      &types.AttributeValueMemberS{Value: "ks1"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")
	s.Require().NoError(err)

	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			key, ok := in.ExclusiveStartKey["id"].(*types.AttributeValueMemberS)
			return ok && key.Value == "ks1"
		})).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil).Once()

	_, _, err = s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, cursor)
	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestList_InvalidCursor_ReturnsErrInvalidCursor() {
	sets, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "not-valid-base64!!")

	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
	s.Nil(sets)
	s.Empty(next)
}

func (s *KeycapSetRepositorySuite) TestList_QueryError_Propagates() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	sets, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().Error(err)
	s.Nil(sets)
	s.Empty(next)
}

func (s *KeycapSetRepositorySuite) TestGet_Succeeds() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.GetItemInput) bool {
			userID, ok := in.Key["user_id"].(*types.AttributeValueMemberS)
			id, ok2 := in.Key["id"].(*types.AttributeValueMemberS)
			return ok && ok2 && userID.Value == "alice" && id.Value == "ks1"
		})).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"user_id": &types.AttributeValueMemberS{Value: "alice"},
				"id":      &types.AttributeValueMemberS{Value: "ks1"},
				"brand":   &types.AttributeValueMemberS{Value: "GMK"},
			},
		}, nil)

	ks, err := s.repo.Get(s.T().Context(), "alice", "ks1")

	s.Require().NoError(err)
	s.Equal("ks1", ks.ID)
	s.Equal("GMK", ks.Brand)
}

func (s *KeycapSetRepositorySuite) TestGet_NotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	ks, err := s.repo.Get(s.T().Context(), "alice", "missing")

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestGet_GetItemError_Propagates() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ks, err := s.repo.Get(s.T().Context(), "alice", "ks1")

	s.Require().Error(err)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestCreate_Succeeds() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.Anything).
		Return(&dynamodb.PutItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	ks, err := s.repo.Create(ctx, repository.KeycapSet{ID: "ks1", Brand: "GMK"})

	s.Require().NoError(err)
	s.Equal(&repository.KeycapSet{UserID: "alice", ID: "ks1", Brand: "GMK"}, ks)
}

func (s *KeycapSetRepositorySuite) TestCreate_StoresKitsAsEmptyMap() {
	// AddKit's SET kits.<kit_id> requires "kits" to already exist as a Map.
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.PutItemInput) bool {
			kits, ok := in.Item["kits"].(*types.AttributeValueMemberM)
			return ok && len(kits.Value) == 0
		})).
		Return(&dynamodb.PutItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.Create(ctx, repository.KeycapSet{ID: "ks1"})

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestCreate_AlreadyExists_ReturnsErrAlreadyExists() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	ks, err := s.repo.Create(ctx, repository.KeycapSet{ID: "ks1"})

	s.Require().ErrorIs(err, repository.ErrAlreadyExists)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestCreate_PutItemError_Propagates() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	ks, err := s.repo.Create(ctx, repository.KeycapSet{ID: "ks1"})

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrAlreadyExists)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestCreate_NoUserIDInContext_ReturnsError() {
	ks, err := s.repo.Create(s.T().Context(), repository.KeycapSet{ID: "ks1"})

	s.Require().Error(err)
	s.Nil(ks)
}

// updatedItem is a stand-in for an UpdateItem ALL_NEW response.
func (s *KeycapSetRepositorySuite) updatedItem() map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"user_id": &types.AttributeValueMemberS{Value: "alice"},
		"id":      &types.AttributeValueMemberS{Value: "ks1"},
		"brand":   &types.AttributeValueMemberS{Value: "GMK"},
	}
}

func (s *KeycapSetRepositorySuite) TestUpdate_Succeeds() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			key := in.Key["id"].(*types.AttributeValueMemberS)
			return key.Value == "ks1" &&
				strings.Contains(*in.ConditionExpression, "attribute_exists") &&
				in.ReturnValues == types.ReturnValueAllNew
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.updatedItem()}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	ks, err := s.repo.Update(ctx, repository.KeycapSet{ID: "ks1", Brand: "GMK"})

	s.Require().NoError(err)
	s.Equal("GMK", ks.Brand)
}

func (s *KeycapSetRepositorySuite) TestUpdate_OmittedOptionalFields_AreRemoved() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.UpdateExpression, "REMOVE")
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.updatedItem()}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.Update(ctx, repository.KeycapSet{ID: "ks1", Brand: "GMK"})

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestUpdate_DoesNotNameKitsOrPrimaryKitID() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			for _, name := range in.ExpressionAttributeNames {
				if name == "kits" || name == "primary_kit_id" {
					return false
				}
			}
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.updatedItem()}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.Update(ctx, repository.KeycapSet{ID: "ks1", Brand: "GMK"})

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestUpdate_ConditionFails_ReturnsErrNotFound() {
	// attribute_exists(id) on the sort key can only fail when no item exists
	// at (user_id, id) - a concurrent delete. No follow-up read.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	ks, err := s.repo.Update(ctx, repository.KeycapSet{ID: "ks1", Brand: "GMK"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestUpdate_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	ks, err := s.repo.Update(ctx, repository.KeycapSet{ID: "ks1"})

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrNotFound)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestUpdate_NoUserIDInContext_ReturnsError() {
	ks, err := s.repo.Update(s.T().Context(), repository.KeycapSet{ID: "ks1"})

	s.Require().Error(err)
	s.Nil(ks)
}

func (s *KeycapSetRepositorySuite) TestDelete_Succeeds() {
	s.mockClient.EXPECT().
		DeleteItem(mock.Anything, mock.Anything).
		Return(&dynamodb.DeleteItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.Delete(ctx, "ks1")

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestDelete_NoUserIDInContext_ReturnsError() {
	err := s.repo.Delete(s.T().Context(), "ks1")

	s.Require().Error(err)
}

func (s *KeycapSetRepositorySuite) TestDelete_DeleteItemError_Propagates() {
	s.mockClient.EXPECT().
		DeleteItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.Delete(ctx, "ks1")

	s.Require().Error(err)
}

// kitAttr builds one kit's M attribute value.
func kitAttr(kitID, name string, imagePath *string) *types.AttributeValueMemberM {
	m := map[string]types.AttributeValue{
		"kit_id":   &types.AttributeValueMemberS{Value: kitID},
		"name":     &types.AttributeValueMemberS{Value: name},
		"purchase": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
	}
	if imagePath != nil {
		m["image_path"] = &types.AttributeValueMemberS{Value: *imagePath}
	}
	return &types.AttributeValueMemberM{Value: m}
}

// itemWithKit is a GetItem/UpdateItem Attributes stand-in for a set with one
// existing kit, "kit1".
func (s *KeycapSetRepositorySuite) itemWithKit(name string, imagePath *string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"user_id": &types.AttributeValueMemberS{Value: "alice"},
		"id":      &types.AttributeValueMemberS{Value: "ks1"},
		"kits":    &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"kit1": kitAttr("kit1", name, imagePath)}},
	}
}

func (s *KeycapSetRepositorySuite) TestAddKit_Succeeds() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			key := in.Key["id"].(*types.AttributeValueMemberS)
			return key.Value == "ks1" &&
				strings.Contains(*in.ConditionExpression, "attribute_exists") &&
				strings.Contains(*in.ConditionExpression, "attribute_not_exists")
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().NoError(err)
	s.Require().NotNil(kit)
	s.Equal("kit1", kit.KitID)
	s.Equal("Base", kit.Name)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_DottedKitID_IsOneKeyNotANestedField() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			has := map[string]bool{}
			for _, n := range in.ExpressionAttributeNames {
				has[n] = true
			}
			return has["kits"] && has["kit1.image_path"] && !has["image_path"]
		})).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1.image_path", Name: "Base"}, nil)

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeycapSetRepositorySuite) TestBracketedKitID_NeverReachesDynamo() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")

	_, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1[0]", Name: "Base"}, nil)
	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Require().ErrorIs(s.repo.SetKitImagePath(ctx, "ks1", "kit1[0]", "k"), repository.ErrNotFound)
	s.Require().NoError(s.repo.DeleteKit(ctx, "ks1", "kit1[0]"))
}

func (s *KeycapSetRepositorySuite) TestAddKit_AssignsSeq() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().NoError(err)
	s.Positive(kit.Seq, "a new kit gets a seq that sorts after existing ones")
}

func (s *KeycapSetRepositorySuite) TestAddKit_PrimaryTrue_SetsPrimaryKitIDInSameUpdate() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return namesPrimaryKitID(in.ExpressionAttributeNames)
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	primary := true
	_, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, &primary)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestAddKit_PrimaryFalse_LeavesPrimaryKitIDUntouched() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return !namesPrimaryKitID(in.ExpressionAttributeNames)
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	primary := false
	_, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, &primary)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestAddKit_PrimaryNil_LeavesPrimaryKitIDUntouched() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return !namesPrimaryKitID(in.ExpressionAttributeNames)
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestAddKit_ParentSetNotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.GetItemInput) bool {
			return in.ConsistentRead != nil && *in.ConsistentRead
		})).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "missing", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestAddKit_DuplicateKitID_ReturnsError() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: s.itemWithKit("Base", nil)}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Extension"}, nil)

	s.Require().ErrorIs(err, errDuplicateKitID)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestAddKit_ClassifyGetItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrNotFound)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestAddKit_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().Error(err)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestAddKit_EmptyKitID_ReturnsError() {
	// No EXPECT() on UpdateItem - caught before any DynamoDB call.
	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.AddKit(ctx, "ks1", repository.KeycapKit{Name: "Base"}, nil)

	s.Require().ErrorIs(err, errEmptyKitID)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestAddKit_NoUserIDInContext_ReturnsError() {
	kit, err := s.repo.AddKit(s.T().Context(), "ks1", repository.KeycapKit{KitID: "kit1"}, nil)

	s.Require().Error(err)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_Succeeds() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.ConditionExpression, "attribute_exists")
		})).
		Return(&dynamodb.UpdateItemOutput{
			Attributes: s.itemWithKit("Extension", nil),
		}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	vendor := "CannonKeys"
	kit, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{
		KitID:    "kit1",
		Name:     "Extension",
		Purchase: repository.KeycapKitPurchase{Vendor: &vendor},
	}, nil)

	s.Require().NoError(err)
	s.Require().NotNil(kit)
	s.Equal("kit1", kit.KitID)
	s.Equal("Extension", kit.Name)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_PrimaryTrue_SetsPrimaryKitIDInSameUpdate() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return namesPrimaryKitID(in.ExpressionAttributeNames)
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	primary := true
	_, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, &primary)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_PrimaryFalse_ClearsPrimaryKitID() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil).
		Once()
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return *in.UpdateExpression == "REMOVE primary_kit_id" &&
				*in.ConditionExpression == "primary_kit_id = :kid"
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	primary := false
	_, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, &primary)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_PrimaryFalse_NotActuallyPrimary_SwallowsConditionFailure() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil).
		Once()
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{}).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	primary := false
	_, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, &primary)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_PrimaryFalse_ClearPrimaryError_DoesNotFailTheUpdate() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil).
		Once()
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled")).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	primary := false
	kit, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, &primary)

	s.Require().NoError(err)
	s.Require().NotNil(kit)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_KitMissingFromResponse_ReturnsError() {
	// Defensive/should-be-unreachable: ALL_NEW omits the kit this call just
	// wrote.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: map[string]types.AttributeValue{
			"user_id": &types.AttributeValueMemberS{Value: "alice"},
			"id":      &types.AttributeValueMemberS{Value: "ks1"},
			"kits":    &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
		}}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().ErrorIs(err, errKitMissingAfterWrite)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_PrimaryNil_LeavesPrimaryKitIDUntouched_NoSecondCall() {
	// A single EXPECT() on UpdateItem - a second call would fail the mock.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Base"}, nil)

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_KitOrSetNotFound_ReturnsErrNotFound() {
	// A single EXPECT() on UpdateItem - both a missing set and a missing kit
	// fail the same compound condition, so no classify read follows.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{}).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "missing-kit", Name: "Extension"}, nil)

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_EmptyKitID_ReturnsError() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{Name: "Extension"}, nil)

	s.Require().ErrorIs(err, errEmptyKitID)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kit, err := s.repo.UpdateKit(ctx, "ks1", repository.KeycapKit{KitID: "kit1", Name: "Extension"}, nil)

	s.Require().Error(err)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestUpdateKit_NoUserIDInContext_ReturnsError() {
	kit, err := s.repo.UpdateKit(s.T().Context(), "ks1", repository.KeycapKit{KitID: "kit1"}, nil)

	s.Require().Error(err)
	s.Nil(kit)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_Succeeds_ClearsMatchingPrimaryKitID() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.UpdateExpression, "REMOVE") &&
				strings.Contains(*in.ConditionExpression, "attribute_exists")
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil).
		Once()
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return *in.UpdateExpression == "REMOVE primary_kit_id"
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "ks1", "kit1")

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_NotPrimary_SwallowsConditionFailure() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{}, nil).
		Once()
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{}).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "ks1", "kit1")

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_ParentSetNotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{}).
		Once()
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "missing", "kit1")

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_ClassifyGetItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{}).
		Once()
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "ks1", "kit1")

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrNotFound)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_KitAlreadyAbsent_SucceedsIdempotently() {
	// A single EXPECT() on UpdateItem - the classify path returns success
	// without attempting the primary-kit-id clear.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{}).
		Once()
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: s.itemWithKit("Base", nil)}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "ks1", "no-such-kit")

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "ks1", "kit1")

	s.Require().Error(err)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_ClearPrimaryError_DoesNotFailTheDelete() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{}, nil).
		Once()
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled")).
		Once()

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.DeleteKit(ctx, "ks1", "kit1")

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestDeleteKit_NoUserIDInContext_ReturnsError() {
	err := s.repo.DeleteKit(s.T().Context(), "ks1", "kit1")

	s.Require().Error(err)
}

func (s *KeycapSetRepositorySuite) TestSetKitImagePath_Succeeds() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.UpdateExpression, "SET") &&
				strings.Contains(*in.ConditionExpression, "attribute_exists")
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.SetKitImagePath(ctx, "ks1", "kit1", "keycap-sets/alice/ks1/kits/kit1/image/up1")

	s.Require().NoError(err)
}

func (s *KeycapSetRepositorySuite) TestSetKitImagePath_NotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.SetKitImagePath(ctx, "ks1", "missing-kit", "p")

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeycapSetRepositorySuite) TestSetKitImagePath_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.SetKitImagePath(ctx, "ks1", "kit1", "p")

	s.Require().Error(err)
}

func (s *KeycapSetRepositorySuite) TestSetKitImagePath_NoUserIDInContext_ReturnsError() {
	err := s.repo.SetKitImagePath(s.T().Context(), "ks1", "kit1", "p")

	s.Require().Error(err)
}

func (s *KeycapSetRepositorySuite) TestSetKitImagePath_AlsoClearsCachedGetURL() {
	// A cached GET URL was signed against the old ImagePath, so it must
	// not outlive it - see SetKitImagePath's doc comment.
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.SetKitImagePath(ctx, "ks1", "kit1", "keycap-sets/alice/ks1/kits/kit1/image/up1")
	s.Require().NoError(err)

	s.Contains(*captured.UpdateExpression, "REMOVE", "expected a REMOVE clause")
	names := make(map[string]bool, len(captured.ExpressionAttributeNames))
	for _, v := range captured.ExpressionAttributeNames {
		names[v] = true
	}
	s.True(names["get_url"], "get_url should be REMOVEd")
	s.True(names["get_url_expires_at"], "get_url_expires_at should be REMOVEd")
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_Succeeds() {
	imagePath := "keycap-sets/alice/ks1/kits/kit1/image/up1"
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.UpdateExpression, "REMOVE") &&
				in.ReturnValues == types.ReturnValueAllOld
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", &imagePath)}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	cleared, err := s.repo.ClearKitImagePath(ctx, "ks1", "kit1")

	s.Require().NoError(err)
	s.Require().NotNil(cleared)
	s.Equal(repository.KeycapKitImageKey(imagePath), *cleared)
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_AlreadyAbsent_ReturnsNilWithoutError() {
	// The REMOVE still runs; ALL_OLD without image_path means nothing was set.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	cleared, err := s.repo.ClearKitImagePath(ctx, "ks1", "kit1")

	s.Require().NoError(err)
	s.Nil(cleared)
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_KitOrSetNotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	cleared, err := s.repo.ClearKitImagePath(ctx, "ks1", "missing-kit")

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(cleared)
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	cleared, err := s.repo.ClearKitImagePath(ctx, "ks1", "kit1")

	s.Require().Error(err)
	s.Nil(cleared)
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_NoUserIDInContext_ReturnsError() {
	cleared, err := s.repo.ClearKitImagePath(s.T().Context(), "ks1", "kit1")

	s.Require().Error(err)
	s.Nil(cleared)
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_AlsoClearsCachedGetURL() {
	// A cached GET URL is meaningless once ImagePath is gone - see
	// ClearKitImagePath's doc comment.
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.itemWithKit("Base", nil)}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.ClearKitImagePath(ctx, "ks1", "kit1")
	s.Require().NoError(err)

	names := make(map[string]bool, len(captured.ExpressionAttributeNames))
	for _, v := range captured.ExpressionAttributeNames {
		names[v] = true
	}
	s.True(names["get_url"], "get_url should be REMOVEd")
	s.True(names["get_url_expires_at"], "get_url_expires_at should be REMOVEd")
}

func (s *KeycapSetRepositorySuite) TestSetKitImageGetCache_PathMatches_WritesURLAndReturnsTrue() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	expiresAt := time.Now().Add(24 * time.Hour)
	ok, err := s.repo.SetKitImageGetCache(s.T().Context(), "alice", "ks1", "kit1", "keycap-sets/alice/ks1/kits/kit1/image/up1", "https://example.com/presigned", expiresAt)

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", captured.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.Equal("ks1", captured.Key["id"].(*types.AttributeValueMemberS).Value)
	s.Contains(*captured.ConditionExpression, "attribute_exists")
}

func (s *KeycapSetRepositorySuite) TestSetKitImageGetCache_PathChanged_ReturnsFalseWithoutError() {
	// kits.<id>.image_path no longer equals forPath (or the kit itself is
	// gone) - a concurrent SetKitImagePath/ClearKitImagePath raced this
	// write, so the condition fails and it's not an error - the caller's
	// freshly-minted URL is still returned to the requester, just not
	// persisted.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ok, err := s.repo.SetKitImageGetCache(s.T().Context(), "alice", "ks1", "kit1", "keycap-sets/alice/ks1/kits/kit1/image/up1", "https://example.com/presigned", time.Now().Add(24*time.Hour))

	s.Require().NoError(err)
	s.False(ok)
}

func (s *KeycapSetRepositorySuite) TestSetKitImageGetCache_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ok, err := s.repo.SetKitImageGetCache(s.T().Context(), "alice", "ks1", "kit1", "keycap-sets/alice/ks1/kits/kit1/image/up1", "https://example.com/presigned", time.Now().Add(24*time.Hour))

	s.Require().Error(err)
	s.False(ok)
}

// namesPrimaryKitID reports whether "primary_kit_id" was referenced
// anywhere in an UpdateItem's ExpressionAttributeNames (placeholder -> real
// name), i.e. whether the expression touches it at all.
func namesPrimaryKitID(m map[string]string) bool {
	for _, val := range m {
		if val == "primary_kit_id" {
			return true
		}
	}
	return false
}

func (s *KeycapSetRepositorySuite) TestSetKitImagePath_MarksPending() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	s.Require().NoError(s.repo.SetKitImagePath(ctx, "set1", "kit1", "keycap-sets/alice/set1/kits/kit1/image/up1"))

	s.True(setsAttr(captured, "image_status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "pending"))
	s.True(removesAttr(captured, "image_rejection_reason"))
}

func (s *KeycapSetRepositorySuite) TestClearKitImagePath_ClearsStatus() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.ClearKitImagePath(ctx, "set1", "kit1")
	s.Require().NoError(err)

	s.True(removesAttr(captured, "image_status"))
	s.True(removesAttr(captured, "image_rejection_reason"))
}

func (s *KeycapSetRepositorySuite) TestSetKitImageStatus_ConditionedOnPathAndStatus() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	reason := "unprocessable"
	ok, err := s.repo.SetKitImageStatus(s.T().Context(), "alice", "set1", "kit1", "keycap-sets/alice/set1/kits/kit1/image/up1", repository.ImageStatusPending, repository.ImageStatusRejected, &reason)

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", captured.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.Equal("set1", captured.Key["id"].(*types.AttributeValueMemberS).Value)
	s.True(names(captured.ExpressionAttributeNames, "kits", "kit1", "image_path", "image_status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "keycap-sets/alice/set1/kits/kit1/image/up1"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "pending"))
	s.True(setsAttr(captured, "image_status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "rejected"))
	s.True(setsAttr(captured, "image_rejection_reason"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "unprocessable"))
}

func (s *KeycapSetRepositorySuite) TestSetKitImageStatus_ConditionFails_ReturnsFalseWithoutError() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ok, err := s.repo.SetKitImageStatus(s.T().Context(), "alice", "set1", "kit1", "keycap-sets/alice/set1/kits/kit1/image/up1", repository.ImageStatusPending, repository.ImageStatusApproved, nil)

	s.Require().NoError(err)
	s.False(ok)
}

func (s *KeycapSetRepositorySuite) TestSetKitImageStatus_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ok, err := s.repo.SetKitImageStatus(s.T().Context(), "alice", "set1", "kit1", "keycap-sets/alice/set1/kits/kit1/image/up1", repository.ImageStatusPending, repository.ImageStatusApproved, nil)

	s.Require().Error(err)
	s.False(ok)
}
