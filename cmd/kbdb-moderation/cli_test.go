package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type CLISuite struct {
	suite.Suite
}

func TestCLISuite(t *testing.T) {
	suite.Run(t, new(CLISuite))
}

func (s *CLISuite) TestParseSince() {
	cases := map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"60d": 60 * 24 * time.Hour,
		"12h": 12 * time.Hour,
		"90m": 90 * time.Minute,
	}
	for in, want := range cases {
		got, err := parseSince(in)
		s.Require().NoError(err, in)
		s.Equal(want, got, in)
	}
}

func (s *CLISuite) TestParseSince_Invalid() {
	for _, in := range []string{"", "d", "0d", "-3d", "xd", "7", "-1h", "week"} {
		_, err := parseSince(in)
		s.Error(err, in)
	}
}
