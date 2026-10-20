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

// `-R` is `-r` plus the end of this shell's option parsing, and it is not
// zsh's letter of the same name. Measured 2026-09-05, ksh93u+: the rest of the
// bundle goes unread except for an `n`, and of the words after it only a bare
// `-n` is still an option — neither `-` nor `--` ends anything there, and a
// later `-e` prints as a word instead of putting the expansion back.
func TestPrintCapitalRStopsTheOptionParser(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print -R a b`, "a b\n"},
		{`print -R 'a\tb'`, `a\tb` + "\n"},
		{`print -R -e 'a\tb'`, `-e a\tb` + "\n"},
		{`print -R -en 'a\tb'`, `-en a\tb` + "\n"},
		{`print -R -r a`, "-r a\n"},
		{`print -R -x a`, "-x a\n"},
		{`print -R -f x a`, "-f x a\n"},
		{`print -R -- -n a`, "-- -n a\n"},
		{`print -R - -n a`, "- -n a\n"},
		{`print -R`, "\n"},
		{`print -R -`, "-\n"},
		// One `-n` is read and the next is an operand.
		{`print -R -n a b`, "a b"},
		{`print -R -n -n a`, "-n a"},
		{`print -R -n`, ""},
		// In the bundle only `n` still counts; the letters that would be
		// options anywhere else are not read at all.
		{`print -Rn a b`, "a b"},
		{`print -Rnz a b`, "a b"},
		{`print -Rzn a b`, "a b"},
		{`print -Rz a b`, "a b\n"},
		{`print -Rzq a b`, "a b\n"},
		{`print -Re a`, "a\n"},
		{`print -Rf '%s' a`, "%s a\n"},
		{`print -Rl a b`, "a b\n"},
		// Before an `R` the letters are read as usual.
		{`print -eR a`, "a\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A third dash makes the word an operand, and a long option is refused as
// the whole word (#3163). Measured 2026-09-16 against ksh93u+ 2012-08-01
// under `env -i` from a script file.
//
// Two separate things wear one shape. Option parsing ends at a bare `--` and
// a third dash puts the word past it, so `print "--- a title ---"` prints —
// this shell read the first two characters and refused it at 2 with the title
// missing. And where the word genuinely is a long option, the refusal quotes
// the word rather than the `--`, so a script can tell which one was refused.
func TestPrintReadsALineOfDashesAsAWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print ---`, "---\n"},
		{`print ----`, "----\n"},
		{`print '--- a title ---'`, "--- a title ---\n"},
		// The dashes end the options exactly as any other operand does, and
		// a letter in front of them is still read.
		{`print -n --- x`, "--- x"},
		{`print --- -n x`, "--- -n x\n"},
		{`print -r ---`, "---\n"},
		// The prefix and not the whole word: a third dash is enough,
		// whatever follows it.
		{`print ---x`, "---x\n"},
		{`print ---=`, "---=\n"},
		// The two shorter spellings are unchanged: `--` ends the options and
		// `-` is an operand.
		{`print -- ---`, "---\n"},
		{`print --`, "\n"},
		{`print -`, "\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
	for _, tc := range []struct{ src, named string }{
		{`print --x`, "--x"},
		{`print --after`, "--after"},
		{`print --1`, "--1"},
		// The name is the word cut at its first `=`, and an empty name
		// falls back to the single dash the reference writes there.
		{`print --x=1`, "--x"},
		{`print --=`, "-"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if st != 2 || !strings.Contains(out, "print: "+tc.named+": unknown option") ||
			!strings.Contains(out, "Usage: print [ options ] [string ...]") {
			t.Errorf("%s: out %q status %d, want %q named and the long usage at 2", tc.src, out, st, tc.named)
		}
	}
}
