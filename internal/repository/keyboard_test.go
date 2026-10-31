package repository_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

type KeyboardTotalCostSuite struct {
	suite.Suite
}

func TestKeyboardTotalCostSuite(t *testing.T) {
	suite.Run(t, new(KeyboardTotalCostSuite))
}

func (s *KeyboardTotalCostSuite) TestSumsBaseAndPartsSkippingUnpriced() {
	kb := repository.Keyboard{
		Purchase: repository.KeyboardPurchase{Price: new(380.0)},
		Plates: repository.KeyboardPlatesMap([]repository.KeyboardPlate{
			{ID: "p1"},
			{ID: "p2", Purchase: repository.KeyboardPurchase{Price: new(35.0)}},
		}),
		PCBs: repository.KeyboardPCBsMap([]repository.KeyboardPCB{
			{ID: "b1", Purchase: repository.KeyboardPurchase{Price: new(45.5)}},
		}),
	}

	s.Equal(new(460.5), kb.TotalCost())
}

func (s *KeyboardTotalCostSuite) TestPartsOnly_SumsWithoutBase() {
	kb := repository.Keyboard{
		Plates: repository.KeyboardPlatesMap([]repository.KeyboardPlate{{ID: "p1", Purchase: repository.KeyboardPurchase{Price: new(0.1)}}}),
		PCBs:   repository.KeyboardPCBsMap([]repository.KeyboardPCB{{ID: "b1", Purchase: repository.KeyboardPurchase{Price: new(0.2)}}}),
	}

	s.Equal(new(0.3), kb.TotalCost())
}

func (s *KeyboardTotalCostSuite) TestNothingPriced_Nil() {
	kb := repository.Keyboard{Plates: repository.KeyboardPlatesMap([]repository.KeyboardPlate{{ID: "p1"}})}

	s.Nil(kb.TotalCost())
}

type KeyboardPartLookupSuite struct {
	suite.Suite

	kb repository.Keyboard
}

func TestKeyboardPartLookupSuite(t *testing.T) {
	suite.Run(t, new(KeyboardPartLookupSuite))
}

func (s *KeyboardPartLookupSuite) SetupTest() {
	s.kb = repository.Keyboard{
		Plates: repository.KeyboardPlatesMap([]repository.KeyboardPlate{{ID: "p1", Purchase: repository.KeyboardPurchase{Price: new(35.0)}}}),
		PCBs:   repository.KeyboardPCBsMap([]repository.KeyboardPCB{{ID: "b1", Purchase: repository.KeyboardPurchase{Price: new(45.0)}}}),
	}
}

func (s *KeyboardPartLookupSuite) TestKnownIDs_ReturnPartAndPrice() {
	s.Equal(new(35.0), s.kb.Plate(new("p1")).Price())
	s.Equal(new(45.0), s.kb.PCB(new("b1")).Price())
}

func (s *KeyboardPartLookupSuite) TestUnknownOrNilID_ReturnsNilWithNilPrice() {
	s.Nil(s.kb.Plate(new("b1")))
	s.Nil(s.kb.PCB(nil))
	s.Nil(s.kb.Plate(nil).Price())
	s.Nil(s.kb.PCB(new("p1")).Price())
}

type SortedBySeqSuite struct {
	suite.Suite
}

func TestSortedBySeqSuite(t *testing.T) {
	suite.Run(t, new(SortedBySeqSuite))
}

func (s *SortedBySeqSuite) TestPlates_OrderedBySeqThenID() {
	plates := map[string]repository.KeyboardPlate{
		"c": {ID: "c", Seq: 5},
		"b": {ID: "b", Seq: 0},
		"a": {ID: "a", Seq: 0},
		"d": {ID: "d", Seq: 2},
	}

	ids := []string{}
	for _, p := range repository.SortedPlates(plates) {
		ids = append(ids, p.ID)
	}
	s.Equal([]string{"a", "b", "d", "c"}, ids)
}

func (s *SortedBySeqSuite) TestPCBs_OrderedBySeq() {
	pcbs := map[string]repository.KeyboardPCB{"x": {ID: "x", Seq: 9}, "y": {ID: "y", Seq: 1}}

	sorted := repository.SortedPCBs(pcbs)
	s.Require().Len(sorted, 2)
	s.Equal("y", sorted[0].ID)
	s.Equal("x", sorted[1].ID)
}

func (s *SortedBySeqSuite) TestKits_WithoutSeqKeepKitIDOrderBeforeNewerKits() {
	kits := map[string]repository.KeycapKit{
		"new": {KitID: "new", Seq: 1_700_000_000},
		"k2":  {KitID: "k2"},
		"k1":  {KitID: "k1"},
	}

	ids := []string{}
	for _, k := range repository.SortedKits(kits) {
		ids = append(ids, k.KitID)
	}
	s.Equal([]string{"k1", "k2", "new"}, ids)
}

func (s *SortedBySeqSuite) TestPlatesMap_AssignsSeqByPositionAndRoundTrips() {
	in := []repository.KeyboardPlate{{ID: "z", Material: "PC"}, {ID: "a", Material: "AL"}}

	m := repository.KeyboardPlatesMap(in)
	s.Equal(0, m["z"].Seq)
	s.Equal(1, m["a"].Seq)

	out := repository.SortedPlates(m)
	s.Require().Len(out, 2)
	s.Equal("z", out[0].ID)
	s.Equal("a", out[1].ID)
}
