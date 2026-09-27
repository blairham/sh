// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration a deeper function frame reads **past** — see
// interp/privatescope.go for the measured table this suite is written from.
//
// Every row here is paired with the same row written `local`, and that
// pairing is the point rather than tidiness. The mechanism is switched on by
// a *word* and not by an axis, so a suite that only ran the private spelling
// could not tell "the seal works" from "every declaration behaves this way" —
// and the second is what the code would do if the flag were never read. The
// control rows fail loudly if the seal ever escapes the word.

// privateWord is what a dialect does to get the word: register the builtin
// and give it a letter set. Nothing else, which is the claim.
func privateWord(r *Runner) { r.Register("private", PrivateBuiltin()) }

// withPrivate is the semantics a shell with the word holds: `local`'s letters
// on both, so that the two spellings differ in nothing but the word.
func withPrivate(s *Semantics) {
	s.LocalOptions = "aAirx"
	s.PrivateOptions = "aAirx"
}

// runBothWords runs one body twice, once per declaration word, and reports
// what each wrote. The `%s` in src is where the word goes.
func runBothWords(t *testing.T, src string) (private, local string) {
	t.Helper()
	out, errs, _ := declRunWith(t, strings.ReplaceAll(src, "%s", "private"),
		withPrivate, Diagnostics{}, nil, privateWord)
	private = out + errs
	out, errs, _ = declRunWith(t, strings.ReplaceAll(src, "%s", "local"),
		withPrivate, Diagnostics{}, nil, privateWord)
	local = out + errs
	return private, local
}

// TestACalleeReadsPastAPrivateToWhatItDisplaced is the headline row and the
// one the word exists for.
//
// The callee does not read *nothing*: it reads the binding the declaration
// displaced, which is the distinction a name merely hidden would not make.
// The control is the same body under `local`, where the callee reads the
// declaration.
func TestACalleeReadsPastAPrivateToWhatItDisplaced(t *testing.T) {
	t.Parallel()
	private, local := runBothWords(t, `v=9
g() { echo "g=[$v] set=${v+yes}"; }
f() { %s v=1; g; echo "f=[$v]"; }
f
echo "top=[$v]"`)
	if want := "g=[9] set=yes\nf=[1]\ntop=[9]\n"; private != want {
		t.Errorf("private = %q, want %q", private, want)
	}
	if want := "g=[1] set=yes\nf=[1]\ntop=[9]\n"; local != want {
		t.Errorf("local = %q, want %q — the control, which says the seal "+
			"belongs to the word and not to every declaration", local, want)
	}
}

// And with nothing underneath, what the callee reads past the private to is
// the **absence** of the name: `${v+yes}` fires no default there, and the
// declaring call still holds its own value when the callee returns.
func TestACalleeReadsPastAPrivateToNothingWhenNothingWasDisplaced(t *testing.T) {
	t.Parallel()
	private, local := runBothWords(t, `g() { echo "g=[$v] set=${v+yes}"; }
f() { %s v=1; g; echo "f=[$v]"; }
f
echo "top=${v+yes}"`)
	if want := "g=[] set=\nf=[1]\ntop=\n"; private != want {
		t.Errorf("private = %q, want %q", private, want)
	}
	if want := "g=[1] set=yes\nf=[1]\ntop=\n"; local != want {
		t.Errorf("local = %q, want %q", local, want)
	}
}

// The boundary is the **frame** and not the scope, which is what makes a
// subshell and a command substitution inside the declaring call see the
// private while a function it calls does not. Both spellings agree here, and
// that agreement is the row: it says the seal is not a scope rule wearing a
// word's clothes.
func TestWhatRunsAtTheDeclaringFrameSeesAPrivate(t *testing.T) {
	t.Parallel()
	private, local := runBothWords(t, `v=9
f() { %s v=1; ( echo "sub=[$v]" ); echo "cs=[$(echo "$v")]"; }
f`)
	const want = "sub=[1]\ncs=[1]\n"
	if private != want {
		t.Errorf("private = %q, want %q", private, want)
	}
	if local != want {
		t.Errorf("local = %q, want %q", local, want)
	}
}

// A callee may write through the seal, and what it writes stays written to
// the cell the private displaced — the swap-not-copy property, which a seal
// that merely restored what it saved would get wrong in the invisible
// direction.
func TestACalleeWritesThroughTheSealToTheDisplacedCell(t *testing.T) {
	t.Parallel()
	private, _ := runBothWords(t, `v=9
g() { v=7; }
f() { %s v=1; g; echo "f=[$v]"; }
f
echo "top=[$v]"`)
	if want := "f=[1]\ntop=[7]\n"; private != want {
		t.Errorf("private = %q, want %q — the callee's write belongs to the "+
			"displaced cell and the declaration is unmoved", private, want)
	}
}

// And it may not write one the private displaced nothing under: the frame can
// see the name is spoken for and cannot have it, which ends the script.
func TestACalleeMayNotWriteAPrivateThatDisplacedNothing(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `g() { v=7; echo "in-g"; }
f() { private v=1; g; echo "back"; }
f
echo "after"`, withPrivate, Diagnostics{}, nil, privateWord)
	if out != "" {
		t.Errorf("stdout = %q, want nothing: the write is refused and the "+
			"script ends there", out)
	}
	if !strings.Contains(errs, "can't change parameter attribute") {
		t.Errorf("stderr = %q, want the refusal in it", errs)
	}
	if st == 0 {
		t.Errorf("status = %d, want a failure", st)
	}
	// The control, and it is the one that says the refusal is about the
	// *seal* and not about the name: a callee that declares the name first
	// has a binding of its own and writes it at 0.
	out, errs, st = declRunWith(t, `g() { local v=3; v=7; echo "in-g=[$v]"; }
f() { private v=1; g; echo "back=[$v]"; }
f`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "in-g=[7]\nback=[1]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("a callee declaring first = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And a subshell inside such a callee refuses it too, which is the behavioral
// half of the clone tables: a subshell is a clone, so the shield has to be
// copied into it or the `( … )` writes a global the shell outside it cannot
// have. The frame dies on the refusal and the callee carries on, which is the
// row's other half.
func TestASubshellInsideTheCalleeRefusesItToo(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `g() { ( v=7; echo "sub"; ); echo "after the subshell"; }
f() { private v=1; g; echo "f=[$v]"; }
f
echo "top=${v+yes}"`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "after the subshell\nf=[1]\ntop=\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
	if !strings.Contains(errs, "can't change parameter attribute") {
		t.Errorf("stderr = %q, want the refusal in it", errs)
	}
}

// A callee that **unsets** the name the seal exposed takes the private with
// it, and the control is the case where the seal exposed nothing: there the
// callee has no parameter to unset and the declaration is untouched.
//
// The two rows are the whole of why this is keyed on what the seal exposed
// rather than on what the private held. Keyed on the second, they agree on
// every case but the control — which is exactly the shape of a rule that is
// right for the wrong reason.
func TestUnsettingTheDisplacedNameTakesThePrivateWithIt(t *testing.T) {
	t.Parallel()
	out, _, _ := declRunWith(t, `v=9
g() { unset v; }
f() { private v=1; g; echo "f=[$v] set=${v+yes}"; }
f
echo "top=${v+yes}"`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "f=[] set=\ntop=\n"; out != want {
		t.Errorf("with something displaced = %q, want %q", out, want)
	}
	out, _, _ = declRunWith(t, `g() { unset v; }
f() { private v=1; g; echo "f=[$v] set=${v+yes}"; }
f`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "f=[1] set=yes\n"; out != want {
		t.Errorf("with nothing displaced = %q, want %q — the control the rule "+
			"is keyed on", out, want)
	}
}

// `private` over a name the running call has already declared is refused, and
// it is refused whichever word made that name. Reported and survivable, where
// the write refusal above ends the script.
func TestPrivateOverANameThisCallAlreadyDeclaredIsRefused(t *testing.T) {
	t.Parallel()
	for _, first := range []string{"private", "local"} {
		out, errs, _ := declRunWith(t, `f() { `+first+` v=1; private v=6; echo "st=$? v=[$v]"; }
f
echo "after"`, withPrivate, Diagnostics{}, nil, privateWord)
		if want := "st=1 v=[1]\nafter\n"; out != want {
			t.Errorf("%s then private = %q, want %q", first, out, want)
		}
		if !strings.Contains(errs, "can't change scope of existing param") {
			t.Errorf("%s then private: stderr = %q, want the refusal", first, errs)
		}
	}
	// The other direction is taken, which is the control: a declaration may
	// narrow what a private binding is, it may not move a binding that is
	// already this call's.
	out, errs, st := declRunWith(t, `f() { private v=1; local v=6; echo "st=$? v=[$v]"; }
f`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "st=0 v=[6]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("private then local = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The `-p` listing writes no row for a private, and the control beside it is
// the same listing over an ordinary local in the same call: a walk that wrote
// neither would pass the first assertion for the wrong reason.
func TestTheDashPListingWritesNoRowForAPrivate(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `f() { private v=1; local w=2; typeset -p v; echo "st=$?"; typeset -p w; }
f`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "st=0\ndeclare -- w=\"2\"\n"; out != want || errs != "" || st != 0 {
		t.Errorf("listing = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	// And the name is still there to be read, which is what separates this
	// from a name the shell does not have.
	out, _, _ = declRunWith(t, `f() { private v=1; echo "[$v] ${v+yes}"; }
f`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "[1] yes\n"; out != want {
		t.Errorf("the private reads back as %q, want %q", out, want)
	}
}

// A shell that never declares one pays nothing, and this is the row that says
// the switch is really a switch: with the word registered but unused, a
// caller's local reaches its callee exactly as it always did.
//
// It cannot measure the cost — that is not a thing a test asserts — but it can
// measure the thing the cost rests on, which is that the seal does not run.
func TestWithNoPrivateDeclaredNothingIsSealed(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `v=9
g() { echo "g=[$v]"; }
f() { local v=1; g; }
f
echo "top=[$v]"`, withPrivate, Diagnostics{}, nil, privateWord)
	if want := "g=[1]\ntop=[9]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
