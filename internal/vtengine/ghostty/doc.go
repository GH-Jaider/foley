// Package ghostty implements the vtengine contract on libghostty-vt via cgo.
// This package is the project's cgo quarantine: the pinned static
// library, its C headers and every cgo directive live here and nowhere else.
//
// The pin lives in libbuild/build.zig.zon (URL+content-hash);
// `make engine-lib` rebuilds lib/<goos-goarch>/libghostty-vt.a for every v1
// target inside a Linux container (darwin targets are cross-compiled and
// repacked with Apple's libtool). include/ vendors the C headers from the
// pinned ghostty commit (MIT). During M3 the cgo files build only with
// `-tags ghosttyvt` so default builds never require the artifact.
package ghostty
