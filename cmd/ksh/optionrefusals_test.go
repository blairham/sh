// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// Builtin option refusals in this shell's words. Measured 2026-10-04 on
// ksh93u+ 2012-08-01 under `-c` (#5722).
func TestBuiltinOptionRefusalsAreKsh93s(t *testing.T) {
	const (
		readUsage = "Usage: read [-ACprsSv] [-d delim] [-u fd] [-t timeout] [-n count] [-N count]\n" +
			"            [var?prompt] [var ...]\n"
		killUsage    = "Usage: kill [-lL] [-n signum] [-s signame] job ...\n   Or: kill [ options ] -l [arg ...]\n"
		typesetUsage = "Usage: typeset [-bflmnprstuxACHS] [-a[type]] [-i[base]] [-E[n]] [-F[n]] [-L[n]]\n" +
			"               [-M[mapping]] [-R[n]] [-X[n]] [-h string] [-T[tname]] [-Z[n]]\n" +
			"               [name[=value]...]\n" +
			"   Or: typeset [ options ] -f [name...]\n"
	)
	for _, c := range []struct{ src, out, errs string }{
		// Every bad letter of every option word, up to an operand or `--`.
		{"read -q -r -z x; echo st=$?", "st=2\n", "ksh: read: -q: unknown option\nksh: read: -z: unknown option\n" + readUsage},
		{"read -q -u 3 -z x; echo st=$?", "st=2\n", "ksh: read: -q: unknown option\nksh: read: -z: unknown option\n" + readUsage},
		{"read -q x -z; echo st=$?", "st=2\n", "ksh: read: -q: unknown option\n" + readUsage},
		{"read -q -- -z; echo st=$?", "st=2\n", "ksh: read: -q: unknown option\n" + readUsage},
		{"alias -g -s q=v; echo st=$?", "", "alias: -g: unknown option\nalias: -s: unknown option\nUsage: alias [-ptx] [name[=value]...]\n"},
		// `-n` takes the rest of its word, so a signum that is not a number
		// is the bare usage block.
		{"kill -nKILL 99999; echo st=$?", "st=2\n", killUsage},
		{"kill -nZ 99999; echo st=$?", "st=2\n", killUsage},
		// `--version` is the builtin's own line.
		{"umask --version; echo st=$?", "st=2\n", "  version         umask (AT&T Research) 1999-04-07\n"},
		{"umask -S --version; echo st=$?", "st=2\n", "  version         umask (AT&T Research) 1999-04-07\n"},
		{"cd --version; echo st=$?", "st=2\n", "  version         cd (AT&T Research) 1999-06-05\n"},
		// `X` on a line of functions is the bare usage, and it ends the
		// script.
		{"autoload +X; echo st=$?", "", typesetUsage},
		{"typeset -X -f; echo st=$?", "", typesetUsage},
		// A special builtin is a restricted name to `builtin`.
		{"builtin shift 5; echo st=$?", "st=1\n", "builtin: shift: restricted name\nbuiltin: 5: not found\n"},
		{"builtin cd echo; echo st=$?", "st=0\n", ""},
	} {
		out, errs, _ := runKshArgs(t, "-c", c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%s:\n got %q, %q\nwant %q, %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
