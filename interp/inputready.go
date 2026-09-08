// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"

	"github.com/blairham/sh/internal/fdset"
)

// inputWaiting reports whether a read on in would find something without
// waiting — a byte, or the end of the stream.
//
// It is the one question a shell asks about a stream without touching it, and
// asking it with a read would destroy the thing being asked about: a poll that
// consumed the byte it reported would leave a script polling in a loop eating
// its own input a character at a time.
//
// Only a descriptor can make a read wait. Anything a Runner holds in memory
// answers immediately — with a byte, or with the end of it — so it is always
// waiting, which is also the answer where the descriptor cannot be asked. A
// read is the only other way to find out and a read is what the question
// exists to avoid, so the honest fallback is the one that says "go ahead".
//
// The asking itself is internal/fdset's, which is also where the line editor
// asks its own version of this question — see that package for why the two
// callers share the substrate rather than each carrying a copy of it.
func inputWaiting(in io.Reader) bool {
	f, ok := in.(interface{ Fd() uintptr })
	if !ok {
		return true
	}
	fd := int(f.Fd())
	if fd < 0 {
		// Closed out from under the Runner. A read on it returns at once.
		return true
	}
	return fdset.ReadableNow(fd)
}
