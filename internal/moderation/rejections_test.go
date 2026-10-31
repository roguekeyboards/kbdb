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

	"github.com/roguekeyboards/mykeebs-api/internal/moderation/mocks"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
	repomocks "github.com/roguekeyboards/mykeebs-api/internal/repository/mocks"
)

const testLogGroup = "/aws/lambda/kbdb-moderate-test"

type RejectionsSuite struct {
	suite.Suite
	logs         *mocks.MockLogsAPI
	profiles     *repomocks.MockProfileRepository
	images       *Images
	pollInterval time.Duration
}

func TestRejectionsSuite(t *testing.T) {
	suite.Run(t, new(RejectionsSuite))
}

func (s *RejectionsSuite) SetupTest() {
	s.logs = mocks.NewMockLogsAPI(s.T())
	s.profiles = repomocks.NewMockProfileRepository(s.T())
	s.images = &Images{Profiles: s.profiles}
	s.pollInterval, queryPollInterval = queryPollInterval, 0
}

func (s *RejectionsSuite) TearDownTest() {
	queryPollInterval = s.pollInterval
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

	rs, truncated, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.False(truncated)
	s.Equal([]Rejection{
		{Time: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC), Key: "switches/alice/sw1/image/up1", Reason: "Graphic Violence", Confidence: 91.25},
		{Time: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC), Key: "profiles/bob/avatar/up2", Reason: "unprocessable"},
	}, rs)
}

func (s *RejectionsSuite) TestKeepsNewestPerKey() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-12T10:00:05Z","key":"profiles/bob/avatar/up2","reason":"unprocessable"}`),
		row(`{"time":"2026-10-12T10:00:00Z","key":"profiles/bob/avatar/up2","reason":"unprocessable"}`),
	)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.Equal([]Rejection{
		{Time: time.Date(2026, 10, 12, 10, 0, 5, 0, time.UTC), Key: "profiles/bob/avatar/up2", Reason: "unprocessable"},
	}, rs)
}

// Truncation is judged on the rows the query returned, before duplicate
// keys are dropped.
func (s *RejectionsSuite) TestTruncated() {
	start := time.Now()
	s.started(start)
	rows := make([][]logtypes.ResultField, RejectionsLimit)
	for i := range rows {
		rows[i] = row(`{"time":"2026-10-12T10:00:00Z","key":"profiles/bob/avatar/up2","reason":"unprocessable"}`)
	}
	s.results(logtypes.QueryStatusComplete, rows...)

	rs, truncated, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.True(truncated)
	s.Len(rs, 1)
}

// Results are newest first, so a review line comes before the decision
// line it reviews.
func (s *RejectionsSuite) reviewedResults(start time.Time) {
	s.started(start)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-13T12:00:00Z","msg":"operator review","key":"builds/carol/b1/images/i1","action":"rejected","reason":"spam"}`),
		row(`{"time":"2026-10-13T11:00:00Z","msg":"operator review","key":"switches/alice/sw1/image/up1","action":"approved"}`),
		row(`{"time":"2026-10-12T10:00:00Z","msg":"moderated image","key":"switches/alice/sw1/image/up1","decision":"rejected","reason":"Explicit"}`),
		row(`{"time":"2026-10-11T09:00:00Z","msg":"moderated image","key":"profiles/bob/avatar/up2","decision":"rejected","reason":"unprocessable"}`),
	)
}

// avatar stores owner's avatar at path with status.
func (s *RejectionsSuite) avatar(owner, path string, status repository.ImageStatus) {
	p := repository.ProfileImageKey(path)
	s.profiles.EXPECT().Get(mock.Anything, owner).Return(&repository.Profile{AvatarPath: &p, AvatarStatus: status}, nil)
}

func (s *RejectionsSuite) TestHidesReviewed() {
	start := time.Now()
	s.reviewedResults(start)
	s.avatar("bob", "profiles/bob/avatar/up2", repository.ImageStatusRejected)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, false)
	s.Require().NoError(err)
	s.Equal([]Rejection{
		{Time: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC), Key: "profiles/bob/avatar/up2", Reason: "unprocessable"},
	}, rs)
}

func (s *RejectionsSuite) TestAllIncludesReviewedAndTakedowns() {
	start := time.Now()
	s.reviewedResults(start)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.Equal([]Rejection{
		{
			Time: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC), Key: "builds/carol/b1/images/i1", Reason: "spam", Manual: true,
			Review: &Review{Time: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC), Msg: "operator review", Key: "builds/carol/b1/images/i1", Action: ActionRejected, Reason: "spam"},
		},
		{
			Time: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC), Key: "switches/alice/sw1/image/up1", Reason: "Explicit",
			Review: &Review{Time: time.Date(2026, 10, 13, 11, 0, 0, 0, time.UTC), Msg: "operator review", Key: "switches/alice/sw1/image/up1", Action: ActionApproved},
		},
		{Time: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC), Key: "profiles/bob/avatar/up2", Reason: "unprocessable"},
	}, rs)
}

// Only the latest review counts: a rejection upheld and later approved
// shows as approved.
func (s *RejectionsSuite) TestLatestReviewWins() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-13T12:00:00Z","msg":"operator review","key":"profiles/bob/avatar/up2","action":"approved"}`),
		row(`{"time":"2026-10-12T12:00:00Z","msg":"operator review","key":"profiles/bob/avatar/up2","action":"upheld"}`),
		row(`{"time":"2026-10-11T09:00:00Z","msg":"moderated image","key":"profiles/bob/avatar/up2","decision":"rejected","reason":"Explicit"}`),
	)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.Require().Len(rs, 1)
	s.Equal(ActionApproved, rs[0].Review.Action)
}

// A takedown later reversed stays in the history, with the approval as
// its review.
func (s *RejectionsSuite) TestReversedTakedown() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-13T12:00:00Z","msg":"operator review","key":"builds/carol/b1/images/i1","action":"approved"}`),
		row(`{"time":"2026-10-12T12:00:00Z","msg":"operator review","key":"builds/carol/b1/images/i1","action":"rejected","reason":"spam"}`),
	)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.Require().Len(rs, 1)
	s.True(rs[0].Manual)
	s.Equal("spam", rs[0].Reason)
	s.Equal(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), rs[0].Time)
	s.Equal(ActionApproved, rs[0].Review.Action)
}

// An image its owner replaced or deleted, or one approved without a
// recorded review, has nothing left to decide.
func (s *RejectionsSuite) TestHidesImagesNoLongerRejected() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-13T09:00:00Z","msg":"moderated image","key":"profiles/bob/avatar/up2","decision":"rejected","reason":"Explicit"}`),
		row(`{"time":"2026-10-12T09:00:00Z","msg":"moderated image","key":"profiles/carol/avatar/up3","decision":"rejected","reason":"Explicit"}`),
		row(`{"time":"2026-10-11T09:00:00Z","msg":"moderated image","key":"profiles/dave/avatar/up4","decision":"rejected","reason":"Explicit"}`),
		row(`{"time":"2026-10-10T09:00:00Z","msg":"moderated image","key":"profiles/erin/avatar/up5","decision":"rejected","reason":"Explicit"}`),
	)
	s.avatar("bob", "profiles/bob/avatar/up2", repository.ImageStatusRejected)
	s.avatar("carol", "profiles/carol/avatar/new", repository.ImageStatusPending)
	s.profiles.EXPECT().Get(mock.Anything, "dave").Return(nil, repository.ErrNotFound)
	s.avatar("erin", "profiles/erin/avatar/up5", repository.ImageStatusApproved)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, false)
	s.Require().NoError(err)
	s.Require().Len(rs, 1)
	s.Equal("profiles/bob/avatar/up2", rs[0].Key)
}

func (s *RejectionsSuite) TestQueryFailed() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusFailed)

	_, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, false)
	s.Require().ErrorContains(err, "ended Failed")
}

func (s *RejectionsSuite) TestStartFails() {
	s.logs.EXPECT().StartQuery(mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	_, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, time.Now(), false)
	s.Require().ErrorContains(err, "boom")
}

func (s *RejectionsSuite) TestSkipsMalformedLine() {
	start := time.Now()
	s.started(start)
	s.results(logtypes.QueryStatusComplete,
		row(`{"time":"2026-10-12T10:00:00Z","key":"profiles/bob/avatar/up2","rea`),
		row(`{"time":"2026-10-11T09:00:00Z","key":"profiles/carol/avatar/up3","reason":"unprocessable"}`),
	)

	rs, _, err := Rejections(s.T().Context(), s.logs, s.images, testLogGroup, start, true)
	s.Require().NoError(err)
	s.Equal([]Rejection{
		{Time: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC), Key: "profiles/carol/avatar/up3", Reason: "unprocessable"},
	}, rs)
}
