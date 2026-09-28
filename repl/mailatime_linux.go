// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package repl

import "syscall"

// atimeParts is the access time's seconds and nanoseconds. See the darwin
// file beside this one for why the split is per platform.
func atimeParts(st *syscall.Stat_t) (int64, int64) {
	return st.Atim.Sec, st.Atim.Nsec
}
