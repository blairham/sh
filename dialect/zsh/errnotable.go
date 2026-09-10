// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"
	"strings"
	"syscall"
)

// buildErrnoNames lays a platform's error names out by number, so that a name
// sits at the index its errno is.
//
// The result is one longer than the largest errno, with index 0 unused: there
// is no errno 0 to name, and `$errnos` drops the hole on its way out — see
// errnosView, which is the only reader.
//
// A number a platform leaves unused is an empty string in the middle of the
// table, which is what a shell array with a hole in it reads as. Linux has two
// such holes and this platform has none, so the shape is not hypothetical.
func buildErrnoNames(names map[syscall.Errno]string) []string {
	highest := 0
	for e := range names {
		if int(e) > highest {
			highest = int(e)
		}
	}
	out := make([]string, highest+1)
	for e, name := range names {
		out[e] = name
	}
	return out
}

// errnoText is the operating system's own sentence for one error number, in
// the capitalization `syserror` prints it in.
//
// **Two different sentences for one errno, and both are the platform's.** The
// shell being measured writes `No such file or directory` from `syserror` and
// `no such file or directory` when one of its own builtins reports a failure,
// so the case is not decoration and not ours to normalize — see sysErrnoText,
// which is the lower-cased one and is what every other refusal in this dialect
// uses.
//
// The text comes from Go's own table for the platform rather than from a table
// written here, which is the same source the *names* come from and reached the
// same way. Measured against the platform's `strerror` for every number both
// name: darwin 1 to 106 and linux 1 to 133 agree on every one of them once the
// first letter is put back, which is the whole of the difference between the C
// library's spelling and Go's.
//
// What Go's table cannot answer is answered per platform, and there are three
// such numbers: zero, a number the platform leaves unused, and a number past
// the end. See errnoUnknownText, and errnoTextOverrides for the one darwin
// errno that is newer than the table Go's package was generated from.
func errnoText(n int) string {
	if n > 0 && n < len(errnoNames) && errnoNames[n] != "" {
		if text, ok := errnoTextOverrides[n]; ok {
			return text
		}
		// Go says `errno 107` for a number it has no sentence for, which is
		// the one answer that must not be passed through: it is neither the
		// platform's wording nor an admission that there is none.
		if text := syscall.Errno(n).Error(); !strings.HasPrefix(text, "errno ") {
			return strings.ToUpper(text[:1]) + text[1:]
		}
	}
	return errnoUnknownText(n)
}

// errnoNumber reads `syserror`'s operand, which is a number or one of the
// names in `$errnos`.
//
// The name lookup is **case-sensitive**, measured: `syserror ENOENT` is the
// sentence and `syserror enoent` is status 2 with nothing said. The two
// halves are one function because the operand is one word and which of the
// two it is cannot be known before it is read.
func errnoNumber(text string) (int, bool) {
	if n, err := strconv.Atoi(text); err == nil {
		return n, true
	}
	for n, name := range errnoNames {
		if name != "" && name == text {
			return n, true
		}
	}
	return 0, false
}
