// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

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
