// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "syscall"

// platformKeptSignal reports this kernel's own signals a disposition window
// leaves alone, beyond the shared list in passedToAChild: EMT, a fault the
// runtime answers itself.
func platformKeptSignal(sig syscall.Signal) bool {
	return sig == syscall.SIGEMT
}
