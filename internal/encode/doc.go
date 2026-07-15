// Package encode turns captured frames and streams into the final outputs:
// mp4/webm via ffmpeg (with the 2x supersample → Lanczos downscale chain),
// high-quality GIF via gifski, and raw PNG sequences.
package encode
