//go:build ghosttyvt

package ghostty

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo darwin,arm64 LDFLAGS: ${SRCDIR}/lib/darwin-arm64/libghostty-vt.a
#cgo darwin,amd64 LDFLAGS: ${SRCDIR}/lib/darwin-amd64/libghostty-vt.a
#cgo linux,arm64 LDFLAGS: ${SRCDIR}/lib/linux-arm64/libghostty-vt.a
#cgo linux,amd64 LDFLAGS: ${SRCDIR}/lib/linux-amd64/libghostty-vt.a
#include <ghostty/vt.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// smoke proves the pinned static library links and runs: it creates a
// terminal, writes bytes through the VT parser and frees it. The full
// vtengine.Engine implementation replaces this file as M3 progresses.
func smoke() error {
	var term C.GhosttyTerminal
	opts := C.GhosttyTerminalOptions{
		cols:           80,
		rows:           24,
		max_scrollback: 0,
	}
	if rc := C.ghostty_terminal_new(nil, &term, opts); rc != C.GHOSTTY_SUCCESS {
		return fmt.Errorf("ghostty_terminal_new: rc=%d", int(rc))
	}
	defer C.ghostty_terminal_free(term)

	msg := []byte("hola desde foley (cgo + libghostty-vt pineada)\r\n")
	C.ghostty_terminal_vt_write(term,
		(*C.uint8_t)(unsafe.Pointer(&msg[0])), C.size_t(len(msg)))

	return nil
}
