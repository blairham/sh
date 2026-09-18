// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `set -u` reaches arithmetic here too: a name an expression reads and nothing
// ever set is `b: parameter not set` rather than a zero.
//
// Measured 2026-09-18 against zsh 5.9.2, `env -i HOME=… PATH=/usr/bin:/bin
// LC_ALL=C` from a script file, with `b` never set. The empty row is the
// control that makes this about being unset (#3574).
func TestTheOptionReachesEveryArithmeticReadOfAName(t *testing.T) {
	for _, src := range []string{
		`set -u; : $((b)); echo OK`,
		`set -u; a=1; : $((a+b)); echo OK`,
		`set -u; a=(x); : "${a[b]}"; echo OK`,
		`set -u; for ((i=0;i<b;i++)); do :; done; echo OK`,
	} {
		out, st := answersRun(t, src)
		if st == 0 || strings.Contains(out, "OK") {
			t.Errorf("%s = %q status %d, want a refusal and no OK", src, out, st)
		}
		if want := "b: parameter not set"; !strings.Contains(out, want) {
			t.Errorf("%s = %q, want it to name %q", src, out, want)
		}
	}
	if out, st := answersRun(t, `set -u; b=; printf "[%s]" "$((b))"`); out != "[0]" || st != 0 {
		t.Errorf("an empty name = %q status %d, want [0] at 0", out, st)
	}
}

// And here the refusal is the *expression's* failure rather than the shell's,
// so the construct holding it answers — which is what makes the fatality an
// axis of its own instead of a consequence of the refusal.
//
// Measured 2026-09-18, zsh 5.9.2: `set -u; (( b ))` leaves 2 and the line runs
// on, `|| echo caught` catches it, and `let "x=b"` leaves 1 and runs on —
// where the same refusal inside a word expansion stops the shell, as any
// failed expression there does (#3574).
func TestTheArithmeticNounsetRefusalIsTheExpressionsFailure(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
		st   int
	}{
		{`set -u; (( b )); printf "[%s]" "$?"`, "[2]", 0},
		{`set -u; (( b )) || printf caught`, "caught", 0},
		{`set -u; let "x=b"; printf "[%s]" "$?"`, "[1]", 0},
	} {
		out, st := answersRun(t, c.src)
		if st != c.st || !strings.Contains(out, c.want) {
			t.Errorf("%s = %q status %d, want %q at %d", c.src, out, st, c.want, c.st)
		}
	}
	// The word route is the contrast, and it stops the shell.
	if out, st := answersRun(t, `set -u; x=$((b)); echo OK`); st == 0 || strings.Contains(out, "OK") {
		t.Errorf("a word expansion = %q status %d, want the shell stopped", out, st)
	}
}

// The two axes.
func TestTheArithmeticNounsetAxes(t *testing.T) {
	s := zsh.Semantics()
	if got := s.ArithUnsetNameUnderNounsetIsRefused; got != interp.Yes {
		t.Errorf("ArithUnsetNameUnderNounsetIsRefused = %v, want yes", got)
	}
	if got := s.ArithNounsetRefusalIsFatal; got != interp.No {
		t.Errorf("ArithNounsetRefusalIsFatal = %v, want no", got)
	}
}
