package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	ctxpkg "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

const (
	callerID = "caller-0001"
	otherID  = "other-0002"
)

// callerContext mimics what identityMiddleware puts on ctx for a verified
// caller, since these handlers run downstream of it.
func callerContext(t *testing.T) context.Context {
	t.Helper()

	return ctxpkg.WithUserID(t.Context(), callerID)
}

// expectUploadReserved expects one upload counted against the suites'
// limit of 500 and returns the call, so a test can order presigning after it.
func expectUploadReserved(c *mocks.MockUploadCounter) *mock.Call {
	return c.EXPECT().Increment(mock.Anything, mock.Anything, int64(500), mock.Anything).Return(nil).Call
}
