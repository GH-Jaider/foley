// Package raster turns a vtengine frame snapshot into RGBA pixels: HarfBuzz
// shaping in style runs (ligature clusters anchored to their origin cell),
// FreeType glyph rendering with an atlas cache, color emoji (CBDT strikes),
// kitty graphics compositing by z-layer, cursor and decorations, at 1x or 2x.
// Quality is pinned by the typographic golden suite (byte-exact frames).
//
// DETERMINISM RULE: frames must be byte-identical across OS and CPU
// architecture (the PRD's north metric). Blending and span math stay
// integer-only. Any float expression shaped like c + a*b in the render
// path must round the product through an explicit conversion — e.g.
// c + float32(a*b), see mask() in text.go — because on arm64 the Go
// compiler otherwise fuses it into a single-rounding FMA and coverage
// can drift by one alpha step relative to amd64 (found empirically:
// one pixel in the first cross-arch CI run, bisected with -d=fmahash).
package raster
