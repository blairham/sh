// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `print` joins its operands with single spaces, expands echo's escapes, and
// ends with a newline — and `-r` withholds the expansion, `-n` the newline.
// Measured 2026-09-04, ksh93u+.
func TestPrintExpandsAndJoins(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print hello world`, "hello world\n"},
		{`print -n abc`, "abc"},
		{`print 'a\tb\nc'`, "a\tb\nc\n"},
		{`print -r 'a\tb'`, `a\tb` + "\n"},
		// In one bundle the later letter wins, which reading them in order does.
		{`print -re 'a\tb'`, "a\tb\n"},
		{`print`, "\n"},
		// The measured escape set: \E is the escape character, \0 takes up to
		// three octal digits, and \x is not an escape here.
		{`print '\a\b\E\061\x41'`, "\a\b\x1b1\\x41\n"},
		{`print '\0101\01012'`, "AA2\n"},
		{`print 'end\\'`, "end\\\n"},
		// \c stops the whole command's output, the newline included.
		{`print 'ab\c def'; print after`, "abafter\n"},
		// `--` ends the options, and so does a lone `-`.
		{`print -- -n; print - -n`, "-n\n-n\n"},
		// `-f` hands everything to printf: format reused, no newline added.
		{`print -f '%s|' a b`, "a|b|"},
		// `-s` aims at a history this shell does not keep: consumed, 0.
		{`print -s hist; echo st=$?`, "st=0\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
}

// `-u` aims the output at a descriptor: 2 is the diagnostic stream, one a
// redirection opened is reachable by its number, and a number nothing
// writable holds — or a word that is no number — is the measured refusal.
func TestPrintUAimsAtADescriptor(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`print -u2 err >/dev/null; print -u 2 err2 >/dev/null; exec 3>f3; print -u3 hi; exec 3>&-; read line < f3; print "$line"`)
	if st != 0 || out != "err\nerr2\nhi\n" {
		t.Errorf("out %q status %d, want both stderr forms and the exec'd descriptor", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `print -u9 x`)
	if st != 1 || !strings.Contains(out, "print: bad file unit number [Bad file descriptor]") {
		t.Errorf("out %q status %d, want the measured refusal at 1", out, st)
	}
}

// The refusals: an unknown letter is `unknown option` with the usage after it
// at 2; `-p` names the coprocess this grammar cannot start, measured as its
// own sentence at 1; `-v` and `-C` are ksh93's and not implemented here.
func TestPrintRefusals(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `print -z foo`)
	if st != 2 || !strings.Contains(out, "print: -z: unknown option") ||
		!strings.Contains(out, "Usage: print [-enprsvC] [-f format] [-u fd] [string ...]") {
		t.Errorf("out %q status %d, want the complaint and the usage at 2", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `print -p x`)
	if st != 1 || !strings.Contains(out, "print: no query process [Bad file descriptor]") {
		t.Errorf("out %q status %d, want the coprocess refusal at 1", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `print -v x`)
	if st != 2 || !strings.Contains(out, "print: -v is not implemented yet") {
		t.Errorf("out %q status %d, want the not-implemented refusal at 2", out, st)
	}
}
