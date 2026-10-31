package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/log"
)

type DeniedReadSuite struct {
	suite.Suite
}

func TestDeniedReadSuite(t *testing.T) {
	suite.Run(t, new(DeniedReadSuite))
}

// The denial is invisible to the caller by design, so this line is the only
// signal an operator gets. Assert its contents, not just that something was
// written.
func (s *DeniedReadSuite) TestRecordsOwnerAndVisibility() {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := log.WithLogger(context.Background(), logger)

	log.DeniedRead(ctx, "keyboard", "owner-1", "private", log.KeyboardID, "kb-1")

	var got map[string]any
	s.Require().NoError(json.Unmarshal(buf.Bytes(), &got))

	s.Equal("INFO", got["level"], "a denial is expected traffic, not an error")
	s.Equal("keyboard", got["resource"])
	s.Equal("kb-1", got["keyboard_id"])
	s.Equal("owner-1", got["owner_id"])
	s.Equal("private", got["visibility"])
}
