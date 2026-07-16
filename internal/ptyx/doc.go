// Package ptyx spawns the demo command on a real pty (creack/pty) and owns
// the read loop: every output byte is timestamped and handed to the driver,
// which feeds the engine and measures settle (quiescence) directly — there is
// no shim process between foley and the app.
//
// Signals are deliberately not exposed: interactive interrupts travel as
// bytes through the pty like any terminal (Ctrl+C = 0x03, delivered by the
// line discipline or read raw by the TUI), and hard termination is Close.
package ptyx
