package dynamo

import (
	"context"
	"errors"
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// queryLimit clamps a caller-supplied page size to [1, math.MaxInt32] and
// returns it as the *int32 a QueryInput.Limit wants. The two returns above
// the final conversion keep limit provably in range at the int32() cast -
// both for correctness and so static analysis can see the bound.
func queryLimit(limit int) *int32 {
	if limit < 1 {
		return aws.Int32(1)
	}
	if limit > math.MaxInt32 {
		return aws.Int32(math.MaxInt32)
	}
	return aws.Int32(int32(limit))
}

// queryReadSize is how many items each of queryFullPage's Queries reads:
// the largest page a client can ask for, so a page whose filter drops few
// items still takes one Query.
const queryReadSize = 100

// queryFullPage runs in, a filtered Query on a user_id/id base table, until
// it has limit matches or the key range runs out. DynamoDB applies Limit
// before FilterExpression, so a single Query can return a short or empty
// page that still has a LastEvaluatedKey. Matches past limit are dropped
// and the returned key resumes after the last one kept.
func queryFullPage(ctx context.Context, client dynamoAPI, in *dynamodb.QueryInput, limit int) ([]map[string]types.AttributeValue, map[string]types.AttributeValue, error) {
	in.Limit = aws.Int32(queryReadSize)
	want := int(*queryLimit(limit))

	var items []map[string]types.AttributeValue
	for {
		out, err := client.Query(ctx, in)
		if err != nil {
			return nil, nil, err
		}

		if len(items)+len(out.Items) > want {
			items = append(items, out.Items[:want-len(items)]...)
			last := items[len(items)-1]
			return items, map[string]types.AttributeValue{"user_id": last["user_id"], "id": last["id"]}, nil
		}

		items = append(items, out.Items...)
		if len(items) == want || len(out.LastEvaluatedKey) == 0 {
			return items, out.LastEvaluatedKey, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// errKitMissingAfterWrite means UpdateKit's own kit isn't present by KitID
// in the response UpdateItem returned - should be unreachable in practice.
var errKitMissingAfterWrite = errors.New("kit not found in set after write")

// dynamoAPI is the subset of *dynamodb.Client's methods used by the
// repository implementations in this package.
type dynamoAPI interface {
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}
