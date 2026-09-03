// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// terminalName is the terminal's name without its directory: `ttys013`.
//
// Not the file's own name. A shell's standard input is the process's, and Go
// calls that `/dev/stdin` whatever is behind it — which is the name of the
// door rather than of the room, and drew `stdin` where every shell draws the
// terminal.
//
// Found the way ttyname(3) finds it: the device number of the open file, then
// a look through /dev for the entry with the same one. There is no portable
// call for this outside libc, and this module does not use cgo.
//
// Asked once. A shell does not change terminals, and the search reads a
// directory.
func terminalName(f *os.File) string {
	if f == nil {
		return ""
	}
	ttyOnce.Do(func() { ttyName = lookupTerminal(f) })
	return ttyName
}

func lookupTerminal(f *os.File) string {
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	// /dev holds the terminals on both families; Linux keeps the
	// pseudo-terminals a directory further down.
	for _, dir := range []string{"/dev", "/dev/pts"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ei, err := e.Info()
			if err != nil || ei.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			est, ok := ei.Sys().(*syscall.Stat_t)
			if ok && est.Rdev == st.Rdev {
				return filepath.Base(e.Name())
			}
		}
	}
	return ""
}

var (
	ttyOnce sync.Once
	ttyName string
)
