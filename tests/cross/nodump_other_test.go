// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package cross_test

import "syscall"

// refuseACoreDump keeps a shell half that dies of a core-dumping signal from
// being dumped. The resource limit is the whole of it here: no core_pattern
// pipes a dump past it.
func refuseACoreDump() {
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
}
