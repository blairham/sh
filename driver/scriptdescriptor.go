// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/blairham/sh/interp"
)

// holdTheScript puts the script file at the descriptor the dialect keeps it
// on while it runs, where the dialect keeps one — see Shell.ScriptDescriptor.
//
// The number is taken, so the first number `exec {a}>…` allocates is the one
// above it, and a command reading from it reads the script from where the
// shell has got to: before each command of the script itself the file is
// moved to the end of the furthest line a command has started on. Measured
// 2026-10-03 on ksh93u+ from a script file, `exec {a}>/dev/null` is 11 and
// `cat <&10` on line 3 of a four-line script prints line 4; through -c or on
// standard input the allocation is 10 and the number is closed. A command the
// shell runs is not handed the descriptor: `/bin/ls /dev/fd` there lists 0 to
// 4 and no 10.
//
// The furthest line and not the latest, because a function the script
// defined earlier runs lines above the one that called it and the shell's
// reading has not gone back for them.
func (sh Shell) holdTheScript(r *interp.Runner, in source) {
	n := sh.ScriptDescriptor
	if n <= 0 || in.file == "" {
		return
	}
	f, err := os.Open(in.file)
	if err != nil {
		return
	}
	r.SetDescriptor(n, f)
	r.KeepDescriptorFromChildren(n)
	ends := lineEnds(in.src)
	var mu sync.Mutex
	furthest := 0
	next := r.Events
	r.Events = interp.SinkFunc(func(ctx context.Context, e interp.Event) {
		if e.Kind == interp.EventCommandStart && e.File == in.file && e.Line > 0 {
			mu.Lock()
			if e.Line > furthest {
				furthest = e.Line
				at := int64(len(in.src))
				if e.Line <= len(ends) {
					at = int64(ends[e.Line-1])
				}
				// Past any empty lines straight after it, which the shell
				// has read on its way to the next command: measured, a `cat
				// <&10` above two empty lines and a comment prints from the
				// comment, and above a line of blanks prints the blanks.
				for at < int64(len(in.src)) && in.src[at] == '\n' {
					at++
				}
				_, _ = f.Seek(at, io.SeekStart)
			}
			mu.Unlock()
		}
		if next != nil {
			next.Emit(ctx, e)
		}
	})
}

// lineEnds is the offset just past each line's newline.
func lineEnds(src string) []int {
	var ends []int
	for i := 0; ; {
		j := strings.IndexByte(src[i:], '\n')
		if j < 0 {
			return append(ends, len(src))
		}
		i += j + 1
		ends = append(ends, i)
	}
}
