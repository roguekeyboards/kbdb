// Package notices receives what the stack publishes to its alarm topic, so
// specs can check an account delete notice went out. It subscribes a queue
// of its own: nothing in the stack does unless an operator address is set.
package notices

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
)

// Listener is one queue subscribed to the alarm topic.
type Listener struct {
	sqs             *sqs.Client
	sns             *sns.Client
	queueURL        string
	subscriptionArn string
}

// Listen creates a queue, lets the alarm topic send to it, and subscribes
// it. Close removes both.
func Listen(ctx context.Context) (*Listener, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := support.AWSEndpointURL()
	if endpoint != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
	}
	l := &Listener{
		sqs: sqs.NewFromConfig(cfg, func(o *sqs.Options) {
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
		}),
		sns: sns.NewFromConfig(cfg, func(o *sns.Options) {
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
		}),
	}

	topicArn := support.AlarmTopicArn()
	topic, err := arn.Parse(topicArn)
	if err != nil {
		return nil, fmt.Errorf("parsing alarm topic ARN %q: %w", topicArn, err)
	}
	queueName := support.StackName() + "-notices-" + uuid.NewString()[:8]
	queueArn := arn.ARN{
		Partition: topic.Partition,
		Service:   "sqs",
		Region:    topic.Region,
		AccountID: topic.AccountID,
		Resource:  queueName,
	}.String()

	// The policy is set at creation rather than afterwards: SQS can take up
	// to a minute to apply a changed one, and a notice sent before it
	// applies is refused.
	policy, err := json.Marshal(map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect":    "Allow",
			"Principal": map[string]string{"Service": "sns.amazonaws.com"},
			"Action":    "sqs:SendMessage",
			"Resource":  queueArn,
			"Condition": map[string]any{"ArnEquals": map[string]string{"aws:SourceArn": topicArn}},
		}},
	})
	if err != nil {
		return nil, err
	}
	created, err := l.sqs.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  aws.String(queueName),
		Attributes: map[string]string{string(sqstypes.QueueAttributeNamePolicy): string(policy)},
	})
	if err != nil {
		return nil, fmt.Errorf("creating queue: %w", err)
	}
	l.queueURL = aws.ToString(created.QueueUrl)

	sub, err := l.sns.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn:              aws.String(topicArn),
		Protocol:              aws.String("sqs"),
		Endpoint:              aws.String(queueArn),
		Attributes:            map[string]string{"RawMessageDelivery": "true"},
		ReturnSubscriptionArn: true,
	})
	if err != nil {
		return l, fmt.Errorf("subscribing queue: %w", err)
	}
	l.subscriptionArn = aws.ToString(sub.SubscriptionArn)
	return l, nil
}

// Received reports whether a message containing text has arrived, waiting
// up to a few seconds for one. Messages that don't match, such as another
// spec's notice, are ignored.
func (l *Listener) Received(ctx context.Context, text string) (bool, error) {
	out, err := l.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(l.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     5,
	})
	if err != nil {
		return false, err
	}
	for _, m := range out.Messages {
		if strings.Contains(aws.ToString(m.Body), text) {
			return true, nil
		}
	}
	return false, nil
}

// Close unsubscribes and deletes the queue, best-effort.
func (l *Listener) Close(ctx context.Context) {
	if l == nil {
		return
	}
	if l.subscriptionArn != "" {
		_, _ = l.sns.Unsubscribe(ctx, &sns.UnsubscribeInput{SubscriptionArn: aws.String(l.subscriptionArn)})
	}
	if l.queueURL != "" {
		_, _ = l.sqs.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(l.queueURL)})
	}
}
