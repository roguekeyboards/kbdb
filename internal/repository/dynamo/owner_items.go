package dynamo

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	kbdbctx "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// batchWriteLimit is BatchWriteItem's per-request item cap.
const batchWriteLimit = 25

// unprocessedRetries bounds how often deleteOwnerItems resends the
// requests BatchWriteItem hands back unprocessed (throttling) before
// giving up; the whole delete is safe to retry from the top.
const unprocessedRetries = 5

// deleteOwnerItems deletes every item in tableName's user_id partition for
// the caller. DynamoDB has no delete-by-partition-key, so it queries the
// partition's keys a page at a time, reading consistently so an item
// written just before isn't missed, and batch-deletes each page.
func deleteOwnerItems(ctx context.Context, client dynamoAPI, tableName string) error {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return fmt.Errorf("deleting all items in %s: %w", tableName, repository.ErrNoUserID)
	}

	var startKey map[string]types.AttributeValue
	for {
		out, err := client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 &tableName,
			KeyConditionExpression:    aws.String("user_id = :u"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":u": &types.AttributeValueMemberS{Value: ownerID}},
			ProjectionExpression:      aws.String("user_id, id"),
			ConsistentRead:            aws.Bool(true),
			ExclusiveStartKey:         startKey,
		})
		if err != nil {
			return fmt.Errorf("listing %s items for owner %q: %w", tableName, ownerID, err)
		}
		for chunk := range slices.Chunk(out.Items, batchWriteLimit) {
			if err := batchDelete(ctx, client, tableName, chunk); err != nil {
				return fmt.Errorf("deleting %s items for owner %q: %w", tableName, ownerID, err)
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		startKey = out.LastEvaluatedKey
	}
}

func batchDelete(ctx context.Context, client dynamoAPI, tableName string, keys []map[string]types.AttributeValue) error {
	requests := make([]types.WriteRequest, 0, len(keys))
	for _, key := range keys {
		requests = append(requests, types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: key}})
	}
	pending := map[string][]types.WriteRequest{tableName: requests}
	for attempt := 0; ; attempt++ {
		out, err := client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{RequestItems: pending})
		if err != nil {
			return err
		}
		if len(out.UnprocessedItems) == 0 {
			return nil
		}
		if attempt == unprocessedRetries {
			return fmt.Errorf("%d deletes still unprocessed after %d retries", len(out.UnprocessedItems[tableName]), unprocessedRetries)
		}
		pending = out.UnprocessedItems
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(50<<attempt) * time.Millisecond):
		}
	}
}
