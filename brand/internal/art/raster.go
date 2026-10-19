package art

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"golang.org/x/image/vector"
)

// kappa places cubic Bézier control points to approximate a quarter circle.
const kappa = 0.5522847498

// Render rasterizes the scene at width px (height keeps the aspect ratio),
// antialiased, onto a transparent background.
//
// The output can differ by a shade or two on a few edge pixels between CPU
// architectures: x/image/vector accumulates coverage with SIMD on amd64 and
// scalar code elsewhere, and they round differently. That's why
// check-generated compares PNGs by pixel within a tolerance rather than by
// bytes (see brand's tests).
func (s Scene) Render(px int) (*image.RGBA, error) {
	scale := float64(px) / s.W
	w, h := px, int(math.Round(s.H*scale))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for _, it := range s.Items {
		c, err := parseHex(it.Fill)
		if err != nil {
			return nil, err
		}
		r := vector.NewRasterizer(w, h)
		r.DrawOp = draw.Over
		var p Path
		switch sh := it.Shape.(type) {
		case Rect:
			p = sh.path()
		case Path:
			p = sh
		}
		trace(r, p, scale)
		r.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
	}
	return img, nil
}

// PNG rasterizes the scene at width px and encodes it.
func (s Scene) PNG(px int) ([]byte, error) {
	img, err := s.Render(px)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// path outlines the rectangle, rounding its corners with cubic curves.
func (r Rect) path() Path {
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
	k := r.R * kappa
	pt := func(x, y float64) Point { return Point{x, y} }
	return Path{
		{Op: MoveTo, Pts: [3]Point{pt(x0+r.R, y0)}},
		{Op: LineTo, Pts: [3]Point{pt(x1-r.R, y0)}},
		{Op: CubeTo, Pts: [3]Point{pt(x1-r.R+k, y0), pt(x1, y0+r.R-k), pt(x1, y0+r.R)}},
		{Op: LineTo, Pts: [3]Point{pt(x1, y1-r.R)}},
		{Op: CubeTo, Pts: [3]Point{pt(x1, y1-r.R+k), pt(x1-r.R+k, y1), pt(x1-r.R, y1)}},
		{Op: LineTo, Pts: [3]Point{pt(x0+r.R, y1)}},
		{Op: CubeTo, Pts: [3]Point{pt(x0+r.R-k, y1), pt(x0, y1-r.R+k), pt(x0, y1-r.R)}},
		{Op: LineTo, Pts: [3]Point{pt(x0, y0+r.R)}},
		{Op: CubeTo, Pts: [3]Point{pt(x0, y0+r.R-k), pt(x0+r.R-k, y0), pt(x0+r.R, y0)}},
		{Op: Close},
	}
}

// trace feeds the path to r, scaled to pixels.
func trace(r *vector.Rasterizer, p Path, scale float64) {
	f := func(q Point) (float32, float32) { return float32(q.X * scale), float32(q.Y * scale) }
	for _, c := range p {
		switch c.Op {
		case MoveTo:
			r.MoveTo(f(c.Pts[0]))
		case LineTo:
			r.LineTo(f(c.Pts[0]))
		case QuadTo:
			bx, by := f(c.Pts[0])
			cx, cy := f(c.Pts[1])
			r.QuadTo(bx, by, cx, cy)
		case CubeTo:
			bx, by := f(c.Pts[0])
			cx, cy := f(c.Pts[1])
			dx, dy := f(c.Pts[2])
			r.CubeTo(bx, by, cx, cy, dx, dy)
		case Close:
			r.ClosePath()
		}
	}
	r.ClosePath()
}

func parseHex(s string) (color.NRGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("colour %q: want #rrggbb", s)
	}
	rgb, err := hex.DecodeString(s[1:])
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("colour %q: %w", s, err)
	}
	return color.NRGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 0xff}, nil
}

// ICO packs PNG-encoded square images into a .ico file. Each image is
// stored as PNG, which every browser supports.
func ICO(images ...[]byte) ([]byte, error) {
	if len(images) > math.MaxUint8 {
		return nil, fmt.Errorf("ico: %d images is too many", len(images))
	}
	var out bytes.Buffer
	// reserved, type (1 = icon), image count
	if err := binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))}); err != nil { //nolint:gosec // count checked against MaxUint8 above
		return nil, err
	}
	offset := 6 + 16*len(images)
	for _, data := range images {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if cfg.Width != cfg.Height || cfg.Width < 1 || cfg.Width > 256 {
			return nil, fmt.Errorf("ico image must be square and at most 256px, got %dx%d", cfg.Width, cfg.Height)
		}
		if len(data) > math.MaxInt32 || offset > math.MaxInt32-len(data) {
			return nil, errors.New("ico: images too large")
		}
		side := byte(0) // 0 means 256
		if cfg.Width < 256 {
			side = byte(cfg.Width)
		}
		entry := struct {
			W, H, Colors, Reserved uint8
			Planes, BPP            uint16
			Size, Offset           uint32
		}{
			W: side, H: side,
			Planes: 1, BPP: 32,
			Size: uint32(len(data)), Offset: uint32(offset), //nolint:gosec // both checked against MaxInt32 above
		}
		if err := binary.Write(&out, binary.LittleEndian, entry); err != nil {
			return nil, err
		}
		offset += len(data)
	}
	for _, data := range images {
		out.Write(data)
	}
	return out.Bytes(), nil
}
