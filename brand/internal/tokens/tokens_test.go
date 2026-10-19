package tokens

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ParseSuite struct {
	suite.Suite
}

func TestParseSuite(t *testing.T) {
	suite.Run(t, new(ParseSuite))
}

func (s *ParseSuite) TestKeepsFileOrder() {
	t, err := Parse([]byte(`{"logo":{"b":"#2","a":"#1"},"theme":{"z":"1","y":"2","x":"3"}}`))
	s.Require().NoError(err)
	s.Equal(Palette{{"b", "#2"}, {"a", "#1"}}, t.Logo)
	s.Equal(Palette{{"z", "1"}, {"y", "2"}, {"x", "3"}}, t.Theme)
}

func (s *ParseSuite) TestLookup() {
	p := Palette{{"accent", "#e8a33d"}}
	v, ok := p.Lookup("accent")
	s.True(ok)
	s.Equal("#e8a33d", v)
	_, ok = p.Lookup("missing")
	s.False(ok)
}

func (s *ParseSuite) TestRejectsDuplicateNames() {
	_, err := Parse([]byte(`{"logo":{"a":"#1","a":"#2"},"theme":{"x":"1"}}`))
	s.ErrorContains(err, `"a" is listed twice`)
}

func (s *ParseSuite) TestRejectsEmptyValues() {
	_, err := Parse([]byte(`{"logo":{"a":""},"theme":{"x":"1"}}`))
	s.ErrorContains(err, `"a" is empty`)
}

func (s *ParseSuite) TestRejectsNonStringValues() {
	_, err := Parse([]byte(`{"logo":{"a":1},"theme":{"x":"1"}}`))
	s.ErrorContains(err, `"a": value must be a string`)
}

func (s *ParseSuite) TestRejectsAMissingSection() {
	_, err := Parse([]byte(`{"logo":{"a":"#1"}}`))
	s.ErrorContains(err, "needs both a logo and a theme section")
}
