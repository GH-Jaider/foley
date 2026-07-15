// Package execx is the single seam for external binaries — after the v0.8
// thesis just the encoders: ffmpeg and gifski. One tool table with minimum
// versions, context-aware execution and fakes for tests. os/exec is forbidden
// outside this package (enforced by depguard); foley doctor reads the same
// table.
package execx
