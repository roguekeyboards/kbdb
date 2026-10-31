package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type VerifySuite struct {
	suite.Suite
}

func TestVerifySuite(t *testing.T) {
	suite.Run(t, new(VerifySuite))
}

func (s *VerifySuite) TestCompareScalars_CurrencyDiffers_Matches() {
	dump := `{"id":"a","brand":"B","purchase":{"price":10,"currency":"EUR"},"stabs":{"price":5,"currency":"EUR"},"currency":"EUR"}`
	live := `{"id":"b","brand":"B","purchase":{"price":10,"currency":"USD"},"stabs":{"price":5,"currency":"USD"},"currency":"USD"}`

	ok, detail := compareScalars([]byte(dump), []byte(live))

	s.True(ok, detail)
}

func (s *VerifySuite) TestCompareScalars_DumpPredatesCurrency_Matches() {
	dump := `{"brand":"B","kits":[{"name":"Base","purchase":{"price":10}}]}`
	live := `{"brand":"B","kits":[{"name":"Base","purchase":{"price":10,"currency":"USD"}}]}`

	ok, detail := compareScalars([]byte(dump), []byte(live))

	s.True(ok, detail)
}

func (s *VerifySuite) TestCompareScalars_PriceDiffers_Mismatch() {
	dump := `{"brand":"B","purchase":{"price":10,"currency":"USD"}}`
	live := `{"brand":"B","purchase":{"price":12,"currency":"USD"}}`

	ok, _ := compareScalars([]byte(dump), []byte(live))

	s.False(ok)
}

func (s *VerifySuite) TestCompareKeyboards_LegacyDumpMatchesUpgradedLive() {
	dump := `{"id":"a","brand":"B","purchase":{"price":10},` +
		`"design":{"top_case":{"material":"Aluminum"},"plates":["AL","AL","PC"]},"pcb":{"firmware":"QMK/VIA"}}`
	live := `{"id":"b","brand":"B","purchase":{"price":10,"currency":"USD"},"total_cost":10,"currency":"USD",` +
		`"design":{"top_case":{"material":"Aluminum"}},` +
		`"plates":[{"id":"n1","material":"AL"},{"id":"n2","material":"AL"},{"id":"n3","material":"PC"}],` +
		`"pcbs":[{"id":"n4","firmware":"QMK/VIA"}]}`

	ok, detail := compareKeyboards([]byte(dump), []byte(live))

	s.True(ok, detail)
}

func (s *VerifySuite) TestCompareKeyboards_PartDiffers_Mismatch() {
	dump := `{"brand":"B","plates":[{"id":"p1","material":"AL","color":"Black"}]}`
	live := `{"brand":"B","plates":[{"id":"n1","material":"AL","color":"Silver"}]}`

	ok, _ := compareKeyboards([]byte(dump), []byte(live))

	s.False(ok)
}

func (s *VerifySuite) TestComparePartRef() {
	ids := map[string]string{"p-old": "p-new"}

	s.Empty(comparePartRef("plate", nil, nil, ids))
	s.Empty(comparePartRef("plate", new("p-old"), new("p-new"), ids))
	s.NotEmpty(comparePartRef("plate", new("p-old"), new("other"), ids))
	s.NotEmpty(comparePartRef("plate", new("p-old"), nil, ids))
	s.NotEmpty(comparePartRef("plate", nil, new("p-new"), ids))
}
