package moderation

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/moderation/mocks"
)

const (
	testQueue    = "https://sqs.us-east-2.amazonaws.com/1/dlq"
	testFunction = "mykeebs-api-moderate-test"
)

// A RetriesExhausted record as Lambda writes it, for an object key S3
// URL-encoded in the event.
const failedRecord = `{
  "version": "1.0",
  "timestamp": "2026-10-13T10:00:00.000Z",
  "requestContext": {"requestId": "r1", "functionArn": "arn", "condition": "RetriesExhausted", "approximateInvokeCount": 3},
  "requestPayload": {"Records": [{"s3": {"bucket": {"name": "images"}, "object": {"key": "profiles/user%7C1/avatar/up1"}}}]},
  "responseContext": {"statusCode": 200, "executedVersion": "$LATEST", "functionError": "Unhandled"},
  "responsePayload": {"errorMessage": "detecting moderation labels: throttled", "errorType": "errorString"}
}`

// An EventAgeExceeded record: the event never ran, so it has no response.
const agedRecord = `{
  "version": "1.0",
  "timestamp": "2026-10-13T11:00:00.000Z",
  "requestContext": {"requestId": "r2", "functionArn": "arn", "condition": "EventAgeExceeded", "approximateInvokeCount": 0},
  "requestPayload": {"Records": [{"s3": {"bucket": {"name": "images"}, "object": {"key": "switches/alice/sw1/image/up2"}}}]}
}`

type DLQSuite struct {
	suite.Suite
	sqs    *mocks.MockSQSAPI
	lambda *mocks.MockLambdaAPI
	dlq    *DLQ
}

func TestDLQSuite(t *testing.T) {
	suite.Run(t, new(DLQSuite))
}

func (s *DLQSuite) SetupTest() {
	s.sqs = mocks.NewMockSQSAPI(s.T())
	s.lambda = mocks.NewMockLambdaAPI(s.T())
	s.dlq = &DLQ{sqs: s.sqs, lambda: s.lambda, queueURL: testQueue, function: testFunction}
}

// queued expects receives returning bodies, then an empty one.
func (s *DLQSuite) queued(visibility int32, bodies ...string) {
	msgs := make([]sqstypes.Message, len(bodies))
	for i, b := range bodies {
		msgs[i] = sqstypes.Message{Body: aws.String(b), ReceiptHandle: aws.String("rh" + string(rune('1'+i)))}
	}
	matches := mock.MatchedBy(func(in *sqs.ReceiveMessageInput) bool {
		return aws.ToString(in.QueueUrl) == testQueue && in.VisibilityTimeout == visibility
	})
	s.sqs.EXPECT().ReceiveMessage(mock.Anything, matches).Return(&sqs.ReceiveMessageOutput{Messages: msgs}, nil).Once()
	s.sqs.EXPECT().ReceiveMessage(mock.Anything, matches).Return(&sqs.ReceiveMessageOutput{}, nil).Once()
}

func (s *DLQSuite) invoked(key string, functionError bool) {
	out := &lambda.InvokeOutput{StatusCode: 200}
	if functionError {
		out.FunctionError = aws.String("Unhandled")
		out.Payload = []byte(`{"errorMessage":"still failing"}`)
	}
	s.lambda.EXPECT().Invoke(mock.Anything, mock.MatchedBy(func(in *lambda.InvokeInput) bool {
		return aws.ToString(in.FunctionName) == testFunction &&
			in.InvocationType == lambdatypes.InvocationTypeRequestResponse &&
			string(in.Payload) != "" && containsKey(in.Payload, key)
	})).Return(out, nil).Once()
}

func containsKey(payload []byte, key string) bool {
	ev, err := parseFailedEvent(`{"requestPayload":` + string(payload) + `}`)
	return err == nil && ev.hasKey(key)
}

func (s *DLQSuite) deleted(receipt string) {
	s.sqs.EXPECT().DeleteMessage(mock.Anything, mock.MatchedBy(func(in *sqs.DeleteMessageInput) bool {
		return aws.ToString(in.QueueUrl) == testQueue && aws.ToString(in.ReceiptHandle) == receipt
	})).Return(&sqs.DeleteMessageOutput{}, nil).Once()
}

// released expects each receipt made visible again.
func (s *DLQSuite) released(receipts ...string) {
	for _, r := range receipts {
		s.sqs.EXPECT().ChangeMessageVisibility(mock.Anything, mock.MatchedBy(func(in *sqs.ChangeMessageVisibilityInput) bool {
			return aws.ToString(in.ReceiptHandle) == r && in.VisibilityTimeout == 0
		})).Return(&sqs.ChangeMessageVisibilityOutput{}, nil).Once()
	}
}

func (s *DLQSuite) TestList() {
	s.queued(listVisibility, failedRecord, agedRecord)
	s.released("rh1", "rh2")

	evs, err := s.dlq.List(s.T().Context())
	s.Require().NoError(err)
	s.Require().Len(evs, 2)
	s.Equal(time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC), evs[0].Time)
	s.Equal([]string{"profiles/user|1/avatar/up1"}, evs[0].Keys)
	s.Equal("RetriesExhausted", evs[0].Condition)
	s.Equal(3, evs[0].Attempts)
	s.Equal("detecting moderation labels: throttled", evs[0].Error)
	s.Equal("EventAgeExceeded", evs[1].Condition)
	s.Empty(evs[1].Error)
}

func (s *DLQSuite) TestList_MalformedMessage() {
	s.sqs.EXPECT().ReceiveMessage(mock.Anything, mock.Anything).
		Return(&sqs.ReceiveMessageOutput{Messages: []sqstypes.Message{{Body: aws.String("not json")}}}, nil)

	_, err := s.dlq.List(s.T().Context())
	s.Require().ErrorContains(err, "parsing a dead-letter message")
}

func (s *DLQSuite) TestCount() {
	s.sqs.EXPECT().GetQueueAttributes(mock.Anything, mock.Anything).Return(&sqs.GetQueueAttributesOutput{Attributes: map[string]string{
		"ApproximateNumberOfMessages":           "2",
		"ApproximateNumberOfMessagesNotVisible": "1",
	}}, nil)

	n, err := s.dlq.Count(s.T().Context())
	s.Require().NoError(err)
	s.Equal(3, n)
}

func (s *DLQSuite) TestRedrive() {
	s.queued(redriveVisibility, failedRecord, agedRecord)
	s.invoked("profiles/user|1/avatar/up1", false)
	s.deleted("rh1")
	s.invoked("switches/alice/sw1/image/up2", false)
	s.deleted("rh2")

	var done []string
	err := s.dlq.Redrive(s.T().Context(), "", func(ev FailedEvent) { done = append(done, ev.Keys...) })
	s.Require().NoError(err)
	s.Equal([]string{"profiles/user|1/avatar/up1", "switches/alice/sw1/image/up2"}, done)
}

// The first event fails again: it stays queued and the rest aren't tried.
func (s *DLQSuite) TestRedrive_StopsAtFirstFailure() {
	s.queued(redriveVisibility, failedRecord, agedRecord)
	s.invoked("profiles/user|1/avatar/up1", true)
	s.released("rh1", "rh2")

	err := s.dlq.Redrive(s.T().Context(), "", func(FailedEvent) { s.Fail("nothing should be redriven") })
	s.Require().ErrorContains(err, "still failing")
}

func (s *DLQSuite) TestRedrive_InvokeFails() {
	s.queued(redriveVisibility, failedRecord)
	s.lambda.EXPECT().Invoke(mock.Anything, mock.Anything).Return(nil, errors.New("throttled"))
	s.released("rh1")

	err := s.dlq.Redrive(s.T().Context(), "", func(FailedEvent) {})
	s.Require().ErrorContains(err, "throttled")
}

func (s *DLQSuite) TestRedrive_OneKey() {
	s.queued(redriveVisibility, failedRecord, agedRecord)
	s.released("rh1")
	s.invoked("switches/alice/sw1/image/up2", false)
	s.deleted("rh2")

	var done []string
	err := s.dlq.Redrive(s.T().Context(), "switches/alice/sw1/image/up2", func(ev FailedEvent) { done = append(done, ev.Keys...) })
	s.Require().NoError(err)
	s.Equal([]string{"switches/alice/sw1/image/up2"}, done)
}
