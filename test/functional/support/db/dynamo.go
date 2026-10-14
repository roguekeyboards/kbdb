package db

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// DynamoTable seeds and deletes items directly in one DynamoDB table,
// bypassing the API - used by specs to set up/tear down fixture state for
// routes other than the one under test, so those specs don't depend on the
// API's own create/delete routes working correctly first.
type DynamoTable struct {
	client    *dynamodb.Client
	tableName string
}

// NewDynamoTable returns a DynamoTable for tableName, using
// support.DynamoDBEndpointURL() (floci locally, real AWS endpoint
// resolution against a real deployed stack in CI - see that function's doc
// comment).
func NewDynamoTable(ctx context.Context, tableName string) *DynamoTable {
	endpoint := support.DynamoDBEndpointURL()

	opts := []func(*awsconfig.LoadOptions) error{}
	if endpoint != "" {
		opts = append(opts, awsconfig.WithRegion("us-east-2"))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		panic(err) // config loading failing is an environment problem, not a per-spec one
	}

	if endpoint != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
	}

	client := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &DynamoTable{client: client, tableName: tableName}
}

// PutItem marshals item (a plain struct or map, per
// github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue's rules) and
// writes it to the table, overwriting any existing item at the same key.
func (t *DynamoTable) PutItem(ctx context.Context, item any) error {
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}

	_, err = t.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &t.tableName,
		Item:      av,
	})
	return err
}

// DeleteItem deletes the item at key (attribute name -> plain string value
// - every table this package seeds has an all-string key). Idempotent: a
// key that doesn't exist is not an error, matching real DynamoDB DeleteItem
// semantics - so cleanup can call this unconditionally without tracking
// whether the item was actually created.
func (t *DynamoTable) DeleteItem(ctx context.Context, key map[string]string) error {
	avKey := make(map[string]types.AttributeValue, len(key))
	for k, v := range key {
		avKey[k] = &types.AttributeValueMemberS{Value: v}
	}

	_, err := t.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &t.tableName,
		Key:       avKey,
	})
	return err
}

// GetItem reads the item at key into out (see DeleteItem for key's shape).
// found is false when there's no such item.
func (t *DynamoTable) GetItem(ctx context.Context, key map[string]string, out any) (found bool, err error) {
	avKey := make(map[string]types.AttributeValue, len(key))
	for k, v := range key {
		avKey[k] = &types.AttributeValueMemberS{Value: v}
	}

	res, err := t.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &t.tableName,
		Key:            avKey,
		ConsistentRead: aws.Bool(true),
	})
	if err != nil || res.Item == nil {
		return false, err
	}
	return true, attributevalue.UnmarshalMap(res.Item, out)
}
