package art

import (
	"fmt"
	"math"

	"github.com/roguekeyboards/kbdb/brand/internal/tokens"
)

// Colors are the logo colours, read from tokens.json's "logo" section.
type Colors struct {
	Ink, Cream, CreamLip      string
	Amber, AmberLip           string
	AmberOnLight              string
	KeyOnLight, KeyLipOnLight string
	DevBanner, DevBannerText  string
}

// ColorsFrom reads the logo colours from the tokens' logo palette.
func ColorsFrom(p tokens.Palette) (Colors, error) {
	var c Colors
	for name, dst := range map[string]*string{
		"ink": &c.Ink, "cream": &c.Cream, "cream-lip": &c.CreamLip,
		"amber": &c.Amber, "amber-lip": &c.AmberLip, "amber-on-light": &c.AmberOnLight,
		"key-on-light": &c.KeyOnLight, "key-lip-on-light": &c.KeyLipOnLight,
		"dev-banner": &c.DevBanner, "dev-banner-text": &c.DevBannerText,
	} {
		v, ok := p.Lookup(name)
		if !ok {
			return Colors{}, fmt.Errorf("tokens.json logo section is missing %q", name)
		}
		*dst = v
	}
	return c, nil
}

// Palette is the set of fills one rendition of the logo uses.
type Palette struct {
	Key, KeyLip, Space, SpaceLip string
	My, Keebs                    string // wordmark: "my" and "keebs"
}

// OnDark is the logo for dark backgrounds: cream keys, amber spacebar,
// cream "my", amber "keebs".
func (c Colors) OnDark() Palette {
	return Palette{Key: c.Cream, KeyLip: c.CreamLip, Space: c.Amber, SpaceLip: c.AmberLip, My: c.Cream, Keebs: c.Amber}
}

// OnLight is the logo for light backgrounds. "keebs" uses a darker amber,
// since the brand amber is too light to read on cream.
func (c Colors) OnLight() Palette {
	return Palette{Key: c.KeyOnLight, KeyLip: c.KeyLipOnLight, Space: c.Amber, SpaceLip: c.AmberLip, My: c.Ink, Keebs: c.AmberOnLight}
}

// The mark: two keycaps over a spacebar, each a top face over a slightly
// taller, darker front lip. Drawn in a 100-unit box; the visible artwork
// spans markX0..markX0+markW, markY0..markY0+markH.
const (
	markX0, markY0 = 6.0, 8.0
	markW, markH   = 88.0, 86.0
)

type key struct {
	x, y, w, lip, top, r float64
	space                bool
}

var keys = []key{
	{x: 6, y: 8, w: 42, lip: 40, top: 34, r: 10},
	{x: 52, y: 8, w: 42, lip: 40, top: 34, r: 10},
	{x: 6, y: 54, w: 88, lip: 40, top: 34, r: 10, space: true},
}

// mark places the mark with its visible top-left at (x, y), scaled by s.
// Products are wrapped in float64() so arm64 doesn't fuse them into the
// following addition (see Rotate).
func mark(p Palette, x, y, s float64) []Item {
	tx, ty := x-float64(markX0*s), y-float64(markY0*s)
	var items []Item
	for _, k := range keys {
		face, lip := p.Key, p.KeyLip
		if k.space {
			face, lip = p.Space, p.SpaceLip
		}
		kx := tx + float64(k.x*s)
		items = append(items,
			Item{Fill: lip, Shape: Rect{kx, ty + float64((k.y+2)*s), k.w * s, (k.lip - 2) * s, k.r * s}},
			Item{Fill: face, Shape: Rect{kx, ty + float64(k.y*s), k.w * s, k.top * s, k.r * s}},
		)
	}
	return items
}

func wordmark(p Palette) []Run {
	return []Run{{Text: "my", Fill: p.My}, {Text: "keebs", Fill: p.Keebs}}
}

// Mark is the mark alone, cropped to its artwork.
func Mark(p Palette) Scene {
	return Scene{W: markW, H: markH, Label: "mykeebs", Items: mark(p, 0, 0, 1)}
}

// Horizontal is the mark beside the wordmark, for headers and nav bars.
// The wordmark's cap height is centred on the mark.
func Horizontal(f *Font, p Palette) (Scene, error) {
	const size, s, gap, tracking = 50.0, 0.88, 25.0, -0.05
	h := markH * s
	tx := markW*s + gap
	text, tw, err := f.Text(wordmark(p), size, tracking, tx, h/2+f.CapHeight(size)/2)
	if err != nil {
		return Scene{}, err
	}
	return Scene{W: tx + tw, H: h, Label: "mykeebs", Items: append(mark(p, 0, 0, s), text...)}, nil
}

// Stacked is the mark above the wordmark, for square spaces.
func Stacked(f *Font, p Palette) (Scene, error) {
	const size, gap, tracking = 40.0, 18.0, -0.06
	s := 104 / markW
	mw, mh := markW*s, markH*s
	_, tw, err := f.Text(wordmark(p), size, tracking, 0, 0)
	if err != nil {
		return Scene{}, err
	}
	w := math.Max(mw, tw)
	baseline := mh + gap + f.CapHeight(size)
	text, _, err := f.Text(wordmark(p), size, tracking, (w-tw)/2, baseline)
	if err != nil {
		return Scene{}, err
	}
	descender := 0.24 * size // room for the "y"
	return Scene{W: w, H: baseline + descender, Label: "mykeebs", Items: append(mark(p, (w-mw)/2, 0, s), text...)}, nil
}

// Favicon32 is the mark redrawn on a 32px grid so every edge lands on a
// pixel boundary: 2px gaps and a 2px lip.
func Favicon32(c Colors) Scene {
	return Scene{W: 32, H: 32, Label: "mykeebs", Items: []Item{
		{Fill: c.Ink, Shape: Rect{0, 0, 32, 32, 7}},
		{Fill: c.CreamLip, Shape: Rect{4, 5, 11, 10, 2.5}},
		{Fill: c.Cream, Shape: Rect{4, 5, 11, 8, 2.5}},
		{Fill: c.CreamLip, Shape: Rect{17, 5, 11, 10, 2.5}},
		{Fill: c.Cream, Shape: Rect{17, 5, 11, 8, 2.5}},
		{Fill: c.AmberLip, Shape: Rect{4, 17, 24, 10, 2.5}},
		{Fill: c.Amber, Shape: Rect{4, 17, 24, 8, 2.5}},
	}}
}

// Favicon16 is the mark redrawn on a 16px grid. There's no room for a lip
// at this size, so the keys are flat.
func Favicon16(c Colors) Scene {
	return Scene{W: 16, H: 16, Label: "mykeebs", Items: []Item{
		{Fill: c.Ink, Shape: Rect{0, 0, 16, 16, 3.5}},
		{Fill: c.Cream, Shape: Rect{2, 3, 5, 4, 1}},
		{Fill: c.Cream, Shape: Rect{9, 3, 5, 4, 1}},
		{Fill: c.Amber, Shape: Rect{2, 9, 12, 4, 1}},
	}}
}

// tileSize is the drawing size of the square icons; they're rendered down
// from it.
const tileSize = 1024.0

// Tile is the on-dark mark centred on a full-bleed square, spanning frac of
// its width. Platforms that show it (iOS, Android, Discord) round or crop
// the corners themselves.
func Tile(c Colors, frac float64, label string) Scene {
	s := tileSize * frac / markW
	items := []Item{{Fill: c.Ink, Shape: Rect{0, 0, tileSize, tileSize, 0}}}
	items = append(items, mark(c.OnDark(), (tileSize-float64(markW*s))/2, (tileSize-float64(markH*s))/2, s)...)
	return Scene{W: tileSize, H: tileSize, Label: label, Items: items}
}

// Icon sizes: AppIconFrac for home-screen/app icons, DiscordFrac for the
// Discord avatar, which Discord crops to a circle - the mark has to fit
// inside it.
const (
	AppIconFrac = 0.6
	DiscordFrac = 0.563
)

// DevAvatar is the Discord avatar with a diagonal "DEV" banner across the
// lower right, for the dev Discord application. The banner sits inside
// Discord's circular crop, and its text reads lower-left to upper-right.
func DevAvatar(f *Font, c Colors) (Scene, error) {
	const (
		bandOffset = 300.0 // banner centre line's distance from the centre, along the diagonal
		bandWidth  = 150.0
		size       = 104.0
		tracking   = 0.12
		reach      = 2 * tileSize // long enough to cross the whole tile
	)
	sc := Tile(c, DiscordFrac, "mykeebs dev Discord avatar")
	d := math.Sqrt2 / 2
	// float64() around each product keeps arm64 from fusing multiply-adds
	// (see Rotate), so the PNG is byte-identical on every platform.
	q := Point{tileSize/2 + float64(bandOffset*d), tileSize/2 + float64(bandOffset*d)} // along the lower-right diagonal
	along := Point{d, -d}                                                              // lower-left -> upper-right
	across := Point{d, d}
	corner := func(sa, sb float64) Point {
		return Point{
			q.X + float64(sa*reach*along.X) + float64(sb*bandWidth/2*across.X),
			q.Y + float64(sa*reach*along.Y) + float64(sb*bandWidth/2*across.Y),
		}
	}
	sc.Items = append(sc.Items, Item{Fill: c.DevBanner, Shape: Polygon(corner(-1, -1), corner(1, -1), corner(1, 1), corner(-1, 1))})

	runs := []Run{{Text: "DEV", Fill: c.DevBannerText}}
	_, tw, err := f.Text(runs, size, tracking, 0, 0)
	if err != nil {
		return Scene{}, err
	}
	text, _, err := f.Text(runs, size, tracking, q.X-tw/2, q.Y+f.CapHeight(size)/2)
	if err != nil {
		return Scene{}, err
	}
	rot := Rotate(q, -d, d) // -45 degrees: the text runs up and to the right
	for _, it := range text {
		sc.Items = append(sc.Items, Item{Fill: it.Fill, Shape: it.Shape.(Path).Transform(rot)})
	}
	return sc, nil
}
