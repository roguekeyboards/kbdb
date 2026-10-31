package cascadedelete_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/cascadedelete"
)

type ParseOnDeleteSuite struct {
	suite.Suite
}

func TestParseOnDeleteSuite(t *testing.T) {
	suite.Run(t, new(ParseOnDeleteSuite))
}

func (s *ParseOnDeleteSuite) TestEmpty_DefaultsToBlock() {
	got, ok := cascadedelete.ParseOnDelete("")
	s.True(ok)
	s.Equal(cascadedelete.OnDeleteBlock, got)
}

func (s *ParseOnDeleteSuite) TestBlock_Parses() {
	got, ok := cascadedelete.ParseOnDelete("block")
	s.True(ok)
	s.Equal(cascadedelete.OnDeleteBlock, got)
}

func (s *ParseOnDeleteSuite) TestCascade_Parses() {
	got, ok := cascadedelete.ParseOnDelete("cascade")
	s.True(ok)
	s.Equal(cascadedelete.OnDeleteCascade, got)
}

func (s *ParseOnDeleteSuite) TestUnknown_ReturnsFalse() {
	_, ok := cascadedelete.ParseOnDelete("bogus")
	s.False(ok)
}

func (s *ParseOnDeleteSuite) TestSurroundingWhitespace_Trimmed() {
	got, ok := cascadedelete.ParseOnDelete("  cascade\n")
	s.True(ok)
	s.Equal(cascadedelete.OnDeleteCascade, got)
}

func (s *ParseOnDeleteSuite) TestOnlyWhitespace_DefaultsToBlock() {
	got, ok := cascadedelete.ParseOnDelete("   ")
	s.True(ok)
	s.Equal(cascadedelete.OnDeleteBlock, got)
}
