package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// UploadCounter is the DynamoDB-backed repository.UploadCounter: one item
// per day, keyed by day, holding count and an expires_at TTL.
type UploadCounter struct {
	client    dynamoAPI
	tableName string
}

var _ repository.UploadCounter = (*UploadCounter)(nil)

// NewUploadCounter returns an UploadCounter backed by client.
func NewUploadCounter(client *dynamodb.Client, tableName string) *UploadCounter {
	return &UploadCounter{client: client, tableName: tableName}
}

// Increment implements repository.UploadCounter.
func (c *UploadCounter) Increment(ctx context.Context, day string, limit int64, expiresAt time.Time) error {
	count := expression.Name("count")
	update := expression.Add(count, expression.Value(1)).
		Set(expression.Name("expires_at"), expression.Value(expiresAt.Unix()))
	cond := expression.AttributeNotExists(count).Or(expression.LessThan(count, expression.Value(limit)))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
	if err != nil {
		return fmt.Errorf("building upload count increment for %q: %w", day, err)
	}

	_, err = c.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &c.tableName,
		Key:                       map[string]types.AttributeValue{"day": &types.AttributeValueMemberS{Value: day}},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ConditionalCheckFailedException](err); ok {
			return repository.ErrUploadLimitReached
		}
		return fmt.Errorf("incrementing upload count for %q: %w", day, err)
	}
	return nil
}
