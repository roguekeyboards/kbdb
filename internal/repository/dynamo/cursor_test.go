package dynamo

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository"
)

type DecodeOwnerCursorSuite struct {
	suite.Suite
}

func TestDecodeOwnerCursorSuite(t *testing.T) {
	suite.Run(t, new(DecodeOwnerCursorSuite))
}

func (s *DecodeOwnerCursorSuite) cursor(key map[string]string, pfx string) string {
	av := make(map[string]types.AttributeValue, len(key))
	for k, v := range key {
		av[k] = &types.AttributeValueMemberS{Value: v}
	}
	c, err := encodeCursor(av, "", pfx)
	s.Require().NoError(err)
	return c
}

func (s *DecodeOwnerCursorSuite) TestOwnKey_ReturnsKeyAndFilter() {
	key, pfx, err := decodeOwnerCursor(s.cursor(map[string]string{"user_id": "alice", "id": "sw1"}, "kb1"), "alice")

	s.Require().NoError(err)
	s.Equal("sw1", key["id"].(*types.AttributeValueMemberS).Value)
	s.Equal("kb1", pfx)
}

func (s *DecodeOwnerCursorSuite) TestEmptyCursor_ReturnsNoKey() {
	key, pfx, err := decodeOwnerCursor("", "alice")

	s.Require().NoError(err)
	s.Nil(key)
	s.Empty(pfx)
}

func (s *DecodeOwnerCursorSuite) TestMalformedKey_ReturnsErrInvalidCursor() {
	cases := []struct {
		name string
		key  map[string]string
	}{
		{"another owner's key", map[string]string{"user_id": "bob", "id": "sw1"}},
		{"extra attribute", map[string]string{"user_id": "alice", "id": "sw1", "extra": "x"}},
		{"missing id", map[string]string{"user_id": "alice", "other": "x"}},
		{"missing user_id", map[string]string{"id": "sw1"}},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			key, _, err := decodeOwnerCursor(s.cursor(c.key, ""), "alice")

			s.Require().ErrorIs(err, repository.ErrInvalidCursor)
			s.Nil(key)
		})
	}
}

func (s *DecodeOwnerCursorSuite) TestUndecodable_ReturnsErrInvalidCursor() {
	_, _, err := decodeOwnerCursor("not-valid-base64!!", "alice")

	s.Require().ErrorIs(err, repository.ErrInvalidCursor)
}
