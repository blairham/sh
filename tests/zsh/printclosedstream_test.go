// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A write to a stream **something else** closed is reported; one the command
// closed itself is not.
//
// This is the third and last case of one rule, and the rule is worth having in
// one place because each case looks like a different bug from the others:
//
//	fd 1 closed by the command's own redirection	silent, status 0
//	fd 1 closed before the command                 	`write error: bad file
//	                                               	descriptor`, **no builtin
//	                                               	in the location**, status 0
//	a descriptor named with `-uN`, closed          	`bad file number: N`, status 1
//	a descriptor named with `-uN`, open for reading 	`bad mode on fd N`, status 1
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device.
//
// **`echo` and `printf` already had the second case** — they record a failed
// write and the shell says the sentence — and `print` wrote through a stream of
// its own, handled the error itself, and dropped it. That is the whole of the
// defect, and it is why the fix is one call rather than a second sentence: see
// interp.Runner.BuiltinWriteFailed.
func TestAWriteToAStreamSomethingElseClosedIsReported(t *testing.T) {
	dir := t.TempDir()
	const said = "zsh:1: write error: bad file descriptor\n"
	for _, tc := range []struct{ name, src, wantErr string }{
		{"a subshell that closed its own output first", "(exec >&-\nprint foo)\n", "zsh:2: write error: bad file descriptor\n"},
		{"a group's redirection", "{ print foo } >&-\n", said},
		{"a function called with one", "f(){ print foo }\nf >&-\n", "f: write error: bad file descriptor\n"},
		// The neighbors, which have said it all along.
		{"echo, for comparison", "{ echo foo } >&-\n", said},
		{"printf, for comparison", "{ printf '%s\\n' foo } >&-\n", said},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src+"print -r -- \"st $?\"\n")
			if out != "st 0\n" || errs != tc.wantErr || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, "st 0\n", tc.wantErr)
			}
		})
	}
}

// And the command that closed the stream itself is still quiet, which is the
// case this rule turns on: the same closed descriptor, the same failed write,
// and the answer decided by **who wrote the redirection**.
func TestAWriteToAStreamTheCommandClosedItselfIsQuiet(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"on the command", "print foo >&-\n"},
		{"written with the number", "print foo 1>&-\n"},
		{"inside a subshell, on the command", "( print foo >&- )\n"},
		{"inside a function, on the command", "f(){ print foo >&- }\nf\n"},
		{"with another redirection beside it", "print foo 2>&- >&-\n"},
		{"echo, for comparison", "echo foo >&-\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src+"print -r -- \"st $?\"\n")
			if out != "st 0\n" || errs != "" || st != 0 {
				t.Errorf("out %q err %q status %d, want %q and no diagnostic", out, errs, st, "st 0\n")
			}
		})
	}
	// A close of something that is not this command's output says nothing
	// either way, which is the control that keeps the rule about fd 1.
	for _, tc := range []struct{ name, src, want string }{
		{"a closed fd 2", "print foo 2>&-\n", "foo\nst 0\n"},
		{"a closed fd 3", "print foo 3>&-\n", "foo\nst 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src+"print -r -- \"st $?\"\n")
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("out %q err %q status %d, want %q and no diagnostic", out, errs, st, tc.want)
			}
		})
	}
}
