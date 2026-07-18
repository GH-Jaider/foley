package raster

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"github.com/go-text/typesetting/font"
	xdraw "golang.org/x/image/draw"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// maxKittyCacheEntries bounds decoded kitty-image memory.
const maxKittyCacheEntries = 64

// drawPlacements composites kitty-graphics placements of one layer.
func (r *Rasterizer) drawPlacements(dst *image.RGBA, src ImageSource, ps []vtengine.Placement) error {
	for _, p := range ps {
		img, err := r.kittyImage(src, p)
		if err != nil {
			return err
		}
		if img == nil {
			continue
		}
		ox := r.orgX + int(p.Col)*r.cellW + int(p.OffX)*r.s
		oy := r.orgY + int(p.Row)*r.cellH + int(p.OffY)*r.s
		dr := image.Rect(ox, oy, ox+int(p.PixelW)*r.s, oy+int(p.PixelH)*r.s)
		sr := image.Rect(int(p.SrcX), int(p.SrcY), int(p.SrcX+p.SrcW), int(p.SrcY+p.SrcH))
		if dr.Empty() || sr.Empty() {
			// Zero-area placement — nothing to paint, and the ratio
			// math below must never divide by an empty source. The
			// engine skips unresolvable placements, but Render is the
			// robustness boundary: it must not panic on ANY frame.
			continue
		}
		// Scaler choice follows kitty's ground truth: exact integer
		// upscales stay NearestNeighbor (1:1 pixel art at our supersample
		// keeps its crisp edges), everything else is filtered like a real
		// terminal's GPU sampler. NN at fractional ratios SAMPLES the
		// source: most 1px texture lines vanish but the ones the lattice
		// hits survive at full contrast — phantom cut lines at the beat
		// period (found live by tenten's fine-pixel sprites).
		scaler := xdraw.Interpolator(xdraw.ApproxBiLinear)
		if fx, fy := dr.Dx()/sr.Dx(), dr.Dy()/sr.Dy(); fx >= 1 && fx == fy &&
			fx*sr.Dx() == dr.Dx() && fy*sr.Dy() == dr.Dy() {
			scaler = xdraw.NearestNeighbor
		}
		scaler.Scale(dst, dr, img, sr, xdraw.Over, nil)
	}
	return nil
}

// kittyImage fetches and caches decoded placement pixels keyed by image
// generation (stale cache entries never alias: stamps are unique).
func (r *Rasterizer) kittyImage(src ImageSource, p vtengine.Placement) (*image.RGBA, error) {
	data, err := src.ImagePixels(p.ImageID)
	if err != nil {
		return nil, fmt.Errorf("raster: placement image %d: %w", p.ImageID, err)
	}
	key := kittyKey{id: p.ImageID, gen: data.Generation}
	if img, ok := r.kitty[key]; ok {
		return img, nil
	}
	if len(data.Pix) == 0 {
		return nil, nil
	}
	// Bound the cache: long realtime recordings of image-heavy TUIs
	// (inline video!) would otherwise grow without limit. Clearing is
	// crude but predictable; stamps are monotonic so entries never alias.
	if len(r.kitty) >= maxKittyCacheEntries {
		clear(r.kitty)
	}
	img := image.NewRGBA(image.Rect(0, 0, data.W, data.H))
	// ImageData.Pix borrows engine memory (valid until next Write): copy
	// into the cache, converting straight alpha to premultiplied.
	nrgba := &image.NRGBA{Pix: data.Pix, Stride: data.W * 4, Rect: img.Rect}
	xdraw.Draw(img, img.Rect, nrgba, image.Point{}, xdraw.Src)
	r.kitty[key] = img
	return img, nil
}

// drawEmojiCell renders a color-emoji grapheme from the CBDT bitmap font,
// scaled to the cell span reported by the engine.
func (r *Rasterizer) drawEmojiCell(dst *image.RGBA, f *vtengine.Frame, x, y int) {
	cell := f.CellAt(x, y)
	out := r.shape(r.emoji, cell.Runes)
	if len(out.Glyphs) == 0 {
		return
	}
	gid := out.Glyphs[0].GlyphID
	img := r.emojiImage(gid)
	if img == nil {
		return
	}
	span := max(int(cell.Width), 1)
	rect := r.cellRect(x, y, span)
	// Aspect-fit inside the cell span.
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	scale := min(float32(rect.Dx())/float32(iw), float32(rect.Dy())/float32(ih))
	w, h := int(float32(iw)*scale), int(float32(ih)*scale)
	ox := rect.Min.X + (rect.Dx()-w)/2
	oy := rect.Min.Y + (rect.Dy()-h)/2
	xdraw.ApproxBiLinear.Scale(dst, image.Rect(ox, oy, ox+w, oy+h), img, img.Bounds(), xdraw.Over, nil)
}

func (r *Rasterizer) emojiImage(gid font.GID) *image.RGBA {
	if img, ok := r.emojis[gid]; ok {
		return img
	}
	bm, ok := r.emoji.GlyphData(gid).(font.GlyphBitmap)
	if !ok || bm.Format != font.PNG {
		r.emojis[gid] = nil
		return nil
	}
	decoded, err := png.Decode(bytes.NewReader(bm.Data))
	if err != nil {
		r.emojis[gid] = nil
		return nil
	}
	img := image.NewRGBA(decoded.Bounds())
	xdraw.Draw(img, img.Bounds(), decoded, decoded.Bounds().Min, xdraw.Src)
	r.emojis[gid] = img
	return img
}
