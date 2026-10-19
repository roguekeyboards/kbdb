package assets

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

// Rasterizing can differ by a shade on a few edge pixels between CPU
// architectures (see art.Scene.Render), so images are compared within these
// limits instead of byte for byte. A real change to a design moves far more
// pixels by far more than this.
const (
	maxChannelDelta   = 2
	maxDifferingShare = 0.001
)

type DistSuite struct {
	suite.Suite
	built     map[string][]byte
	committed map[string][]byte
}

func TestDistSuite(t *testing.T) {
	suite.Run(t, new(DistSuite))
}

func (s *DistSuite) SetupSuite() {
	brandDir, err := os.OpenRoot(filepath.Join("..", ".."))
	s.Require().NoError(err)
	defer func() { _ = brandDir.Close() }()

	tokensJSON, err := brandDir.ReadFile(TokensFile)
	s.Require().NoError(err)
	font, err := brandDir.ReadFile(FontFile)
	s.Require().NoError(err)
	s.built, err = Build(tokensJSON, font)
	s.Require().NoError(err)

	s.committed = map[string][]byte{}
	entries, err := fs.ReadDir(brandDir.FS(), DistDir)
	s.Require().NoError(err)
	for _, e := range entries {
		data, err := brandDir.ReadFile(DistDir + "/" + e.Name())
		s.Require().NoError(err)
		s.committed[e.Name()] = data
	}
}

func (s *DistSuite) TestDistHasExactlyTheBuiltFiles() {
	s.Equal(
		slices.Sorted(maps.Keys(s.built)),
		slices.Sorted(maps.Keys(s.committed)),
		"brand/dist is out of date: run `mise run gen` and commit the result",
	)
}

// The check-generated task compares the text files byte for byte; this
// covers the images, which it can't.
func (s *DistSuite) TestCommittedImagesMatchTheirSources() {
	for name, want := range s.built {
		got, ok := s.committed[name]
		if !ok {
			continue // reported by TestDistHasExactlyTheBuiltFiles
		}
		switch {
		case strings.HasSuffix(name, ".png"):
			s.assertSimilarPNG(name, want, got)
		case strings.HasSuffix(name, ".ico"):
			wantImgs, gotImgs := s.icoImages(want), s.icoImages(got)
			s.Require().Len(gotImgs, len(wantImgs), name)
			for i := range wantImgs {
				s.assertSimilarPNG(name, wantImgs[i], gotImgs[i])
			}
		}
	}
}

func (s *DistSuite) assertSimilarPNG(name string, want, got []byte) {
	a, err := png.Decode(bytes.NewReader(want))
	s.Require().NoError(err, name)
	b, err := png.Decode(bytes.NewReader(got))
	s.Require().NoError(err, name)
	s.Require().Equal(a.Bounds(), b.Bounds(), "%s: size changed; run `mise run gen`", name)

	differing, worst := pixelDiff(a, b)
	total := a.Bounds().Dx() * a.Bounds().Dy()
	s.LessOrEqual(worst, maxChannelDelta, "%s differs from its source; run `mise run gen`", name)
	s.LessOrEqual(float64(differing)/float64(total), maxDifferingShare, "%s differs from its source; run `mise run gen`", name)
}

// pixelDiff counts pixels whose 8-bit channels differ and the largest
// difference in any channel.
func pixelDiff(a, b image.Image) (differing, worst int) {
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			d := 0
			for _, c := range [][2]uint32{{r1, r2}, {g1, g2}, {b1, b2}, {a1, a2}} {
				d = max(d, abs(int(c[0]>>8)-int(c[1]>>8)))
			}
			if d > 0 {
				differing++
				worst = max(worst, d)
			}
		}
	}
	return differing, worst
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// icoImages returns the PNG payloads of an .ico file in directory order.
func (s *DistSuite) icoImages(ico []byte) [][]byte {
	s.Require().GreaterOrEqual(len(ico), 6)
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	var out [][]byte
	for i := range count {
		entry := ico[6+16*i : 6+16*(i+1)]
		size := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])
		out = append(out, ico[offset:offset+size])
	}
	return out
}
