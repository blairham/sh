// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// `fc` against bash's own list, which it never read until #4009.
//
// Every expected string is a transcript. The script below each `src` was run
// through bash 5.3.20 on 2026-09-21, `env -i` with a scratch `HOME`,
// `HISTFILE=/dev/null` and `--norc --noprofile`, with the two streams
// captured separately — which is half the point, since `fc -s` says what it
// is about to run on standard **error** and runs it on standard output.
//
// `set -o history` opens every one of them, because a bash script keeps no
// list without it; the `fc` line itself joins the list like any other
// command, which is why the default range stops one short of it.

// fcListRun runs src with the list turned on.
func fcListRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	return historyRun(t, "set -o history\n"+src)
}

func TestFcListsBashsOwnHistory(t *testing.T) {
	t.Parallel()
	const seed = "echo one\necho two\necho three\n"
	for _, c := range []struct{ src, want string }{
		{"fc -l\n", "one\ntwo\nthree\n1\t echo one\n2\t echo two\n3\t echo three\n"},
		{"fc -nl\n", "one\ntwo\nthree\n\t echo one\n\t echo two\n\t echo three\n"},
		{"fc -lr\n", "one\ntwo\nthree\n3\t echo three\n2\t echo two\n1\t echo one\n"},
		// The operand the option reader used to eat.
		{"fc -l -2\n", "one\ntwo\nthree\n2\t echo two\n3\t echo three\n"},
		{"fc -l 1 2\n", "one\ntwo\nthree\n1\t echo one\n2\t echo two\n"},
	} {
		out, errs, code := fcListRun(t, seed+c.src)
		if out != c.want || errs != "" || code != 0 {
			t.Errorf("%q:\n out %q\n err %q\n status %d\nwant\n out %q\n err %q\n status 0",
				c.src, out, errs, code, c.want, "")
		}
	}
}

// `-s` runs the previous command again, says which one on standard error, and
// takes that command's place in the list — `history` afterwards is the whole
// of the third claim, and it is the one nothing else here would notice.
func TestFcRerunsAndTakesItsOwnPlaceInTheList(t *testing.T) {
	t.Parallel()
	out, errs, code := fcListRun(t, "echo one\nfc -s\nhistory\n")
	wantOut := "one\none\n    1  echo one\n    2  echo one\n    3  history\n"
	wantErr := "echo one\n"
	if out != wantOut || errs != wantErr || code != 0 {
		t.Errorf("ran\n out %q\n err %q\n status %d\nwant\n out %q\n err %q\n status 0",
			out, errs, code, wantOut, wantErr)
	}
}

// `pat=rep` replaces every occurrence and not the first, measured on a word
// holding two of them.
func TestFcRerunSubstitutesEveryOccurrence(t *testing.T) {
	t.Parallel()
	out, errs, code := fcListRun(t, "echo aa ab ac\nfc -s a=x\n")
	wantOut := "aa ab ac\nxx xb xc\n"
	wantErr := "echo xx xb xc\n"
	if out != wantOut || errs != wantErr || code != 0 {
		t.Errorf("ran\n out %q\n err %q\n status %d\nwant\n out %q\n err %q\n status 0",
			out, errs, code, wantOut, wantErr)
	}
}

// An event that resolves to the `fc` call itself is refused, and the two
// roads word it differently. Measured together, on the same list and the same
// operand, which is why neither wording may stand in for the other.
func TestFcRefusesAnEventThatWouldBeItself(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"fc -0\n", "S: line 3: fc: history specification out of range\n"},
		{"fc -s -0\n", "S: line 3: fc: no command found\n"},
	} {
		out, errs, code := fcListRun(t, "echo one\n"+c.src)
		if out != "one\n" || errs != c.want || code != 1 {
			t.Errorf("%q:\n out %q\n err %q\n status %d\nwant\n out %q\n err %q\n status 1",
				c.src, out, errs, code, "one\n", c.want)
		}
	}
}
