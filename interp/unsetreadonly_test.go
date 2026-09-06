// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// runUnsetReadonly runs src with this axis answered and nothing else moved, so
// what changes between two runs is the axis and not the setup.
func runUnsetReadonly(t *testing.T, src string, fatal, statusIsOne Answer, d Diagnostics) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := permissive()
		s.UnsetReadonlyFatal = fatal
		s.FatalErrorStatusIsOne = statusIsOne
		r.Semantics, r.Diagnostics = &s, &d
	})
}

// `unset` of a readonly name is refused, says so, and leaves the value
// standing — in every shell in the panel, which makes all three of those the
// core answer rather than anything a dialect chooses.
//
// Measured 2026-09-05 against bash 5.3.15, bash 3.2.57, that 5.3.15 build
// named `sh`, dash, ksh93u+ 2012-08-01 and zsh 5.9.2, on the snippet this test
// runs. Ours deleted the name, recorded the removal and reported success,
// which is #792.
//
// Asserted on the whole of what the shell wrote rather than on the status: the
// value surviving and the complaint are two thirds of the answer, and a shell
// that reported 1 and deleted the name anyway would pass a status assertion.
func TestUnsetOfAReadonlyNameIsRefused(t *testing.T) {
	const src = `readonly x=1; unset x; echo "st=$? [${x-gone}]"; echo after`
	const complaint = "sh: unset: x: cannot unset: readonly variable\n"

	out, st := runUnsetReadonly(t, src, No, Yes, Diagnostics{})
	if want := complaint + "st=1 [1]\nafter\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want the 0 of the `echo` that carried on", st)
	}
}

// The name that was refused keeps its value, and the refusal is that name's
// rather than the builtin's: the operands after it are still removed.
//
// The pair is what makes it a *refusal* rather than a failure — a builtin that
// gave up at the first bad operand would leave `y` standing too, and the three
// shells that carry on all remove it.
func TestARefusedNameDoesNotStopTheRest(t *testing.T) {
	const src = `readonly x=1; y=2; unset x y; echo "st=$? [${x-gone}][${y-gone}]"; echo after`
	const complaint = "sh: unset: x: cannot unset: readonly variable\n"

	out, _ := runUnsetReadonly(t, src, No, Yes, Diagnostics{})
	if want := complaint + "st=1 [1][gone]\nafter\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
}

// The attribute is what is refused and not the value. `readonly y` with
// nothing assigned leaves a name that is unset and unremovable, and every
// shell in the panel complains about removing it — so the `gone` below is the
// name never having had a value rather than the `unset` succeeding.
func TestAReadonlyNameWithNoValueIsRefusedToo(t *testing.T) {
	const src = `readonly y; unset y; echo "st=$? [${y-gone}]"; echo after`
	const complaint = "sh: unset: y: cannot unset: readonly variable\n"

	out, _ := runUnsetReadonly(t, src, No, Yes, Diagnostics{})
	if want := complaint + "st=1 [gone]\nafter\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
}

// A subscripted operand is refused by the variable the subscript indexes, and
// the complaint names the *base*: `unset a[0]` against a readonly `a` says `a`
// and not `a[0]` in both shells with arrays that get this far. So the refusal
// stands ahead of the subscript rather than inside the element path, and the
// subscript is never evaluated.
func TestASubscriptedOperandIsRefusedByItsBase(t *testing.T) {
	const src = `readonly a=1; unset a[0]; echo "st=$? [${a-gone}]"; echo after`
	const complaint = "sh: unset: a: cannot unset: readonly variable\n"

	out, _ := runUnsetReadonly(t, src, No, Yes, Diagnostics{})
	if want := complaint + "st=1 [1]\nafter\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
}

// What follows the refusal is the axis, and it is asserted on what came
// *after* rather than on the status alone: a shell that stopped and a shell
// that carried on can report the same number.
func TestUnsetOfAReadonlyNameEndsTheScriptWhereTheAxisSaysSo(t *testing.T) {
	const src = `readonly x=1; unset x; echo "st=$?"; echo after`
	const complaint = "sh: unset: x: cannot unset: readonly variable\n"

	out, st := runUnsetReadonly(t, src, Yes, Yes, Diagnostics{})
	if out != complaint {
		t.Errorf("wrote %q, want the complaint alone — the script should have stopped", out)
	}
	if st != 1 {
		t.Errorf("status %d, want the 1 this vector's fatal errors carry", st)
	}

	out, st = runUnsetReadonly(t, src, No, Yes, Diagnostics{})
	if want := complaint + "st=1\nafter\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want the 0 of the `echo` that carried on", st)
	}
}

// The status of the shell that stops is the vector's fatal status and not one
// of this error's own, which is why the axis is a single Answer. Five of the
// six panel shells stop or report at 1 and dash exits 2, and that split is
// FatalErrorStatusIsOne's everywhere else too.
func TestTheStoppedShellTakesTheVectorsFatalStatusForThisToo(t *testing.T) {
	const src = `readonly x=1; unset x; echo after`
	if _, st := runUnsetReadonly(t, src, Yes, Yes, Diagnostics{}); st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	if _, st := runUnsetReadonly(t, src, Yes, No, Diagnostics{}); st != 2 {
		t.Errorf("status %d, want the 2 the other answer gives", st)
	}
}

// The wording is the dialect's, and no two in the panel agree. Whole lines,
// because the sentence and the location are one answer: the dialect that puts
// a builtin's name in the location must not put this one there, and a
// substring assertion could not see the difference.
func TestTheWordingIsTheDialects(t *testing.T) {
	const src = `readonly x=1; unset x; echo after`
	for _, c := range []struct{ name, wording, want string }{
		{
			// Empty takes the default, which is what the dialect the default
			// was measured from wants.
			"the default names the builtin and the trouble",
			"",
			"sh: unset: x: cannot unset: readonly variable\n",
		},
		{
			"a shorter sentence, still naming the builtin",
			"unset: %s: is read only",
			"sh: unset: x: is read only\n",
		},
		{
			// The one member of the panel that calls it a warning outright,
			// and carries on afterwards — the two are its answer together.
			"a warning rather than an error",
			"unset: warning: %s: is read only",
			"sh: unset: warning: x: is read only\n",
		},
		{
			// The dialect that words this exactly as it words a refused
			// assignment, and names no builtin at all.
			"no builtin named anywhere in it",
			"read-only variable: %s",
			"sh: read-only variable: x\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runUnsetReadonly(t, src, Yes, Yes, Diagnostics{UnsetReadonly: c.wording})
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// The builtin is named in the sentence by three of the four dialects, so it
// must not be named in the *location* by the one that puts every other
// builtin's name there. Measured: that shell writes `zsh:1: read-only
// variable: x` here and `zsh:unset:1: 1x: invalid parameter name` for a bad
// name, out of the same builtin.
func TestTheLocationDoesNotNameTheBuiltin(t *testing.T) {
	const src = `readonly x=1; unset x; echo after`
	d := Diagnostics{UnsetReadonly: "read-only variable: %s", NamesBuiltinInLocation: true}
	out, _ := runUnsetReadonly(t, src, Yes, Yes, d)
	if want := "sh: read-only variable: x\n"; out != want {
		t.Errorf("wrote %q, want %q — the builtin belongs in neither half here", out, want)
	}
}

// POSIX mode moves two axes now, and it has to remember them apart.
//
// The pair that makes the difference visible is a vector where they disagree,
// which is not a hypothetical: zsh carries on past a failed redirection on a
// special builtin and stops on this, and ksh93 is the reverse. A single saved
// answer would put whichever one was remembered back over both on the way out,
// and no dialect in the tree would notice — bash is the only shell that
// reaches SetPosixMode from a script, and bash starts with both axes No.
func TestLeavingPosixModeRestoresBothAxesSeparately(t *testing.T) {
	s := permissive()
	s.RedirectErrorOnSpecialBuiltinFatal = No
	s.UnsetReadonlyFatal = Yes
	r := newTestRunner(t, &Runner{Semantics: &s})

	r.SetPosixMode(true)
	if got := r.Semantics.RedirectErrorOnSpecialBuiltinFatal; got != Yes {
		t.Errorf("redirection axis in posix mode = %v, want %v", got, Yes)
	}
	if got := r.Semantics.UnsetReadonlyFatal; got != Yes {
		t.Errorf("unset axis in posix mode = %v, want %v", got, Yes)
	}

	r.SetPosixMode(false)
	if got := r.Semantics.RedirectErrorOnSpecialBuiltinFatal; got != No {
		t.Errorf("redirection axis after leaving = %v, want the vector's %v", got, No)
	}
	if got := r.Semantics.UnsetReadonlyFatal; got != Yes {
		t.Errorf("unset axis after leaving = %v, want the vector's %v — "+
			"one saved answer cannot put back two that disagree", got, Yes)
	}
}
