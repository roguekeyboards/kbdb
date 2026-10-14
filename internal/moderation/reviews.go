package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"
)

// reviewMsg marks an operator's review line, as "moderated image" marks
// the moderation function's decision lines.
const reviewMsg = "operator review"

// reviewStream is the log stream review lines go to, in the moderation
// function's log group, so they're kept as long as its decisions.
const reviewStream = "operator-reviews"

// The actions a review records.
const (
	ActionUpheld   = "upheld"
	ActionApproved = "approved"
	ActionRejected = "rejected"
)

// Review is an operator's decision on an image: upholding a rejection,
// approving it, or taking down an approved image.
type Review struct {
	Time   time.Time `json:"time"`
	Msg    string    `json:"msg"`
	Key    string    `json:"key"`
	Action string    `json:"action"`
	Reason string    `json:"reason,omitempty"`
}

// ReviewLog writes review lines to the moderation function's log group.
type ReviewLog struct {
	logs     logsAPI
	logGroup string
	now      func() time.Time
}

// NewReviewLog returns a ReviewLog writing to logGroup.
func NewReviewLog(client *cloudwatchlogs.Client, logGroup string) *ReviewLog {
	return &ReviewLog{logs: client, logGroup: logGroup, now: time.Now}
}

// Record writes one review line for key.
func (l *ReviewLog) Record(ctx context.Context, key, action, reason string) error {
	r := Review{Time: l.now().UTC(), Msg: reviewMsg, Key: key, Action: action, Reason: reason}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = l.logs.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  &l.logGroup,
		LogStreamName: aws.String(reviewStream),
	})
	if err != nil && !isAPIError(err, "ResourceAlreadyExistsException") {
		return fmt.Errorf("creating the review log stream: %w", err)
	}
	_, err = l.logs.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  &l.logGroup,
		LogStreamName: aws.String(reviewStream),
		LogEvents: []logtypes.InputLogEvent{{
			Message:   aws.String(string(line)),
			Timestamp: aws.Int64(r.Time.UnixMilli()),
		}},
	})
	if err != nil {
		return fmt.Errorf("recording the review of %q: %w", key, err)
	}
	return nil
}

// isAPIError reports an AWS error with code, whether or not the SDK maps
// it to a typed error.
func isAPIError(err error, code string) bool {
	apiErr, ok := errors.AsType[smithy.APIError](err)
	return ok && apiErr.ErrorCode() == code
}
