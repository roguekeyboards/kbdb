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
		"7d":    7 * 24 * time.Hour,
		"60d":   60 * 24 * time.Hour,
		"1440h": 60 * 24 * time.Hour,
		"12h":   12 * time.Hour,
		"90m":   90 * time.Minute,
	}
	for in, want := range cases {
		got, err := parseSince(in)
		s.Require().NoError(err, in)
		s.Equal(want, got, in)
	}
}

func (s *CLISuite) TestParseSince_Invalid() {
	for _, in := range []string{"", "d", "0d", "-3d", "xd", "7", "-1h", "week", "61d", "200000d", "1441h"} {
		_, err := parseSince(in)
		s.Error(err, in)
	}
}

func (s *CLISuite) TestKeyArg() {
	const bucket = "kbdb-images-1-kbdb-dev"
	const key = "builds/user-1/b1/images/img1"
	cases := map[string]string{
		key: key,
		"https://" + bucket + ".s3.us-east-2.amazonaws.com/" + key + "?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=abc": key,
		"https://s3.us-east-2.amazonaws.com/" + bucket + "/" + key + "?X-Amz-Expires=3540":                                   key,
		"http://" + bucket + ".localhost:4566/" + key:                                                                        key,
		"https://" + bucket + ".s3.us-east-2.amazonaws.com/profiles/user%7C1/avatar/up1":                                     "profiles/user|1/avatar/up1",
	}
	for in, want := range cases {
		got, err := keyArg(in, bucket)
		s.Require().NoError(err, in)
		s.Equal(want, got, in)
	}
}

func (s *CLISuite) TestKeyArg_InvalidLink() {
	_, err := keyArg("https://bad host/x", "bucket")
	s.Require().ErrorContains(err, "invalid image link")
}
