package dynamo

import (
	"errors"
	"fmt"
	"strconv"
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

type KeyboardRepositorySuite struct {
	suite.Suite

	mockClient *mocks.MockDynamoAPI
	repo       *KeyboardRepository
}

func TestKeyboardRepositorySuite(t *testing.T) {
	suite.Run(t, new(KeyboardRepositorySuite))
}

func (s *KeyboardRepositorySuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
	s.repo = &KeyboardRepository{client: s.mockClient, tableName: "keyboard-table"}
}

func (s *KeyboardRepositorySuite) TestList_Succeeds() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return len(in.ExclusiveStartKey) == 0 && *in.Limit == 20
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"user_id": &types.AttributeValueMemberS{Value: "alice"},
					"id":      &types.AttributeValueMemberS{Value: "kb1"},
					"brand":   &types.AttributeValueMemberS{Value: "Keychron"},
				},
			},
		}, nil)

	keyboards, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().NoError(err)
	s.Empty(next)
	s.Require().Len(keyboards, 1)
	s.Equal("kb1", keyboards[0].ID)
	s.Equal("Keychron", keyboards[0].Brand)
}

func (s *KeyboardRepositorySuite) TestList_EmptyVisibilities_ReturnsEmptySliceWithoutQuerying() {
	// No EXPECT() on s.mockClient.Query - an empty visibilities slice must
	// short-circuit before building a Query, since expression.In(...)
	// requires at least one value and would otherwise panic.
	keyboards, next, err := s.repo.List(s.T().Context(), "alice", nil, 20, "")

	s.Require().NoError(err)
	s.NotNil(keyboards)
	s.Empty(keyboards)
	s.Empty(next)
}

func (s *KeyboardRepositorySuite) TestList_EmptyResult_ReturnsEmptySliceNotNil() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)

	keyboards, _, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().NoError(err)
	s.NotNil(keyboards)
	s.Empty(keyboards)
}

func (s *KeyboardRepositorySuite) TestList_ReturnsEncodedCursor_WhenMorePagesExist() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id": &types.AttributeValueMemberS{Value: "alice"},
				"id":      &types.AttributeValueMemberS{Value: "kb1"},
			},
		}, nil)

	_, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().NoError(err)
	s.NotEmpty(next)
}

func (s *KeyboardRepositorySuite) TestList_DecodesCursor_IntoExclusiveStartKey() {
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
				"id":      &types.AttributeValueMemberS{Value: "kb1"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")
	s.Require().NoError(err)

	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			key, ok := in.ExclusiveStartKey["id"].(*types.AttributeValueMemberS)
			return ok && key.Value == "kb1"
		})).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil).Once()

	_, _, err = s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, cursor)
	s.Require().NoError(err)
}

func (s *KeyboardRepositorySuite) TestList_InvalidCursor_ReturnsErrInvalidCursor() {
	keyboards, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "not-valid-base64!!")

	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
	s.Nil(keyboards)
	s.Empty(next)
}

func (s *KeyboardRepositorySuite) TestList_QueryError_Propagates() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	keyboards, next, err := s.repo.List(s.T().Context(), "alice",
		[]repository.Visibility{repository.VisibilityPublic}, 20, "")

	s.Require().Error(err)
	s.Nil(keyboards)
	s.Empty(next)
}

func (s *KeyboardRepositorySuite) TestGet_Succeeds() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.GetItemInput) bool {
			userID, ok := in.Key["user_id"].(*types.AttributeValueMemberS)
			id, ok2 := in.Key["id"].(*types.AttributeValueMemberS)
			return ok && ok2 && userID.Value == "alice" && id.Value == "kb1"
		})).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"user_id": &types.AttributeValueMemberS{Value: "alice"},
				"id":      &types.AttributeValueMemberS{Value: "kb1"},
				"brand":   &types.AttributeValueMemberS{Value: "Keychron"},
			},
		}, nil)

	kb, err := s.repo.Get(s.T().Context(), "alice", "kb1")

	s.Require().NoError(err)
	s.Equal("kb1", kb.ID)
	s.Equal("Keychron", kb.Brand)
}

func (s *KeyboardRepositorySuite) TestGet_NotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	kb, err := s.repo.Get(s.T().Context(), "alice", "missing")

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestGet_GetItemError_Propagates() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	kb, err := s.repo.Get(s.T().Context(), "alice", "kb1")

	s.Require().Error(err)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestCreate_Succeeds() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.Anything).
		Return(&dynamodb.PutItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Create(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().NoError(err)
	s.Equal(&repository.Keyboard{UserID: "alice", ID: "kb1", Brand: "Keychron"}, kb)
}

func (s *KeyboardRepositorySuite) TestCreate_SeedsEmptyPartAndImageMaps() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.PutItemInput) bool {
			for _, attr := range []string{"images", "plates", "pcbs"} {
				m, ok := in.Item[attr].(*types.AttributeValueMemberM)
				if !ok || len(m.Value) != 0 {
					return false
				}
			}
			return true
		})).
		Return(&dynamodb.PutItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.Create(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().NoError(err)
}

func (s *KeyboardRepositorySuite) TestCreate_AlreadyExists_ReturnsErrAlreadyExists() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Create(ctx, repository.Keyboard{ID: "kb1"})

	s.Require().ErrorIs(err, repository.ErrAlreadyExists)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestCreate_PutItemError_Propagates() {
	s.mockClient.EXPECT().
		PutItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Create(ctx, repository.Keyboard{ID: "kb1"})

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrAlreadyExists)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestCreate_NoUserIDInContext_ReturnsError() {
	// No EXPECT() on PutItem - see repository.ErrNoUserID.
	kb, err := s.repo.Create(s.T().Context(), repository.Keyboard{ID: "kb1"})

	s.Require().Error(err)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestUpdate_Succeeds() {
	// No GetItem on the happy path - images carries forward by
	// not being named.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			key := in.Key["id"].(*types.AttributeValueMemberS)
			return key.Value == "kb1" &&
				strings.Contains(*in.ConditionExpression, "attribute_exists") &&
				in.ReturnValues == types.ReturnValueAllNew
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.updatedItem()}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Update(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().NoError(err)
	s.Equal("Keychron", kb.Brand)
}

func (s *KeyboardRepositorySuite) TestDeleteImage_DottedImageID_IsOneKeyNotANestedField() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			has := map[string]bool{}
			for _, n := range in.ExpressionAttributeNames {
				has[n] = true
			}
			return has["images"] && has["img1.path"] && !has["path"]
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.DeleteImage(ctx, "kb1", "img1.path")

	s.Require().NoError(err)
}

func (s *KeyboardRepositorySuite) TestBracketedImageID_NeverReachesDynamo() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")

	key, err := s.repo.DeleteImage(ctx, "kb1", "img1[0]")
	s.Require().NoError(err)
	s.Nil(key)
}

func (s *KeyboardRepositorySuite) TestUpdate_LeavesPartsUntouched() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			for _, name := range in.ExpressionAttributeNames {
				if name == "plates" || name == "pcbs" || name == "images" {
					return false
				}
			}
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.updatedItem()}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.Update(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().NoError(err)
}

func (s *KeyboardRepositorySuite) TestUpdate_OmittedOptionalFields_AreRemoved() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			// size/layout/notes are nil on the input, so each is a REMOVE.
			return strings.Contains(*in.UpdateExpression, "REMOVE")
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: s.updatedItem()}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.Update(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().NoError(err)
}

func (s *KeyboardRepositorySuite) TestUpdate_ReturnsPersistedImages() {
	after := s.updatedItem()
	after["images"] = &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"img1": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"path": &types.AttributeValueMemberS{Value: "keyboards/alice/kb1/images/img1"},
			"seq":  &types.AttributeValueMemberN{Value: "0"},
		}},
	}}
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: after}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Update(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().NoError(err)
	s.Require().Len(kb.Images, 1)
	s.Equal(repository.KeyboardImageKey("keyboards/alice/kb1/images/img1"), kb.Images["img1"].Path)
}

func (s *KeyboardRepositorySuite) TestUpdate_ConditionFails_ReturnsErrNotFound() {
	// id is the sort key, so a failed attribute_exists(id) can only mean the
	// keyboard is gone.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Update(ctx, repository.Keyboard{ID: "kb1", Brand: "Keychron"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestUpdate_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	kb, err := s.repo.Update(ctx, repository.Keyboard{ID: "kb1"})

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrNotFound)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestUpdate_NoUserIDInContext_ReturnsError() {
	// No EXPECT() on UpdateItem - see repository.ErrNoUserID.
	kb, err := s.repo.Update(s.T().Context(), repository.Keyboard{ID: "kb1"})

	s.Require().Error(err)
	s.Nil(kb)
}

func (s *KeyboardRepositorySuite) TestDelete_Succeeds() {
	s.mockClient.EXPECT().
		DeleteItem(mock.Anything, mock.Anything).
		Return(&dynamodb.DeleteItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.Delete(ctx, "kb1")

	s.Require().NoError(err)
}

func (s *KeyboardRepositorySuite) TestDelete_NoUserIDInContext_ReturnsError() {
	// No EXPECT() on DeleteItem - see repository.ErrNoUserID.
	err := s.repo.Delete(s.T().Context(), "kb1")

	s.Require().Error(err)
}

func (s *KeyboardRepositorySuite) TestDelete_DeleteItemError_Propagates() {
	s.mockClient.EXPECT().
		DeleteItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.Delete(ctx, "kb1")

	s.Require().Error(err)
}

// updatedItem is a stand-in for an UpdateItem ALL_NEW response.
func (s *KeyboardRepositorySuite) updatedItem() map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"user_id": &types.AttributeValueMemberS{Value: "alice"},
		"id":      &types.AttributeValueMemberS{Value: "kb1"},
		"brand":   &types.AttributeValueMemberS{Value: "Keychron"},
	}
}

func (s *KeyboardRepositorySuite) TestAddImage_Succeeds_SetsMonotonicSeq() {
	// One UpdateItem, no read. The entry's seq is a wall-clock nanosecond
	// stamp taken just now.
	before := time.Now().UnixNano()
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return strings.Contains(*in.ConditionExpression, "attribute_exists") &&
				strings.Contains(*in.ConditionExpression, "attribute_not_exists") &&
				strings.Contains(*in.ConditionExpression, "size (") &&
				in.ReturnValuesOnConditionCheckFailure == types.ReturnValuesOnConditionCheckFailureAllOld
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.AddImage(ctx, "kb1", repository.KeyboardImage{ImageID: "img2", Path: "keyboards/alice/kb1/images/img2"})
	s.Require().NoError(err)

	var entry *types.AttributeValueMemberM
	for _, v := range captured.ExpressionAttributeValues {
		if m, ok := v.(*types.AttributeValueMemberM); ok {
			entry = m
		}
	}
	s.Require().NotNil(entry)
	seq, convErr := strconv.ParseInt(entry.Value["seq"].(*types.AttributeValueMemberN).Value, 10, 64)
	s.Require().NoError(convErr)
	s.GreaterOrEqual(seq, before)
	s.LessOrEqual(seq, time.Now().UnixNano())
	s.Equal("pending", entry.Value["status"].(*types.AttributeValueMemberS).Value)
}

func (s *KeyboardRepositorySuite) TestAddImage_ParentKeyboardNotFound_ReturnsErrNotFound() {
	// attribute_exists(id) fails; the returned item is empty.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.AddImage(ctx, "kb1", repository.KeyboardImage{ImageID: "img1", Path: "p"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *KeyboardRepositorySuite) TestAddImage_DuplicateImageID_ReturnsError() {
	// attribute_not_exists(images.img1) fails; the returned item exists and
	// is under the cap, so the id is a duplicate.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{Item: s.storedKeyboard().Item})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.AddImage(ctx, "kb1", repository.KeyboardImage{ImageID: "img1", Path: "p2"})

	s.Require().ErrorIs(err, errDuplicateKeyboardImageID)
}

func (s *KeyboardRepositorySuite) TestAddImage_AtImageLimit_ReturnsErrImageLimitReached() {
	// size(images) < MaxImagesPerItem fails; the returned item is at the cap.
	full := map[string]types.AttributeValue{}
	for i := range repository.MaxImagesPerItem {
		full[fmt.Sprintf("img%d", i)] = &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}}
	}
	item := s.storedKeyboard().Item
	item["images"] = &types.AttributeValueMemberM{Value: full}
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{Item: item})

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.AddImage(ctx, "kb1", repository.KeyboardImage{ImageID: "new", Path: "p"})

	s.Require().ErrorIs(err, repository.ErrImageLimitReached)
}

func (s *KeyboardRepositorySuite) TestAddImage_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	err := s.repo.AddImage(ctx, "kb1", repository.KeyboardImage{ImageID: "img1", Path: "p"})

	s.Require().Error(err)
}

func (s *KeyboardRepositorySuite) TestAddImage_EmptyImageID_ReturnsError() {
	// No EXPECT() - caught before any client call.
	err := s.repo.AddImage(kbdbctx.WithUserID(s.T().Context(), "alice"), "kb1", repository.KeyboardImage{Path: "p"})

	s.Require().Error(err)
}

func (s *KeyboardRepositorySuite) TestAddImage_NoUserIDInContext_ReturnsError() {
	// No EXPECT() - see repository.ErrNoUserID.
	err := s.repo.AddImage(s.T().Context(), "kb1", repository.KeyboardImage{ImageID: "img1", Path: "p"})

	s.Require().Error(err)
}

func (s *KeyboardRepositorySuite) TestDeleteImage_Succeeds() {
	// REMOVE images.<id>, ALL_OLD returns the pre-image whose images.<id>.path
	// is the removed key.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			return strings.Contains(*in.UpdateExpression, "REMOVE") &&
				in.ReturnValues == types.ReturnValueAllOld
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: map[string]types.AttributeValue{
			"images": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"img1": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"path": &types.AttributeValueMemberS{Value: "keyboards/alice/kb1/images/img1"},
					"seq":  &types.AttributeValueMemberN{Value: "0"},
				}},
			}},
		}}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	removed, err := s.repo.DeleteImage(ctx, "kb1", "img1")

	s.Require().NoError(err)
	s.Require().NotNil(removed)
	s.Equal(repository.KeyboardImageKey("keyboards/alice/kb1/images/img1"), *removed)
}

func (s *KeyboardRepositorySuite) TestDeleteImage_ImageAlreadyAbsent_ReturnsNilWithoutError() {
	// Condition failed but the keyboard exists -> the image id simply wasn't
	// there. Idempotent nil-nil.
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(s.storedKeyboard(), nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	removed, err := s.repo.DeleteImage(ctx, "kb1", "missing")

	s.Require().NoError(err)
	s.Nil(removed)
}

func (s *KeyboardRepositorySuite) TestDeleteImage_ParentKeyboardNotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	removed, err := s.repo.DeleteImage(ctx, "kb1", "img1")

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(removed)
}

func (s *KeyboardRepositorySuite) TestDeleteImage_NoUserIDInContext_ReturnsError() {
	// No EXPECT() - see repository.ErrNoUserID.
	removed, err := s.repo.DeleteImage(s.T().Context(), "kb1", "img1")

	s.Require().Error(err)
	s.Nil(removed)
}

// storedKeyboard is a GetItemOutput for the classify path's existence check.
func (s *KeyboardRepositorySuite) storedKeyboard() *dynamodb.GetItemOutput {
	return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
		"user_id": &types.AttributeValueMemberS{Value: "alice"},
		"id":      &types.AttributeValueMemberS{Value: "kb1"},
		"brand":   &types.AttributeValueMemberS{Value: "Keychron"},
	}}
}

func (s *KeyboardRepositorySuite) TestSetImageStatus_ConditionedOnPathAndStatus() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	reason := "unprocessable"
	ok, err := s.repo.SetImageStatus(s.T().Context(), "alice", "kb1", "img1", "keyboards/alice/kb1/images/img1", repository.ImageStatusPending, repository.ImageStatusRejected, &reason)

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", captured.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.Equal("kb1", captured.Key["id"].(*types.AttributeValueMemberS).Value)
	s.True(names(captured.ExpressionAttributeNames, "images", "img1", "path", "status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "keyboards/alice/kb1/images/img1"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "pending"))
	s.NotContains(*captured.ConditionExpression, "attribute_not_exists", "only approved may match a missing status")
	s.True(setsAttr(captured, "status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "rejected"))
	s.True(setsAttr(captured, "rejection_reason"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "unprocessable"))
}

func (s *KeyboardRepositorySuite) TestSetImageStatus_NoReason_RemovesRejectionReason() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ok, err := s.repo.SetImageStatus(s.T().Context(), "alice", "kb1", "img1", "keyboards/alice/kb1/images/img1", repository.ImageStatusRejected, repository.ImageStatusApproved, nil)

	s.Require().NoError(err)
	s.True(ok)
	s.True(removesAttr(captured, "rejection_reason"))
}

func (s *KeyboardRepositorySuite) TestSetImageStatus_ConditionFails_ReturnsFalseWithoutError() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ok, err := s.repo.SetImageStatus(s.T().Context(), "alice", "kb1", "img1", "keyboards/alice/kb1/images/img1", repository.ImageStatusPending, repository.ImageStatusApproved, nil)

	s.Require().NoError(err)
	s.False(ok)
}

func (s *KeyboardRepositorySuite) TestSetImageStatus_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ok, err := s.repo.SetImageStatus(s.T().Context(), "alice", "kb1", "img1", "keyboards/alice/kb1/images/img1", repository.ImageStatusPending, repository.ImageStatusApproved, nil)

	s.Require().Error(err)
	s.False(ok)
}
