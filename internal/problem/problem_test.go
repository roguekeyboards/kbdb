package problem_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/problem"
)

type StillReferencedSuite struct {
	suite.Suite
}

func TestStillReferencedSuite(t *testing.T) {
	suite.Run(t, new(StillReferencedSuite))
}

func (s *StillReferencedSuite) TestStillReferenced_Writes409WithBlockingBuildIDs() {
	rec := httptest.NewRecorder()

	problem.StillReferenced(rec, "referenced by 2 builds", []string{"b1", "b2"})

	s.Equal(http.StatusConflict, rec.Code)
	s.Equal("application/problem+json", rec.Header().Get("Content-Type"))

	var body struct {
		Type             string   `json:"type"`
		Title            string   `json:"title"`
		Status           int      `json:"status"`
		Detail           string   `json:"detail"`
		BlockingBuildIDs []string `json:"blocking_build_ids"`
	}
	s.Require().NoError(json.NewDecoder(rec.Body).Decode(&body))
	s.Equal(http.StatusConflict, body.Status)
	s.Equal("referenced by 2 builds", body.Detail)
	s.Equal([]string{"b1", "b2"}, body.BlockingBuildIDs)
}
