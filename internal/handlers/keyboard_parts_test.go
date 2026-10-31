package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/handlers/api"
	"github.com/roguekeyboards/mykeebs-api/internal/ownerprefs"
	"github.com/roguekeyboards/mykeebs-api/internal/problem"
	"github.com/roguekeyboards/mykeebs-api/internal/repoapi"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
	"github.com/roguekeyboards/mykeebs-api/internal/repository/mocks"
)

// partRequest builds a request against keyboard kb1 owned by alice, with
// partParam/partID set when the route names a part.
func partRequest(ctx context.Context, method, target, body, partParam, partID string) *http.Request {
	ctx = ownerprefs.WithPreferences(ctx, repository.ProfilePreferences{Currency: "EUR"})
	req := httptest.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	req.SetPathValue("userId", "alice")
	req.SetPathValue("keyboardId", "kb1")
	if partParam != "" {
		req.SetPathValue(partParam, partID)
	}
	return req
}

func invalidParamNames(body []byte) []string {
	var got struct {
		InvalidParams []problem.InvalidParam `json:"invalid_params"`
	}
	_ = json.Unmarshal(body, &got)
	names := make([]string, len(got.InvalidParams))
	for i, p := range got.InvalidParams {
		names[i] = p.Name
	}
	return names
}

type KeyboardPlateHandlersSuite struct {
	suite.Suite

	mockRepo *mocks.MockKeyboardRepository
	kr       repoapi.Keyboard
}

func TestKeyboardPlateHandlersSuite(t *testing.T) {
	suite.Run(t, new(KeyboardPlateHandlersSuite))
}

func (s *KeyboardPlateHandlersSuite) SetupTest() {
	s.mockRepo = mocks.NewMockKeyboardRepository(s.T())
	s.kr = repoapi.Keyboard{}
}

func (s *KeyboardPlateHandlersSuite) ownerCtx() context.Context {
	return kbdbctx.WithUserID(s.T().Context(), "alice")
}

func (s *KeyboardPlateHandlersSuite) TestCreate_AssignsIDAndReturns201() {
	s.mockRepo.EXPECT().
		AddPlate(mock.Anything, "kb1", mock.MatchedBy(func(p repository.KeyboardPlate) bool {
			return p.ID != "" && p.Material == "PC" && *p.Purchase.Price == 35
		})).
		RunAndReturn(func(_ context.Context, _ string, p repository.KeyboardPlate) (*repository.KeyboardPlate, error) {
			return &p, nil
		})

	rec := httptest.NewRecorder()
	CreateKeyboardPlate(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPost,
		"/users/alice/keyboards/kb1/plates", `{"material":"PC","purchase":{"price":35}}`, "", ""))

	s.Equal(http.StatusCreated, rec.Code)
	var got api.KeyboardPlate
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.NotEmpty(got.Id)
	s.Equal("PC", got.Material)
	s.Require().NotNil(got.Purchase)
	s.Equal(new(35.0), got.Purchase.Price)
	s.Equal(new("EUR"), got.Purchase.Currency)
}

func (s *KeyboardPlateHandlersSuite) TestCreate_UnapprovedValues_Returns400NamingEach() {
	rec := httptest.NewRecorder()
	CreateKeyboardPlate(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPost,
		"/users/alice/keyboards/kb1/plates", `{"material":"NotAMaterial","purchase":{"vendor":"NotAVendor"}}`, "", ""))

	s.Equal(http.StatusBadRequest, rec.Code)
	s.ElementsMatch([]string{"material", "purchase.vendor"}, invalidParamNames(rec.Body.Bytes()))
}

func (s *KeyboardPlateHandlersSuite) TestCreate_KeyboardMissing_Returns404() {
	s.mockRepo.EXPECT().AddPlate(mock.Anything, "kb1", mock.Anything).Return(nil, repository.ErrNotFound)

	rec := httptest.NewRecorder()
	CreateKeyboardPlate(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPost,
		"/users/alice/keyboards/kb1/plates", `{"material":"PC"}`, "", ""))

	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *KeyboardPlateHandlersSuite) TestCreate_NotOwner_Returns404() {
	rec := httptest.NewRecorder()
	CreateKeyboardPlate(s.mockRepo, s.kr)(rec, partRequest(kbdbctx.WithUserID(s.T().Context(), "bob"), http.MethodPost,
		"/users/alice/keyboards/kb1/plates", `{"material":"PC"}`, "", ""))

	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *KeyboardPlateHandlersSuite) TestUpdate_UsesPathIDAndReturns200() {
	s.mockRepo.EXPECT().
		UpdatePlate(mock.Anything, "kb1", mock.MatchedBy(func(p repository.KeyboardPlate) bool {
			return p.ID == "plate-1" && p.Material == "AL" && *p.Color == "Black"
		})).
		RunAndReturn(func(_ context.Context, _ string, p repository.KeyboardPlate) (*repository.KeyboardPlate, error) {
			return &p, nil
		})

	rec := httptest.NewRecorder()
	UpdateKeyboardPlate(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPut,
		"/users/alice/keyboards/kb1/plates/plate-1", `{"material":"AL","color":"Black"}`, "plateId", "plate-1"))

	s.Equal(http.StatusOK, rec.Code)
	var got api.KeyboardPlate
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.Equal("plate-1", got.Id)
}

func (s *KeyboardPlateHandlersSuite) TestUpdate_PlateMissing_Returns404() {
	s.mockRepo.EXPECT().UpdatePlate(mock.Anything, "kb1", mock.Anything).Return(nil, repository.ErrNotFound)

	rec := httptest.NewRecorder()
	UpdateKeyboardPlate(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPut,
		"/users/alice/keyboards/kb1/plates/nope", `{"material":"AL"}`, "plateId", "nope"))

	s.Equal(http.StatusNotFound, rec.Code)
}

type DeleteKeyboardPartSuite struct {
	suite.Suite

	mockRepo     *mocks.MockKeyboardRepository
	mockBuilds   *mocks.MockBuildRepository
	mockBuildImg *mocks.MockBuildImageStore
}

func TestDeleteKeyboardPartSuite(t *testing.T) {
	suite.Run(t, new(DeleteKeyboardPartSuite))
}

func (s *DeleteKeyboardPartSuite) SetupTest() {
	s.mockRepo = mocks.NewMockKeyboardRepository(s.T())
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImg = mocks.NewMockBuildImageStore(s.T())
}

func (s *DeleteKeyboardPartSuite) ownerCtx() context.Context {
	return kbdbctx.WithUserID(s.T().Context(), "alice")
}

func (s *DeleteKeyboardPartSuite) deletePlate(onDelete string) *httptest.ResponseRecorder {
	target := "/users/alice/keyboards/kb1/plates/plate-1"
	if onDelete != "" {
		target += "?on_delete=" + onDelete
	}
	rec := httptest.NewRecorder()
	DeleteKeyboardPlate(s.mockRepo, s.mockBuilds, s.mockBuildImg)(rec,
		partRequest(s.ownerCtx(), http.MethodDelete, target, "", "plateId", "plate-1"))
	return rec
}

func (s *DeleteKeyboardPartSuite) TestPlateNoBuildUsesIt_Returns204() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(mock.Anything, "alice", "kb1").Return([]string{"b1"}, nil)
	s.mockBuilds.EXPECT().Get(mock.Anything, "alice", "b1").Return(&repository.Build{ID: "b1", Plate: new("other")}, nil)
	s.mockRepo.EXPECT().DeletePlate(mock.Anything, "kb1", "plate-1").Return(nil)

	s.Equal(http.StatusNoContent, s.deletePlate("").Code)
}

func (s *DeleteKeyboardPartSuite) TestPlateUsedByABuild_Block_Returns409ListingIt() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(mock.Anything, "alice", "kb1").Return([]string{"b1", "b2"}, nil)
	s.mockBuilds.EXPECT().Get(mock.Anything, "alice", "b1").Return(&repository.Build{ID: "b1", Plate: new("plate-1")}, nil)
	s.mockBuilds.EXPECT().Get(mock.Anything, "alice", "b2").Return(&repository.Build{ID: "b2"}, nil)

	rec := s.deletePlate("")

	s.Equal(http.StatusConflict, rec.Code)
	var got struct {
		BlockingBuildIDs []string `json:"blocking_build_ids"`
	}
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.Equal([]string{"b1"}, got.BlockingBuildIDs)
}

func (s *DeleteKeyboardPartSuite) TestPlateUsedByABuild_Cascade_DeletesBuildAndReturnsIt() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(mock.Anything, "alice", "kb1").Return([]string{"b1"}, nil)
	s.mockBuilds.EXPECT().Get(mock.Anything, "alice", "b1").Return(&repository.Build{ID: "b1", Plate: new("plate-1")}, nil)
	s.mockBuilds.EXPECT().Delete(mock.Anything, "b1").Return(nil)
	s.mockRepo.EXPECT().DeletePlate(mock.Anything, "kb1", "plate-1").Return(nil)

	rec := s.deletePlate("cascade")

	s.Equal(http.StatusOK, rec.Code)
	var got api.CascadeDeleteResult
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.Equal([]string{"b1"}, got.DeletedBuildIds)
}

func (s *DeleteKeyboardPartSuite) TestInvalidOnDelete_Returns400() {
	s.Equal(http.StatusBadRequest, s.deletePlate("detach").Code)
}

func (s *DeleteKeyboardPartSuite) TestKeyboardMissing_Returns404() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(mock.Anything, "alice", "kb1").Return(nil, nil)
	s.mockRepo.EXPECT().DeletePlate(mock.Anything, "kb1", "plate-1").Return(repository.ErrNotFound)

	s.Equal(http.StatusNotFound, s.deletePlate("").Code)
}

func (s *DeleteKeyboardPartSuite) TestPCBUsedByABuild_Block_Returns409() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(mock.Anything, "alice", "kb1").Return([]string{"b1"}, nil)
	s.mockBuilds.EXPECT().Get(mock.Anything, "alice", "b1").Return(&repository.Build{ID: "b1", PCB: new("pcb-1")}, nil)

	rec := httptest.NewRecorder()
	DeleteKeyboardPCB(s.mockRepo, s.mockBuilds, s.mockBuildImg)(rec,
		partRequest(s.ownerCtx(), http.MethodDelete, "/users/alice/keyboards/kb1/pcbs/pcb-1", "", "pcbId", "pcb-1"))

	s.Equal(http.StatusConflict, rec.Code)
}

type KeyboardPCBHandlersSuite struct {
	suite.Suite

	mockRepo *mocks.MockKeyboardRepository
	kr       repoapi.Keyboard
}

func TestKeyboardPCBHandlersSuite(t *testing.T) {
	suite.Run(t, new(KeyboardPCBHandlersSuite))
}

func (s *KeyboardPCBHandlersSuite) SetupTest() {
	s.mockRepo = mocks.NewMockKeyboardRepository(s.T())
	s.kr = repoapi.Keyboard{}
}

func (s *KeyboardPCBHandlersSuite) ownerCtx() context.Context {
	return kbdbctx.WithUserID(s.T().Context(), "alice")
}

func (s *KeyboardPCBHandlersSuite) TestCreate_AssignsIDAndReturns201() {
	s.mockRepo.EXPECT().
		AddPCB(mock.Anything, "kb1", mock.MatchedBy(func(p repository.KeyboardPCB) bool {
			return p.ID != "" && *p.Assembly == "EC"
		})).
		RunAndReturn(func(_ context.Context, _ string, p repository.KeyboardPCB) (*repository.KeyboardPCB, error) {
			return &p, nil
		})

	rec := httptest.NewRecorder()
	CreateKeyboardPCB(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPost,
		"/users/alice/keyboards/kb1/pcbs", `{"assembly":"EC","connectivity":"Wired"}`, "", ""))

	s.Equal(http.StatusCreated, rec.Code)
	var got api.KeyboardPCB
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.NotEmpty(got.Id)
	s.Equal(new("EC"), got.Assembly)
}

func (s *KeyboardPCBHandlersSuite) TestCreate_UnapprovedValues_Returns400NamingEach() {
	rec := httptest.NewRecorder()
	CreateKeyboardPCB(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPost,
		"/users/alice/keyboards/kb1/pcbs", `{"firmware":"X","assembly":"X","connectivity":"X"}`, "", ""))

	s.Equal(http.StatusBadRequest, rec.Code)
	s.ElementsMatch([]string{"firmware", "assembly", "connectivity"}, invalidParamNames(rec.Body.Bytes()))
}

func (s *KeyboardPCBHandlersSuite) TestUpdate_UsesPathIDAndReturns200() {
	s.mockRepo.EXPECT().
		UpdatePCB(mock.Anything, "kb1", mock.MatchedBy(func(p repository.KeyboardPCB) bool { return p.ID == "pcb-1" })).
		RunAndReturn(func(_ context.Context, _ string, p repository.KeyboardPCB) (*repository.KeyboardPCB, error) {
			return &p, nil
		})

	rec := httptest.NewRecorder()
	UpdateKeyboardPCB(s.mockRepo, s.kr)(rec, partRequest(s.ownerCtx(), http.MethodPut,
		"/users/alice/keyboards/kb1/pcbs/pcb-1", `{"connectivity":"Tri-Mode"}`, "pcbId", "pcb-1"))

	s.Equal(http.StatusOK, rec.Code)
}
