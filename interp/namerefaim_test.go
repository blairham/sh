// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// namerefAimSemantics answers the axes a reference needs and nothing else, so
// that what these cases assert is the core's rule and not a dialect's.
//
// The rule itself is unanimous across the only two shells that spell a
// reference — measured 2026-09-17 on bash 5.3.20 and ksh93u+ 2012-08-01 — so
// there is no axis here to ask. What differs is the *fatality* of the
// refusal, which BadNameToDeclarationFatal already records; it is set to the
// carrying-on answer so a case can assert on the line behind the refusal.
func namerefAimSemantics() Semantics {
	sem := PosixSemantics()
	sem.DeclareOptions = "aAfFgilnprux"
	sem.UnsetOptions = "vfn"
	sem.NamerefCycleIsRefused = No
	sem.BadNameToDeclarationFatal = No
	sem.ScalarOverACompoundIsAnInconsistentType = No
	sem.ArrayScalarIsTheWholeArray = No
	sem.NamerefArrayRefusal = NamerefArrayCheckedLastOnTheAttribute
	// And the `n` letter is read beside another one rather than refusing
	// its company, which is what lets a case here write `typeset -rn` at
	// all. The other answer is its own subject — see
	// interp/namerefletters_test.go.
	sem.NamerefLetterStandsAlone = No
	// The letters `local` takes, and the two axes a case walks past on its
	// way to this one: `${!r}` naming the reference's target rather than
	// expanding it twice, and a valueless declaration bringing the name into
	// being empty.
	sem.LocalOptions = "aAfFgilnprux"
	sem.IndirectionYieldsName = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	// And a listing shape, because the only way to ask whether a reference is
	// aimed *without* reading through it is to have it listed back: `${!r}`
	// on an unaimed reference is its own question, and one the shells answer
	// differently.
	sem.DeclareListing = DeclareListingClustered
	sem.DeclareValueQuoting = ListingQuoteAlwaysEscaped
	return sem
}

// runNameref runs src with the two constructs a reference case needs to say
// what it means — `${!r}`, which is the only way to ask a reference where it
// points, and `<<<`, which feeds `read` without a file.
func runNameref(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
		d.Herestring = true
	}, func(r *Runner) { r.Semantics = &sem })
}

// The `n` letter on its own does not always leave a reference with nothing to
// point at: **a value the name already holds aims it.** See declareNameref,
// where the measurement is.
//
// The three rows are the three answers, and the middle one is what says the
// value is taken as written rather than as a name: an element is a possible
// target and reads through.
func TestAValueTheNameHoldsAimsTheReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a scalar the name held",
			src:  `good=G; r=good; typeset -n r; echo "[${!r}] [$r]"`,
			want: "[good] [G]",
		},
		{
			name: "an element the name held",
			src:  `a=(p q); t=a[1]; typeset -n t; echo "[${!t}] [$t]"`,
			want: "[a[1]] [q]",
		},
		{
			// A name nothing has set is the unaimed reference, which is what
			// the letter meant on its own before this rule. Asked of the
			// *listing* rather than of `${!u}`: an unaimed reference is
			// exactly the state `${!u}` has no agreed answer for.
			name: "a name nothing has set",
			src:  `typeset -n u; typeset -p u`,
			want: "declare -n u\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runNameref(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A value that is no possible target is refused in the declaration's own
// words, and the name is left exactly as it was found — a plain scalar, not a
// reference to text no shell can name.
func TestAValueThatCannotAimTheReferenceIsRefused(t *testing.T) {
	for _, value := range []string{"/", "", "1x", "a b"} {
		src := `b=` + quoteForShell(value) + `; typeset -n b; echo "st=$?"; echo "[${b-GONE}]"`
		out, _ := runNameref(t, src)
		if !strings.Contains(out, "invalid variable name for name reference") {
			t.Errorf("%q: got %q, want the bad-target refusal", value, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%q: got %q, want status 1", value, out)
		}
		if !strings.Contains(out, "["+value+"]") {
			t.Errorf("%q: got %q, want the name still holding its value", value, out)
		}
	}
}

// And a binding **this declaration made** adopts nothing, because there is no
// value under it whatever the name meant outside: the cell is new.
//
// This is the half that keeps the rule from reaching through a function's own
// `local`, and it is measured rather than reasoned — `-g`, which makes no new
// cell, aims at the outer value on the same line that `local` leaves unaimed.
func TestAFreshBindingHasNoValueToAdopt(t *testing.T) {
	out, _ := runNameref(t, `good=G
outer=good
f() { local -n outer; typeset -p outer; }
f
g() { typeset -gn outer; typeset -p outer; }
g`)
	if !strings.Contains(out, "declare -n outer\n") {
		t.Errorf("got %q, want a fresh local binding left unaimed", out)
	}
	if !strings.Contains(out, `declare -n outer='good'`) {
		t.Errorf("got %q, want the global cell's own value adopted", out)
	}
}

// The assignment's half of the same check: a reference with nothing to point
// at is aimed by its first value, and a value that is no possible name aims
// it nowhere. See refuseNamerefAim.
func TestAnAssignmentCannotAimAReferenceAtSomethingThatIsNoName(t *testing.T) {
	out, _ := runNameref(t, `typeset -n q
q=/
echo "st=$?"
typeset -p q`)
	if !strings.Contains(out, "`/': not a valid identifier") {
		t.Errorf("got %q, want the identifier complaint", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want status 1", out)
	}
	if !strings.Contains(out, "declare -n q\n") {
		t.Errorf("got %q, want the reference left unaimed", out)
	}

	// A value that *is* a possible name still aims it, which is what makes
	// the refusal above a check rather than a wall.
	out, _ = runNameref(t, `v=V; typeset -n q; q=v; echo "[${!q}] [$q] st=$?"`)
	if !strings.Contains(out, "[v] [V] st=0") {
		t.Errorf("got %q, want the reference aimed at v", out)
	}
}

// What the refusal costs the line differs by **how the write was written**,
// and it is the split [assignForm] already draws for a frozen name: a bare
// assignment gives up the rest of its input line and a builtin's write does
// not. Measured on bash 5.3.20 — `declare -n t; t=/ ; echo after` never
// prints `after`, where the `read` spelling does.
func TestWhatABadAimCostsTheRestOfTheLine(t *testing.T) {
	out, _ := runNameref(t, `typeset -n t; t=/ ; echo after-the-assignment`)
	if strings.Contains(out, "after-the-assignment") {
		t.Errorf("got %q, want the rest of the line given up", out)
	}

	out, _ = runNameref(t, `typeset -n u; read u <<< "/" ; echo after-the-read`)
	if !strings.Contains(out, "read: `/': not a valid identifier") {
		t.Errorf("got %q, want the builtin to name itself", out)
	}
	if !strings.Contains(out, "after-the-read") {
		t.Errorf("got %q, want the rest of the line kept", out)
	}
}

// A loop re-points its reference rather than writing through it, so a word
// that is no name **ends the loop** — reported once, at 1, with the body
// never reached for that word and the reference left where it was.
func TestALoopWordThatCannotAimTheReferenceEndsTheLoop(t *testing.T) {
	out, _ := runNameref(t, `typeset -n w
for w in / a; do echo BODY; done
echo "st=$?"
typeset -p w`)
	if !strings.Contains(out, "`/': not a valid identifier") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the complaint and status 1", out)
	}
	if strings.Contains(out, "BODY") {
		t.Errorf("got %q, want the body left unrun", out)
	}

	// With the words the other way round the body runs once and the loop
	// stops at the bad one, which is what says the refusal ends the loop
	// rather than costing the construct.
	out, _ = runNameref(t, `v=V; typeset -n x
for x in v /; do echo "BODY [$x]"; done
echo "st=$?"`)
	if got := strings.Count(out, "BODY"); got != 1 {
		t.Errorf("got %q with %d passes, want exactly one", out, got)
	}
	if !strings.Contains(out, "BODY [V]") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the first pass through v and then the refusal", out)
	}
}

// quoteForShell wraps a test's value in single quotes so an empty one is an
// operand rather than nothing at all.
func quoteForShell(s string) string { return "'" + s + "'" }
