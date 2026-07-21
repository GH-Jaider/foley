package raster

import "image"

// The keys band's drawn key faces. A font's dingbats are
// not an icon set — coverage gambles per family and the weights that
// do exist disagree (JetBrains' ⌫ is a filled mass next to a hairline
// ↩). So, exactly like box-drawing in sprites.go, foley draws its own:
// polyline geometry in a 16-unit design space, stroked into an alpha
// mask at the cap size — one weight, any size, byte-identical
// everywhere. Only keys a physical keyboard draws get one: the arrows
// always, and enter/tab/bksp/del under KeysIcons. esc is never an
// icon; keyboards print the word.

// keyIcon identifies a drawn key face; iconNone means the cap is text.
type keyIcon uint8

const (
	iconNone keyIcon = iota
	iconUp
	iconDown
	iconLeft
	iconRight
	iconEnter
	iconTab
	iconBksp
	iconDel
)

// keyIconKey caches one rendered icon per size.
type keyIconKey struct {
	icon keyIcon
	px   int
}

// keyIconGeom is an icon's polylines in the 16-unit design space, plus
// its box width (arrows and enter are square; bksp/del run wider).
// Right-angle corners round naturally: strokeAlpha inks by distance to
// the segments, so joints inherit the stroke's own radius.
type keyIconGeom struct {
	w     float64
	polys [][]spritePt
}

// keyIconGeoms returns the design-space geometry (a function, not a
// package variable: the house bans mutable globals, and this is cold
// path — masks cache per size).
func keyIconGeoms(icon keyIcon) keyIconGeom {
	switch icon {
	case iconNone:
		return keyIconGeom{}
	case iconUp:
		return keyIconGeom{w: 16, polys: [][]spritePt{
			{{8, 13}, {8, 3.5}},
			{{4.4, 7.1}, {8, 3.5}, {11.6, 7.1}},
		}}
	case iconDown:
		return keyIconGeom{w: 16, polys: [][]spritePt{
			{{8, 3}, {8, 12.5}},
			{{4.4, 8.9}, {8, 12.5}, {11.6, 8.9}},
		}}
	case iconLeft:
		return keyIconGeom{w: 16, polys: [][]spritePt{
			{{13, 8}, {3.5, 8}},
			{{7.1, 4.4}, {3.5, 8}, {7.1, 11.6}},
		}}
	case iconRight:
		return keyIconGeom{w: 16, polys: [][]spritePt{
			{{3, 8}, {12.5, 8}},
			{{8.9, 4.4}, {12.5, 8}, {8.9, 11.6}},
		}}
	case iconEnter:
		return keyIconGeom{w: 16, polys: [][]spritePt{
			{{12.5, 4}, {12.5, 8.7}, {4.2, 8.7}},
			{{7, 5.9}, {4.2, 8.7}, {7, 11.5}},
		}}
	case iconTab:
		return keyIconGeom{w: 16, polys: [][]spritePt{
			{{2.5, 8}, {11, 8}},
			{{7.5, 4.5}, {11, 8}, {7.5, 11.5}},
			{{13.5, 4.5}, {13.5, 11.5}},
		}}
	case iconBksp:
		// The × keeps AIR inside the box (Jaider's note on v1).
		return keyIconGeom{w: 18, polys: [][]spritePt{
			{{7, 3.5}, {14.5, 3.5}, {14.5, 12.5}, {7, 12.5}, {3, 8}, {7, 3.5}},
			{{9.6, 6.7}, {12.2, 9.3}},
			{{12.2, 6.7}, {9.6, 9.3}},
		}}
	case iconDel:
		return keyIconGeom{w: 18, polys: [][]spritePt{
			{{11, 3.5}, {3.5, 3.5}, {3.5, 12.5}, {11, 12.5}, {15, 8}, {11, 3.5}},
			{{5.8, 6.7}, {8.4, 9.3}},
			{{8.4, 6.7}, {5.8, 9.3}},
		}}
	}
	return keyIconGeom{}
}

// keyIconStrip renders (and caches per size) a drawn key face as a
// strip, so caps treat text and icons uniformly. The ascent anchors a
// coalescing counter's baseline the way a text strip's would.
func (r *Rasterizer) keyIconStrip(icon keyIcon, px int) textStrip {
	ck := keyIconKey{icon: icon, px: px}
	if m, ok := r.keyIcons[ck]; ok {
		return m
	}
	g := keyIconGeoms(icon)
	if len(g.polys) == 0 || px <= 0 {
		return textStrip{}
	}
	// Scale the design space to the cap size. Pure products — no
	// c + a·b shapes — so the package FMA rule is satisfied by form.
	u := float64(px) / 16
	w := int(g.w * u)
	a := image.NewAlpha(image.Rect(0, 0, w, px))
	polys := make([][]spritePt, len(g.polys))
	for i, poly := range g.polys {
		pts := make([]spritePt, len(poly))
		for j, p := range poly {
			pts[j] = spritePt{x: float64(p.x * u), y: float64(p.y * u)}
		}
		polys[i] = pts
	}
	// Stroke weight follows the cap size (the mockup's 1.7/16 ratio).
	strokeAlpha(a, polys, float64(px)*17/160)
	ts := textStrip{mask: &glyphMask{alpha: a}, asc: px * 4 / 5}
	r.keyIcons[ck] = ts
	return ts
}
