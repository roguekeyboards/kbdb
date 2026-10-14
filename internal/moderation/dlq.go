package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// listVisibility hides each message List receives for long enough to
// finish listing, so none is received twice; List then makes them all
// visible again.
const listVisibility = 30

// redriveVisibility hides each message Redrive receives for longer than
// the function's 30 s timeout, so a message being redriven isn't
// received twice.
const redriveVisibility = 300

// sqsAPI is the subset of the SQS client DLQ uses.
type sqsAPI interface {
	ReceiveMessage(ctx context.Context, in *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, in *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	ChangeMessageVisibility(ctx context.Context, in *sqs.ChangeMessageVisibilityInput, opts ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error)
	GetQueueAttributes(ctx context.Context, in *sqs.GetQueueAttributesInput, opts ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

// lambdaAPI is the subset of the Lambda client DLQ uses.
type lambdaAPI interface {
	Invoke(ctx context.Context, in *lambda.InvokeInput, opts ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
}

// DLQ is the moderation function's dead-letter queue: events that failed
// every retry, each held as Lambda's invocation record.
type DLQ struct {
	sqs      sqsAPI
	lambda   lambdaAPI
	queueURL string
	function string
}

// NewDLQ returns the DLQ at queueURL, redriven into function.
func NewDLQ(sqsClient *sqs.Client, lambdaClient *lambda.Client, queueURL, function string) *DLQ {
	return &DLQ{sqs: sqsClient, lambda: lambdaClient, queueURL: queueURL, function: function}
}

// FailedEvent is one dead-lettered event. Condition is why Lambda gave up
// (RetriesExhausted, EventAgeExceeded); Error is the last error, empty
// when the event aged out without running.
type FailedEvent struct {
	Time      time.Time
	Keys      []string
	Condition string
	Error     string
	Attempts  int

	payload json.RawMessage
	receipt string
}

// invocationRecord is the part of Lambda's OnFailure record DLQ reads.
type invocationRecord struct {
	Timestamp      time.Time       `json:"timestamp"`
	RequestPayload json.RawMessage `json:"requestPayload"`
	RequestContext struct {
		Condition              string `json:"condition"`
		ApproximateInvokeCount int    `json:"approximateInvokeCount"`
	} `json:"requestContext"`
	ResponsePayload struct {
		ErrorMessage string `json:"errorMessage"`
	} `json:"responsePayload"`
}

// Count is roughly how many events are in the queue.
func (d *DLQ) Count(ctx context.Context) (int, error) {
	out, err := d.sqs.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: &d.queueURL,
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("reading the dead-letter queue's size: %w", err)
	}
	var n int
	for _, v := range out.Attributes {
		c, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("reading the dead-letter queue's size: %w", err)
		}
		n += c
	}
	return n, nil
}

// List returns every event in the queue without consuming any.
func (d *DLQ) List(ctx context.Context) ([]FailedEvent, error) {
	evs, err := d.receiveAll(ctx, listVisibility)
	if err != nil {
		return nil, err
	}
	return evs, d.release(ctx, evs)
}

// Redrive invokes the function synchronously with each event, or only
// those for key when it's set, deleting an event once its invocation
// succeeds. It stops at the first failure. Safe to repeat: the function
// skips any image that isn't pending any more. done is called after each
// event is redriven.
func (d *DLQ) Redrive(ctx context.Context, key string, done func(FailedEvent)) error {
	evs, err := d.receiveAll(ctx, redriveVisibility)
	if err != nil {
		return err
	}
	for i, ev := range evs {
		if key != "" && !ev.hasKey(key) {
			if err := d.release(ctx, evs[i:i+1]); err != nil {
				return err
			}
			continue
		}
		if err := d.redrive(ctx, ev); err != nil {
			return errors.Join(err, d.release(ctx, evs[i:]))
		}
		done(ev)
	}
	return nil
}

// redrive invokes the function with ev and deletes it once that succeeds.
func (d *DLQ) redrive(ctx context.Context, ev FailedEvent) error {
	out, err := d.lambda.Invoke(ctx, &lambda.InvokeInput{
		FunctionName:   &d.function,
		InvocationType: lambdatypes.InvocationTypeRequestResponse,
		Payload:        ev.payload,
	})
	if err != nil {
		return fmt.Errorf("redriving %v: %w", ev.Keys, err)
	}
	if out.FunctionError != nil {
		return fmt.Errorf("redriving %v: the function failed again: %s", ev.Keys, out.Payload)
	}
	if _, err := d.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &d.queueURL, ReceiptHandle: &ev.receipt}); err != nil {
		return fmt.Errorf("removing %v from the dead-letter queue after redriving it: %w", ev.Keys, err)
	}
	return nil
}

// release makes evs visible on the queue again straight away.
func (d *DLQ) release(ctx context.Context, evs []FailedEvent) error {
	for _, ev := range evs {
		_, err := d.sqs.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
			QueueUrl:          &d.queueURL,
			ReceiptHandle:     &ev.receipt,
			VisibilityTimeout: 0,
		})
		if err != nil {
			return fmt.Errorf("returning %v to the dead-letter queue: %w", ev.Keys, err)
		}
	}
	return nil
}

func (ev FailedEvent) hasKey(key string) bool {
	for _, k := range ev.Keys {
		if k == key {
			return true
		}
	}
	return false
}

// receiveAll receives until the queue returns nothing, hiding each message
// for visibility seconds so none is received twice.
func (d *DLQ) receiveAll(ctx context.Context, visibility int32) ([]FailedEvent, error) {
	var evs []FailedEvent
	for {
		out, err := d.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &d.queueURL,
			MaxNumberOfMessages: 10,
			VisibilityTimeout:   visibility,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			return nil, fmt.Errorf("reading the dead-letter queue: %w", err)
		}
		if len(out.Messages) == 0 {
			return evs, nil
		}
		for _, m := range out.Messages {
			ev, err := parseFailedEvent(aws.ToString(m.Body))
			if err != nil {
				return nil, err
			}
			ev.receipt = aws.ToString(m.ReceiptHandle)
			evs = append(evs, ev)
		}
	}
}

func parseFailedEvent(body string) (FailedEvent, error) {
	var rec invocationRecord
	if err := json.Unmarshal([]byte(body), &rec); err != nil {
		return FailedEvent{}, fmt.Errorf("parsing a dead-letter message: %w", err)
	}
	var event events.S3Event
	if err := json.Unmarshal(rec.RequestPayload, &event); err != nil {
		return FailedEvent{}, fmt.Errorf("parsing a dead-lettered event: %w", err)
	}
	ev := FailedEvent{
		Time:      rec.Timestamp,
		Condition: rec.RequestContext.Condition,
		Error:     rec.ResponsePayload.ErrorMessage,
		Attempts:  rec.RequestContext.ApproximateInvokeCount,
		payload:   rec.RequestPayload,
	}
	for _, r := range event.Records {
		ev.Keys = append(ev.Keys, r.S3.Object.URLDecodedKey)
	}
	return ev, nil
}
