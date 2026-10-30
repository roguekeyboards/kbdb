package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/log"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

// errEmptyKitID guards AddKit/UpdateKit against a caller-skipped KitID.
var errEmptyKitID = errors.New("kit id must not be empty")

// errDuplicateKitID guards AddKit against a KitID collision.
var errDuplicateKitID = errors.New("kit id already exists in this set")

// KeycapSetRepository is the DynamoDB-backed repository.KeycapSetRepository.
type KeycapSetRepository struct {
	client    dynamoAPI
	tableName string
}

var _ repository.KeycapSetRepository = (*KeycapSetRepository)(nil)

// NewKeycapSetRepository returns a KeycapSetRepository backed by client.
func NewKeycapSetRepository(client *dynamodb.Client, tableName string) *KeycapSetRepository {
	return &KeycapSetRepository{client: client, tableName: tableName}
}

// List implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) List(
	ctx context.Context,
	ownerID string,
	visibilities []repository.Visibility,
	limit int,
	cursor string,
) ([]repository.KeycapSet, string, error) {
	// No visibility tier is readable, so nothing can match - short-circuit
	// rather than build a Query with an empty IN(...) filter (which the
	// expression builder below would panic constructing).
	if len(visibilities) == 0 {
		return []repository.KeycapSet{}, "", nil
	}

	startKey, _, err := decodeOwnerCursor(cursor, ownerID)
	if err != nil {
		return nil, "", err
	}

	visValues := make([]expression.OperandBuilder, len(visibilities))
	for i, v := range visibilities {
		visValues[i] = expression.Value(v)
	}

	builder := expression.NewBuilder().
		WithKeyCondition(expression.Key("user_id").Equal(expression.Value(ownerID))).
		WithFilter(expression.Name("visibility").In(visValues[0], visValues[1:]...))

	expr, err := builder.Build()
	if err != nil {
		return nil, "", fmt.Errorf("building keycap set list expression: %w", err)
	}

	items, lastKey, err := queryFullPage(ctx, r.client, &dynamodb.QueryInput{
		TableName:                 &r.tableName,
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ExclusiveStartKey:         startKey,
	}, limit)
	if err != nil {
		return nil, "", fmt.Errorf("querying keycap sets for owner %q: %w", ownerID, err)
	}

	sets := []repository.KeycapSet{}
	if err := attributevalue.UnmarshalListOfMaps(items, &sets); err != nil {
		return nil, "", fmt.Errorf("unmarshalling keycap sets for owner %q: %w", ownerID, err)
	}

	nextCursor, err := encodeCursor(lastKey, "", "")
	if err != nil {
		return nil, "", fmt.Errorf("encoding next cursor: %w", err)
	}

	return sets, nextCursor, nil
}

// Get implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) Get(ctx context.Context, ownerID, id string) (*repository.KeycapSet, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key:       keycapSetKey(ownerID, id),
	})
	if err != nil {
		return nil, fmt.Errorf("getting keycap set %q for owner %q: %w", id, ownerID, err)
	}

	if len(out.Item) == 0 {
		return nil, repository.ErrNotFound
	}

	var ks repository.KeycapSet
	if err := attributevalue.UnmarshalMap(out.Item, &ks); err != nil {
		return nil, fmt.Errorf("unmarshalling keycap set %q for owner %q: %w", id, ownerID, err)
	}

	return &ks, nil
}

// Create implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) Create(ctx context.Context, ks repository.KeycapSet) (*repository.KeycapSet, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("creating keycap set %q: %w", ks.ID, repository.ErrNoUserID)
	}
	ks.UserID = ownerID

	item, err := attributevalue.MarshalMap(ks)
	if err != nil {
		return nil, fmt.Errorf("marshalling keycap set %q for owner %q: %w", ks.ID, ks.UserID, err)
	}
	// AddKit needs kits to exist as a Map - DynamoDB won't auto-vivify it.
	if _, ok := item["kits"]; !ok {
		item["kits"] = &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}}
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &r.tableName,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return nil, repository.ErrAlreadyExists
		}
		return nil, fmt.Errorf("creating keycap set %q for owner %q: %w", ks.ID, ks.UserID, err)
	}

	return &ks, nil
}

// Update implements repository.KeycapSetRepository. Kits/PrimaryKitID
// aren't named, so left untouched; id is the sort key, so a condition
// failure only ever means ErrNotFound.
func (r *KeycapSetRepository) Update(ctx context.Context, ks repository.KeycapSet) (*repository.KeycapSet, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("updating keycap set %q: %w", ks.ID, repository.ErrNoUserID)
	}

	update := expression.
		Set(expression.Name("brand"), expression.Value(ks.Brand)).
		Set(expression.Name("name"), expression.Value(ks.Name)).
		Set(expression.Name("visibility"), expression.Value(ks.Visibility))
	update = setOrRemovePtr(update, "profile", ks.Profile)
	update = setOrRemovePtr(update, "material", ks.Material)
	update = setOrRemovePtr(update, "notes", ks.Notes)

	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(expression.Name("id"))).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building keycap set update expression for set %q: %w", ks.ID, err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, ks.ID),
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
		return nil, fmt.Errorf("updating keycap set %q for owner %q: %w", ks.ID, ownerID, err)
	}

	var updated repository.KeycapSet
	if err := attributevalue.UnmarshalMap(out.Attributes, &updated); err != nil {
		return nil, fmt.Errorf("unmarshalling updated keycap set %q for owner %q: %w", ks.ID, ownerID, err)
	}

	return &updated, nil
}

// keycapSetExists is a strongly consistent existence check.
func (r *KeycapSetRepository) keycapSetExists(ctx context.Context, ownerID, id string) (bool, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &r.tableName,
		Key:            keycapSetKey(ownerID, id),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, err
	}
	return len(out.Item) > 0, nil
}

// Delete implements repository.KeycapSetRepository. Idempotent: a
// nonexistent id is not an error.
func (r *KeycapSetRepository) Delete(ctx context.Context, id string) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("deleting keycap set %q: %w", id, repository.ErrNoUserID)
	}

	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &r.tableName,
		Key:       keycapSetKey(ownerID, id),
	})
	if err != nil {
		return fmt.Errorf("deleting keycap set %q for owner %q: %w", id, ownerID, err)
	}

	return nil
}

// DeleteAll implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) DeleteAll(ctx context.Context) error {
	return deleteOwnerItems(ctx, r.client, r.tableName)
}

// AddKit implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) AddKit(ctx context.Context, setID string, kit repository.KeycapKit, primary *bool) (*repository.KeycapKit, error) {
	if kit.KitID == "" {
		return nil, fmt.Errorf("adding kit to keycap set %q: %w", setID, errEmptyKitID)
	}

	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("adding kit to keycap set %q: %w", setID, repository.ErrNoUserID)
	}

	kit.Seq = int(time.Now().UnixNano())
	kitPath, ok := mapKey("kits", kit.KitID)
	if !ok {
		return nil, fmt.Errorf("adding kit %q to keycap set %q: %w", kit.KitID, setID, errInvalidMapKey)
	}
	update := expression.Set(kitPath, expression.Value(kit))
	if primary != nil && *primary {
		update = update.Set(expression.Name("primary_kit_id"), expression.Value(kit.KitID))
	}
	cond := expression.AttributeExists(expression.Name("id")).
		And(expression.AttributeNotExists(kitPath))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
	if err != nil {
		return nil, fmt.Errorf("building add-kit expression for keycap set %q: %w", setID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, setID),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return nil, r.classifyAddKitConflict(ctx, ownerID, setID, kit.KitID)
		}
		return nil, fmt.Errorf("adding kit %q to keycap set %q owner %q: %w", kit.KitID, setID, ownerID, err)
	}

	return &kit, nil
}

// classifyAddKitConflict: set gone -> ErrNotFound, else errDuplicateKitID.
func (r *KeycapSetRepository) classifyAddKitConflict(ctx context.Context, ownerID, setID, kitID string) error {
	exists, err := r.keycapSetExists(ctx, ownerID, setID)
	if err != nil {
		return fmt.Errorf("classifying add-kit conflict for keycap set %q: %w", setID, err)
	}
	if !exists {
		return repository.ErrNotFound
	}
	return fmt.Errorf("adding kit %q to keycap set %q: %w", kitID, setID, errDuplicateKitID)
}

// UpdateKit implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) UpdateKit(ctx context.Context, setID string, kit repository.KeycapKit, primary *bool) (*repository.KeycapKit, error) {
	if kit.KitID == "" {
		return nil, fmt.Errorf("updating kit in keycap set %q: %w", setID, errEmptyKitID)
	}

	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("updating kit in keycap set %q: %w", setID, repository.ErrNoUserID)
	}

	kitPath, ok := mapKey("kits", kit.KitID)
	if !ok {
		return nil, repository.ErrNotFound
	}
	// name/purchase only, so image_path is left untouched.
	update := expression.
		Set(field(kitPath, "name"), expression.Value(kit.Name)).
		Set(field(kitPath, "purchase"), expression.Value(kit.Purchase))
	if primary != nil && *primary {
		update = update.Set(expression.Name("primary_kit_id"), expression.Value(kit.KitID))
	}

	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(kitPath)).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building update-kit expression for keycap set %q: %w", setID, err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, setID),
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
		return nil, fmt.Errorf("updating kit %q in keycap set %q owner %q: %w", kit.KitID, setID, ownerID, err)
	}

	if primary != nil && !*primary {
		r.clearPrimaryIfMatches(ctx, ownerID, setID, kit.KitID)
	}

	var updated repository.KeycapSet
	if err := attributevalue.UnmarshalMap(out.Attributes, &updated); err != nil {
		return nil, fmt.Errorf("unmarshalling updated keycap set %q for owner %q: %w", setID, ownerID, err)
	}
	updatedKit, ok := updated.Kits[kit.KitID]
	if !ok {
		return nil, fmt.Errorf("updating kit %q in keycap set %q: %w", kit.KitID, setID, errKitMissingAfterWrite)
	}

	return &updatedKit, nil
}

// clearPrimaryIfMatches best-effort clears primary_kit_id if it names
// kitID. Not atomic with the caller's write; failures are logged, not
// returned - the read path already tolerates a dangling reference.
func (r *KeycapSetRepository) clearPrimaryIfMatches(ctx context.Context, ownerID, setID, kitID string) {
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &r.tableName,
		Key:                 keycapSetKey(ownerID, setID),
		UpdateExpression:    aws.String("REMOVE primary_kit_id"),
		ConditionExpression: aws.String("primary_kit_id = :kid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":kid": &types.AttributeValueMemberS{Value: kitID},
		},
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); !ok {
			log.FromContext(ctx).Warn("clearing primary kit id failed", log.KeycapSetID, setID, log.KeycapKitID, kitID, log.Error, err)
		}
	}
}

// DeleteKit implements repository.KeycapSetRepository. Idempotent: a kitID
// not present in the set is not an error.
func (r *KeycapSetRepository) DeleteKit(ctx context.Context, setID, kitID string) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("deleting kit from keycap set %q: %w", setID, repository.ErrNoUserID)
	}

	kitPath, ok := mapKey("kits", kitID)
	if !ok {
		return nil
	}
	expr, err := expression.NewBuilder().
		WithUpdate(expression.Remove(kitPath)).
		WithCondition(expression.AttributeExists(kitPath)).
		Build()
	if err != nil {
		return fmt.Errorf("building delete-kit expression for keycap set %q: %w", setID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, setID),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return r.classifyDeleteKitConflict(ctx, ownerID, setID)
		}
		return fmt.Errorf("deleting kit %q from keycap set %q owner %q: %w", kitID, setID, ownerID, err)
	}

	r.clearPrimaryIfMatches(ctx, ownerID, setID, kitID)

	return nil
}

// classifyDeleteKitConflict: set gone -> ErrNotFound, else already absent (idempotent).
func (r *KeycapSetRepository) classifyDeleteKitConflict(ctx context.Context, ownerID, setID string) error {
	exists, err := r.keycapSetExists(ctx, ownerID, setID)
	if err != nil {
		return fmt.Errorf("classifying delete-kit conflict for keycap set %q: %w", setID, err)
	}
	if !exists {
		return repository.ErrNotFound
	}
	return nil
}

// SetKitImagePath implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) SetKitImagePath(ctx context.Context, setID, kitID string, key repository.KeycapKitImageKey) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("setting kit image path in keycap set %q: %w", setID, repository.ErrNoUserID)
	}

	kitPath, ok := mapKey("kits", kitID)
	if !ok {
		return repository.ErrNotFound
	}
	update := expression.Set(field(kitPath, "image_path"), expression.Value(key)).
		Set(field(kitPath, "image_status"), expression.Value(repository.ImageStatusPending)).
		Remove(field(kitPath, "image_rejection_reason"))
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(kitPath)).
		Build()
	if err != nil {
		return fmt.Errorf("building set-kit-image expression for keycap set %q: %w", setID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, setID),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return repository.ErrNotFound
		}
		return fmt.Errorf("setting kit %q image path in keycap set %q owner %q: %w", kitID, setID, ownerID, err)
	}

	return nil
}

// ClearKitImagePath implements repository.KeycapSetRepository. ALL_OLD
// reports the key that was cleared, or nil when nothing was set.
func (r *KeycapSetRepository) ClearKitImagePath(ctx context.Context, setID, kitID string) (*repository.KeycapKitImageKey, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("clearing kit image path in keycap set %q: %w", setID, repository.ErrNoUserID)
	}

	kitPath, ok := mapKey("kits", kitID)
	if !ok {
		return nil, repository.ErrNotFound
	}
	update := expression.Remove(field(kitPath, "image_path")).
		Remove(field(kitPath, "image_status")).
		Remove(field(kitPath, "image_rejection_reason"))
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(kitPath)).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building clear-kit-image expression for keycap set %q: %w", setID, err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, setID),
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
		return nil, fmt.Errorf("clearing kit %q image path in keycap set %q owner %q: %w", kitID, setID, ownerID, err)
	}

	old := struct {
		Kits map[string]repository.KeycapKit `dynamodbav:"kits"`
	}{}
	if err := attributevalue.UnmarshalMap(out.Attributes, &old); err != nil {
		return nil, fmt.Errorf("unmarshalling cleared kit %q image path in keycap set %q owner %q: %w", kitID, setID, ownerID, err)
	}
	kit, ok := old.Kits[kitID]
	if !ok || kit.ImagePath == nil {
		return nil, nil //nolint:nilnil // no image already set is a valid, expected result
	}

	return kit.ImagePath, nil
}

// SetKitImageStatus implements repository.KeycapSetRepository.
func (r *KeycapSetRepository) SetKitImageStatus(ctx context.Context, ownerID, setID, kitID string, forPath repository.KeycapKitImageKey, from, to repository.ImageStatus, reason *string) (bool, error) {
	kitPath, ok := mapKey("kits", kitID)
	if !ok {
		return false, nil
	}
	update := statusUpdate(field(kitPath, "image_status"), field(kitPath, "image_rejection_reason"), to, reason)
	cond := expression.AttributeExists(kitPath).
		And(field(kitPath, "image_path").Equal(expression.Value(forPath))).
		And(statusIs(field(kitPath, "image_status"), from))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
	if err != nil {
		return false, fmt.Errorf("building kit image status update for keycap set %q: %w", setID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keycapSetKey(ownerID, setID),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return false, nil
		}
		return false, fmt.Errorf("setting kit %q image status in keycap set %q owner %q: %w", kitID, setID, ownerID, err)
	}

	return true, nil
}

func keycapSetKey(ownerID, setID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"user_id": &types.AttributeValueMemberS{Value: ownerID},
		"id":      &types.AttributeValueMemberS{Value: setID},
	}
}
