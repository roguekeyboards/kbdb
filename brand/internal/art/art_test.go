package art

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/brand/internal/tokens"
)

type SceneSuite struct {
	suite.Suite
}

func TestSceneSuite(t *testing.T) {
	suite.Run(t, new(SceneSuite))
}

func (s *SceneSuite) TestSVGWritesRectsAndPaths() {
	sc := Scene{W: 10, H: 5, Label: "x", Items: []Item{
		{Fill: "#000000", Shape: Rect{0, 0, 10, 5, 0}},
		{Fill: "#ffffff", Shape: Rect{1, 1, 2, 2, 0.5}},
		{Fill: "#ff0000", Shape: Polygon(Point{0, 0}, Point{1.006, 0}, Point{1, 1})},
	}}
	s.Equal(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 5" width="10" height="5" role="img" aria-label="x">`+
		`<rect x="0" y="0" width="10" height="5" fill="#000000"/>`+
		`<rect x="1" y="1" width="2" height="2" rx="0.5" fill="#ffffff"/>`+
		`<path fill="#ff0000" d="M0 0 L1.01 0 L1 1 Z"/>`+
		"</svg>\n", string(sc.SVG()))
}

func (s *SceneSuite) TestNumNormalisesNegativeZero() {
	s.Equal("0", num(-0.001))
}

func (s *SceneSuite) TestRotateTurnsCounterclockwiseForNegativeSine() {
	d := 0.7071067811865476
	p := Rotate(Point{0, 0}, -d, d)(Point{1, 0})
	s.InDelta(d, p.X, 1e-9)
	s.InDelta(-d, p.Y, 1e-9) // up, since y points down
}

func (s *SceneSuite) TestRenderFillsTheShape() {
	sc := Scene{W: 4, H: 4, Items: []Item{{Fill: "#ff0000", Shape: Rect{0, 0, 2, 4, 0}}}}
	img, err := sc.Render(4)
	s.Require().NoError(err)
	s.Equal([]uint8{0xff, 0, 0, 0xff}, img.Pix[0:4])                                 // inside
	s.Equal([]uint8{0, 0, 0, 0}, img.Pix[img.PixOffset(3, 0):img.PixOffset(3, 0)+4]) // outside, transparent
}

func (s *SceneSuite) TestRenderRejectsBadColours() {
	_, err := Scene{W: 1, H: 1, Items: []Item{{Fill: "red", Shape: Rect{0, 0, 1, 1, 0}}}}.Render(1)
	s.ErrorContains(err, "want #rrggbb")
}

type ICOSuite struct {
	suite.Suite
}

func TestICOSuite(t *testing.T) {
	suite.Run(t, new(ICOSuite))
}

func (s *ICOSuite) png(px int) []byte {
	b, err := Scene{W: 1, H: 1, Items: []Item{{Fill: "#141316", Shape: Rect{0, 0, 1, 1, 0}}}}.PNG(px)
	s.Require().NoError(err)
	return b
}

func (s *ICOSuite) TestDirectoryPointsAtEachImage() {
	small, big := s.png(16), s.png(32)
	ico, err := ICO(small, big)
	s.Require().NoError(err)

	s.Equal(uint16(1), binary.LittleEndian.Uint16(ico[2:4])) // type: icon
	s.Equal(uint16(2), binary.LittleEndian.Uint16(ico[4:6])) // count
	for i, want := range [][]byte{small, big} {
		entry := ico[6+16*i:]
		cfg, err := png.DecodeConfig(bytes.NewReader(want))
		s.Require().NoError(err)
		s.Equal(cfg.Width, int(entry[0]))
		size := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])
		s.Equal(want, ico[offset:offset+size])
	}
}

func (s *ICOSuite) TestRejectsImagesOver256px() {
	_, err := ICO(s.png(257))
	s.ErrorContains(err, "at most 256px")
}

type LogoSuite struct {
	suite.Suite
}

func TestLogoSuite(t *testing.T) {
	suite.Run(t, new(LogoSuite))
}

func (s *LogoSuite) TestColorsFromNamesTheMissingToken() {
	_, err := ColorsFrom(tokens.Palette{{Name: "ink", Value: "#141316"}})
	s.ErrorContains(err, "logo section is missing")
}

func (s *LogoSuite) TestMarkIsCroppedToItsArtwork() {
	sc := Mark(Palette{Key: "#1", KeyLip: "#2", Space: "#3", SpaceLip: "#4"})
	s.InDelta(markW, sc.W, 1e-9)
	s.InDelta(markH, sc.H, 1e-9)
	minX, minY, maxX, maxY := 1e9, 1e9, -1e9, -1e9
	for _, it := range sc.Items {
		r := it.Shape.(Rect)
		minX, minY = min(minX, r.X), min(minY, r.Y)
		maxX, maxY = max(maxX, r.X+r.W), max(maxY, r.Y+r.H)
	}
	s.Equal([4]float64{0, 0, markW, markH}, [4]float64{minX, minY, maxX, maxY})
}

func (s *LogoSuite) TestTileCentresTheMark() {
	sc := Tile(Colors{Ink: "#0", Cream: "#1", CreamLip: "#2", Amber: "#3", AmberLip: "#4"}, 0.5, "x")
	keys := sc.Items[1:] // after the background
	minX, maxX := 1e9, -1e9
	for _, it := range keys {
		r := it.Shape.(Rect)
		minX, maxX = min(minX, r.X), max(maxX, r.X+r.W)
	}
	s.InDelta(tileSize/2, (minX+maxX)/2, 1e-9)
	s.InDelta(tileSize*0.5, maxX-minX, 1e-9)
}
