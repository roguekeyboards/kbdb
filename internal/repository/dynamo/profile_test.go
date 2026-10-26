package dynamo

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/rogueserenity/kbdb/internal/ctx"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/dynamo/mocks"
)

type ProfileRepositorySuite struct {
	suite.Suite

	mockClient *mocks.MockDynamoAPI
	repo       *ProfileRepository
}

func TestProfileRepositorySuite(t *testing.T) {
	suite.Run(t, new(ProfileRepositorySuite))
}

func (s *ProfileRepositorySuite) SetupTest() {
	s.mockClient = mocks.NewMockDynamoAPI(s.T())
	s.repo = &ProfileRepository{
		client:            s.mockClient,
		profileTableName:  "profile-table",
		usernameTableName: "profile-username-table",
	}
}

func (s *ProfileRepositorySuite) TestGet_Succeeds() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.GetItemInput) bool {
			userID, ok := in.Key["user_id"].(*types.AttributeValueMemberS)
			return *in.TableName == "profile-table" && ok && userID.Value == "user-alice" && len(in.Key) == 1
		})).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"user_id":      &types.AttributeValueMemberS{Value: "user-alice"},
				"username":     &types.AttributeValueMemberS{Value: "alice"},
				"discoverable": &types.AttributeValueMemberBOOL{Value: true},
				"bio":          &types.AttributeValueMemberS{Value: "keebs enjoyer"},
			},
		}, nil)

	p, err := s.repo.Get(s.T().Context(), "user-alice")

	s.Require().NoError(err)
	s.Equal("user-alice", p.OwnerID)
	s.Equal("alice", p.Username)
	s.True(p.Discoverable)
	s.Require().NotNil(p.Bio)
	s.Equal("keebs enjoyer", *p.Bio)
}

func (s *ProfileRepositorySuite) TestGet_NoStoredPreferences_DefaultsApplied() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"user_id":  &types.AttributeValueMemberS{Value: "user-alice"},
				"username": &types.AttributeValueMemberS{Value: "alice"},
			},
		}, nil)

	p, err := s.repo.Get(s.T().Context(), "user-alice")

	s.Require().NoError(err)
	s.Equal(repository.DefaultProfilePreferences(), p.Preferences)
}

func (s *ProfileRepositorySuite) TestGet_StoredPreferences_NotOverwritten() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"user_id":  &types.AttributeValueMemberS{Value: "user-alice"},
				"username": &types.AttributeValueMemberS{Value: "alice"},
				"preferences": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"currency":             &types.AttributeValueMemberS{Value: "EUR"},
					"show_price_to_me":     &types.AttributeValueMemberBOOL{Value: false},
					"show_price_to_others": &types.AttributeValueMemberBOOL{Value: true},
				}},
			},
		}, nil)

	p, err := s.repo.Get(s.T().Context(), "user-alice")

	s.Require().NoError(err)
	s.Equal(repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true}, p.Preferences)
}

func (s *ProfileRepositorySuite) TestGet_NotFound_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	_, err := s.repo.Get(s.T().Context(), "user-nobody")

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestGet_GetItemError_Propagates() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("boom"))

	_, err := s.repo.Get(s.T().Context(), "user-alice")

	s.Require().Error(err)
	s.NotErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestGetPreferences_ProfileExists_ReturnsStoredPreferences() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"user_id":  &types.AttributeValueMemberS{Value: "user-alice"},
				"username": &types.AttributeValueMemberS{Value: "alice"},
				"preferences": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"currency":             &types.AttributeValueMemberS{Value: "EUR"},
					"show_price_to_me":     &types.AttributeValueMemberBOOL{Value: false},
					"show_price_to_others": &types.AttributeValueMemberBOOL{Value: true},
				}},
			},
		}, nil)

	prefs, err := s.repo.GetPreferences(s.T().Context(), "user-alice")

	s.Require().NoError(err)
	s.Equal(repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true}, prefs)
}

func (s *ProfileRepositorySuite) TestGetPreferences_NoProfile_ReturnsDefaults() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	prefs, err := s.repo.GetPreferences(s.T().Context(), "user-nobody")

	s.Require().NoError(err)
	s.Equal(repository.DefaultProfilePreferences(), prefs)
}

func (s *ProfileRepositorySuite) TestGetPreferences_GetItemError_Propagates() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("boom"))

	_, err := s.repo.GetPreferences(s.T().Context(), "user-alice")

	s.Require().Error(err)
	s.NotErrorIs(err, repository.ErrNotFound)
}

// unmarshalProfileItem decodes the profile Put's item (TransactItems[0])
// back into a repository.Profile for assertions on the derived attributes.
func (s *ProfileRepositorySuite) unmarshalProfileItem(in *dynamodb.TransactWriteItemsInput) repository.Profile {
	s.T().Helper()
	var p repository.Profile
	s.Require().NoError(attributevalue.UnmarshalMap(in.TransactItems[0].Put.Item, &p))
	return p
}

func (s *ProfileRepositorySuite) TestCreate_NoUserID_ReturnsErrNoUserID() {
	_, err := s.repo.Create(s.T().Context(), repository.Profile{Username: "alice"})

	s.Require().ErrorIs(err, repository.ErrNoUserID)
}

func (s *ProfileRepositorySuite) TestCreate_WritesProfileAndClaimAtomically() {
	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return len(in.TransactItems) == 2 &&
				in.TransactItems[0].Put != nil && in.TransactItems[1].Put != nil
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	got, err := s.repo.Create(ctx, repository.Profile{Username: "alice"})

	s.Require().NoError(err)
	s.Equal("user-alice", got.OwnerID)

	profilePut := captured.TransactItems[0].Put
	s.Equal("profile-table", *profilePut.TableName)
	s.Equal("attribute_not_exists(user_id)", *profilePut.ConditionExpression)

	claimPut := captured.TransactItems[1].Put
	s.Equal("profile-username-table", *claimPut.TableName)
	s.Equal("attribute_not_exists(username)", *claimPut.ConditionExpression)
	s.Equal("alice", claimPut.Item["username"].(*types.AttributeValueMemberS).Value)
	s.Equal("user-alice", claimPut.Item["user_id"].(*types.AttributeValueMemberS).Value)
}

func (s *ProfileRepositorySuite) TestCreate_NonDiscoverable_OmitsAllGSIKeys() {
	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	discord := "alice_kb"
	_, err := s.repo.Create(ctx, repository.Profile{
		Username: "alice", Discoverable: false, DiscordUsername: &discord,
	})
	s.Require().NoError(err)

	item := captured.TransactItems[0].Put.Item
	s.NotContains(item, "discoverable_pk")
	s.NotContains(item, "discord_pk")
}

func (s *ProfileRepositorySuite) TestCreate_DiscoverableNoDiscord_SetsOnlyDiscoverablePK() {
	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	_, err := s.repo.Create(ctx, repository.Profile{Username: "alice", Discoverable: true})
	s.Require().NoError(err)

	p := s.unmarshalProfileItem(captured)
	s.Require().NotNil(p.DiscoverablePK)
	s.Equal("1", *p.DiscoverablePK)
	s.Nil(p.DiscordPK)
}

func (s *ProfileRepositorySuite) TestCreate_DiscoverableWithDiscord_SetsAllGSIKeys() {
	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	discord := "alice_kb"
	_, err := s.repo.Create(ctx, repository.Profile{
		Username: "alice", Discoverable: true, DiscordUsername: &discord,
	})
	s.Require().NoError(err)

	p := s.unmarshalProfileItem(captured)
	s.Require().NotNil(p.DiscoverablePK)
	s.Equal("1", *p.DiscoverablePK)
	s.Require().NotNil(p.DiscordPK)
	s.Equal("1", *p.DiscordPK)
	s.Require().NotNil(p.DiscordUsername)
	s.Equal("alice_kb", *p.DiscordUsername)
}

func (s *ProfileRepositorySuite) TestCreate_ProfilePutConditionFails_ReturnsErrAlreadyExists() {
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("ConditionalCheckFailed")},
				{Code: aws.String("None")},
			},
		})

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	_, err := s.repo.Create(ctx, repository.Profile{Username: "alice"})

	s.Require().ErrorIs(err, repository.ErrAlreadyExists)
	s.NotErrorIs(err, repository.ErrUsernameTaken)
}

func (s *ProfileRepositorySuite) TestCreate_ClaimPutConditionFails_ReturnsErrUsernameTaken() {
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("None")},
				{Code: aws.String("ConditionalCheckFailed")},
			},
		})

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	_, err := s.repo.Create(ctx, repository.Profile{Username: "alice"})

	s.Require().ErrorIs(err, repository.ErrUsernameTaken)
	s.NotErrorIs(err, repository.ErrAlreadyExists)
}

func (s *ProfileRepositorySuite) TestCreate_TransactionConflict_ReturnsErrMutationConflict() {
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("TransactionConflict")},
				{Code: aws.String("None")},
			},
		})

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	_, err := s.repo.Create(ctx, repository.Profile{Username: "alice"})

	s.Require().ErrorIs(err, repository.ErrMutationConflict)
}

func (s *ProfileRepositorySuite) TestCreate_TransactWriteItemsError_Propagates() {
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	_, err := s.repo.Create(ctx, repository.Profile{Username: "alice"})

	s.Require().Error(err)
	s.Require().NotErrorIs(err, repository.ErrAlreadyExists)
	s.Require().NotErrorIs(err, repository.ErrUsernameTaken)
}

// storedProfile is a GetItemOutput for the caller's existing profile;
// Update reads it once to learn the current username and branch. extra
// overlays or adds attributes.
func storedProfile(extra map[string]types.AttributeValue) *dynamodb.GetItemOutput {
	item := map[string]types.AttributeValue{
		"user_id":      &types.AttributeValueMemberS{Value: "user-alice"},
		"username":     &types.AttributeValueMemberS{Value: "alice"},
		"discoverable": &types.AttributeValueMemberBOOL{Value: true},
	}
	maps.Copy(item, extra)
	return &dynamodb.GetItemOutput{Item: item}
}

func (s *ProfileRepositorySuite) updateCtx() context.Context {
	return kbdbctx.WithUserID(s.T().Context(), "user-alice")
}

// updatedProfileAttrs is a stand-in for an UpdateItem ALL_NEW response.
func updatedProfileAttrs(extra map[string]types.AttributeValue) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"user_id":      &types.AttributeValueMemberS{Value: "user-alice"},
		"username":     &types.AttributeValueMemberS{Value: "alice"},
		"discoverable": &types.AttributeValueMemberBOOL{Value: true},
	}
	maps.Copy(item, extra)
	return item
}

func (s *ProfileRepositorySuite) TestUpdate_NoUserID_ReturnsErrNoUserID() {
	_, err := s.repo.Update(s.T().Context(), repository.Profile{Username: "alice"})

	s.Require().ErrorIs(err, repository.ErrNoUserID)
}

func (s *ProfileRepositorySuite) TestUpdate_SameUsername_OneUpdateItem_NoTransaction() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)

	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: updatedProfileAttrs(map[string]types.AttributeValue{
			"bio": &types.AttributeValueMemberS{Value: "updated"},
		})}, nil)
	// No EXPECT() on TransactWriteItems - a same-username Update is a single
	// in-place write.

	newBio := "updated"
	got, err := s.repo.Update(s.updateCtx(), repository.Profile{
		Username: "alice", Discoverable: true, Bio: &newBio,
	})

	s.Require().NoError(err)
	s.Equal("profile-table", *captured.TableName)
	s.Equal("user-alice", captured.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.Contains(*captured.ConditionExpression, "attribute_exists")
	s.Require().NotNil(got.Bio)
	s.Equal("updated", *got.Bio)
}

func (s *ProfileRepositorySuite) TestUpdate_SetsPreferences() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)

	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: updatedProfileAttrs(nil)}, nil)

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{
		Username:    "alice",
		Preferences: repository.ProfilePreferences{Currency: "EUR", ShowPriceToMe: false, ShowPriceToOthers: true},
	})
	s.Require().NoError(err)

	found := false
	for _, name := range captured.ExpressionAttributeNames {
		if name == "preferences" {
			found = true
		}
	}
	s.True(found, "preferences should be named in the update expression")
}

func (s *ProfileRepositorySuite) TestUpdate_OmittingBioAndLinks_RemovesThem() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(map[string]types.AttributeValue{
			"bio": &types.AttributeValueMemberS{Value: "old bio"},
		}), nil)

	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: updatedProfileAttrs(nil)}, nil)

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice", Discoverable: true})
	s.Require().NoError(err)

	// bio and links are absent from the body -> REMOVE clauses.
	s.True(removesAttr(captured, "bio"), "bio should be REMOVEd")
	s.True(removesAttr(captured, "links"), "links should be REMOVEd")
}

// removesAttr reports whether in's UpdateExpression REMOVEs the given
// attribute name (resolving the #placeholder via ExpressionAttributeNames).
func removesAttr(in *dynamodb.UpdateItemInput, attr string) bool {
	expr := *in.UpdateExpression
	remove := ""
	if i := strings.Index(expr, "REMOVE "); i >= 0 {
		remove = expr[i:]
		if j := strings.Index(remove, "SET "); j >= 0 {
			remove = remove[:j]
		}
	}
	for placeholder, name := range in.ExpressionAttributeNames {
		if name == attr && strings.Contains(remove, placeholder) {
			return true
		}
	}
	return false
}

// setsAttr is removesAttr's counterpart for the SET section.
func setsAttr(in *dynamodb.UpdateItemInput, attr string) bool {
	expr := *in.UpdateExpression
	set := ""
	if i := strings.Index(expr, "SET "); i >= 0 {
		set = expr[i:]
		if j := strings.Index(set, "REMOVE "); j >= 0 {
			set = set[:j]
		}
	}
	for placeholder, name := range in.ExpressionAttributeNames {
		if name == attr && strings.Contains(set, placeholder) {
			return true
		}
	}
	return false
}

func (s *ProfileRepositorySuite) TestUpdate_DoesNotNameAvatarPath_SoItCarriesForward() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)

	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: updatedProfileAttrs(map[string]types.AttributeValue{
			"avatar_path": &types.AttributeValueMemberS{Value: "profiles/user-alice/avatar/up1"},
		})}, nil)

	got, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice", Discoverable: true})
	s.Require().NoError(err)

	s.False(setsAttr(captured, "avatar_path"), "avatar_path must not be SET")
	s.False(removesAttr(captured, "avatar_path"), "avatar_path must not be REMOVEd")
	// ALL_NEW response still carries the stored avatar_path.
	s.Require().NotNil(got.AvatarPath)
	s.Equal(repository.ProfileImageKey("profiles/user-alice/avatar/up1"), *got.AvatarPath)
}

func (s *ProfileRepositorySuite) TestUpdate_FlipDiscoverableFalse_RemovesAllGSIKeys() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(map[string]types.AttributeValue{
			"discoverable_pk":  &types.AttributeValueMemberS{Value: "1"},
			"discord_pk":       &types.AttributeValueMemberS{Value: "1"},
			"discord_username": &types.AttributeValueMemberS{Value: "alice_kb"},
		}), nil)

	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: updatedProfileAttrs(nil)}, nil)

	discord := "alice_kb"
	_, err := s.repo.Update(s.updateCtx(), repository.Profile{
		Username: "alice", Discoverable: false, DiscordUsername: &discord,
	})
	s.Require().NoError(err)

	s.True(removesAttr(captured, "discoverable_pk"))
	s.True(removesAttr(captured, "discord_pk"))
}

func (s *ProfileRepositorySuite) TestUpdate_ChangedUsername_MovesClaimAtomically() {
	// Update's branch Get, then renameProfile's per-attempt Get, both still
	// "alice".
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil).Twice()

	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil)
	// Success re-reads the profile to return it.
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: "alice2"},
		}), nil).Once()

	got, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice2", Discoverable: true})
	s.Require().NoError(err)
	s.Equal("alice2", got.Username)

	s.Require().Len(captured.TransactItems, 3)

	// Item 0 is an Update action on the profile item, not a Put.
	profileUpdate := captured.TransactItems[0].Update
	s.Require().NotNil(profileUpdate)
	s.Nil(captured.TransactItems[0].Put)
	s.Equal("profile-table", *profileUpdate.TableName)
	s.Equal("user-alice", profileUpdate.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.Contains(*profileUpdate.ConditionExpression, "attribute_exists")

	claimDel := captured.TransactItems[1].Delete
	s.Require().NotNil(claimDel)
	s.Equal("profile-username-table", *claimDel.TableName)
	s.Equal("alice", claimDel.Key["username"].(*types.AttributeValueMemberS).Value)
	// Conditioned on user_id = caller, so a reissued claim is left alone.
	ownerVal := ""
	for _, v := range claimDel.ExpressionAttributeValues {
		if sv, ok := v.(*types.AttributeValueMemberS); ok {
			ownerVal = sv.Value
		}
	}
	s.Equal("user-alice", ownerVal)

	claimPut := captured.TransactItems[2].Put
	s.Require().NotNil(claimPut)
	s.Equal("profile-username-table", *claimPut.TableName)
	s.Equal("attribute_not_exists(username)", *claimPut.ConditionExpression)
	s.Equal("alice2", claimPut.Item["username"].(*types.AttributeValueMemberS).Value)
}

func (s *ProfileRepositorySuite) TestUpdate_NoProfile_ReturnsErrNotFound() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice"})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestUpdate_SameUsername_ConditionFails_ReturnsErrNotFound() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice", Discoverable: true})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestUpdate_ChangedUsernameClaimFails_ReturnsErrUsernameTaken() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("None")},
				{Code: aws.String("None")},
				{Code: aws.String("ConditionalCheckFailed")},
			},
		})

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "taken", Discoverable: true})

	s.Require().ErrorIs(err, repository.ErrUsernameTaken)
}

func (s *ProfileRepositorySuite) TestUpdate_ChangedUsernameProfileGone_ReturnsErrNotFound() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("ConditionalCheckFailed")},
				{Code: aws.String("None")},
				{Code: aws.String("None")},
			},
		})

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice2", Discoverable: true})

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestUpdate_RenameTransactionConflict_RetriesThenSucceeds() {
	// Every pre-success Get still reads "alice"; only the final read (after
	// the transaction commits) shows "alice2".
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil).Times(3)
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{{Code: aws.String("TransactionConflict")}},
		}).Once()
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil).Once()
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: "alice2"},
		}), nil).Once()

	got, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice2", Discoverable: true})

	s.Require().NoError(err)
	s.Equal("alice2", got.Username)
}

func (s *ProfileRepositorySuite) TestUpdate_RenameClaimDeleteRaced_RetriesFromFreshRead() {
	// The caller's own "alice" claim moved out from under the claim-delete
	// (reason 1) - a concurrent rename of the same profile. Retry from a
	// fresh read that now shows the new current username.
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil).Twice()
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("None")},
				{Code: aws.String("ConditionalCheckFailed")},
				{Code: aws.String("None")},
			},
		}).Once()
	// Retry: the profile now reads as "bob"; the transaction targets that
	// claim and commits.
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: "bob"},
		}), nil).Once()
	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil).Once()
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: "carol"},
		}), nil).Once()

	got, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "carol", Discoverable: true})

	s.Require().NoError(err)
	s.Equal("carol", got.Username)
	// The retry's claim-delete keyed on the fresh username, not the stale one.
	s.Equal("bob", captured.TransactItems[1].Delete.Key["username"].(*types.AttributeValueMemberS).Value)
}

func (s *ProfileRepositorySuite) TestUpdate_RenameConflictExhaustsRetries_ReturnsErrMutationConflict() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{{Code: aws.String("TransactionConflict")}},
		})

	_, err := s.repo.Update(s.updateCtx(), repository.Profile{Username: "alice2", Discoverable: true})

	s.Require().ErrorIs(err, repository.ErrMutationConflict)
}

func (s *ProfileRepositorySuite) TestSetAvatarPath_NoUserID_ReturnsErrNoUserID() {
	err := s.repo.SetAvatarPath(s.T().Context(), "profiles/user-alice/avatar/up1")

	s.Require().ErrorIs(err, repository.ErrNoUserID)
}

func (s *ProfileRepositorySuite) TestSetAvatarPath_OneUpdateItem_NoRead() {
	// No GetItem - SetAvatarPath writes avatar_path directly under
	// attribute_exists(user_id).
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	err := s.repo.SetAvatarPath(s.updateCtx(), "profiles/user-alice/avatar/up1")
	s.Require().NoError(err)

	s.Equal("profile-table", *captured.TableName)
	s.Equal("user-alice", captured.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.True(setsAttr(captured, "avatar_path"))
	s.Contains(*captured.ConditionExpression, "attribute_exists")
}

func (s *ProfileRepositorySuite) TestSetAvatarPath_NoProfile_ReturnsErrNotFound() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	err := s.repo.SetAvatarPath(s.updateCtx(), "profiles/user-alice/avatar/up1")

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestClearAvatarPath_NoUserID_ReturnsErrNoUserID() {
	_, err := s.repo.ClearAvatarPath(s.T().Context())

	s.Require().ErrorIs(err, repository.ErrNoUserID)
}

func (s *ProfileRepositorySuite) TestClearAvatarPath_RemovesAndReturnsClearedKey() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{Attributes: map[string]types.AttributeValue{
			"avatar_path": &types.AttributeValueMemberS{Value: "profiles/user-alice/avatar/up1"},
		}}, nil)

	cleared, err := s.repo.ClearAvatarPath(s.updateCtx())
	s.Require().NoError(err)
	s.Require().NotNil(cleared)
	s.Equal(repository.ProfileImageKey("profiles/user-alice/avatar/up1"), *cleared)

	s.True(removesAttr(captured, "avatar_path"))
	s.Equal(types.ReturnValueAllOld, captured.ReturnValues)
}

func (s *ProfileRepositorySuite) TestClearAvatarPath_NoAvatarSet_ReturnsNil() {
	// The REMOVE still runs; ALL_OLD without avatar_path means nothing was set.
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{Attributes: map[string]types.AttributeValue{
			"user_id": &types.AttributeValueMemberS{Value: "user-alice"},
		}}, nil)

	cleared, err := s.repo.ClearAvatarPath(s.updateCtx())

	s.Require().NoError(err)
	s.Nil(cleared)
}

func (s *ProfileRepositorySuite) TestClearAvatarPath_NoProfile_ReturnsErrNotFound() {
	s.mockClient.EXPECT().UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	cleared, err := s.repo.ClearAvatarPath(s.updateCtx())

	s.Require().ErrorIs(err, repository.ErrNotFound)
	s.Nil(cleared)
}

func (s *ProfileRepositorySuite) TestDelete_NoUserID_ReturnsErrNoUserID() {
	err := s.repo.Delete(s.T().Context())

	s.Require().ErrorIs(err, repository.ErrNoUserID)
}

func (s *ProfileRepositorySuite) TestDelete_RemovesProfileAndClaimAtomically_BothConditioned() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)

	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	err := s.repo.Delete(s.updateCtx())

	s.Require().NoError(err)
	s.Require().Len(captured.TransactItems, 2)

	profileDel := captured.TransactItems[0].Delete
	s.Require().NotNil(profileDel)
	s.Equal("profile-table", *profileDel.TableName)
	s.Equal("user-alice", profileDel.Key["user_id"].(*types.AttributeValueMemberS).Value)
	// Guards against deleting nothing when the profile was removed under us.
	s.Require().NotNil(profileDel.ConditionExpression)
	s.Contains(*profileDel.ConditionExpression, "attribute_exists")

	claimDel := captured.TransactItems[1].Delete
	s.Require().NotNil(claimDel)
	s.Equal("profile-username-table", *claimDel.TableName)
	s.Equal("alice", claimDel.Key["username"].(*types.AttributeValueMemberS).Value)
	// Guards against deleting a claim that has since been reissued to
	// someone else (a rename or delete+recreate race).
	s.Require().NotNil(claimDel.ConditionExpression)
	ownerVal := ""
	for _, v := range claimDel.ExpressionAttributeValues {
		if sv, ok := v.(*types.AttributeValueMemberS); ok {
			ownerVal = sv.Value
		}
	}
	s.Equal("user-alice", ownerVal)
}

func (s *ProfileRepositorySuite) TestDelete_ConcurrentRename_RetriesWithFreshUsername() {
	// Attempt 1: profile still reads as "alice"; the claim-delete condition
	// fails because a rename already moved the "alice" claim.
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil).Once()
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("None")},
				{Code: aws.String("ConditionalCheckFailed")},
			},
		}).Once()

	// Attempt 2: fresh Get returns the renamed profile; delete targets the
	// new claim and succeeds.
	renamed := storedProfile(map[string]types.AttributeValue{
		"username": &types.AttributeValueMemberS{Value: "bob"},
	})
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).Return(renamed, nil).Once()

	var captured *dynamodb.TransactWriteItemsInput
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.MatchedBy(func(in *dynamodb.TransactWriteItemsInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil).Once()

	err := s.repo.Delete(s.updateCtx())

	s.Require().NoError(err)
	s.Equal("bob", captured.TransactItems[1].Delete.Key["username"].(*types.AttributeValueMemberS).Value)
}

func (s *ProfileRepositorySuite) TestDelete_TransactionConflict_Retries() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("TransactionConflict")},
				{Code: aws.String("None")},
			},
		}).Once()
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(&dynamodb.TransactWriteItemsOutput{}, nil).Once()

	err := s.repo.Delete(s.updateCtx())

	s.Require().NoError(err)
}

func (s *ProfileRepositorySuite) TestDelete_ConflictExhaustsRetries_ReturnsErrMutationConflict() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil)
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("ConditionalCheckFailed")},
				{Code: aws.String("None")},
			},
		})

	err := s.repo.Delete(s.updateCtx())

	s.Require().ErrorIs(err, repository.ErrMutationConflict)
}

func (s *ProfileRepositorySuite) TestDelete_NoProfile_IsNoOpSuccess() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	err := s.repo.Delete(s.updateCtx())

	s.Require().NoError(err)
	s.mockClient.AssertNotCalled(s.T(), "TransactWriteItems", mock.Anything, mock.Anything)
}

func (s *ProfileRepositorySuite) TestDelete_GetError_Propagates() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	err := s.repo.Delete(s.updateCtx())

	s.Require().Error(err)
	s.mockClient.AssertNotCalled(s.T(), "TransactWriteItems", mock.Anything, mock.Anything)
}

func (s *ProfileRepositorySuite) TestDelete_NonRetryableError_PropagatesWithoutRetry() {
	s.mockClient.EXPECT().GetItem(mock.Anything, mock.Anything).
		Return(storedProfile(nil), nil).Once()
	s.mockClient.EXPECT().
		TransactWriteItems(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled")).Once()

	err := s.repo.Delete(s.updateCtx())

	s.Require().Error(err)
	s.NotErrorIs(err, repository.ErrMutationConflict)
}

func (s *ProfileRepositorySuite) TestResolveUsername_Succeeds() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.GetItemInput) bool {
			username, ok := in.Key["username"].(*types.AttributeValueMemberS)
			return *in.TableName == "profile-username-table" && ok && username.Value == "alice"
		})).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"username": &types.AttributeValueMemberS{Value: "alice"},
				"user_id":  &types.AttributeValueMemberS{Value: "user-alice"},
			},
		}, nil)

	got, err := s.repo.ResolveUsername(s.T().Context(), "alice")

	s.Require().NoError(err)
	s.Equal("user-alice", got)
}

func (s *ProfileRepositorySuite) TestResolveUsername_NoClaim_ReturnsErrNotFound() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{}}, nil)

	_, err := s.repo.ResolveUsername(s.T().Context(), "ghost")

	s.Require().ErrorIs(err, repository.ErrNotFound)
}

func (s *ProfileRepositorySuite) TestResolveUsername_ClaimMissingUserID_IsError() {
	s.mockClient.EXPECT().
		GetItem(mock.Anything, mock.Anything).
		Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"username": &types.AttributeValueMemberS{Value: "alice"},
			},
		}, nil)

	_, err := s.repo.ResolveUsername(s.T().Context(), "alice")

	s.Require().Error(err)
	s.NotErrorIs(err, repository.ErrNotFound)
}

func discoverableRow(userID, username string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"user_id":         &types.AttributeValueMemberS{Value: userID},
		"username":        &types.AttributeValueMemberS{Value: username},
		"discoverable":    &types.AttributeValueMemberBOOL{Value: true},
		"discoverable_pk": &types.AttributeValueMemberS{Value: "1"},
	}
}

func (s *ProfileRepositorySuite) TestListPublic_QueriesUsernameIndex_NoPrefix() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return *in.TableName == "profile-table" &&
				in.IndexName != nil && *in.IndexName == "DiscoverableUsernameIndex" &&
				*in.Limit == 20 && len(in.ExclusiveStartKey) == 0 &&
				!containsAttrName(in.ExpressionAttributeNames, "username")
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				discoverableRow("user-alice", "alice"),
				discoverableRow("user-bob", "bob"),
			},
		}, nil)

	profiles, next, err := s.repo.ListPublic(s.T().Context(), "", "", 20, "")

	s.Require().NoError(err)
	s.Empty(next)
	s.Require().Len(profiles, 2)
	s.Equal("alice", profiles[0].Username)
	s.Equal("user-alice", profiles[0].OwnerID)
	s.True(profiles[0].Discoverable)
}

func containsAttrName(names map[string]string, attr string) bool {
	for _, v := range names {
		if v == attr {
			return true
		}
	}
	return false
}

func (s *ProfileRepositorySuite) TestListPublic_UsernamePrefix_AddsBeginsWith() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return *in.IndexName == "DiscoverableUsernameIndex" &&
				containsAttrName(in.ExpressionAttributeNames, "username") &&
				exprValuesContainS(in.ExpressionAttributeValues, "al")
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{discoverableRow("user-alice", "alice")},
		}, nil)

	profiles, _, err := s.repo.ListPublic(s.T().Context(), "al", "", 20, "")

	s.Require().NoError(err)
	s.Require().Len(profiles, 1)
	s.Equal("alice", profiles[0].Username)
}

func (s *ProfileRepositorySuite) TestListPublic_UsernamePrefix_Lowercased() {
	// Stored usernames are all-lowercase, so a mixed-case caller prefix
	// must be lowercased before the begins_with or it matches nothing.
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return *in.IndexName == "DiscoverableUsernameIndex" &&
				containsAttrName(in.ExpressionAttributeNames, "username") &&
				exprValuesContainS(in.ExpressionAttributeValues, "al") // lowercased
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{discoverableRow("user-alice", "alice")},
		}, nil).Twice()

	for _, prefix := range []string{"Al", "AL"} {
		profiles, _, err := s.repo.ListPublic(s.T().Context(), prefix, "", 20, "")
		s.Require().NoError(err)
		s.Require().Len(profiles, 1)
		s.Equal("alice", profiles[0].Username)
	}
}

func (s *ProfileRepositorySuite) TestListPublic_DiscordPrefix_UsesDiscordIndex_Lowercased() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return *in.IndexName == "DiscoverableDiscordIndex" &&
				containsAttrName(in.ExpressionAttributeNames, "discord_pk") &&
				containsAttrName(in.ExpressionAttributeNames, "discord_username") &&
				exprValuesContainS(in.ExpressionAttributeValues, "cool") // lowercased
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{discoverableRow("user-alice", "alice")},
		}, nil)

	_, _, err := s.repo.ListPublic(s.T().Context(), "", "CooL", 20, "")

	s.Require().NoError(err)
}

func exprValuesContainS(values map[string]types.AttributeValue, want string) bool {
	for _, v := range values {
		if sv, ok := v.(*types.AttributeValueMemberS); ok && sv.Value == want {
			return true
		}
	}
	return false
}

func (s *ProfileRepositorySuite) TestListPublic_EmptyResult_ReturnsEmptySliceNotNil() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)

	profiles, _, err := s.repo.ListPublic(s.T().Context(), "", "", 20, "")

	s.Require().NoError(err)
	s.NotNil(profiles)
	s.Empty(profiles)
}

func (s *ProfileRepositorySuite) TestListPublic_Cursor_RoundTripsThreeKeyGSIKey() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return len(in.ExclusiveStartKey) == 0
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id":         &types.AttributeValueMemberS{Value: "user-bob"},
				"discoverable_pk": &types.AttributeValueMemberS{Value: "1"},
				"username":        &types.AttributeValueMemberS{Value: "bob"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.ListPublic(s.T().Context(), "", "", 20, "")
	s.Require().NoError(err)
	s.Require().NotEmpty(cursor)

	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			k := in.ExclusiveStartKey
			uid, ok1 := k["user_id"].(*types.AttributeValueMemberS)
			pk, ok2 := k["discoverable_pk"].(*types.AttributeValueMemberS)
			un, ok3 := k["username"].(*types.AttributeValueMemberS)
			return len(k) == 3 && ok1 && ok2 && ok3 &&
				uid.Value == "user-bob" && pk.Value == "1" && un.Value == "bob"
		})).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil).Once()

	_, _, err = s.repo.ListPublic(s.T().Context(), "", "", 20, cursor)
	s.Require().NoError(err)
}

func (s *ProfileRepositorySuite) TestListPublic_InvalidCursor_ReturnsErrInvalidCursor() {
	profiles, next, err := s.repo.ListPublic(s.T().Context(), "", "", 20, "not-valid-base64!!")

	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
	s.Nil(profiles)
	s.Empty(next)
}

func (s *ProfileRepositorySuite) TestListPublic_CursorFromOtherFilter_ReturnsErrInvalidCursor() {
	// Page 1: no filter -> a username-index cursor.
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id":         &types.AttributeValueMemberS{Value: "user-bob"},
				"discoverable_pk": &types.AttributeValueMemberS{Value: "1"},
				"username":        &types.AttributeValueMemberS{Value: "bob"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.ListPublic(s.T().Context(), "", "", 20, "")
	s.Require().NoError(err)

	// Reusing it with a discord_username filter must be rejected, not passed
	// to Query as a mismatched ExclusiveStartKey.
	_, _, err = s.repo.ListPublic(s.T().Context(), "", "cool", 20, cursor)
	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
}

func (s *ProfileRepositorySuite) TestListPublic_CursorFromDifferentPrefix_SameIndex_ReturnsErrInvalidCursor() {
	// Page 1: username=a -> a username-index cursor bound to prefix "a".
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id":         &types.AttributeValueMemberS{Value: "user-alice"},
				"discoverable_pk": &types.AttributeValueMemberS{Value: "1"},
				"username":        &types.AttributeValueMemberS{Value: "alice"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.ListPublic(s.T().Context(), "a", "", 20, "")
	s.Require().NoError(err)
	s.Require().NotEmpty(cursor)

	// Replaying it under a narrower prefix on the same index must be
	// rejected, not silently queried against the broader scan.
	_, _, err = s.repo.ListPublic(s.T().Context(), "al", "", 20, cursor)
	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
}

func (s *ProfileRepositorySuite) TestListPublic_CursorSamePrefix_RoundTrips() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return len(in.ExclusiveStartKey) == 0
		})).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id":         &types.AttributeValueMemberS{Value: "user-alice"},
				"discoverable_pk": &types.AttributeValueMemberS{Value: "1"},
				"username":        &types.AttributeValueMemberS{Value: "alice"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.ListPublic(s.T().Context(), "al", "", 20, "")
	s.Require().NoError(err)
	s.Require().NotEmpty(cursor)

	s.mockClient.EXPECT().
		Query(mock.Anything, mock.MatchedBy(func(in *dynamodb.QueryInput) bool {
			return len(in.ExclusiveStartKey) == 3
		})).
		Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil).Once()

	_, _, err = s.repo.ListPublic(s.T().Context(), "al", "", 20, cursor)
	s.Require().NoError(err)
}

func (s *ProfileRepositorySuite) TestListPublic_DiscordCursorReplayedUnderUsernameFilter_ReturnsErrInvalidCursor() {
	// Page 1: discord_username=cool -> a discord-index cursor.
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"user_id":          &types.AttributeValueMemberS{Value: "user-alice"},
				"discord_pk":       &types.AttributeValueMemberS{Value: "1"},
				"discord_username": &types.AttributeValueMemberS{Value: "cool"},
			},
		}, nil).Once()

	_, cursor, err := s.repo.ListPublic(s.T().Context(), "", "cool", 20, "")
	s.Require().NoError(err)
	s.Require().NotEmpty(cursor)

	_, _, err = s.repo.ListPublic(s.T().Context(), "al", "", 20, cursor)
	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
}

func (s *ProfileRepositorySuite) TestListPublic_ForgedCursorLabelsMatchButKeyDoesNot_ReturnsErrInvalidCursor() {
	// A cursor whose idx/pfx labels claim the username index/prefix "a",
	// but whose key attributes actually belong to the discord index. The
	// label check alone would pass this straight to Query as a malformed
	// ExclusiveStartKey; the structural check must catch it first.
	forged, err := encodeCursor(map[string]types.AttributeValue{
		"user_id":          &types.AttributeValueMemberS{Value: "user-alice"},
		"discord_pk":       &types.AttributeValueMemberS{Value: "1"},
		"discord_username": &types.AttributeValueMemberS{Value: "cool"},
	}, "DiscoverableUsernameIndex", "a")
	s.Require().NoError(err)

	_, _, listErr := s.repo.ListPublic(s.T().Context(), "a", "", 20, forged)
	s.Require().ErrorIs(listErr, repository.ErrInvalidCursor)
}

func (s *ProfileRepositorySuite) TestListPublic_QueryError_Propagates() {
	s.mockClient.EXPECT().
		Query(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	_, _, err := s.repo.ListPublic(s.T().Context(), "", "", 20, "")

	s.Require().Error(err)
}

func (s *ProfileRepositorySuite) TestSetAvatarPath_MarksPending() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	s.Require().NoError(s.repo.SetAvatarPath(ctx, "profiles/alice/avatar/up1"))

	s.True(setsAttr(captured, "avatar_status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "pending"))
	s.True(removesAttr(captured, "avatar_rejection_reason"))
}

func (s *ProfileRepositorySuite) TestClearAvatarPath_ClearsStatus() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")
	_, err := s.repo.ClearAvatarPath(ctx)
	s.Require().NoError(err)

	s.True(removesAttr(captured, "avatar_status"))
	s.True(removesAttr(captured, "avatar_rejection_reason"))
}

func (s *ProfileRepositorySuite) TestSetAvatarStatus_ConditionedOnPathAndStatus() {
	var captured *dynamodb.UpdateItemInput
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
			captured = in
			return true
		})).
		Return(&dynamodb.UpdateItemOutput{}, nil)

	reason := "unprocessable"
	ok, err := s.repo.SetAvatarStatus(s.T().Context(), "alice", "profiles/alice/avatar/up1", repository.ImageStatusPending, repository.ImageStatusRejected, &reason)

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", captured.Key["user_id"].(*types.AttributeValueMemberS).Value)
	s.True(names(captured.ExpressionAttributeNames, "avatar_path", "avatar_status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "profiles/alice/avatar/up1"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "pending"))
	s.True(setsAttr(captured, "avatar_status"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "rejected"))
	s.True(setsAttr(captured, "avatar_rejection_reason"))
	s.True(exprValuesContainS(captured.ExpressionAttributeValues, "unprocessable"))
}

func (s *ProfileRepositorySuite) TestSetAvatarStatus_ConditionFails_ReturnsFalseWithoutError() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, &types.ConditionalCheckFailedException{})

	ok, err := s.repo.SetAvatarStatus(s.T().Context(), "alice", "profiles/alice/avatar/up1", repository.ImageStatusPending, repository.ImageStatusApproved, nil)

	s.Require().NoError(err)
	s.False(ok)
}

func (s *ProfileRepositorySuite) TestSetAvatarStatus_UpdateItemError_Propagates() {
	s.mockClient.EXPECT().
		UpdateItem(mock.Anything, mock.Anything).
		Return(nil, errors.New("dynamodb: throttled"))

	ok, err := s.repo.SetAvatarStatus(s.T().Context(), "alice", "profiles/alice/avatar/up1", repository.ImageStatusPending, repository.ImageStatusApproved, nil)

	s.Require().Error(err)
	s.False(ok)
}
