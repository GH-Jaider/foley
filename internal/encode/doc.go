// Package encode turns the driver's (frame, duration) stream into final
// outputs (ADR-013). PNGSink is the interchange: a directory of exact
// frames plus an ffconcat manifest with their exact durations — it is a
// driver.Sink, foley's native PNG output and ffmpeg's staging at once.
// GIF/MP4/WebM assemble it with ffmpeg through execx (palettegen recipe
// for GIF, libx264/vp9 for video). Byte-determinism ends at the PNGs and
// the manifest; the assembled videos are tool-versioned artifacts.
package encode
