// Package art draws mykeebs' logo artwork. Each design is built once as a
// Scene (filled rectangles and paths in absolute coordinates) and that same
// Scene is written out as SVG and rasterized to PNG, so the vector and
// bitmap versions of a design can't disagree.
package art

import (
	"math"
	"strconv"
	"strings"
)

// Point is a position in scene units.
type Point struct{ X, Y float64 }

// Op is a path drawing operation.
type Op int

// Path operations, matching SVG's M, L, Q, C and Z.
const (
	MoveTo Op = iota
	LineTo
	QuadTo
	CubeTo
	Close
)

// Cmd is one path operation. Pts holds 1 point for MoveTo/LineTo, 2 for
// QuadTo (control, end), 3 for CubeTo (control, control, end), none for Close.
type Cmd struct {
	Op  Op
	Pts [3]Point
}

// Shape is a Rect or a Path.
type Shape interface{ isShape() }

// Rect is an axis-aligned rectangle with corner radius R.
type Rect struct{ X, Y, W, H, R float64 }

// Path is a sequence of path operations, filled with the nonzero rule.
type Path []Cmd

func (Rect) isShape() {}
func (Path) isShape() {}

// Item is a shape filled with a solid colour ("#rrggbb").
type Item struct {
	Fill  string
	Shape Shape
}

// Scene is a design: a W x H canvas of items, painted in order.
type Scene struct {
	W, H  float64
	Label string
	Items []Item
}

// num formats a coordinate to at most two decimals, the precision every
// asset is drawn at.
func num(v float64) string {
	v = math.Round(v*100) / 100
	if v == 0 {
		v = 0 // normalise -0
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// SVG renders the scene as a standalone SVG document.
func (s Scene) SVG() []byte {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 `)
	b.WriteString(num(s.W) + " " + num(s.H))
	b.WriteString(`" width="` + num(s.W) + `" height="` + num(s.H))
	b.WriteString(`" role="img" aria-label="` + s.Label + `">`)
	for _, it := range s.Items {
		switch sh := it.Shape.(type) {
		case Rect:
			b.WriteString(`<rect x="` + num(sh.X) + `" y="` + num(sh.Y) +
				`" width="` + num(sh.W) + `" height="` + num(sh.H))
			if sh.R > 0 {
				b.WriteString(`" rx="` + num(sh.R))
			}
			b.WriteString(`" fill="` + it.Fill + `"/>`)
		case Path:
			b.WriteString(`<path fill="` + it.Fill + `" d="` + pathData(sh) + `"/>`)
		}
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

func pathData(p Path) string {
	var b strings.Builder
	pt := func(q Point) { b.WriteString(num(q.X) + " " + num(q.Y)) }
	for i, c := range p {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch c.Op {
		case MoveTo:
			b.WriteString("M")
			pt(c.Pts[0])
		case LineTo:
			b.WriteString("L")
			pt(c.Pts[0])
		case QuadTo:
			b.WriteString("Q")
			pt(c.Pts[0])
			b.WriteByte(' ')
			pt(c.Pts[1])
		case CubeTo:
			b.WriteString("C")
			pt(c.Pts[0])
			b.WriteByte(' ')
			pt(c.Pts[1])
			b.WriteByte(' ')
			pt(c.Pts[2])
		case Close:
			b.WriteString("Z")
		}
	}
	return b.String()
}

// Transform maps every point of the path through f.
func (p Path) Transform(f func(Point) Point) Path {
	out := make(Path, len(p))
	for i, c := range p {
		out[i] = c
		for j := range c.Pts {
			out[i].Pts[j] = f(c.Pts[j])
		}
	}
	return out
}

// Rotate returns a function rotating points around c by the angle whose sine
// and cosine are given. Scene y points down, so a negative sine turns
// counterclockwise on screen.
//
// The angle is passed as sin/cos rather than degrees so callers can use
// exact constants: math.Sincos's result differs in the last bit between
// arm64 and amd64. For the same reason each product is wrapped in float64():
// Go may fuse a*b+c into one FMA instruction on arm64 but not on amd64, and
// an explicit conversion forces the product to be rounded on its own.
// Either difference is enough to make a PNG differ between a Mac and CI.
func Rotate(c Point, sin, cos float64) func(Point) Point {
	return func(p Point) Point {
		dx, dy := p.X-c.X, p.Y-c.Y
		return Point{
			c.X + float64(dx*cos) - float64(dy*sin),
			c.Y + float64(dx*sin) + float64(dy*cos),
		}
	}
}

// Polygon is a closed path through pts.
func Polygon(pts ...Point) Path {
	p := Path{{Op: MoveTo, Pts: [3]Point{pts[0]}}}
	for _, q := range pts[1:] {
		p = append(p, Cmd{Op: LineTo, Pts: [3]Point{q}})
	}
	return append(p, Cmd{Op: Close})
}
