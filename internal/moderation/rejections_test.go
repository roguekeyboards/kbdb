package moderation

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/moderation/mocks"
)

const testLogGroup = "/aws/lambda/kbdb-moderate-test"

type RejectionsSuite struct {
	suite.Suite
	logs *mocks.MockLogsAPI
}

func TestRejectionsSuite(t *testing.T) {
	suite.Run(t, new(RejectionsSuite))
}

func (s *RejectionsSuite) SetupTest() {
	s.logs = mocks.NewMockLogsAPI(s.T())
	queryPollInterval = 0
}

func row(message string) []logtypes.ResultField {
	return []logtypes.ResultField{
		{Field: aws.String("@message"), Value: aws.String(message)},
		{Field: aws.String("@ptr"), Value: aws.String("ptr")},
	}
}

func (s *RejectionsSuite) started(start time.Time) {
	s.logs.EXPECT().StartQuery(mock.Anything, mock.MatchedBy(func(in *cloudwatchlogs.StartQueryInput) bool {
		return len(in.LogGroupNames) == 1 && in.LogGroupNames[0] == testLogGroup &&
			aws.ToInt64(in.StartTime) == start.Unix() &&
			aws.ToString(in.QueryString) == rejectionsQuery
	})).Return(&cloudwatchlogs.StartQueryOutput{QueryId: aws.String("q1")}, nil)
}

func (s *RejectionsSuite) results(status logtypes.QueryStatus, rows ...[]logtypes.ResultField) {
	s.logs.EXPECT().GetQueryResults(mock.Anything, mock.MatchedBy(func(in *cloudwatchlogs.GetQueryResultsInput) bool {
		return aws.ToString(in.QueryId) == "q1"
	})).Return(&cloudwatchlogs.GetQueryResultsOutput{Status: status, Results: rows}, nil).Once()
}

func (s *RejectionsSuite) TestParsesDecisionLines() {
	start := time.Now().Add(-time.Hour)
	s.started(start)
	s.results(logtypes.QueryStatusRunning)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-12T10:00:00Z","msg":"moderated image","key":"switches/alice/sw1/image/up1","decision":"rejected","reason":"Graphic Violence","labels":[{"name":"Violence","level":1,"confidence":97.5},{"name":"Graphic Violence","parent":"Violence","level":2,"confidence":91.25}]}`),
		row(`{"time":"2026-10-11T09:00:00Z","msg":"moderated image","key":"profiles/bob/avatar/up2","decision":"rejected","reason":"unprocessable","error":"bad image"}`),
	)

	rs, err := Rejections(s.T().Context(), s.logs, testLogGroup, start)
	s.Require().NoError(err)
	s.Equal([]Rejection{
		{Time: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC), Key: "switches/alice/sw1/image/up1", Reason: "Graphic Violence", Confidence: 91.25},
		{Time: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC), Key: "profiles/bob/avatar/up2", Reason: "unprocessable"},
	}, rs)
}

func (s *RejectionsSuite) TestQueryFailed() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusFailed)

	_, err := Rejections(s.T().Context(), s.logs, testLogGroup, start)
	s.Require().ErrorContains(err, "ended Failed")
}

func (s *RejectionsSuite) TestStartFails() {
	s.logs.EXPECT().StartQuery(mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	_, err := Rejections(s.T().Context(), s.logs, testLogGroup, time.Now())
	s.Require().ErrorContains(err, "boom")
}

func (s *RejectionsSuite) TestMalformedLine() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusComplete, row(`not json`))

	_, err := Rejections(s.T().Context(), s.logs, testLogGroup, start)
	s.Require().ErrorContains(err, "parsing a decision line")
}
