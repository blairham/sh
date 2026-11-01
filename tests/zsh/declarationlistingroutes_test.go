// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// runZshListing runs src as a script file under `-f`, keeping the two streams
// apart: several rows below are about a listing writing nothing, and a
// refusal on the other stream must not read as that.
func runZshListing(t *testing.T, src string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	code = driver.MainArgs(zshWriting(&o, &e), []string{"zsh", "-f", zshScriptFileAt(t, src)})
	return o.String(), e.String(), code
}

// TestAPrintMatchingListingThatMatchesNothingWritesNothing — `typeset -p -m`
// whose every pattern matches no parameter writes nothing at 0. Measured
// 2026-10-15 against zsh 5.9.2 under `env -i PATH=/usr/bin:/bin` and `-f`;
// this wrote the whole parameter table, because an empty list of matches
// reached the listing as "no names written" (#6449).
//
// The control rows are what make an empty answer evidence: the same listing
// with a pattern that does match writes its rows, so the route is live.
func TestAPrintMatchingListingThatMatchesNothingWritesNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"one pattern matching nothing", "typeset -p -m 'zzq*'\nprint rc=$?\n", "rc=0\n"},
		{"the letters joined", "typeset -pm 'zzq*'\nprint rc=$?\n", "rc=0\n"},
		{"under declare", "declare -p -m 'zzq*'\nprint rc=$?\n", "rc=0\n"},
		{"in a function", "f() { typeset -pm 'zzq*'; print rc=$?; }\nf\n", "rc=0\n"},
		{"the engine's own names, which a listing never shows", "typeset -p -m '.zsh.*'\nprint rc=$?\n", "rc=0\n"},
		{"an assigning pattern matching nothing", "typeset -pm 'zzq*'=9\nprint rc=$?\n", "rc=0\n"},
		{"control: a second pattern that matches", "zq1=a\ntypeset -pm 'zzq*' 'zq*'\n", "typeset zq1=a\n"},
		{"control: an assigning pattern that matches", "zq1=a\ntypeset -pm 'zq*'=9\n", "typeset zq1=9\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runZshListing(t, tc.src)
			if out != tc.want || errs != "" || code != 0 {
				t.Errorf("wrote %q (stderr %q) at %d, want %q", out, errs, code, tc.want)
			}
		})
	}
}

// TestTopLevelLocalListsAsTypesetDoes — outside every function, zsh's `local`
// is `typeset`, listings included. Measured 2026-10-15 against zsh 5.9.2
// under `env -i PATH=/usr/bin:/bin` and `-f`: `local -p NAME` writes the
// row, a missing name is `no such variable` at 1, and with no operand `local
// -p`, `local` and `local +` write what `typeset -p`, `typeset` and `typeset
// +` write. Each wrote nothing at 0 here, and `local -p .x` was refused as
// not valid in this context (#6450).
//
// Compared as line counts against `typeset`'s own over the same table, and
// the count is required to be non-zero, so a route that writes nothing on
// both sides cannot pass. `RANDOM` moves between reads, which is why the
// whole tables are not compared byte for byte.
func TestTopLevelLocalListsAsTypesetDoes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src, want, wantErr string
		wantCode                 int
	}{
		{"a named row", "zl=v\nlocal -p zl\nprint rc=$?\n", "typeset zl=v\nrc=0\n", "", 0},
		{"a missing name", "local -p nosuch\nprint rc=$?\n", "rc=1\n", ":local:1: no such variable: nosuch\n", 0},
		{"a dotted name is missing, not refused", "local -p .x\nprint rc=$?\n", "rc=1\n", ":local:1: no such variable: .x\n", 0},
		{"an operand carrying a value is a name alone", "x=5\nlocal -p x=7\ntypeset -p x\n", "typeset x=5\ntypeset x=5\n", "", 0},
		{
			"the -p table",
			"a=0 b=0\na=$(local -p | grep -vc RANDOM); b=$(typeset -p | grep -vc RANDOM)\n" +
				"(( a > 0 && a == b )) && print same || print $a $b\n",
			"same\n", "", 0,
		},
		{
			"the -p table is -p's form and not the bare word's",
			"a=0 b=0\na=$(local -p | grep -c .); b=$(local | grep -c .)\n(( a < b )) && print fewer || print $a $b\n",
			"fewer\n", "", 0,
		},
		{
			"the bare table",
			"a=0 b=0\na=$(local | grep -vc -e RANDOM -e '^_='); b=$(typeset | grep -vc -e RANDOM -e '^_=')\n" +
				"(( a > 0 && a == b )) && print same || print $a $b\n",
			"same\n", "", 0,
		},
		{
			"the plus table",
			"a=0 b=0\na=$(local + | grep -c .); b=$(typeset + | grep -c .)\n(( a > 0 && a == b )) && print same || print $a $b\n",
			"same\n", "", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runZshListing(t, tc.src)
			// The script's own path heads a diagnostic, so the wording is
			// compared from the builtin's name on.
			if out != tc.want || !strings.HasSuffix(errs, tc.wantErr) || (errs == "") != (tc.wantErr == "") ||
				code != tc.wantCode {
				t.Errorf("wrote %q (stderr %q) at %d, want %q (stderr %q) at %d",
					out, errs, code, tc.want, tc.wantErr, tc.wantCode)
			}
		})
	}
}

// TestLocalPrintWithNoOperandIsThePrintTableInAFunction — the same `-p` form
// inside a function, where it was the bare word's form: measured 2026-10-15
// on zsh 5.9.2, `f() { local a=1; local -p; }` writes exactly the rows
// `typeset -p` writes there, `typeset a=1` among them (#6450).
func TestLocalPrintWithNoOperandIsThePrintTableInAFunction(t *testing.T) {
	t.Parallel()
	src := "f() {\n  local a=1\n" +
		"  x=0 y=0\n  x=$(local -p | grep -vc RANDOM); y=$(typeset -p | grep -vc RANDOM)\n" +
		"  (( x > 0 && x == y )) && print same || print $x $y\n" +
		"  local -p | grep -x 'typeset a=1'\n}\nf\n"
	out, errs, code := runZshListing(t, src)
	if want := "same\ntypeset a=1\n"; out != want || errs != "" || code != 0 {
		t.Errorf("wrote %q (stderr %q) at %d, want %q", out, errs, code, want)
	}
}

// TestAValuelessExportArrayLetterMakesAnEmptyArray — zsh's `export` takes the
// indexed container letter and it takes effect without a value: an empty
// exported array, converting a standing scalar. Measured 2026-10-15 against
// zsh 5.9.2 from a script file under `env -i PATH=/usr/bin:/bin` and `-f`.
// This reached Semantics.ExportContainerLetterNeedsAValue, which the dialect
// left unanswered on the premise that its `export` took no container letter,
// and refused the line (#6452).
func TestAValuelessExportArrayLetterMakesAnEmptyArray(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"a new name", "export -a b\ntypeset -p b\n", "typeset -ax b=(  )\n"},
		{"over a standing scalar", "x=s\nexport -a x\ntypeset -p x\n", "typeset -ax x=(  )\n"},
		{"over a standing array", "y=(1 2)\nexport -a y\ntypeset -p y\n", "typeset -ax y=( 1 2 )\n"},
		{"control: with a value", "export -a q=(1 2)\ntypeset -p q\n", "typeset -ax q=( 1 2 )\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runZshListing(t, tc.src)
			if out != tc.want || errs != "" || code != 0 {
				t.Errorf("wrote %q (stderr %q) at %d, want %q", out, errs, code, tc.want)
			}
		})
	}
}
