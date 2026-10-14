package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// RejectionsLimit is the most rejections one Rejections call returns,
// newest first.
const RejectionsLimit = 10000

// rejectionsQuery finds the moderation function's rejected decision lines
// (see Moderator.moderate) and the operator's review lines (see
// ReviewLog).
var rejectionsQuery = fmt.Sprintf(`fields @message
| filter (msg = "moderated image" and decision = "rejected") or msg = %q
| sort @timestamp desc
| limit %d`, reviewMsg, RejectionsLimit)

// queryPollInterval is how often Rejections checks whether its query has
// finished.
var queryPollInterval = time.Second

// logsAPI is the subset of the CloudWatch Logs client Rejections and
// ReviewLog use.
type logsAPI interface {
	CreateLogStream(ctx context.Context, in *cloudwatchlogs.CreateLogStreamInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error)
	PutLogEvents(ctx context.Context, in *cloudwatchlogs.PutLogEventsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error)
	StartQuery(ctx context.Context, in *cloudwatchlogs.StartQueryInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartQueryOutput, error)
	GetQueryResults(ctx context.Context, in *cloudwatchlogs.GetQueryResultsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetQueryResultsOutput, error)
}

// Rejection is one rejection: the moderation function's, or an
// operator's takedown (Manual). Confidence is the triggering label's, and
// 0 for an unprocessable image or a takedown. Review is the latest review
// of it, nil while it awaits one.
type Rejection struct {
	Time       time.Time
	Key        string
	Reason     string
	Confidence float32
	Manual     bool
	Review     *Review
}

// logLine is the part of a decision or review line Rejections reads.
type logLine struct {
	Time   time.Time     `json:"time"`
	Msg    string        `json:"msg"`
	Key    string        `json:"key"`
	Reason string        `json:"reason"`
	Action string        `json:"action"`
	Labels []loggedLabel `json:"labels"`
}

// Rejections lists the rejections logged to logGroup since start, newest
// first, once per key: a retried event logs its decision again. Unless
// all is set, only those awaiting review are listed, leaving out any whose
// image is no longer a current rejected one (its owner deleted or
// replaced it): there's nothing left to decide. all adds reviewed ones
// and takedowns. truncated reports that the query hit RejectionsLimit, so
// older ones were cut.
func Rejections(ctx context.Context, client logsAPI, images *Images, logGroup string, start time.Time, all bool) (rs []Rejection, truncated bool, err error) {
	q, err := client.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupNames: []string{logGroup},
		QueryString:   aws.String(rejectionsQuery),
		StartTime:     aws.Int64(start.Unix()),
		EndTime:       aws.Int64(time.Now().Unix()),
	})
	if err != nil {
		return nil, false, fmt.Errorf("querying %s: %w", logGroup, err)
	}

	for {
		out, err := client.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{QueryId: q.QueryId})
		if err != nil {
			return nil, false, fmt.Errorf("reading the results of the query on %s: %w", logGroup, err)
		}
		switch out.Status {
		case logtypes.QueryStatusComplete:
			rs := parseRejections(out.Results, all)
			if !all {
				if rs, err = stillRejected(ctx, images, rs); err != nil {
					return nil, false, err
				}
			}
			return rs, len(out.Results) >= RejectionsLimit, nil
		case logtypes.QueryStatusScheduled, logtypes.QueryStatusRunning:
		default:
			return nil, false, fmt.Errorf("the query on %s ended %s", logGroup, out.Status)
		}

		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(queryPollInterval):
		}
	}
}

// stillRejected keeps the rejections whose image an item still holds as
// rejected. A key that doesn't parse is kept, so it's seen.
func stillRejected(ctx context.Context, images *Images, rs []Rejection) ([]Rejection, error) {
	kept := rs[:0]
	for _, r := range rs {
		ref, err := imagekey.Parse(r.Key)
		if err != nil {
			kept = append(kept, r)
			continue
		}
		status, current, err := images.Status(ctx, ref, r.Key)
		if err != nil {
			return nil, fmt.Errorf("reading the status of %q: %w", r.Key, err)
		}
		if current && status == repository.ImageStatusRejected {
			kept = append(kept, r)
		}
	}
	return kept, nil
}

// parseRejections reads query results, which are newest first: a review
// seen before a key's decision line is a later review of it.
func parseRejections(results [][]logtypes.ResultField, all bool) []Rejection {
	reviews := map[string]*Review{}   // each key's latest review
	takedowns := map[string]*Review{} // each key's earliest takedown
	listed := map[string]bool{}
	var out []Rejection
	for _, row := range results {
		for _, f := range row {
			if aws.ToString(f.Field) != "@message" {
				continue
			}
			var line logLine
			if err := json.Unmarshal([]byte(aws.ToString(f.Value)), &line); err != nil {
				slog.Warn("skipping a log line that isn't valid JSON", "error", err)
				continue
			}
			if line.Msg == reviewMsg {
				rv := &Review{Time: line.Time, Msg: line.Msg, Key: line.Key, Action: line.Action, Reason: line.Reason}
				if reviews[line.Key] == nil {
					reviews[line.Key] = rv
				}
				if rv.Action == ActionRejected {
					takedowns[line.Key] = rv
				}
				continue
			}
			if listed[line.Key] {
				continue
			}
			listed[line.Key] = true
			r := Rejection{Time: line.Time, Key: line.Key, Reason: line.Reason, Review: reviews[line.Key]}
			for _, l := range line.Labels {
				if l.Name == line.Reason {
					r.Confidence = l.Confidence
				}
			}
			out = append(out, r)
		}
	}

	// A takedown has no decision line of its own: its review is the record,
	// and any later review (an approval reversing it) is its Review.
	for key, td := range takedowns {
		if !listed[key] {
			out = append(out, Rejection{Time: td.Time, Key: key, Reason: td.Reason, Manual: true, Review: reviews[key]})
		}
	}
	if !all {
		out = slices.DeleteFunc(out, func(r Rejection) bool { return r.Review != nil })
	}
	slices.SortStableFunc(out, func(a, b Rejection) int { return b.Time.Compare(a.Time) })
	return out
}
