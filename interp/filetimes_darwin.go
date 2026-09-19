// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package interp

import (
	"io/fs"
	"syscall"
	"time"
)

// fileTime is the access or inode-change time behind a stat, which is the
// half of the three file-time glob qualifiers that no portable interface
// answers: fs.FileInfo carries the modification time alone.
//
// Per platform because the field names are, and nothing else about the
// qualifier is: this kernel's stat spells them Atimespec and Ctimespec where
// Linux spells them Atim and Ctim. Same shape as the signal table one package
// over — a fact about the machine, in a file the machine chooses.
func fileTime(info fs.FileInfo, kind byte) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	if kind == 'c' {
		return time.Unix(st.Ctimespec.Unix()), true
	}
	return time.Unix(st.Atimespec.Unix()), true
}
