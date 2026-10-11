package uploadlimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/mocks"
)

type LimiterSuite struct {
	suite.Suite
	mockCounter *mocks.MockUploadCounter
	limiter     *Limiter
}

func TestLimiterSuite(t *testing.T) {
	suite.Run(t, new(LimiterSuite))
}

func (s *LimiterSuite) SetupTest() {
	s.mockCounter = mocks.NewMockUploadCounter(s.T())
	s.limiter = New(s.mockCounter, 500)
}

func (s *LimiterSuite) at(t time.Time) {
	s.limiter.now = func() time.Time { return t }
}

func (s *LimiterSuite) TestReserve_CountsAgainstTheUTCDay() {
	s.at(time.Date(2026, 10, 11, 20, 30, 0, 0, time.FixedZone("MDT", -6*3600)))
	s.mockCounter.EXPECT().
		Increment(mock.Anything, "2026-10-12", int64(500), time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)).
		Return(nil)

	s.Require().NoError(s.limiter.Reserve(s.T().Context()))
}

func (s *LimiterSuite) TestReserve_LimitReached_ReturnsIt() {
	s.at(time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC))
	s.mockCounter.EXPECT().
		Increment(mock.Anything, "2026-10-11", int64(500), mock.Anything).
		Return(repository.ErrUploadLimitReached)

	s.Require().ErrorIs(s.limiter.Reserve(s.T().Context()), repository.ErrUploadLimitReached)
}
