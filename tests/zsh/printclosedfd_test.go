// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `bad mode on fd N` belongs to a descriptor the command **named**, and not to
// `print`'s own standard output.
//
// EBADF has two causes and this shell had one sentence for both: a descriptor
// opened for reading, which is what the sentence was measured for, and
// `print`'s own stdout being closed or opened for reading by the command's
// redirections — where the reference says nothing at all and leaves 0.
// `print foo >&-` was `print:1: bad mode on fd 1` at status 1 here, which is
// what `A04redirect.ztst` stops on under `'>&-' redirection`.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device.
func TestPrintToItsOwnClosedOutputIsQuiet(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"closed by the command's own redirection", "print foo >&-\n"},
		{"the same written with the number", "print foo 1>&-\n"},
		{"opened for reading instead", "print x > f\nprint foo 1<f\n"},
		{"aimed at a descriptor open for reading", "print x > f\nexec 3<f\nprint foo 1>&3\n"},
		{"inside a function", "f(){ print foo >&- }\nf\n"},
		{"inside a subshell", "( print foo >&- )\n"},
		{"with another redirection beside it", "print foo 2>&- >&-\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src+"print -r -- \"st $?\"\n")
			if out != "st 0\n" || errs != "" || st != 0 {
				t.Errorf("out %q err %q status %d, want %q and no diagnostic", out, errs, st, "st 0\n")
			}
		})
	}
	// The neighbors, which never said it and must not start.
	for _, tc := range []struct{ name, src, want string }{
		{"echo", "echo foo >&-\nprint -r -- \"st $?\"\n", "st 0\n"},
		{"printf", "printf '%s\\n' foo >&-\nprint -r -- \"st $?\"\n", "st 0\n"},
		// And a redirection that is not this builtin's output at all.
		{"a closed fd 2", "print foo 2>&-\nprint -r -- \"st $?\"\n", "foo\nst 0\n"},
		{"a closed fd 3", "print foo 3>&-\nprint -r -- \"st $?\"\n", "foo\nst 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("out %q err %q status %d, want %q and no diagnostic", out, errs, st, tc.want)
			}
		})
	}
}

// **A descriptor the command named still answers**, which is the other half and
// is what keeps the change from being "stop complaining".
func TestPrintToANamedDescriptorStillComplains(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a number closed by this command", "print -u3 foo 3>&-\n",
			"zsh:print:1: bad file number: 3\n",
		},
		{
			"a number closed earlier", "exec 3>&-\nprint -u3 foo\n",
			"zsh:print:2: bad file number: 3\n",
		},
		{
			"and a number nothing was opened at", "print -u9 foo\n",
			"zsh:print:1: bad file number: 9\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src+"print -r -- \"st $?\"\n")
			if out != "st 1\n" || errs != tc.want || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, "st 1\n", tc.want)
			}
		})
	}
}
