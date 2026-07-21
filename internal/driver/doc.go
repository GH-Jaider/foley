// Package driver orchestrates a recording: it owns the clock (virtual in
// deterministic mode — rendering faster than real time — or wall clock in
// realtime mode), feeds pty bytes into the engine, measures settle
// (quiescence) at the pty read loop, evaluates waits synchronously against
// the grid, applies input cadence and jitter, and schedules frame
// rasterization instants on the timeline (Hide/Show/Screenshot included).
//
// The driver depends only on the vtengine contract and the raster/encode
// seams — never on a concrete engine (enforced by depguard).
package driver
