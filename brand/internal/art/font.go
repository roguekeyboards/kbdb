package art

import (
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Font converts text to outlines, so the generated SVGs render the same
// everywhere without the font installed.
type Font struct {
	f    *sfnt.Font
	buf  sfnt.Buffer
	ppem fixed.Int26_6 // one em = unitsPerEm, so outlines come back in font units (x64)
	upem float64
	cap  float64 // cap height in font units
}

// LoadFont parses a TrueType/OpenType font.
func LoadFont(data []byte) (*Font, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing font: %w", err)
	}
	upem := f.UnitsPerEm()
	fo := &Font{f: f, ppem: fixed.I(int(upem)), upem: float64(upem)}
	m, err := f.Metrics(&fo.buf, fo.ppem, font.HintingNone)
	if err != nil {
		return nil, fmt.Errorf("reading font metrics: %w", err)
	}
	fo.cap = float64(m.CapHeight) / 64
	if fo.cap == 0 {
		return nil, fmt.Errorf("font has no cap height")
	}
	return fo, nil
}

// CapHeight returns the height of capital letters at the given size.
func (f *Font) CapHeight(size float64) float64 { return f.cap * size / f.upem }

// Run is a stretch of text in one colour.
type Run struct {
	Text string
	Fill string
}

// Text lays out runs on one line starting at x, with tracking (extra space
// between letters) in ems, and returns one path item per run plus the
// line's width. y is the baseline.
func (f *Font) Text(runs []Run, size, tracking, x, y float64) ([]Item, float64, error) {
	scale := size / f.upem
	pen := x
	var items []Item
	n := 0
	for _, r := range runs {
		n += len([]rune(r.Text))
	}
	i := 0
	for _, r := range runs {
		var p Path
		for _, ch := range r.Text {
			idx, err := f.f.GlyphIndex(&f.buf, ch)
			if err != nil || idx == 0 {
				return nil, 0, fmt.Errorf("font has no glyph for %q", ch)
			}
			segs, err := f.f.LoadGlyph(&f.buf, idx, f.ppem, nil)
			if err != nil {
				return nil, 0, fmt.Errorf("loading glyph %q: %w", ch, err)
			}
			// float64() keeps arm64 from fusing the multiply-add (see Rotate).
			pt := func(q fixed.Point26_6) Point {
				return Point{pen + float64(float64(q.X)/64*scale), y + float64(float64(q.Y)/64*scale)}
			}
			for j, s := range segs {
				switch s.Op {
				case sfnt.SegmentOpMoveTo:
					if j > 0 {
						p = append(p, Cmd{Op: Close})
					}
					p = append(p, Cmd{Op: MoveTo, Pts: [3]Point{pt(s.Args[0])}})
				case sfnt.SegmentOpLineTo:
					p = append(p, Cmd{Op: LineTo, Pts: [3]Point{pt(s.Args[0])}})
				case sfnt.SegmentOpQuadTo:
					p = append(p, Cmd{Op: QuadTo, Pts: [3]Point{pt(s.Args[0]), pt(s.Args[1])}})
				case sfnt.SegmentOpCubeTo:
					p = append(p, Cmd{Op: CubeTo, Pts: [3]Point{pt(s.Args[0]), pt(s.Args[1]), pt(s.Args[2])}})
				}
			}
			if len(segs) > 0 {
				p = append(p, Cmd{Op: Close})
			}
			adv, err := f.f.GlyphAdvance(&f.buf, idx, f.ppem, font.HintingNone)
			if err != nil {
				return nil, 0, fmt.Errorf("glyph advance %q: %w", ch, err)
			}
			pen += float64(float64(adv) / 64 * scale)
			if i++; i < n {
				pen += float64(tracking * size)
			}
		}
		if len(p) > 0 {
			items = append(items, Item{Fill: r.Fill, Shape: p})
		}
	}
	return items, pen - x, nil
}
