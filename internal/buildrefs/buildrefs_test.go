package buildrefs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/buildrefs"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

type ValidateReferencesSuite struct {
	suite.Suite

	mockKeyboards *mocks.MockKeyboardRepository
	mockSwitches  *mocks.MockSwitchRepository
	mockKeycaps   *mocks.MockKeycapSetRepository
}

func TestValidateReferencesSuite(t *testing.T) {
	suite.Run(t, new(ValidateReferencesSuite))
}

func (s *ValidateReferencesSuite) SetupTest() {
	s.mockKeyboards = mocks.NewMockKeyboardRepository(s.T())
	s.mockSwitches = mocks.NewMockSwitchRepository(s.T())
	s.mockKeycaps = mocks.NewMockKeycapSetRepository(s.T())
}

func (s *ValidateReferencesSuite) validate(b repository.Build) ([]buildrefs.FieldError, error) {
	return buildrefs.ValidateReferences(context.Background(), "alice", b,
		s.mockKeyboards, s.mockSwitches, s.mockKeycaps)
}

func (s *ValidateReferencesSuite) TestNoReferences_NoChecksPerformed() {
	fieldErrs, err := s.validate(repository.Build{})

	s.Require().NoError(err)
	s.Empty(fieldErrs)
}

func (s *ValidateReferencesSuite) TestValidKeyboard_Succeeds() {
	s.mockKeyboards.EXPECT().
		Get(mock.Anything, "alice", "kb1").
		Return(&repository.Keyboard{UserID: "alice", ID: "kb1"}, nil)

	fieldErrs, err := s.validate(repository.Build{Keyboard: "kb1"})

	s.Require().NoError(err)
	s.Empty(fieldErrs)
}

func (s *ValidateReferencesSuite) TestMissingKeyboard_ReturnsFieldError() {
	s.mockKeyboards.EXPECT().
		Get(mock.Anything, "alice", "kb1").
		Return(nil, repository.ErrNotFound)

	fieldErrs, err := s.validate(repository.Build{Keyboard: "kb1"})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 1)
	s.Equal("keyboard", fieldErrs[0].Field)
	s.Equal("kb1", fieldErrs[0].Value)
}

func (s *ValidateReferencesSuite) TestKeyboardRepositoryError_ReturnsError() {
	s.mockKeyboards.EXPECT().
		Get(mock.Anything, "alice", "kb1").
		Return(nil, errors.New("dynamo unavailable"))

	fieldErrs, err := s.validate(repository.Build{Keyboard: "kb1"})

	s.Require().Error(err)
	s.Nil(fieldErrs)
}

func (s *ValidateReferencesSuite) TestPartsOnKeyboard_Succeed() {
	s.mockKeyboards.EXPECT().
		Get(mock.Anything, "alice", "kb1").
		Return(&repository.Keyboard{
			UserID: "alice", ID: "kb1",
			Plates: repository.KeyboardPlatesMap([]repository.KeyboardPlate{{ID: "p1", Material: "Brass"}, {ID: "p2", Material: "FR4"}}),
			PCBs:   repository.KeyboardPCBsMap([]repository.KeyboardPCB{{ID: "b1"}}),
		}, nil)

	plate, pcb := "p2", "b1"
	fieldErrs, err := s.validate(repository.Build{Keyboard: "kb1", Plate: &plate, PCB: &pcb})

	s.Require().NoError(err)
	s.Empty(fieldErrs)
}

func (s *ValidateReferencesSuite) TestPartsNotOnKeyboard_ReturnFieldErrors() {
	s.mockKeyboards.EXPECT().
		Get(mock.Anything, "alice", "kb1").
		Return(&repository.Keyboard{
			UserID: "alice", ID: "kb1",
			Plates: repository.KeyboardPlatesMap([]repository.KeyboardPlate{{ID: "p1", Material: "Brass"}}),
			PCBs:   repository.KeyboardPCBsMap([]repository.KeyboardPCB{{ID: "b1"}}),
		}, nil)

	plate, pcb := "Brass", "b2"
	fieldErrs, err := s.validate(repository.Build{Keyboard: "kb1", Plate: &plate, PCB: &pcb})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 2)
	s.Equal("plate", fieldErrs[0].Field)
	s.Equal("Brass", fieldErrs[0].Value)
	s.Equal("pcb", fieldErrs[1].Field)
	s.Equal("b2", fieldErrs[1].Value)
}

func (s *ValidateReferencesSuite) TestValidSwitch_Succeeds() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockSwitches.EXPECT().
		Get(mock.Anything, "alice", "sw1").
		Return(&repository.Switch{UserID: "alice", ID: "sw1"}, nil)

	fieldErrs, err := s.validate(repository.Build{
		Keyboard: "kb1",
		Switches: []repository.BuildSwitchEntry{{Switch: "sw1", Count: 4}},
	})

	s.Require().NoError(err)
	s.Empty(fieldErrs)
}

func (s *ValidateReferencesSuite) TestMissingSwitch_ReturnsFieldErrorWithIndex() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockSwitches.EXPECT().
		Get(mock.Anything, "alice", "sw1").
		Return(nil, repository.ErrNotFound)

	fieldErrs, err := s.validate(repository.Build{
		Keyboard: "kb1",
		Switches: []repository.BuildSwitchEntry{{Switch: "sw1", Count: 4}},
	})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 1)
	s.Equal("switches[0].switch", fieldErrs[0].Field)
	s.Equal("sw1", fieldErrs[0].Value)
}

func (s *ValidateReferencesSuite) TestValidKeycapSetAndKit_Succeeds() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeycaps.EXPECT().
		Get(mock.Anything, "alice", "ks1").
		Return(&repository.KeycapSet{
			UserID: "alice", ID: "ks1",
			Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1"}, "kit2": {KitID: "kit2"}},
		}, nil)

	fieldErrs, err := s.validate(repository.Build{
		Keyboard:   "kb1",
		KeycapKits: []repository.BuildKeycapKitEntry{{KeycapSet: "ks1", Kit: "kit2"}},
	})

	s.Require().NoError(err)
	s.Empty(fieldErrs)
}

func (s *ValidateReferencesSuite) TestMissingKeycapSet_ReturnsFieldErrorWithIndex() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeycaps.EXPECT().
		Get(mock.Anything, "alice", "ks1").
		Return(nil, repository.ErrNotFound)

	fieldErrs, err := s.validate(repository.Build{
		Keyboard:   "kb1",
		KeycapKits: []repository.BuildKeycapKitEntry{{KeycapSet: "ks1", Kit: "kit1"}},
	})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 1)
	s.Equal("keycap_kits[0].keycap_set", fieldErrs[0].Field)
	s.Equal("ks1", fieldErrs[0].Value)
}

func (s *ValidateReferencesSuite) TestKeycapSetFoundButKitMissing_ReturnsFieldError() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeycaps.EXPECT().
		Get(mock.Anything, "alice", "ks1").
		Return(&repository.KeycapSet{
			UserID: "alice", ID: "ks1",
			Kits: map[string]repository.KeycapKit{"other-kit": {KitID: "other-kit"}},
		}, nil)

	fieldErrs, err := s.validate(repository.Build{
		Keyboard:   "kb1",
		KeycapKits: []repository.BuildKeycapKitEntry{{KeycapSet: "ks1", Kit: "kit1"}},
	})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 1)
	s.Equal("keycap_kits[0].kit", fieldErrs[0].Field)
	s.Equal("kit1", fieldErrs[0].Value)
}

func (s *ValidateReferencesSuite) TestKeycapSetRepositoryError_ReturnsError() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeycaps.EXPECT().
		Get(mock.Anything, "alice", "ks1").
		Return(nil, errors.New("dynamo unavailable"))

	fieldErrs, err := s.validate(repository.Build{
		Keyboard:   "kb1",
		KeycapKits: []repository.BuildKeycapKitEntry{{KeycapSet: "ks1", Kit: "kit1"}},
	})

	s.Require().Error(err)
	s.Nil(fieldErrs)
}

func (s *ValidateReferencesSuite) TestRepeatedKeycapSet_FetchedOnce() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeycaps.EXPECT().
		Get(mock.Anything, "alice", "ks1").
		Return(&repository.KeycapSet{
			UserID: "alice", ID: "ks1",
			Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1"}, "kit2": {KitID: "kit2"}},
		}, nil).
		Once()

	fieldErrs, err := s.validate(repository.Build{
		Keyboard: "kb1",
		KeycapKits: []repository.BuildKeycapKitEntry{
			{KeycapSet: "ks1", Kit: "kit1"},
			{KeycapSet: "ks1", Kit: "kit2"},
		},
	})

	s.Require().NoError(err)
	s.Empty(fieldErrs)
}

func (s *ValidateReferencesSuite) TestRepeatedMissingKeycapSet_FetchedOnceButErrorsForEachEntry() {
	s.mockKeyboards.EXPECT().Get(mock.Anything, "alice", "kb1").Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeycaps.EXPECT().
		Get(mock.Anything, "alice", "ks1").
		Return(nil, repository.ErrNotFound).
		Once()

	fieldErrs, err := s.validate(repository.Build{
		Keyboard: "kb1",
		KeycapKits: []repository.BuildKeycapKitEntry{
			{KeycapSet: "ks1", Kit: "kit1"},
			{KeycapSet: "ks1", Kit: "kit2"},
		},
	})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 2)
	s.Equal("keycap_kits[0].keycap_set", fieldErrs[0].Field)
	s.Equal("keycap_kits[1].keycap_set", fieldErrs[1].Field)
}

func (s *ValidateReferencesSuite) TestMultipleInvalidReferences_ReturnsAll() {
	s.mockKeyboards.EXPECT().
		Get(mock.Anything, "alice", "kb1").
		Return(nil, repository.ErrNotFound)
	s.mockSwitches.EXPECT().
		Get(mock.Anything, "alice", "sw1").
		Return(nil, repository.ErrNotFound)

	fieldErrs, err := s.validate(repository.Build{
		Keyboard: "kb1",
		Switches: []repository.BuildSwitchEntry{{Switch: "sw1", Count: 1}},
	})

	s.Require().NoError(err)
	s.Require().Len(fieldErrs, 2)
}
