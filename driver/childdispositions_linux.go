// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "syscall"

// platformKeptSignal reports this kernel's own signals a disposition window
// leaves alone, beyond the shared list in handedOver: STKFLT, a fault the
// runtime answers itself, and the real-time numbers below the first the
// runtime offers to os/signal, which it keeps for its own threading.
func platformKeptSignal(sig syscall.Signal) bool {
	return sig == syscall.SIGSTKFLT ||
		(sig > syscall.SIGSYS && int(sig) < firstRealTimeTheRuntimeHandles)
}
