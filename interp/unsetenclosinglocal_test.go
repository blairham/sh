// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `unset` of a name a *calling* function made local: take the binding away
// so that what it displaced answers from here on, or leave the name unset
// where it stands until the call that owns it returns —
// Semantics.UnsetRemovesAnEnclosingLocal. Tests name the axis, never shells.

// takesTheBinding is the answer that removes a caller's local outright.
func takesTheBinding(s *Semantics) { s.UnsetRemovesAnEnclosingLocal = Yes }

// keepsTheBinding is the other answer: the name is unset and the scope that
// declared it still holds its copy.
func keepsTheBinding(s *Semantics) { s.UnsetRemovesAnEnclosingLocal = No }

// The axis itself, on the three readings one call can produce: what the
// callee sees, what the owning call sees after the callee returns, and what
// the script sees once the owning call has returned too.
func TestUnsetOfACallersLocalCanTakeTheBindingAway(t *testing.T) {
	const src = `v=GLOBAL
g() { unset v; echo "g=[${v-UNSET}]"; }
f() { local v=L; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`
	out, errs, st := declRun(t, src, takesTheBinding, Diagnostics{})
	want := "g=[GLOBAL]\nf=[GLOBAL]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("taking the binding = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, errs, st = declRun(t, src, keepsTheBinding, Diagnostics{})
	want = "g=[UNSET]\nf=[UNSET]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("keeping the binding = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The control that keeps this from being "what `unset` does to a local": at
// the *running* scope both answers agree, so a rule that reached past the
// declaration it was written beside would fail here rather than pass by
// accident.
func TestUnsetOfTheRunningScopesOwnLocalIsNotTheAxis(t *testing.T) {
	const src = `v=GLOBAL
f() { local v=L; unset v; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`
	want := "f=[UNSET]\ntop=[GLOBAL]\n"
	for _, set := range []func(*Semantics){takesTheBinding, keepsTheBinding} {
		out, errs, st := declRun(t, src, set, Diagnostics{})
		if out != want || st != 0 || errs != "" {
			t.Errorf("unset at the scope that declared it = %q (stderr %q, status %d), want %q",
				out, errs, st, want)
		}
	}
}

// And the local of the running scope wins over an enclosing one that holds
// the same name, which is the sharper form of the control above: a search
// that walked outwards looking for *any* shadow would take the caller's here.
func TestTheRunningScopesLocalAnswersBeforeAnEnclosingOne(t *testing.T) {
	const src = `v=GLOBAL
g() { local v=G; unset v; echo "g=[${v-UNSET}]"; }
f() { local v=F; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`
	want := "g=[UNSET]\nf=[F]\ntop=[GLOBAL]\n"
	for _, set := range []func(*Semantics){takesTheBinding, keepsTheBinding} {
		out, errs, st := declRun(t, src, set, Diagnostics{})
		if out != want || st != 0 || errs != "" {
			t.Errorf("unset over two shadows = %q (stderr %q, status %d), want %q",
				out, errs, st, want)
		}
	}
}

// One binding goes and not every one of them: with two calls holding the
// name, the innermost enclosing one is taken and the *next* one out shows
// through — not the global underneath them both.
func TestTakingABindingTakesTheInnermostEnclosingOne(t *testing.T) {
	out, errs, st := declRun(t, `v=GLOBAL
h() { unset v; echo "h=[${v-UNSET}]"; }
g() { local v=G; h; echo "g=[${v-UNSET}]"; }
f() { local v=F; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, takesTheBinding, Diagnostics{})
	want := "h=[F]\ng=[F]\nf=[F]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unset three calls deep = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A scope with no shadow of the name is not in the way of the search: the
// binding taken is the innermost one that exists, however many calls stand
// between it and the `unset`.
func TestAScopeWithNoShadowIsSteppedOver(t *testing.T) {
	out, errs, st := declRun(t, `v=GLOBAL
h() { unset v; echo "h=[${v-UNSET}]"; }
g() { h; }
f() { local v=F; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, takesTheBinding, Diagnostics{})
	want := "h=[GLOBAL]\nf=[GLOBAL]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unset past a scope with no shadow = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The binding is gone rather than hidden, which is the difference an
// implementation is most likely to miss: an assignment after the `unset`
// reaches what lies beyond, and survives the call that used to own the name.
// A shell that merely made the outer value *visible* would give the first two
// readings and leave `top` at GLOBAL.
func TestATakenBindingIsGoneRatherThanHidden(t *testing.T) {
	out, errs, st := declRun(t, `v=GLOBAL
g() { unset v; v=NEW; echo "g=[${v-UNSET}]"; }
f() { local v=L; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, takesTheBinding, Diagnostics{})
	want := "g=[NEW]\nf=[NEW]\ntop=[NEW]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("assigning after the binding went = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And the call that owned it has no binding left to put back, which is the
// same fact read from the other end: a `local` made *after* the `unset`
// belongs to the callee, and what the caller sees when the callee returns is
// what lay beyond — not the value the caller declared.
func TestTheOwningCallHasNothingLeftToPutBack(t *testing.T) {
	out, errs, st := declRun(t, `v=GLOBAL
g() { unset v; local v=GL; echo "g=[${v-UNSET}]"; }
f() { local v=F; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, takesTheBinding, Diagnostics{})
	want := "g=[GL]\nf=[GLOBAL]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local declared after the binding went = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A second `unset` reaches the next binding out, and at the bottom of the
// stack that is the ordinary removal: the name is gone for good.
func TestASecondUnsetReachesWhatTheFirstRevealed(t *testing.T) {
	out, errs, st := declRun(t, `v=GLOBAL
g() { unset v; echo "one=[${v-UNSET}]"; unset v; echo "two=[${v-UNSET}]"; }
f() { local v=L; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, takesTheBinding, Diagnostics{})
	want := "one=[GLOBAL]\ntwo=[UNSET]\nf=[UNSET]\ntop=[UNSET]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("two unsets = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A name nothing held before the declaration: taking the binding away leaves
// the name absent rather than leaving the call's empty behind, and the script
// sees nothing afterwards either.
func TestTakingABindingThatDisplacedNothing(t *testing.T) {
	const src = `g() { unset v; echo "g=[${v-UNSET}]"; }
f() { local v=L; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`
	want := "g=[UNSET]\nf=[UNSET]\ntop=[UNSET]\n"
	for _, set := range []func(*Semantics){takesTheBinding, keepsTheBinding} {
		out, errs, st := declRun(t, src, set, Diagnostics{})
		if out != want || st != 0 || errs != "" {
			t.Errorf("a binding over nothing = %q (stderr %q, status %d), want %q",
				out, errs, st, want)
		}
	}
}

// An array is a table of its own, so the binding taken has to come out of
// that one as well: a fix that reached the scalar table alone would leave the
// callee reading the caller's elements.
func TestTakingABindingReachesTheArrayTable(t *testing.T) {
	const src = `v=(G1 G2)
g() { unset v; echo "g=[${v[@]-UNSET}]"; }
f() { local v=(L1 L2); g; echo "f=[${v[@]-UNSET}]"; }
f
echo "top=[${v[@]-UNSET}]"`
	out, errs, st := declRun(t, src, takesTheBinding, Diagnostics{})
	want := "g=[G1 G2]\nf=[G1 G2]\ntop=[G1 G2]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("taking an array binding = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, errs, st = declRun(t, src, keepsTheBinding, Diagnostics{})
	want = "g=[UNSET]\nf=[UNSET]\ntop=[G1 G2]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("keeping an array binding = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A subshell holds its own copy of the scopes, so what it takes away is its
// own: the parent's call still has the binding it declared.
func TestABindingTakenInASubshellIsTheSubshells(t *testing.T) {
	out, errs, st := declRun(t, `v=GLOBAL
g() { unset v; echo "g=[${v-UNSET}]"; }
f() { local v=L; ( g ); echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, takesTheBinding, Diagnostics{})
	want := "g=[GLOBAL]\nf=[L]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("taking a binding inside a subshell = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The axis is asked only where the two readings differ, which is where a
// calling scope shadowed the name and the running one did not. A shell that
// has not answered it still unsets a name at the top level, and still unsets
// its own local.
func TestUnsetAsksTheAxisOnlyAtTheDisagreement(t *testing.T) {
	unanswered := func(s *Semantics) { s.UnsetRemovesAnEnclosingLocal = Unspecified }
	for _, tc := range []struct{ name, src, want string }{
		{"a top-level name", `v=GLOBAL
unset v
echo "top=[${v-UNSET}]"`, "top=[UNSET]\n"},
		{"a name no scope holds", `g() { unset v; echo "g=[${v-UNSET}]"; }
v=GLOBAL
f() { g; }
f
echo "top=[${v-UNSET}]"`, "g=[UNSET]\ntop=[UNSET]\n"},
		{"the running scope's own local", `v=GLOBAL
f() { local v=L; unset v; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, "f=[UNSET]\ntop=[GLOBAL]\n"},
	} {
		out, errs, st := declRun(t, tc.src, unanswered, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%s with the axis unanswered = %q (stderr %q, status %d), want %q",
				tc.name, out, errs, st, tc.want)
		}
	}
}

// And where they *do* differ an unanswered axis refuses by name and leaves
// the binding where it is, rather than quietly picking one of the two.
func TestUnsetRefusesAnUnansweredAxisAtTheDisagreement(t *testing.T) {
	out, errs, _ := declRun(t, `v=GLOBAL
g() { unset v; echo "st=$? g=[${v-UNSET}]"; }
f() { local v=L; g; echo "f=[${v-UNSET}]"; }
f
echo "top=[${v-UNSET}]"`, func(s *Semantics) {
		s.UnsetRemovesAnEnclosingLocal = Unspecified
	}, Diagnostics{})
	if !strings.Contains(errs, "calling function made local") ||
		!strings.Contains(errs, "the shells disagree here") {
		t.Errorf("the refusal said %q, which does not name the axis", errs)
	}
	// The status is the refusal's and the binding is where it was: a shell
	// that had just said it did not know which reading to take and then took
	// the name away would have chosen one.
	if want := "st=2 g=[L]\nf=[L]\ntop=[GLOBAL]\n"; out != want {
		t.Errorf("a refused unset = %q, want %q", out, want)
	}
}

// The switch a dialect with a name for the question moves, and it travels
// both ways: the getter reads the axis back rather than a second bit, so the
// two cannot drift apart.
func TestTheSwitchOverTakingACallersBinding(t *testing.T) {
	// The Runner runs nothing here — the question is what the getter reads
	// off the vector — so it needs no directory. testrunner:bare
	r := &Runner{}
	sem := permissive()
	sem.UnsetRemovesAnEnclosingLocal = Yes
	r.Semantics = &sem
	if !r.UnsetRemovesAnEnclosingLocal() {
		t.Error("the getter did not read the axis the vector holds")
	}
	r.SetUnsetRemovesAnEnclosingLocal(false)
	if r.UnsetRemovesAnEnclosingLocal() {
		t.Error("the switch did not move the axis")
	}
	r.SetUnsetRemovesAnEnclosingLocal(true)
	if !r.UnsetRemovesAnEnclosingLocal() {
		t.Error("the switch did not travel back")
	}
}

// The same axis reached through an **assignment prefix** in front of a
// function call rather than through `local`. A prefix takes no scope here, so
// nothing in r.scopes records it and the search above finds nothing — the
// binding is in a frame of its own. See interp/unsetenclosinglocal.go, and
// note that the frame has to be reachable from a function the prefixed one
// *calls*, which is the shape the panel measures.
func TestUnsetOfACallsPrefixBindingCanTakeItAway(t *testing.T) {
	const src = `v=GLOBAL
g() { unset v; echo "g=[${v-UNSET}]"; }
f() { g; echo "f=[${v-UNSET}]"; }
v=PRE f
echo "top=[${v-UNSET}]"`
	out, errs, st := declRun(t, src, takesTheBinding, Diagnostics{})
	want := "g=[GLOBAL]\nf=[GLOBAL]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("taking the binding = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, errs, st = declRun(t, src, keepsTheBinding, Diagnostics{})
	want = "g=[UNSET]\nf=[UNSET]\ntop=[GLOBAL]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("keeping the binding = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And once the binding is gone, a later assignment writes what it revealed —
// which is the row that says the binding was *removed* rather than emptied,
// and the one an early version of the frames got wrong: the prefix-less call
// in the middle put back the slice it had captured on the way in and
// resurrected the entry.
func TestAWriteAfterUnsettingACallsPrefixBindingReachesTheShell(t *testing.T) {
	const src = `q=GLOBAL
d() { unset q; q=NEW; }
c() { d; echo "c=[${q-UNSET}]"; }
q=PRE c
echo "top=[${q-UNSET}]"`
	out, _, st := declRun(t, src, takesTheBinding, Diagnostics{})
	if want := "c=[NEW]\ntop=[NEW]\n"; out != want || st != 0 {
		t.Errorf("taking the binding = %q (status %d), want %q", out, st, want)
	}
	out, _, st = declRun(t, src, keepsTheBinding, Diagnostics{})
	if want := "c=[NEW]\ntop=[GLOBAL]\n"; out != want || st != 0 {
		t.Errorf("keeping the binding = %q (status %d), want %q", out, st, want)
	}
}

// The innermost frame is the one removed: a call two frames out keeps its own
// entry, which is what makes this a stack rather than one live prefix.
func TestUnsetTakesTheInnermostCallsPrefixBinding(t *testing.T) {
	const src = `z=GLOBAL
b() { unset z; echo "b=[${z-UNSET}]"; }
a() { z=INNER b; echo "a=[${z-UNSET}]"; }
z=OUTER a
echo "top=[${z-UNSET}]"`
	out, _, st := declRun(t, src, takesTheBinding, Diagnostics{})
	if want := "b=[OUTER]\na=[OUTER]\ntop=[GLOBAL]\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// And a name the *running* scope declared is still the running scope's,
// whatever a call further out is holding: the local is unset where it stands
// under both answers, and the prefix's entry is untouched.
func TestARunningScopesLocalIsNotACallsPrefixBinding(t *testing.T) {
	const src = `w=GLOBAL
k() { local w=L; unset w; echo "k=[${w-UNSET}]"; }
j() { k; echo "j=[${w-UNSET}]"; }
w=PRE j
echo "top=[${w-UNSET}]"`
	want := "k=[UNSET]\nj=[PRE]\ntop=[GLOBAL]\n"
	for _, set := range []func(*Semantics){takesTheBinding, keepsTheBinding} {
		out, _, st := declRun(t, src, set, Diagnostics{})
		if out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q", out, st, want)
		}
	}
}

// records is the answer that keeps a name a declaration brought into being
// with no value — Semantics.ValuelessDeclarationRecordsTheName, which is
// what `unset` of a running scope's own local leaves behind as well.
func records(yes Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.ValuelessDeclarationRecordsTheName = yes
		// The letters `local` reads, so a suite about what survives an
		// `unset` can write a declaration that carries one, and the
		// sentence a listing gives a name that is not there, which is the
		// control below rather than this suite's subject.
		s.LocalOptions = "agiprux"
		s.DeclarePrintReportsAMissingName = Yes
	}
}

// TestUnsetOfTheRunningScopesOwnLocalLeavesItDeclared is the half beside the
// control above: the name is unset either way, and in the column that keeps
// the record it is still a row in a listing.
func TestUnsetOfTheRunningScopesOwnLocalLeavesItDeclared(t *testing.T) {
	const src = `v=GLOBAL
f() { local v=L; unset v; typeset -p v; echo "st=$?"; echo "f=[${v-UNSET}]"; }
f
typeset -p v`
	out, errs, st := declRun(t, src, records(Yes), Diagnostics{})
	want := "declare -- v\nst=0\nf=[UNSET]\ndeclare -- v=\"GLOBAL\"\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a recorded local = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, _, _ = declRun(t, src, records(No), Diagnostics{})
	if strings.Contains(out, "declare -- v\n") {
		t.Errorf("an unrecorded local = %q, want no row for the name the unset took", out)
	}
}

// The record is a row in a listing and **not** a parameter: a second `unset`
// of the same name falls through to the function table, where the first one
// took the local. The two records are two fields for exactly this reader —
// a bare declaration nothing has unset *is* a parameter and takes the turn.
func TestThePlaceholderIsNotAParameterTheFunctionTableHidesBehind(t *testing.T) {
	const src = `wrap() {
f() { echo "f ran"; }
local f
unset f
f 2>/dev/null; echo "one=$?"
unset f
f 2>/dev/null; echo "two=$?"
}
wrap`
	out, _, st := declRun(t, src, func(s *Semantics) {
		records(Yes)(s)
		s.UnsetReachesTheFunctionTable = Yes
	}, Diagnostics{})
	want := "f ran\none=0\ntwo=127\n"
	if out != want || st != 0 {
		t.Errorf("two unsets over a local and a function = %q (status %d), want %q", out, st, want)
	}
}

// The letters go with the value, which is what says the record is the bare
// one rather than the declaration the scope started with.
func TestUnsetOfALocalDropsItsLettersAndKeepsTheRecord(t *testing.T) {
	for _, tc := range []struct{ name, decl string }{
		{"the export letter", "local -x v"},
		{"the integer letter", "local -i v"},
		{"a value", "local v=zz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "v=GLOBAL\nf() { " + tc.decl + "; unset v; typeset -p v; }\nf"
			out, errs, st := declRun(t, src, records(Yes), Diagnostics{})
			want := "declare -- v\n"
			if out != want || st != 0 || errs != "" {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.decl, out, errs, st, want)
			}
		})
	}
}

// And a name no scope shadows keeps nothing: the removal clears the record
// with the attributes, so this is about the binding rather than about
// `unset`.
func TestUnsetAtTheTopLevelLeavesNoRecord(t *testing.T) {
	const src = `typeset v
v=1
unset v
typeset -p v`
	out, errs, st := declRun(t, src, records(Yes), Diagnostics{})
	if out != "" || st == 0 || !strings.Contains(errs, "v: not found") {
		t.Errorf("unset at the top level = %q (stderr %q, status %d), want the missing-name refusal",
			out, errs, st)
	}
}
