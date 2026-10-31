package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type LegacySuite struct {
	suite.Suite
}

func TestLegacySuite(t *testing.T) {
	suite.Run(t, new(LegacySuite))
}

func (s *LegacySuite) TestUpgradeKeyboardJSON_MovesPlatesAndPCB() {
	raw := `{"brand":"B","design":{"weight":{"material":"Brass"},"plates":["AL","PC","AL"]},"pcb":{"firmware":"ZMK"}}`

	got, err := upgradeKeyboardJSON([]byte(raw))

	s.Require().NoError(err)
	s.JSONEq(`{"brand":"B","design":{"weight":{"material":"Brass"}},`+
		`"plates":[{"id":"AL","material":"AL"},{"id":"PC","material":"PC"},{"id":"AL#2","material":"AL"}],`+
		`"pcbs":[{"id":"pcb","firmware":"ZMK"}]}`, string(got))
}

func (s *LegacySuite) TestUpgradeKeyboardJSON_EmptyLegacyGroupsDropped() {
	raw := `{"brand":"B","design":{"plates":[]},"pcb":{}}`

	got, err := upgradeKeyboardJSON([]byte(raw))

	s.Require().NoError(err)
	s.JSONEq(`{"brand":"B"}`, string(got))
}

func (s *LegacySuite) TestUpgradeKeyboardJSON_CurrentShapeUnchanged() {
	raw := `{"brand":"B","plates":[{"id":"x","material":"AL"}],"pcbs":[{"id":"y"}]}`

	got, err := upgradeKeyboardJSON([]byte(raw))

	s.Require().NoError(err)
	s.Equal(raw, string(got))
}

func (s *LegacySuite) TestUpgradeBuildJSON_PlateStringBecomesFirstPlateOfMaterial() {
	got, err := upgradeBuildJSON([]byte(`{"id":"b","plate":"AL"}`))

	s.Require().NoError(err)
	s.JSONEq(`{"id":"b","plate":{"id":"AL","material":"AL"}}`, string(got))
}

func (s *LegacySuite) TestUpgradeBuildJSON_CurrentShapeUnchanged() {
	raw := `{"id":"b","plate":{"id":"x","material":"AL"}}`

	got, err := upgradeBuildJSON([]byte(raw))

	s.Require().NoError(err)
	s.Equal(raw, string(got))
}
