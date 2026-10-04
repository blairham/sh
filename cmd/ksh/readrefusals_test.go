// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// `read`'s refusals in this shell's words. Measured 2026-10-04 on ksh93u+
// 2012-08-01 under `-c` (#5722).
func TestReadRefusalsAreKsh93s(t *testing.T) {
	const usage = "Usage: read [-ACprsSv] [-d delim] [-u fd] [-t timeout] [-n count] [-N count]\n" +
		"            [var?prompt] [var ...]\n"
	for _, c := range []struct{ src, errs, out string }{
		// A number that is not one names the letter, and the usage follows
		// at 2. See interp.Diagnostics.ReadBadNumberWritesTheUsage.
		{"read -u line; echo st=$?", "ksh: read: -u: numeric fd argument expected\n" + usage, "st=2\n"},
		{"read -u 9x line; echo st=$?", "ksh: read: -u: numeric fd argument expected\n" + usage, "st=2\n"},
		{"read -n x v; echo st=$?", "ksh: read: -n: numeric count argument expected\n" + usage, "st=2\n"},
		{"read -N x v; echo st=$?", "ksh: read: -N: numeric count argument expected\n" + usage, "st=2\n"},
		// A letter this shell has not built is still not an unknown one.
		{"read -kv x; echo st=$?", "ksh: read: -k: unknown option\n" + usage, "st=2\n"},
		{"read -k2v x; echo st=$?", "ksh: read: -k: unknown option\nksh: read: -2: unknown option\n" + usage, "st=2\n"},
		// A closed standard input is not an empty one.
		{"read a <&-; echo st=$?", "ksh: read: bad file unit number\n", "st=1\n"},
		{"read -u 0 a <&-; echo st=$?", "ksh: read: bad file unit number\n", "st=1\n"},
	} {
		out, errs, _ := runKshArgs(t, "-c", c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%s:\n got %q, %q\nwant %q, %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
