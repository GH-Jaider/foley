// Package ptyx spawns the demo command on a real pty (creack/pty) and owns
// the read loop: every output byte is timestamped and handed to the driver,
// which feeds the engine and measures settle (quiescence) directly — there is
// no shim process between foley and the app.
package ptyx
