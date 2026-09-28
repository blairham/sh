// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package repl

import "syscall"

// atimeParts is the access time's seconds and nanoseconds, which the BSDs
// spell `Atimespec` and Linux spells `Atim`. One line each, in files of their
// own, rather than a build tag inside one function.
func atimeParts(st *syscall.Stat_t) (int64, int64) {
	return st.Atimespec.Sec, st.Atimespec.Nsec
}
