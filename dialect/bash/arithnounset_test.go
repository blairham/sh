// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// `set -u` reaches arithmetic here: a name an expression reads and nothing
// ever set is the option's refusal rather than the zero arithmetic otherwise
// gives it.
//
// Measured 2026-09-18 against bash 5.3.20, `env -i HOME=… PATH=/usr/bin:/bin
// LC_ALL=C` from a script file, with `b` never set — every row is `b: unbound
// variable` and the script stops. The last row is the control that makes this
// about being unset and not about being empty (#3574).
func TestTheOptionReachesEveryArithmeticReadOfAName(t *testing.T) {
	for _, src := range []string{
		`set -u; : $((b)); echo OK`,
		`set -u; echo $((b)); echo OK`,
		`set -u; a=1; : $((a+b)); echo OK`,
		`set -u; a=(x); : "${a[b]}"; echo OK`,
		`set -u; a=(x); echo ${#a[b]}; echo OK`,
		`set -u; (( b )); echo OK`,
		`set -u; let "x=b"; echo OK`,
		`set -u; for ((i=0;i<b;i++)); do :; done; echo OK`,
		`set -u; typeset -i n; n=b; echo OK`,
	} {
		out, st := answersRun(t, src)
		if st == 0 || strings.Contains(out, "OK") {
			t.Errorf("%s = %q status %d, want a refusal and no OK", src, out, st)
		}
		if want := "b: unbound variable"; !strings.Contains(out, want) {
			t.Errorf("%s = %q, want it to name %q", src, out, want)
		}
	}
	// Set and empty is zero and quiet, which is the line this is drawn on.
	if out, st := answersRun(t, `set -u; b=; printf "[%s]" "$((b))"`); out != "[0]" || st != 0 {
		t.Errorf("an empty name = %q status %d, want [0] at 0", out, st)
	}
	// And with the option off, the same expression is zero in every one of
	// them — so what changed is the option's reach and not arithmetic.
	if out, st := answersRun(t, `printf "[%s]" "$((b))"`); out != "[0]" || st != 0 {
		t.Errorf("nounset off = %q status %d, want [0] at 0", out, st)
	}
}

// The refusal is the shell's own here, so it stops wherever the expression was
// written — including the two constructs that survive an *ordinary* arithmetic
// failure in this dialect, which is the control that says the fatality belongs
// to the refusal and not to the arithmetic.
//
// Measured 2026-09-18, bash 5.3.20, each row followed by `echo "st=$?"; echo
// OK`: `(( b ))` and `let "x=b"` print neither, where `(( 1+ ))` prints `st=1`
// and `OK` and `let "x=1+"` the same (#3574).
func TestTheArithmeticNounsetRefusalIsFatalWhereverItIsWritten(t *testing.T) {
	for _, src := range []string{
		`set -u; (( b )); echo OK`,
		`set -u; (( b )) || echo caught; echo OK`,
		`set -u; let "x=b"; echo OK`,
		`set -u; x=$((b)) || echo caught; echo OK`,
		`set -u; for ((i=0;i<b;i++)); do :; done; echo OK`,
	} {
		if out, st := answersRun(t, src); st == 0 || strings.Contains(out, "OK") ||
			strings.Contains(out, "caught") {
			t.Errorf("%s = %q status %d, want the shell stopped", src, out, st)
		}
	}
	// The same two constructs failing for an ordinary reason do not stop it.
	for _, src := range []string{
		`set -u; (( 1+ )); echo OK`,
		`set -u; let "x=1+"; echo OK`,
	} {
		if out, _ := answersRun(t, src); !strings.Contains(out, "OK") {
			t.Errorf("%s = %q, want the line to run on", src, out)
		}
	}
}

// And it is not worded as the construct's failure, which is the half a reader
// would get wrong: this shell names `((` and `let` for an expression it could
// not evaluate and names neither here.
//
// Measured 2026-09-18, bash 5.3.20: `set -u; (( b ))` is `b: unbound
// variable` where `(( 1+ ))` is `((: 1+: arithmetic syntax error …`, and
// `set -u; let "x=b"` is bare where `let "x=1+"` is `let: x=1+: …` (#3574).
func TestTheArithmeticNounsetRefusalIsNotTheConstructsFailure(t *testing.T) {
	for _, c := range []struct{ src, absent string }{
		{`set -u; (( b ))`, "((:"},
		{`set -u; for ((i=0;i<b;i++)); do :; done`, "((:"},
		{`set -u; let "x=b"`, "let:"},
	} {
		out, _ := answersRun(t, c.src)
		if strings.Contains(out, c.absent) {
			t.Errorf("%s = %q, want no %q in front of it", c.src, out, c.absent)
		}
	}
	// The controls: the same constructs failing as constructs keep it.
	for _, c := range []struct{ src, want string }{
		{`set -u; (( 1+ ))`, "((:"},
		{`set -u; let "x=1+"`, "let:"},
	} {
		if out, _ := answersRun(t, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s = %q, want %q in front of it", c.src, out, c.want)
		}
	}
}

// A reference aimed at an element is refused about the *subscript's* name and
// not about the reference, which is the defect the silent seam was hiding: with
// the subscript's own refusal never firing, the reference was the only thing
// left to be unbound about.
//
// Measured 2026-09-18, bash 5.3.20 from a script file. The literal subscript
// is the control — there is no name in it, so the reference is what is unset
// (#3574, #3125).
func TestAReferenceToAnElementIsRefusedAboutItsSubscript(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -u; declare -n r=a[b]; : "$r"`, "b: unbound variable"},
		{`set -u; a=(x); declare -n r=a[b]; : "$r"`, "b: unbound variable"},
		{`set -u; declare -n r=a[1]; : "$r"`, "r: unbound variable"},
		{`set -u; declare -n r=v; : "$r"`, "r: unbound variable"},
	} {
		out, st := answersRun(t, c.src)
		if st == 0 || !strings.Contains(out, c.want) {
			t.Errorf("%s = %q status %d, want %q", c.src, out, st, c.want)
		}
	}
}

// The two axes, so a preset moving off either is caught here rather than in a
// row somewhere.
func TestTheArithmeticNounsetAxes(t *testing.T) {
	s := bash.Semantics()
	if got := s.ArithUnsetNameUnderNounsetIsRefused; got != interp.Yes {
		t.Errorf("ArithUnsetNameUnderNounsetIsRefused = %v, want yes", got)
	}
	if got := s.ArithNounsetRefusalIsFatal; got != interp.Yes {
		t.Errorf("ArithNounsetRefusalIsFatal = %v, want yes", got)
	}
}
