package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// RejectionsLimit is the most rejections one Rejections call returns,
// newest first.
const RejectionsLimit = 10000

// rejectionsQuery finds the moderation function's rejected decision lines
// (see Moderator.moderate). Manual rejections aren't logged there.
var rejectionsQuery = fmt.Sprintf(`fields @message
| filter msg = "moderated image" and decision = "rejected"
| sort @timestamp desc
| limit %d`, RejectionsLimit)

// queryPollInterval is how often Rejections checks whether its query has
// finished.
var queryPollInterval = time.Second

// logsAPI is the subset of the CloudWatch Logs client Rejections uses.
type logsAPI interface {
	StartQuery(ctx context.Context, in *cloudwatchlogs.StartQueryInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartQueryOutput, error)
	GetQueryResults(ctx context.Context, in *cloudwatchlogs.GetQueryResultsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetQueryResultsOutput, error)
}

// Rejection is one rejected decision from the moderation function's logs.
// Confidence is the triggering label's, and 0 for an unprocessable image.
type Rejection struct {
	Time       time.Time
	Key        string
	Reason     string
	Confidence float32
}

// decisionLine is the part of a decision line Rejections reads.
type decisionLine struct {
	Time   time.Time     `json:"time"`
	Key    string        `json:"key"`
	Reason string        `json:"reason"`
	Labels []loggedLabel `json:"labels"`
}

// Rejections lists the rejections logged to logGroup since start, newest
// first, once per key: a retried event logs its decision again.
func Rejections(ctx context.Context, client logsAPI, logGroup string, start time.Time) ([]Rejection, error) {
	q, err := client.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupNames: []string{logGroup},
		QueryString:   aws.String(rejectionsQuery),
		StartTime:     aws.Int64(start.Unix()),
		EndTime:       aws.Int64(time.Now().Unix()),
	})
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", logGroup, err)
	}

	for {
		out, err := client.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{QueryId: q.QueryId})
		if err != nil {
			return nil, fmt.Errorf("reading the results of the query on %s: %w", logGroup, err)
		}
		switch out.Status {
		case logtypes.QueryStatusComplete:
			return parseRejections(out.Results)
		case logtypes.QueryStatusScheduled, logtypes.QueryStatusRunning:
		default:
			return nil, fmt.Errorf("the query on %s ended %s", logGroup, out.Status)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(queryPollInterval):
		}
	}
}

func parseRejections(results [][]logtypes.ResultField) ([]Rejection, error) {
	out := make([]Rejection, 0, len(results))
	seen := make(map[string]bool, len(results))
	for _, row := range results {
		for _, f := range row {
			if aws.ToString(f.Field) != "@message" {
				continue
			}
			var line decisionLine
			if err := json.Unmarshal([]byte(aws.ToString(f.Value)), &line); err != nil {
				return nil, fmt.Errorf("parsing a decision line: %w", err)
			}
			if seen[line.Key] {
				continue
			}
			seen[line.Key] = true
			r := Rejection{Time: line.Time, Key: line.Key, Reason: line.Reason}
			for _, l := range line.Labels {
				if l.Name == line.Reason {
					r.Confidence = l.Confidence
				}
			}
			out = append(out, r)
		}
	}
	return out, nil
}
