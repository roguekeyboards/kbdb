package dynamo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	kbdbctx "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/log"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

const maxProfileMutationAttempts = 3

// ProfileRepository is the DynamoDB-backed repository.ProfileRepository.
// profileTableName holds the profile items; usernameTableName holds
// { username -> user_id } claim items that enforce username uniqueness (a
// GSI can't). Writes keep the two in sync via TransactWriteItems.
type ProfileRepository struct {
	client            dynamoAPI
	profileTableName  string
	usernameTableName string
}

var _ repository.ProfileRepository = (*ProfileRepository)(nil)

// NewProfileRepository returns a ProfileRepository backed by client.
func NewProfileRepository(client *dynamodb.Client, profileTableName, usernameTableName string) *ProfileRepository {
	return &ProfileRepository{
		client:            client,
		profileTableName:  profileTableName,
		usernameTableName: usernameTableName,
	}
}

// Get implements repository.ProfileRepository.
func (r *ProfileRepository) Get(ctx context.Context, ownerID string) (*repository.Profile, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.profileTableName,
		Key: map[string]types.AttributeValue{
			"user_id": &types.AttributeValueMemberS{Value: ownerID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting profile for user %q: %w", ownerID, err)
	}

	if len(out.Item) == 0 {
		return nil, repository.ErrNotFound
	}

	var p repository.Profile
	if err := attributevalue.UnmarshalMap(out.Item, &p); err != nil {
		return nil, fmt.Errorf("unmarshalling profile for user %q: %w", ownerID, err)
	}
	if _, ok := out.Item["preferences"]; !ok {
		p.Preferences = repository.DefaultProfilePreferences()
	}

	return &p, nil
}

// GetPreferences implements repository.ProfileRepository. A caller with no
// profile yet gets repository.DefaultProfilePreferences() rather than
// ErrNotFound.
func (r *ProfileRepository) GetPreferences(ctx context.Context, ownerID string) (repository.ProfilePreferences, error) {
	p, err := r.Get(ctx, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.DefaultProfilePreferences(), nil
	}
	if err != nil {
		return repository.ProfilePreferences{}, err
	}

	return p.Preferences, nil
}

// Create implements repository.ProfileRepository. The profile item and the
// { username -> user_id } claim are written in one TransactWriteItems so a
// username is never half-claimed; conflicts are classified by
// mapProfileCreateConflict.
func (r *ProfileRepository) Create(ctx context.Context, p repository.Profile) (*repository.Profile, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("creating profile: %w", repository.ErrNoUserID)
	}
	p.OwnerID = ownerID
	setProfileDirectoryKeys(&p)

	profileItem, err := attributevalue.MarshalMap(p)
	if err != nil {
		return nil, fmt.Errorf("marshalling profile for user %q: %w", ownerID, err)
	}

	claimItem, err := attributevalue.MarshalMap(struct {
		Username string `dynamodbav:"username"`
		UserID   string `dynamodbav:"user_id"`
	}{Username: p.Username, UserID: ownerID})
	if err != nil {
		return nil, fmt.Errorf("marshalling username claim for user %q: %w", ownerID, err)
	}

	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           &r.profileTableName,
				Item:                profileItem,
				ConditionExpression: aws.String("attribute_not_exists(user_id)"),
			}},
			{Put: &types.Put{
				TableName:           &r.usernameTableName,
				Item:                claimItem,
				ConditionExpression: aws.String("attribute_not_exists(username)"),
			}},
		},
	})
	if err != nil {
		if mapped := mapProfileCreateConflict(err); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("creating profile for user %q: %w", ownerID, err)
	}

	return &p, nil
}

// setProfileDirectoryKeys derives the sparse-GSI discriminators from p's
// Discoverable / DiscordUsername fields, leaving each nil when the profile
// shouldn't be in that index. Callers are expected to have already run
// profilevalidate.Normalize on p, so a blank DiscordUsername is nil by the
// time it gets here.
func setProfileDirectoryKeys(p *repository.Profile) {
	p.DiscoverablePK = nil
	p.DiscordPK = nil

	if !p.Discoverable {
		return
	}

	p.DiscoverablePK = aws.String("1")

	if p.DiscordUsername != nil {
		p.DiscordPK = aws.String("1")
	}
}

// mapProfileCreateConflict maps a Create TransactWriteItems cancellation to
// a sentinel. Items are [profilePut, claimPut]; ErrAlreadyExists takes
// priority over ErrUsernameTaken when both conditions fail (you can't
// create at all, so the username is moot). A TransactionConflict on either
// item is transient contention, not a real conflict, so it maps to
// ErrMutationConflict (409, retryable) rather than falling through to a raw
// 500. Returns nil if err isn't a transaction cancellation.
func mapProfileCreateConflict(err error) error {
	txErr, ok := errors.AsType[*types.TransactionCanceledException](err)
	if !ok {
		return nil
	}

	failed := func(i int) bool {
		return i < len(txErr.CancellationReasons) &&
			txErr.CancellationReasons[i].Code != nil &&
			*txErr.CancellationReasons[i].Code == "ConditionalCheckFailed"
	}

	switch {
	case failed(0):
		return repository.ErrAlreadyExists
	case failed(1):
		return repository.ErrUsernameTaken
	case hasCancellationReason(err, "TransactionConflict"):
		return repository.ErrMutationConflict
	default:
		return nil
	}
}

// Update implements repository.ProfileRepository. It reads the stored
// profile once - to learn the current username (so a rename can move the
// { username -> user_id } claim) and to branch same-username vs rename -
// then rewrites the body-settable fields and the directory-GSI keys in
// place. avatar_path is never named, so it carries forward.
func (r *ProfileRepository) Update(ctx context.Context, p repository.Profile) (*repository.Profile, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("updating profile: %w", repository.ErrNoUserID)
	}

	existing, err := r.Get(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	p.OwnerID = ownerID
	setProfileDirectoryKeys(&p)
	update := profileUpdateExpression(&p)

	if p.Username == existing.Username {
		return r.updateProfileItem(ctx, ownerID, update)
	}
	return r.renameProfile(ctx, ownerID, p.Username, update)
}

// updateProfileItem applies update to the profile item with a single
// UpdateItem. user_id is the table's only key, so a failed
// attribute_exists(user_id) can only mean the profile is gone.
func (r *ProfileRepository) updateProfileItem(
	ctx context.Context,
	ownerID string,
	update expression.UpdateBuilder,
) (*repository.Profile, error) {
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(expression.Name("user_id"))).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building profile update expression for user %q: %w", ownerID, err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.profileTableName,
		Key:                       map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: ownerID}},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("updating profile for user %q: %w", ownerID, err)
	}

	// ALL_NEW is the complete post-write item - unmarshal into a zero value,
	// not the pre-Get profile, so a REMOVEd attribute (absent from
	// out.Attributes) doesn't survive as a stale field.
	var updated repository.Profile
	if err := attributevalue.UnmarshalMap(out.Attributes, &updated); err != nil {
		return nil, fmt.Errorf("unmarshalling updated profile for user %q: %w", ownerID, err)
	}
	return &updated, nil
}

// renameProfile moves the { username -> user_id } claim to newUsername
// atomically with the profile-item update:
//
//	[profileUpdate (cond attribute_exists(user_id)),
//	 claimDelete   (cond user_id = caller, so a claim reissued to someone
//	                else is left alone),
//	 claimPut      (cond attribute_not_exists, so a taken username fails)]
//
// It re-reads the profile each attempt to pick up the current username for
// the claim-delete key (a concurrent rename of the same profile moves it).
// Classification of a cancellation: reason 2 ConditionalCheckFailed ->
// ErrUsernameTaken; reason 0 -> ErrNotFound; reason 1 (the claim moved) or
// any TransactionConflict -> retry from a fresh read.
func (r *ProfileRepository) renameProfile(
	ctx context.Context,
	ownerID, newUsername string,
	update expression.UpdateBuilder,
) (*repository.Profile, error) {
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(expression.Name("user_id"))).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building profile rename expression for user %q: %w", ownerID, err)
	}

	claimItem, err := attributevalue.MarshalMap(struct {
		Username string `dynamodbav:"username"`
		UserID   string `dynamodbav:"user_id"`
	}{Username: newUsername, UserID: ownerID})
	if err != nil {
		return nil, fmt.Errorf("marshalling username claim for user %q: %w", ownerID, err)
	}

	claimDeleteExpr, err := expression.NewBuilder().
		WithCondition(expression.Name("user_id").Equal(expression.Value(ownerID))).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building claim delete condition for user %q: %w", ownerID, err)
	}

	for range maxProfileMutationAttempts {
		existing, err := r.Get(ctx, ownerID)
		if err != nil {
			return nil, err
		}
		// A concurrent rename may have already made this the same-username
		// case; the plain UpdateItem path handles it.
		if existing.Username == newUsername {
			return r.updateProfileItem(ctx, ownerID, update)
		}

		_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{Update: &types.Update{
					TableName:                 &r.profileTableName,
					Key:                       map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: ownerID}},
					UpdateExpression:          expr.Update(),
					ConditionExpression:       expr.Condition(),
					ExpressionAttributeNames:  expr.Names(),
					ExpressionAttributeValues: expr.Values(),
				}},
				{Delete: &types.Delete{
					TableName:                 &r.usernameTableName,
					Key:                       map[string]types.AttributeValue{"username": &types.AttributeValueMemberS{Value: existing.Username}},
					ConditionExpression:       claimDeleteExpr.Condition(),
					ExpressionAttributeNames:  claimDeleteExpr.Names(),
					ExpressionAttributeValues: claimDeleteExpr.Values(),
				}},
				{Put: &types.Put{
					TableName:           &r.usernameTableName,
					Item:                claimItem,
					ConditionExpression: aws.String("attribute_not_exists(username)"),
				}},
			},
		})
		if err == nil {
			return r.Get(ctx, ownerID)
		}

		switch classifyProfileRenameConflict(err) {
		case renameConflictUsernameTaken:
			return nil, repository.ErrUsernameTaken
		case renameConflictProfileGone:
			return nil, repository.ErrNotFound
		case renameConflictRetry:
			log.FromContext(ctx).Warn("profile rename retry", log.ProfileID, ownerID)
			continue
		case renameConflictNone:
			return nil, fmt.Errorf("renaming profile for user %q: %w", ownerID, err)
		}
	}

	return nil, fmt.Errorf("renaming profile for user %q: %w", ownerID, repository.ErrMutationConflict)
}

type renameConflict int

const (
	renameConflictNone renameConflict = iota
	renameConflictProfileGone
	renameConflictUsernameTaken
	renameConflictRetry
)

// classifyProfileRenameConflict maps a rename TransactWriteItems
// cancellation. Items are [profileUpdate, claimDelete, claimPut]: reason 2
// ConditionalCheckFailed is the new username being taken (ErrUsernameTaken);
// reason 0 is the profile gone (ErrNotFound); reason 1 (the caller's claim
// moved out from under the delete) or any TransactionConflict is transient
// and retries from a fresh read. renameConflictNone if err isn't a
// transaction cancellation.
func classifyProfileRenameConflict(err error) renameConflict {
	txErr, ok := errors.AsType[*types.TransactionCanceledException](err)
	if !ok {
		return renameConflictNone
	}

	reason := func(i int) string {
		if i < len(txErr.CancellationReasons) && txErr.CancellationReasons[i].Code != nil {
			return *txErr.CancellationReasons[i].Code
		}
		return ""
	}

	for _, r := range txErr.CancellationReasons {
		if r.Code != nil && *r.Code == "TransactionConflict" {
			return renameConflictRetry
		}
	}
	if reason(2) == "ConditionalCheckFailed" {
		return renameConflictUsernameTaken
	}
	if reason(0) == "ConditionalCheckFailed" {
		return renameConflictProfileGone
	}
	if reason(1) == "ConditionalCheckFailed" {
		return renameConflictRetry
	}
	return renameConflictNone
}

// profileUpdateExpression builds the SET/REMOVE clauses for a profile
// Update: the body-settable fields and the directory-GSI keys (which
// setProfileDirectoryKeys must have populated on p first). avatar_path is
// deliberately absent so it carries forward.
func profileUpdateExpression(p *repository.Profile) expression.UpdateBuilder {
	update := expression.
		Set(expression.Name("username"), expression.Value(p.Username)).
		Set(expression.Name("discoverable"), expression.Value(p.Discoverable)).
		Set(expression.Name("preferences"), expression.Value(p.Preferences))
	update = setOrRemovePtr(update, "discord_username", p.DiscordUsername)
	update = setOrRemovePtr(update, "bio", p.Bio)
	if len(p.Links) > 0 {
		update = update.Set(expression.Name("links"), expression.Value(p.Links))
	} else {
		update = update.Remove(expression.Name("links"))
	}
	update = setOrRemovePtr(update, "discoverable_pk", p.DiscoverablePK)
	update = setOrRemovePtr(update, "discord_pk", p.DiscordPK)
	return update
}

// SetAvatarPath implements repository.ProfileRepository.
func (r *ProfileRepository) SetAvatarPath(ctx context.Context, key repository.ProfileImageKey) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("setting avatar path: %w", repository.ErrNoUserID)
	}

	update := expression.Set(expression.Name("avatar_path"), expression.Value(key)).
		Set(expression.Name("avatar_status"), expression.Value(repository.ImageStatusPending)).
		Remove(expression.Name("avatar_rejection_reason"))
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(expression.Name("user_id"))).
		Build()
	if err != nil {
		return fmt.Errorf("building avatar path update for user %q: %w", ownerID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.profileTableName,
		Key:                       map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: ownerID}},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return repository.ErrNotFound
		}
		return fmt.Errorf("setting avatar path for user %q: %w", ownerID, err)
	}

	return nil
}

// ClearAvatarPath implements repository.ProfileRepository. ALL_OLD reports
// the key that was cleared, or nil when nothing was set.
func (r *ProfileRepository) ClearAvatarPath(ctx context.Context) (*repository.ProfileImageKey, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("clearing avatar path: %w", repository.ErrNoUserID)
	}

	update := expression.Remove(expression.Name("avatar_path")).
		Remove(expression.Name("avatar_status")).
		Remove(expression.Name("avatar_rejection_reason"))
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(expression.Name("user_id"))).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building avatar path clear for user %q: %w", ownerID, err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.profileTableName,
		Key:                       map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: ownerID}},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              types.ReturnValueAllOld,
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("clearing avatar path for user %q: %w", ownerID, err)
	}

	old := struct {
		AvatarPath *repository.ProfileImageKey `dynamodbav:"avatar_path"`
	}{}
	if err := attributevalue.UnmarshalMap(out.Attributes, &old); err != nil {
		return nil, fmt.Errorf("unmarshalling cleared avatar path for user %q: %w", ownerID, err)
	}
	if old.AvatarPath == nil {
		return nil, nil //nolint:nilnil // no avatar already set is a valid, expected result
	}

	return old.AvatarPath, nil
}

// SetAvatarStatus implements repository.ProfileRepository.
func (r *ProfileRepository) SetAvatarStatus(ctx context.Context, ownerID string, forPath repository.ProfileImageKey, from, to repository.ImageStatus, reason *string) (bool, error) {
	update := statusUpdate(expression.Name("avatar_status"), expression.Name("avatar_rejection_reason"), to, reason)
	cond := expression.AttributeExists(expression.Name("user_id")).
		And(expression.Name("avatar_path").Equal(expression.Value(forPath))).
		And(statusIs(expression.Name("avatar_status"), from))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
	if err != nil {
		return false, fmt.Errorf("building avatar status update for user %q: %w", ownerID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.profileTableName,
		Key:                       map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: ownerID}},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return false, nil
		}
		return false, fmt.Errorf("setting avatar status for user %q: %w", ownerID, err)
	}

	return true, nil
}

// Delete implements repository.ProfileRepository. A two-item
// TransactWriteItems removes the profile item and its { username ->
// user_id } claim together. A missing profile is a no-op success
// (idempotent).
//
// Delete can't address the claim without first reading the profile for its
// username, so a concurrent rename (or a full delete + recreate) between
// that read and the transaction could point the claim delete at a
// username that has since moved to - or been reclaimed by - a different
// item. Both deletes are therefore conditioned:
//   - profile item: attribute_exists(user_id), so a profile deleted out
//     from under us fails the transaction rather than the delete silently
//     "succeeding" against nothing;
//   - claim item: user_id = caller, so a claim that has since been moved
//     or reissued to someone else is left alone.
//
// A conditioned failure (ConditionalCheckFailed) or a TransactionConflict
// both re-read and retry: the fresh Get either returns ErrNotFound (the
// profile is genuinely gone now - idempotent success) or the current
// username to try again against.
func (r *ProfileRepository) Delete(ctx context.Context) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("deleting profile: %w", repository.ErrNoUserID)
	}

	for range maxProfileMutationAttempts {
		p, err := r.Get(ctx, ownerID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		claimExpr, err := expression.NewBuilder().
			WithCondition(expression.Name("user_id").Equal(expression.Value(ownerID))).
			Build()
		if err != nil {
			return fmt.Errorf("building profile claim delete condition for user %q: %w", ownerID, err)
		}

		_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{Delete: &types.Delete{
					TableName:           &r.profileTableName,
					Key:                 map[string]types.AttributeValue{"user_id": &types.AttributeValueMemberS{Value: ownerID}},
					ConditionExpression: aws.String("attribute_exists(user_id)"),
				}},
				{Delete: &types.Delete{
					TableName:                 &r.usernameTableName,
					Key:                       map[string]types.AttributeValue{"username": &types.AttributeValueMemberS{Value: p.Username}},
					ConditionExpression:       claimExpr.Condition(),
					ExpressionAttributeNames:  claimExpr.Names(),
					ExpressionAttributeValues: claimExpr.Values(),
				}},
			},
		})
		if err == nil {
			return nil
		}

		if profileDeleteShouldRetry(err) {
			log.FromContext(ctx).Warn("profile delete CAS retry", log.ProfileID, ownerID)
			continue
		}
		return fmt.Errorf("deleting profile for user %q: %w", ownerID, err)
	}

	return fmt.Errorf("deleting profile for user %q: %w", ownerID, repository.ErrMutationConflict)
}

// profileDeleteShouldRetry reports whether a failed Delete transaction is a
// transient conflict worth another attempt from a fresh read: one of the
// conditioned deletes failed (a concurrent rename / delete / recreate
// landed), or DynamoDB canceled with TransactionConflict. Any other
// cancellation reason (throttling, validation, ...) is not retried.
func profileDeleteShouldRetry(err error) bool {
	txErr, ok := errors.AsType[*types.TransactionCanceledException](err)
	if !ok {
		return false
	}

	for _, reason := range txErr.CancellationReasons {
		if reason.Code == nil {
			continue
		}
		if *reason.Code == "ConditionalCheckFailed" || *reason.Code == "TransactionConflict" {
			return true
		}
	}
	return false
}

const (
	discoverableUsernameIndex = "DiscoverableUsernameIndex"
	discoverableDiscordIndex  = "DiscoverableDiscordIndex"
	directoryPKValue          = "1"
)

// directoryIndexKeys is each GSI's LastEvaluatedKey attribute set. A
// decoded cursor's key attributes must match this structurally, not just
// its (client-supplied, unsigned) idx/pfx labels - otherwise a forged
// cursor could carry labels matching the current call while its actual key
// addresses a different index, reaching DynamoDB as a malformed
// ExclusiveStartKey instead of being rejected locally.
var directoryIndexKeys = map[string]map[string]struct{}{
	discoverableUsernameIndex: {"discoverable_pk": {}, "username": {}, "user_id": {}},
	discoverableDiscordIndex:  {"discord_pk": {}, "discord_username": {}, "user_id": {}},
}

// ListPublic implements repository.ProfileRepository. discordPrefix routes
// to the discord index, else the username index; either prefix is
// lowercased before the begins_with, matching the all-lowercase stored
// sort keys. The handler guarantees at most one prefix. A bad cursor, or one minted
// under a different index or prefix filter than the current call, is
// repository.ErrInvalidCursor.
func (r *ProfileRepository) ListPublic(
	ctx context.Context,
	usernamePrefix, discordPrefix string,
	limit int,
	cursor string,
) ([]repository.Profile, string, error) {
	startKey, cursorIdx, cursorPfx, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", repository.ErrInvalidCursor, err)
	}

	indexName := discoverableUsernameIndex
	activePrefix := strings.ToLower(usernamePrefix)
	keyCond := expression.Key("discoverable_pk").Equal(expression.Value(directoryPKValue))
	if usernamePrefix != "" {
		keyCond = keyCond.And(expression.KeyBeginsWith(expression.Key("username"), activePrefix))
	}
	if discordPrefix != "" {
		indexName = discoverableDiscordIndex
		activePrefix = strings.ToLower(discordPrefix)
		keyCond = expression.Key("discord_pk").Equal(expression.Value(directoryPKValue)).
			And(expression.KeyBeginsWith(expression.Key("discord_username"), activePrefix))
	}

	if len(startKey) > 0 {
		if cursorIdx != indexName || cursorPfx != activePrefix {
			return nil, "", fmt.Errorf("%w: filter changed since this cursor was issued", repository.ErrInvalidCursor)
		}
		if !cursorMatchesIndex(startKey, indexName) {
			return nil, "", fmt.Errorf("%w: key does not match its claimed index", repository.ErrInvalidCursor)
		}
	}

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, "", fmt.Errorf("building profile directory expression: %w", err)
	}

	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 &r.profileTableName,
		IndexName:                 &indexName,
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ExclusiveStartKey:         startKey,
		Limit:                     queryLimit(limit),
	})
	if err != nil {
		return nil, "", fmt.Errorf("querying discoverable profiles: %w", err)
	}

	profiles := []repository.Profile{}
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &profiles); err != nil {
		return nil, "", fmt.Errorf("unmarshalling discoverable profiles: %w", err)
	}

	nextCursor, err := encodeCursor(out.LastEvaluatedKey, indexName, activePrefix)
	if err != nil {
		return nil, "", fmt.Errorf("encoding next cursor: %w", err)
	}

	return profiles, nextCursor, nil
}

// cursorMatchesIndex reports whether startKey's attributes are exactly
// directoryIndexKeys[indexName].
func cursorMatchesIndex(startKey map[string]types.AttributeValue, indexName string) bool {
	want := directoryIndexKeys[indexName]
	if len(startKey) != len(want) {
		return false
	}
	for k := range startKey {
		if _, ok := want[k]; !ok {
			return false
		}
	}
	return true
}

// ResolveUsername implements repository.ProfileRepository, reading the
// { username -> user_id } claim item. ErrNotFound means the username is
// unclaimed.
func (r *ProfileRepository) ResolveUsername(ctx context.Context, username string) (string, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.usernameTableName,
		Key: map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: username},
		},
	})
	if err != nil {
		return "", fmt.Errorf("resolving username %q: %w", username, err)
	}

	if len(out.Item) == 0 {
		return "", repository.ErrNotFound
	}

	claim := struct {
		UserID string `dynamodbav:"user_id"`
	}{}
	if err := attributevalue.UnmarshalMap(out.Item, &claim); err != nil {
		return "", fmt.Errorf("unmarshalling username claim %q: %w", username, err)
	}
	if claim.UserID == "" {
		return "", fmt.Errorf("username claim %q has no user_id", username)
	}

	return claim.UserID, nil
}
