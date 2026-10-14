package moderation

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/moderation/mocks"
)

type ReviewLogSuite struct {
	suite.Suite
	logs    *mocks.MockLogsAPI
	reviews *ReviewLog
	now     time.Time
}

func TestReviewLogSuite(t *testing.T) {
	suite.Run(t, new(ReviewLogSuite))
}

func (s *ReviewLogSuite) SetupTest() {
	s.logs = mocks.NewMockLogsAPI(s.T())
	s.now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	s.reviews = &ReviewLog{logs: s.logs, logGroup: testLogGroup, now: func() time.Time { return s.now }}
}

func (s *ReviewLogSuite) put() {
	s.logs.EXPECT().PutLogEvents(mock.Anything, mock.MatchedBy(func(in *cloudwatchlogs.PutLogEventsInput) bool {
		if aws.ToString(in.LogGroupName) != testLogGroup || aws.ToString(in.LogStreamName) != reviewStream || len(in.LogEvents) != 1 {
			return false
		}
		var r Review
		s.Require().NoError(json.Unmarshal([]byte(aws.ToString(in.LogEvents[0].Message)), &r))
		return r == Review{Time: s.now, Msg: reviewMsg, Key: testKey, Action: ActionRejected, Reason: "spam"} &&
			aws.ToInt64(in.LogEvents[0].Timestamp) == s.now.UnixMilli()
	})).Return(&cloudwatchlogs.PutLogEventsOutput{}, nil)
}

func (s *ReviewLogSuite) TestRecord() {
	s.logs.EXPECT().CreateLogStream(mock.Anything, mock.MatchedBy(func(in *cloudwatchlogs.CreateLogStreamInput) bool {
		return aws.ToString(in.LogGroupName) == testLogGroup && aws.ToString(in.LogStreamName) == reviewStream
	})).Return(&cloudwatchlogs.CreateLogStreamOutput{}, nil)
	s.put()

	s.Require().NoError(s.reviews.Record(s.T().Context(), testKey, ActionRejected, "spam"))
}

func (s *ReviewLogSuite) TestRecord_StreamExists() {
	s.logs.EXPECT().CreateLogStream(mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("operation error: %w", &smithy.GenericAPIError{Code: "ResourceAlreadyExistsException"}))
	s.put()

	s.Require().NoError(s.reviews.Record(s.T().Context(), testKey, ActionRejected, "spam"))
}

func (s *ReviewLogSuite) TestRecord_CreateStreamFails() {
	s.logs.EXPECT().CreateLogStream(mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	s.Require().ErrorContains(s.reviews.Record(s.T().Context(), testKey, ActionRejected, "spam"), "boom")
}
