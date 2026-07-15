// Package raster turns a vtengine frame snapshot into RGBA pixels: HarfBuzz
// shaping in style runs (ligature clusters anchored to their origin cell),
// FreeType glyph rendering with an atlas cache, color emoji (CBDT strikes),
// kitty graphics compositing by z-layer, cursor and decorations, at 1x or 2x.
// Quality is pinned by the typographic golden suite (byte-exact frames).
package raster
