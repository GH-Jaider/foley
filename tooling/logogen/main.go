// Command logogen composes foley's brand assets from REAL engine
// frames: assets/logo/logo.tape records ">foley" being typed and
// screenshots each state; this tool lays those screenshots into the
// film strip of the brand spec (docs/logo). The logo is not a drawing
// of a recording — it is one.
//
//	usage: logogen <frames-dir> <out-dir>
//
// Geometry and palette follow the spec: frame pitch 72u, screens
// 64×44u, two sprocket holes per frame (12×9u, r3) that are HOLES —
// full transparency, the surface below shows through. All integer
// arithmetic; the emitted PNGs are deterministic.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"

	"github.com/GH-Jaider/foley/assets/logo"
)

var (
	film   = color.RGBA{R: 0x19, G: 0x15, B: 0x14, A: 0xff} //nolint:gochecknoglobals // palette
	screen = color.RGBA{R: 0x0D, G: 0x0B, B: 0x0A, A: 0xff} //nolint:gochecknoglobals // palette
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: logogen <frames-dir> <out-dir>")
		os.Exit(2)
	}
	framesDir, outDir := os.Args[1], os.Args[2]
	var frames []*image.RGBA
	for i := 0; i < 6; i++ {
		f, err := loadPNG(filepath.Join(framesDir, fmt.Sprintf("logo-f%d.png", i)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "logogen: %v (record assets/logo/logo.tape first: make logo)\n", err)
			os.Exit(1)
		}
		frames = append(frames, f)
	}

	// The brand's cursor is ALIVE: every asset ships as a blinking GIF
	// (only the last frame's cursor blinks, per the spec) alongside a
	// static PNG. The off phase repaints the exact cursor red in the
	// SOURCE frame before any scaling.
	lastOff := cursorOff(frames[5])
	firstOff := cursorOff(frames[0])

	// Banner: 460×100u at 3 px/u — six typed states plus a bled half
	// frame on each edge (the strip is a cut from a longer reel).
	bannerXs := []int{-58, 14, 86, 158, 230, 302, 374, 446}
	bannerOn := map[int]*image.RGBA{1: frames[0], 2: frames[1], 3: frames[2], 4: frames[3], 5: frames[4], 6: frames[5]}
	banner := strip(3, 460, bannerXs, bannerOn, 12)
	must(writePNG(filepath.Join(outDir, "banner.png"), banner))
	bannerOffMap := map[int]*image.RGBA{1: frames[0], 2: frames[1], 3: frames[2], 4: frames[3], 5: frames[4], 6: lastOff}
	must(writeBlinkGIF(filepath.Join(outDir, "banner.gif"), banner, strip(3, 460, bannerXs, bannerOffMap, 12)))

	// Compact: 236×100u — sampled, exactly like the engine samples the
	// byte stream: ">", ">fol", ">foley█".
	compactXs := []int{-58, 14, 86, 158, 230}
	compact := strip(3, 236, compactXs, map[int]*image.RGBA{1: frames[0], 2: frames[3], 3: frames[5]}, 6)
	must(writePNG(filepath.Join(outDir, "compact.png"), compact))
	must(writeBlinkGIF(filepath.Join(outDir, "compact.gif"), compact,
		strip(3, 236, compactXs, map[int]*image.RGBA{1: frames[0], 2: frames[3], 3: lastOff}, 6)))

	// logo.gif — the projector, wrapped in film: the single-frame chip
	// TYPING the name letter by letter at the tape's own cadence
	// (220 ms), then the REC light blinking twice, looping forever.
	lastOffChip := iconChip(6, lastOff)
	typing := []struct {
		img   *image.RGBA
		delay int
	}{
		{iconChip(6, frames[0]), 45},
		{iconChip(6, frames[1]), 22},
		{iconChip(6, frames[2]), 22},
		{iconChip(6, frames[3]), 22},
		{iconChip(6, frames[4]), 22},
		{iconChip(6, frames[5]), 62},
		{lastOffChip, 53},
		{iconChip(6, frames[5]), 62},
		{lastOffChip, 53},
		{iconChip(6, frames[5]), 90},
	}
	tg := &gif.GIF{LoopCount: 0}
	for _, fr := range typing {
		pi, perr := palettize(fr.img)
		must(perr)
		tg.Image = append(tg.Image, pi)
		tg.Delay = append(tg.Delay, fr.delay)
		tg.Disposal = append(tg.Disposal, gif.DisposalNone)
	}
	must(writeGIF(filepath.Join(outDir, "logo.gif"), tg))

	// Icon: one frame cropped from the strip. Master at 6 px/u
	// (576 px), then exact integer downscales — the REC light blinks.
	icon := iconChip(6, frames[0])
	iconOff := iconChip(6, firstOff)
	must(writePNG(filepath.Join(outDir, "icon.png"), icon))
	must(writeBlinkGIF(filepath.Join(outDir, "icon.gif"), icon, iconOff))
	for _, size := range []int{64, 32, 16} {
		on := image.NewRGBA(image.Rect(0, 0, size, size))
		scaleTo(on, icon)
		must(writePNG(filepath.Join(outDir, fmt.Sprintf("icon-%d.png", size)), on))
		off := image.NewRGBA(image.Rect(0, 0, size, size))
		scaleTo(off, iconOff)
		must(writeBlinkGIF(filepath.Join(outDir, fmt.Sprintf("icon-%d.gif", size)), on, off))
	}
	// foley.ans: the cell-art chip as a raw ANSI file — fastfetch's
	// custom logo for the fetch tape (--logo-type file-raw), same
	// single source the CLI welcome draws.
	must(os.WriteFile(filepath.Join(outDir, "foley.ans"), []byte(logo.CellArt()), 0o644)) //nolint:gosec // brand asset
	fmt.Println("logogen: wrote banner/compact/icon (.png static + .gif blinking), icon-64/32/16 and foley.ans")
}

// strip composes one film strip: u = px per spec unit, wu = width in
// units, screenXs = screen slot origins (units, bleeds included),
// content maps slot index → screenshot, holes = sprocket count per row
// (pitch 36u from x=24u).
func strip(u, wu int, screenXs []int, content map[int]*image.RGBA, holes int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, wu*u, 100*u))
	fillRounded(img, image.Rect(0, 0, wu*u, 100*u), 0, film)
	for i, x := range screenXs {
		r := image.Rect(x*u, 24*u, (x+64)*u, 68*u)
		fillRounded(img, r, 2*u, screen)
		if shot, ok := content[i]; ok {
			blitInto(img, inset(r, 6*u, 2*u, 3*u), shot)
		}
	}
	for i := 0; i < holes; i++ {
		x := (24 + 36*i) * u
		punchRounded(img, image.Rect(x, 8*u, x+12*u, 17*u), 3*u)
		punchRounded(img, image.Rect(x, 72*u, x+12*u, 81*u), 3*u)
	}
	return img
}

// iconChip is the favicon: a single frame — four holes, the screen,
// the prompt with the REC light — on a rounded chip.
func iconChip(u int, shot *image.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 96*u, 96*u))
	fillRounded(img, image.Rect(4*u, 4*u, 92*u, 92*u), 10*u, film)
	scr := image.Rect(16*u, 30*u, 80*u, 66*u)
	fillRounded(img, scr, 2*u, screen)
	blitInto(img, inset(scr, 6*u, 2*u, 3*u), shot)
	for _, p := range [][2]int{{26, 14}, {58, 14}, {26, 73}, {58, 73}} {
		punchRounded(img, image.Rect(p[0]*u, p[1]*u, (p[0]+12)*u, (p[1]+9)*u), 3*u)
	}
	return img
}

// blitInto crops src to the box's aspect (centered) and scales it in.
func blitInto(dst *image.RGBA, box image.Rectangle, src *image.RGBA) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	bw, bh := box.Dx(), box.Dy()
	cw, ch := sw, sh
	if sw*bh > bw*sh {
		cw = sh * bw / bh // source wider than the box: crop width
	} else {
		ch = sw * bh / bw // source taller: crop height
	}
	// Built as a literal: image.Rect CANONICALIZES (swaps min/max when
	// max < min), which silently discarded the centering offsets —
	// found by measuring the composed pixels.
	crop := image.Rectangle{Min: image.Pt(sb.Min.X+(sw-cw)/2, sb.Min.Y+(sh-ch)/2)}
	crop.Max = crop.Min.Add(image.Pt(cw, ch))
	sub, _ := src.SubImage(crop).(*image.RGBA)
	tmp := image.NewRGBA(image.Rect(0, 0, bw, bh))
	scaleTo(tmp, sub)
	for y := 0; y < bh; y++ {
		so := tmp.PixOffset(0, y)
		do := dst.PixOffset(box.Min.X, box.Min.Y+y)
		copy(dst.Pix[do:do+4*bw], tmp.Pix[so:so+4*bw])
	}
}

// scaleTo shrinks src into dst with an exact integer area mean, alpha
// included (holes must stay holes through a downscale) — the same
// arithmetic family as the renderer's scaler, self-contained here.
func scaleTo(dst, src *image.RGBA) {
	db, sb := dst.Bounds(), src.Bounds()
	ow, oh := db.Dx(), db.Dy()
	sw, sh := sb.Dx(), sb.Dy()
	total := int64(sw) * int64(sh)
	half := total / 2
	spans := func(out, srcN int) [][2][]int64 {
		all := make([][2][]int64, out)
		for o := 0; o < out; o++ {
			lo, hi := o*srcN, (o+1)*srcN
			for s := lo / out; s <= (hi-1)/out; s++ {
				a, b := s*out, (s+1)*out
				ov := min64(int64(hi), int64(b)) - max64(int64(lo), int64(a))
				if ov > 0 {
					all[o][0] = append(all[o][0], int64(s))
					all[o][1] = append(all[o][1], ov)
				}
			}
		}
		return all
	}
	xs, ys := spans(ow, sw), spans(oh, sh)
	for y := 0; y < oh; y++ {
		d := dst.PixOffset(db.Min.X, db.Min.Y+y)
		for x := 0; x < ow; x++ {
			var r, g, b, a int64
			for yi, sy := range ys[y][0] {
				wy := ys[y][1][yi]
				row := src.PixOffset(sb.Min.X, sb.Min.Y+int(sy))
				for xi, sx := range xs[x][0] {
					w := wy * xs[x][1][xi]
					o := row + 4*int(sx)
					r += w * int64(src.Pix[o])
					g += w * int64(src.Pix[o+1])
					b += w * int64(src.Pix[o+2])
					a += w * int64(src.Pix[o+3])
				}
			}
			dst.Pix[d] = uint8((r + half) / total)   //nolint:gosec // weighted mean of bytes
			dst.Pix[d+1] = uint8((g + half) / total) //nolint:gosec
			dst.Pix[d+2] = uint8((b + half) / total) //nolint:gosec
			dst.Pix[d+3] = uint8((a + half) / total) //nolint:gosec
			d += 4
		}
	}
}

// fillRounded paints a rounded rectangle (r=0 is a plain fill).
func fillRounded(img *image.RGBA, rect image.Rectangle, r int, c color.RGBA) {
	forRounded(rect, r, func(x, y int) { img.SetRGBA(x, y, c) })
}

// punchRounded sets a rounded rectangle to FULL transparency: the
// sprocket holes are holes.
func punchRounded(img *image.RGBA, rect image.Rectangle, r int) {
	forRounded(rect, r, func(x, y int) { img.SetRGBA(x, y, color.RGBA{}) })
}

// forRounded visits every pixel inside a rounded rect — integer
// circle test at the corners, nothing fractional.
func forRounded(rect image.Rectangle, r int, visit func(x, y int)) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if r > 0 {
				cx, cy := 0, 0
				switch {
				case x < rect.Min.X+r && y < rect.Min.Y+r:
					cx, cy = rect.Min.X+r, rect.Min.Y+r
				case x >= rect.Max.X-r && y < rect.Min.Y+r:
					cx, cy = rect.Max.X-r-1, rect.Min.Y+r
				case x < rect.Min.X+r && y >= rect.Max.Y-r:
					cx, cy = rect.Min.X+r, rect.Max.Y-r-1
				case x >= rect.Max.X-r && y >= rect.Max.Y-r:
					cx, cy = rect.Max.X-r-1, rect.Max.Y-r-1
				}
				if cx != 0 {
					dx, dy := x-cx, y-cy
					if dx*dx+dy*dy > r*r {
						continue
					}
				}
			}
			visit(x, y)
		}
	}
}

// inset shrinks a rect asymmetrically: the strip's text sits ~7u from
// the screen's left edge in the spec (optical inset), tighter on the
// right where the cursor ends the wordmark.
func inset(r image.Rectangle, left, right, dy int) image.Rectangle {
	return image.Rect(r.Min.X+left, r.Min.Y+dy, r.Max.X-right, r.Max.Y-dy)
}

// cursorOff repaints the REC cursor (the frames' only exact #FF4F45
// pixels — the raster fills cell backgrounds solid) back to screen
// black: the blink's off phase, applied to the SOURCE so scaling
// blends stay consistent.
func cursorOff(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(b)
	copy(out.Pix, src.Pix)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := out.PixOffset(x, y)
			if out.Pix[o] == 0xFF && out.Pix[o+1] == 0x4F && out.Pix[o+2] == 0x45 {
				out.Pix[o], out.Pix[o+1], out.Pix[o+2] = 0x0D, 0x0B, 0x0A
			}
		}
	}
	return out
}

// writeBlinkGIF writes on/off as a looping two-frame GIF with the
// spec's blink cadence (1.15s cycle, ~54% on). Exact palette, holes
// transparent.
func writeBlinkGIF(path string, on, off *image.RGBA) error {
	po, err := palettize(on)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	pf, err := palettize(off)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	g := &gif.GIF{
		Image:     []*image.Paletted{po, pf},
		Delay:     []int{62, 53},
		Disposal:  []byte{gif.DisposalNone, gif.DisposalNone},
		LoopCount: 0,
	}
	return writeGIF(path, g)
}

func writeGIF(path string, g *gif.GIF) error {
	f, err := os.Create(path) //nolint:gosec // the tool's own output dir
	if err != nil {
		return err
	}
	if err := gif.EncodeAll(f, g); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// brandExact are the palette anchors that must survive quantization
// untouched: film, screen, ink, REC.
//
//nolint:gochecknoglobals // immutable palette table
var brandExact = map[[3]uint8]bool{
	{0x19, 0x15, 0x14}: true,
	{0x0D, 0x0B, 0x0A}: true,
	{0xEC, 0xE6, 0xDF}: true,
	{0xFF, 0x4F, 0x45}: true,
}

// palettize builds an adaptive palette. The antialiased blends of the
// scaled type can exceed 256 uniques, so non-brand colors quantize by
// progressively dropping low bits until the palette fits — the four
// brand hexes stay EXACT at every shift. Deterministic by
// construction; alpha 0 maps to one transparent slot.
func palettize(src *image.RGBA) (*image.Paletted, error) {
	for shift := 0; shift <= 4; shift++ {
		if out, ok := palettizeAt(src, shift); ok {
			return out, nil
		}
	}
	return nil, fmt.Errorf("logogen: composition exceeds 256 colors even at 4-bit channels")
}

func palettizeAt(src *image.RGBA, shift int) (*image.Paletted, bool) {
	quant := func(r, g, b uint8) [3]uint8 {
		c := [3]uint8{r, g, b}
		if shift == 0 || brandExact[c] {
			return c
		}
		m := uint8(0xFF) << shift //nolint:gosec // channel mask
		return [3]uint8{r & m, g & m, b & m}
	}
	b := src.Bounds()
	idx := map[[3]uint8]uint8{}
	pal := color.Palette{color.RGBA{}} // slot 0: the holes
	out := image.NewPaletted(b, nil)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := src.PixOffset(x, y)
			if src.Pix[o+3] == 0 {
				out.SetColorIndex(x, y, 0)
				continue
			}
			c := quant(src.Pix[o], src.Pix[o+1], src.Pix[o+2])
			id, ok := idx[c]
			if !ok {
				if len(pal) >= 256 {
					return nil, false
				}
				id = uint8(len(pal)) //nolint:gosec // bounded above
				idx[c] = id
				pal = append(pal, color.RGBA{R: c[0], G: c[1], B: c[2], A: 0xff})
			}
			out.SetColorIndex(x, y, id)
		}
	}
	out.Palette = pal
	return out, true
}

func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path) //nolint:gosec // the tool's own input dir
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path) //nolint:gosec // the tool's own output dir
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "logogen:", err)
		os.Exit(1)
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
