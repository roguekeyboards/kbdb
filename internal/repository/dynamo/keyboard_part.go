package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	kbdbctx "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// errDuplicateKeyboardPartID guards AddPlate/AddPCB against an ID collision
// within one keyboard - practically unreachable given uuid.NewString().
var errDuplicateKeyboardPartID = errors.New("part id already exists for this keyboard")

// The keyboard attributes holding each part map.
const (
	platesAttr = "plates"
	pcbsAttr   = "pcbs"
)

// AddPlate implements repository.KeyboardRepository.
func (r *KeyboardRepository) AddPlate(ctx context.Context, keyboardID string, plate repository.KeyboardPlate) (*repository.KeyboardPlate, error) {
	plate.Seq = int(time.Now().UnixNano())
	if err := r.addPart(ctx, keyboardID, platesAttr, plate.ID, plate); err != nil {
		return nil, err
	}
	return &plate, nil
}

// UpdatePlate implements repository.KeyboardRepository. Names every field
// but seq, so the plate keeps its place.
func (r *KeyboardRepository) UpdatePlate(ctx context.Context, keyboardID string, plate repository.KeyboardPlate) (*repository.KeyboardPlate, error) {
	entry, ok := mapKey(platesAttr, plate.ID)
	if !ok {
		return nil, repository.ErrNotFound
	}
	update := expression.
		Set(field(entry, "material"), expression.Value(plate.Material)).
		Set(field(entry, "purchase"), expression.Value(plate.Purchase))
	update = setOrRemoveName(update, field(entry, "color"), plate.Color)
	update = setOrRemoveName(update, field(entry, "thickness"), plate.Thickness)

	updated, err := r.updatePart(ctx, keyboardID, platesAttr, plate.ID, entry, update)
	if err != nil {
		return nil, err
	}
	p, ok := updated.Plates[plate.ID]
	if !ok {
		return nil, fmt.Errorf("updating plate %q on keyboard %q: part missing after write", plate.ID, keyboardID)
	}
	return &p, nil
}

// DeletePlate implements repository.KeyboardRepository.
func (r *KeyboardRepository) DeletePlate(ctx context.Context, keyboardID, plateID string) error {
	return r.deletePart(ctx, keyboardID, platesAttr, plateID)
}

// AddPCB implements repository.KeyboardRepository.
func (r *KeyboardRepository) AddPCB(ctx context.Context, keyboardID string, pcb repository.KeyboardPCB) (*repository.KeyboardPCB, error) {
	pcb.Seq = int(time.Now().UnixNano())
	if err := r.addPart(ctx, keyboardID, pcbsAttr, pcb.ID, pcb); err != nil {
		return nil, err
	}
	return &pcb, nil
}

// UpdatePCB implements repository.KeyboardRepository. Names every field
// but seq, so the PCB keeps its place.
func (r *KeyboardRepository) UpdatePCB(ctx context.Context, keyboardID string, pcb repository.KeyboardPCB) (*repository.KeyboardPCB, error) {
	entry, ok := mapKey(pcbsAttr, pcb.ID)
	if !ok {
		return nil, repository.ErrNotFound
	}
	update := expression.Set(field(entry, "purchase"), expression.Value(pcb.Purchase))
	update = setOrRemoveName(update, field(entry, "thickness"), pcb.Thickness)
	update = setOrRemoveName(update, field(entry, "firmware"), pcb.Firmware)
	update = setOrRemoveName(update, field(entry, "assembly"), pcb.Assembly)
	update = setOrRemoveName(update, field(entry, "connectivity"), pcb.Connectivity)

	updated, err := r.updatePart(ctx, keyboardID, pcbsAttr, pcb.ID, entry, update)
	if err != nil {
		return nil, err
	}
	p, ok := updated.PCBs[pcb.ID]
	if !ok {
		return nil, fmt.Errorf("updating PCB %q on keyboard %q: part missing after write", pcb.ID, keyboardID)
	}
	return &p, nil
}

// DeletePCB implements repository.KeyboardRepository.
func (r *KeyboardRepository) DeletePCB(ctx context.Context, keyboardID, pcbID string) error {
	return r.deletePart(ctx, keyboardID, pcbsAttr, pcbID)
}

// addPart sets attr.<id> = value, conditioned on the keyboard existing and
// the id being unused. On a condition failure, a strongly consistent read
// tells a missing keyboard from a duplicate id.
func (r *KeyboardRepository) addPart(ctx context.Context, keyboardID, attr, id string, value any) error {
	path, ok := mapKey(attr, id)
	if !ok {
		return fmt.Errorf("adding %s %q to keyboard %q: %w", attr, id, keyboardID, errInvalidMapKey)
	}

	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("adding %s to keyboard %q: %w", attr, keyboardID, repository.ErrNoUserID)
	}

	expr, err := expression.NewBuilder().
		WithUpdate(expression.Set(path, expression.Value(value))).
		WithCondition(expression.AttributeExists(expression.Name("id")).
			And(expression.AttributeNotExists(path))).
		Build()
	if err != nil {
		return fmt.Errorf("building add-%s expression for keyboard %q: %w", attr, keyboardID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keyboardKey(ownerID, keyboardID),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			exists, existsErr := r.keyboardExists(ctx, ownerID, keyboardID)
			if existsErr != nil {
				return fmt.Errorf("classifying add-%s conflict for keyboard %q: %w", attr, keyboardID, existsErr)
			}
			if !exists {
				return repository.ErrNotFound
			}
			return fmt.Errorf("adding %s %q to keyboard %q: %w", attr, id, keyboardID, errDuplicateKeyboardPartID)
		}
		return fmt.Errorf("adding %s %q to keyboard %q owner %q: %w", attr, id, keyboardID, ownerID, err)
	}

	return nil
}

// updatePart applies update, conditioned on entry (attr.<id>, from mapKey)
// existing, which also implies the keyboard does - so a condition failure
// is ErrNotFound.
func (r *KeyboardRepository) updatePart(
	ctx context.Context, keyboardID, attr, id string, entry expression.NameBuilder, update expression.UpdateBuilder,
) (*repository.Keyboard, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("updating %s on keyboard %q: %w", attr, keyboardID, repository.ErrNoUserID)
	}

	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.AttributeExists(entry)).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building update-%s expression for keyboard %q: %w", attr, keyboardID, err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keyboardKey(ownerID, keyboardID),
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
		return nil, fmt.Errorf("updating %s %q on keyboard %q owner %q: %w", attr, id, keyboardID, ownerID, err)
	}

	var updated repository.Keyboard
	if err := attributevalue.UnmarshalMap(out.Attributes, &updated); err != nil {
		return nil, fmt.Errorf("unmarshalling keyboard %q after updating %s %q: %w", keyboardID, attr, id, err)
	}
	return &updated, nil
}

// deletePart removes attr.<id>, conditioned on it existing. On a condition
// failure, a strongly consistent read tells a missing keyboard
// (ErrNotFound) from an id already gone (idempotent success). An id mapKey
// refuses can't be stored, so it's already gone too.
func (r *KeyboardRepository) deletePart(ctx context.Context, keyboardID, attr, id string) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("deleting %s from keyboard %q: %w", attr, keyboardID, repository.ErrNoUserID)
	}

	path, ok := mapKey(attr, id)
	if !ok {
		return nil
	}
	expr, err := expression.NewBuilder().
		WithUpdate(expression.Remove(path)).
		WithCondition(expression.AttributeExists(path)).
		Build()
	if err != nil {
		return fmt.Errorf("building delete-%s expression for keyboard %q: %w", attr, keyboardID, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.tableName,
		Key:                       keyboardKey(ownerID, keyboardID),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			exists, existsErr := r.keyboardExists(ctx, ownerID, keyboardID)
			if existsErr != nil {
				return fmt.Errorf("classifying delete-%s conflict for keyboard %q: %w", attr, keyboardID, existsErr)
			}
			if !exists {
				return repository.ErrNotFound
			}
			return nil
		}
		return fmt.Errorf("deleting %s %q from keyboard %q owner %q: %w", attr, id, keyboardID, ownerID, err)
	}

	return nil
}
