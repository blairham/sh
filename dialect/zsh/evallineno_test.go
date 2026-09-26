// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `unsetopt evallineno` stops text handed to `eval` from being a place of its
// own: `$LINENO` stays at the line the `eval` is on for the whole body.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26, over
// a script file whose `eval` is on line 2 and whose text is two lines. The
// option *on* is the default and the control, and it already agreed before
// this was wired, which is what keeps the change narrow (#4551).
func TestEvalLinenoDecidesWhetherEvalTextHasItsOwnLines(t *testing.T) {
	for _, tc := range []struct{ set, want string }{
		{"setopt evallineno", "1\n2\n"},
		{"unsetopt evallineno", "2\n2\n"},
	} {
		src := tc.set + "\neval $'print $LINENO\\nprint $LINENO'\n"
		out, st := runZsh(t, t.TempDir(), src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q", tc.set, out, st, tc.want)
		}
	}
}

// The option names one fact with two surfaces, which is what the vendor
// manual says: `$LINENO`, the `%i` prompt escape and the `%N` escape that
// writes `(eval)` in place of the script's name. So the diagnostic a command
// inside the text raises moves with the parameter.
func TestEvalLinenoAlsoDecidesWhatADiagnosticInTheTextIsCalled(t *testing.T) {
	for _, tc := range []struct{ set, want string }{
		{"setopt evallineno", "(eval):2: command not found: nosuchcommand_xyz\n"},
		{"unsetopt evallineno", "zsh:2: command not found: nosuchcommand_xyz\n"},
	} {
		src := tc.set + "\neval $'true\\nnosuchcommand_xyz'\n"
		_, _, errs := runZshSplit(t, t.TempDir(), src)
		if errs != tc.want {
			t.Errorf("%s: stderr %q, want %q", tc.set, errs, tc.want)
		}
	}
}

// And the state is reported back on all three surfaces a script reads, in
// both directions — the half that was already right and must stay so.
func TestEvalLinenoIsReportedBackInBothStates(t *testing.T) {
	const src = `[[ -o evallineno ]] && print a=on || print a=off
print b=${options[evallineno]}
unsetopt evallineno
[[ -o evallineno ]] && print c=on || print c=off
print d=${options[evallineno]}
setopt
`
	out, st := runZsh(t, t.TempDir(), src)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "a=on\nb=on\nc=off\nd=off\n"; !strings.HasPrefix(out, want) {
		t.Errorf("out %q, want it to start %q", out, want)
	}
	// The listing prints the spellings that deviate from the mode's
	// defaults, and the printed spelling of a name that defaults on is the
	// negative one.
	if !strings.Contains(out, "noevallineno\n") {
		t.Errorf("out %q, want noevallineno in the deviation listing", out)
	}
}
