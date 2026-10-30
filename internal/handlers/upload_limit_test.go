package handlers

import (
	"github.com/stretchr/testify/mock"

	"github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

// expectUploadReserved expects one upload counted against the suites'
// limit of 500 and returns the call, so a test can order presigning after it.
func expectUploadReserved(c *mocks.MockUploadCounter) *mock.Call {
	return c.EXPECT().Increment(mock.Anything, mock.Anything, int64(500), mock.Anything).Return(nil).Call
}
