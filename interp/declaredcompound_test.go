// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration that names an array letter and no value: `local -a opts`.
//
// The panel is unanimous that the name is an array holding nothing rather
// than a scalar holding nothing — `f() { local -a a; echo ${#a[@]}; }` is 0 in
// every shell that takes the letter — so it is a rule of the core and not an
// axis. What the shells disagree about is a different question this does not
// ask: whether the declared name is *set*, which is
// DeclaredNameWithoutValueIsEmpty and is answered on both sides below.
//
// `${#a[@]}` is the read that tells the two apart, and it is the only one
// that does: an empty scalar and an empty array both answer 0 to `${#a}` and
// both print nothing. That is what made #1535 silent — `local` had the
// table's half of the mark and not the array's, so a name a function declared
// `local -a` was a string for the rest of the function, and every later
// `${opts[@]}` was a plausible answer to the wrong question.
func withArrayLetters(empty Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareOptions = "aAgilprux"
		s.LocalOptions = "aAilprux"
		s.DeclaredNameWithoutValueIsEmpty = empty
		s.DeclareListing = DeclareListingClustered
		s.BareDeclarationListing = DeclareListingPlainAssignment
		// Not what any of this is about: which functions have a scope, and
		// whether a subscripted operand declares one. Answered flat so an
		// unanswered axis cannot stand in for the array the tests look for.
		s.TypesetLocalNeedsKeywordFunction = No
		s.TypesetTakesASubscript = Yes
		s.DeclarationTakesASubscript = Yes
		s.SubscriptedOperandTakesTheContainerAttribute = Yes
		s.SubscriptedOperandTakesALocalDeclaration = Yes
		s.ArraysAreSparse = No
		s.DeclarePrintReportsAMissingName = Yes
	}
}

func TestAValuelessArrayDeclarationLeavesAnArray(t *testing.T) {
	// One line per word that declares, because they are three loops rather
	// than one and the mark went missing from exactly one of them.
	for _, decl := range []string{"local -a a", "typeset -a a"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; echo "n=${#a[@]}"; a+=(z); echo "[${a[@]}] n=${#a[@]}"; }
f`
			out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
			const want = "n=0\n[z] n=1\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// Note what this does *not* claim. Where a declared name without a value is
// unset rather than empty, the panel says the attribute survives the
// declaration anyway — bash 5.3.15 lists `declare -a a` for a `local -a a`
// that `${a+S}` calls unset — and this shell drops the name entirely there.
// That is a second gap and not this one: `${#a[@]}` is 0 either way, so it
// costs a listing rather than handing a script a string where it declared an
// array. It wants its own issue and its own measurement of what an unset name
// with an attribute even is here.

// The table letter, which is the half `local` already had — here so that the
// shared mark cannot lose it while gaining the other.
func TestAValuelessTableDeclarationLeavesATable(t *testing.T) {
	for _, decl := range []string{"local -A m", "typeset -A m"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; m[k]=v; echo "[${m[k]}] n=${#m[@]}"; }
f`
			out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
			const want = "[v] n=1\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// The `+` spelling of the same letters takes nothing off and — the half a
// shared mark could lose — brings nothing into being either. Measured: real
// zsh's `typeset +a a` leaves `typeset a=”`, a scalar, and bash 5.3.15's
// `declare +a a` leaves `declare -- a` with no array attribute at all; ksh93
// refuses the spelling outright. So a mark that ignored the `+` would answer
// a declaration that removes an attribute by creating the store for it.
func TestThePlusSpellingBringsNoCompoundIntoBeing(t *testing.T) {
	for _, decl := range []string{"typeset +a a", "local +a a", "typeset +A a", "local +A a"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; typeset -p a; }
f`
			out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
			const want = "declare -- a=\"\"\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// A subscripted operand carries the attribute to the *base* name, which is
// the third loop through the shared mark. It is a guard rather than a probe:
// the element write brings the array into being on its own here, so the mark
// changes nothing — the reason to run it is that the fold must not make the
// third loop start doing something the other two do not.
func TestASubscriptedArrayDeclarationLeavesAnArray(t *testing.T) {
	src := `f() { local -a a[2]=v; echo "n=${#a[@]} [${a[2]}]"; }
f`
	out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
	const want = "n=3 [v]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The converse question, and the one the shells split three ways: what the
// array or table letter makes of a scalar the name is **already** holding.
//
// It threw the value away in every dialect, which matched zsh by accident and
// lost the script's own value under the other two answers at status 0 — a
// declaration meant to say "this name is an array" emptied it. Measured
// 2026-09-08 with `b=1; typeset -a b`: bash lists `declare -a b=([0]="1")`,
// ksh93 converts nothing and still lists `b=1`, zsh lists `typeset -a b=( )`
// with a count of 0 (#1572).
//
// The two letters take two fields because ksh93 answers them differently: its
// `-A` promotes the value under the key `0` where its `-a` does nothing at
// all.
func withScalarUnderACompound(array, table ScalarUnderACompoundPolicy) func(*Semantics) {
	letters := withArrayLetters(Yes)
	return func(s *Semantics) {
		letters(s)
		// A plain `$b` is the first element rather than the whole array,
		// which is what the two promoting shells read it as and is what makes
		// `[$b]` a probe of whether the value survived at all.
		s.ArrayScalarIsTheWholeArray = No
		s.ScalarUnderAnArrayDeclaration = array
		s.ScalarUnderATableDeclaration = table
	}
}

// The promoting answer: the value becomes the first element, and an append
// after it goes behind rather than over.
func TestAnArrayDeclarationOverAScalarCanKeepIt(t *testing.T) {
	src := `b=1; typeset -a b; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"
b+=(9); printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "[1] n=1\n[1][9] n=2\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The discarding answer: the name is an array of no elements, and `$b` reads
// back empty where the promoting answer still answers `1`.
func TestAnArrayDeclarationOverAScalarCanDiscardIt(t *testing.T) {
	src := `b=1; typeset -a b; echo "[$b] n=${#b[@]}"
b+=(9); printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundDiscardsIt, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "[] n=0\n[9] n=1\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The third answer: the declaration converts nothing and records nothing, so
// the name is still the scalar it was.
//
// The listing is the only thing that says so, and that is the finding rather
// than a weakness of the test: ksh93 lets a scalar be subscripted, so
// `${b[0]}` and `${#b[@]}` answer the same there as they do under the
// promotion, and the append behind it reaches the same two elements by the
// other route — see appendedOverAScalar. A bare `typeset -a`, which lists
// every name carrying the attribute, prints nothing for `b` in that shell.
func TestAnArrayDeclarationOverAScalarCanLeaveItAScalar(t *testing.T) {
	src := `b=1; typeset -a b; typeset -p b; echo "n=${#b[@]}"
b+=(9); printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundStaysAScalar, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "declare -- b=\"1\"\nn=1\n[1][9] n=2\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The table letter, answered on its own field: the value lands under the key
// `0` and an appended entry stands beside it.
//
// The shape the whole issue was found through — `a=1; typeset -A a;
// a+=([k]=v)` is two entries in bash and ksh93 and one in zsh, and the value
// is already gone or already placed by the time the append runs.
func TestATableDeclarationOverAScalarCanKeepItUnderKeyZero(t *testing.T) {
	src := `a=1; typeset -A a; echo "n=${#a[@]} [${a[0]}]"
a+=([k]=v); echo "n=${#a[@]} [${a[0]}][${a[k]}]"`
	for _, c := range []struct {
		table ScalarUnderACompoundPolicy
		want  string
	}{
		{ScalarUnderACompoundBecomesTheFirstElement, "n=1 [1]\nn=2 [1][v]\n"},
		{ScalarUnderACompoundDiscardsIt, "n=0 []\nn=1 [][v]\n"},
	} {
		out, errs, st := declRun(t, src,
			withScalarUnderACompound(ScalarUnderACompoundDiscardsIt, c.table), Diagnostics{})
		if out != c.want || errs != "" || st != 0 {
			t.Errorf("%v: = %q (stderr %q, status %d), want %q", c.table, out, errs, st, c.want)
		}
	}
}

// The empty string is a value and the promoting answer keeps it, so the array
// is one element long and that element is empty.
//
// Its own test because an implementation that promoted a *non-empty* scalar
// and took the empty one for nothing answers this exactly as the discarding
// column does, and no count can tell an empty first element from no first
// element.
func TestAnArrayDeclarationOverAnEmptyScalarKeepsIt(t *testing.T) {
	src := `b=; typeset -a b; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "[] n=1\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The promoted value is one element however many words it looks like.
func TestAnArrayDeclarationDoesNotSplitTheScalar(t *testing.T) {
	src := `b="x y"; typeset -a b; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "[x y] n=1\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A value the name reads back from the *inherited environment* is a value it
// is holding: the question is what `$b` answers, not what this runner happens
// to have stored under the name.
//
// The same hole mutation found in #1502's fix, at the second site: a
// promotion reading only `Runner.Vars` passes every other test here and drops
// the value a script was started with.
func TestAnArrayDeclarationOverAnInheritedScalarKeepsIt(t *testing.T) {
	src := `typeset -a b; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRunEnv(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{}, []string{"b=1"})
	const want = "[1] n=1\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// An unset name has nothing for the three answers to differ about, and no
// dialect is asked for it — the *unanswered* vector is in the loop, so a
// version that asked anyway would refuse `typeset -a opts` under a dialect
// that has no answer to a question `typeset -a opts` does not raise.
//
// The append afterwards is what makes the emptiness observable: an
// implementation that promoted whatever the name read back as would put an
// empty first element in front of the 9.
func TestAnArrayDeclarationOverAnUnsetNameKeepsNothing(t *testing.T) {
	src := `typeset -a b; echo "n=${#b[@]}"
b+=(9); printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	for _, p := range []ScalarUnderACompoundPolicy{
		ScalarUnderACompoundUnspecified,
		ScalarUnderACompoundBecomesTheFirstElement,
		ScalarUnderACompoundStaysAScalar,
		ScalarUnderACompoundDiscardsIt,
	} {
		out, errs, st := declRun(t, src, withScalarUnderACompound(p, p), Diagnostics{})
		const want = "n=0\n[9] n=1\n"
		if out != want || errs != "" || st != 0 {
			t.Errorf("%v: = %q (stderr %q, status %d), want %q", p, out, errs, st, want)
		}
	}
}

// And a name already holding an array is left exactly as it stands, in every
// column and without a dialect.
//
// The negative that keeps the axis about a scalar: a fix that emptied or
// re-created the store whenever the letter was written would pass every
// promoting row above and lose an array here, silently — which is the shape
// #1535 had.
func TestAnArrayDeclarationOverAnArrayKeepsItsElements(t *testing.T) {
	src := `a=(1 2); typeset -a a; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	for _, p := range []ScalarUnderACompoundPolicy{
		ScalarUnderACompoundUnspecified,
		ScalarUnderACompoundBecomesTheFirstElement,
		ScalarUnderACompoundStaysAScalar,
		ScalarUnderACompoundDiscardsIt,
	} {
		out, errs, st := declRun(t, src, withScalarUnderACompound(p, p), Diagnostics{})
		const want = "[1][2] n=2\n"
		if out != want || errs != "" || st != 0 {
			t.Errorf("%v: = %q (stderr %q, status %d), want %q", p, out, errs, st, want)
		}
	}
}

// A declaration carrying its own value replaces whatever the letter left, so
// the promotion is invisible through it: one element, `9`, in every column.
func TestAnArrayDeclarationWithAValueOverAScalarReplacesIt(t *testing.T) {
	src := `b=1; typeset -a b=(9); printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "[9] n=1\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// Where the promotion does *not* happen: a local declaration builds the array
// cell rather than converting one.
//
// Measured — `f() { local b=1; local -a b; }` and the same with `declare` or
// `typeset` on the second line are all `declare -a b=()` in bash, where
// `b=1; typeset -a b` at the top level and `b=1; f() { typeset -ga b; }; f`
// both promote. It is not the shadow being *fresh*, since `local b=1` has
// already taken the copy, and not the letter in general, since `local b=1;
// local -i b` keeps the `1` — it is the array cell in particular. No dialect
// is asked, because zsh discards a held scalar wherever it finds one and
// ksh93 declares nothing local in a plain function, so the two shells with an
// answer of their own reach the same place by their own route.
func TestALocalArrayDeclarationOverALocalScalarKeepsNothing(t *testing.T) {
	for _, decl := range []string{"local -a b", "typeset -a b"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { local b=1; " + decl + `; echo "n=${#b[@]}"; }
f`
			out, errs, st := declRun(t, src,
				withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
				Diagnostics{})
			const want = "n=0\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// The cell that counts is the **innermost** one, which a call one level deeper
// is what shows: `f` shadows `b` and `g` does not, so reading the outermost
// scope instead would find nothing saved and promote a value the function had
// already replaced. Measured — the same `n=0` in bash 5.3.15, bash 3.2.57 and
// zsh whether `f` is called from the top level or from inside `g`.
func TestALocalArrayDeclarationOverALocalScalarKeepsNothingWhenNested(t *testing.T) {
	src := `f() { local b=1; local -a b; echo "n=${#b[@]}"; }
g() { f; }
g`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "n=0\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the pair that says it is the *local* cell and not the function: the
// global letter takes no shadow, so the value the name is holding is promoted
// from inside a function exactly as it is at the top level.
func TestAGlobalArrayDeclarationOverAScalarKeepsIt(t *testing.T) {
	src := `b=1; f() { typeset -ga b; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"; }
f`
	out, errs, st := declRun(t, src,
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement, ScalarUnderACompoundDiscardsIt),
		Diagnostics{})
	const want = "[1] n=1\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// An unanswered dialect is refused by name, and the declaration is not made
// one way and reported afterwards.
func TestACompoundDeclarationOverAScalarRefusesAnUnansweredDialect(t *testing.T) {
	for _, c := range []struct{ decl, axis string }{
		{"typeset -a b", "an array declaration over a name already holding a scalar"},
		{"typeset -A b", "a table declaration over a name already holding a scalar"},
	} {
		t.Run(c.decl, func(t *testing.T) {
			src := "b=1; " + c.decl + `; echo "st=$?"; echo "[$b]"`
			out, errs, _ := declRun(t, src,
				withScalarUnderACompound(ScalarUnderACompoundUnspecified, ScalarUnderACompoundUnspecified),
				Diagnostics{})
			if !strings.Contains(errs, c.axis) {
				t.Errorf("stderr %q, want the axis named", errs)
			}
			if !strings.Contains(out, "st=2") {
				t.Errorf("out %q, want the refusal to leave a status", out)
			}
			if !strings.Contains(out, "[1]") {
				t.Errorf("out %q, want the value left exactly as it was", out)
			}
		})
	}
}

// A *produced* array is not a scalar to be converted, and the guard that says
// so is load-bearing rather than defensive.
//
// Measured against the mutant that drops it: `FUNCNAME=x; typeset -a FUNCNAME`
// then reads `${FUNCNAME[*]}` inside a function as `x` where bash 5.3.15 and
// this engine both answer `f`. The assignment leaves a scalar in the ordinary
// table — nothing stops it — and getVar hands that scalar back, so a
// declaration that promoted whatever the name reads as would write an array
// store over the producer and the call stack would be a stale word from then
// on. The producer is the name's value; there is nothing here to convert.
func TestAnArrayDeclarationOverAProducedArrayLeavesItToItsProducer(t *testing.T) {
	src := `FUNCS=x; typeset -a FUNCS; printf "[%s]" "${FUNCS[@]}"; echo " n=${#FUNCS[@]}"`
	for _, p := range []ScalarUnderACompoundPolicy{
		ScalarUnderACompoundBecomesTheFirstElement,
		ScalarUnderACompoundStaysAScalar,
		ScalarUnderACompoundDiscardsIt,
	} {
		out, st := runGrammar(t, src, nil, func(r *Runner) {
			sem := *r.Semantics
			withScalarUnderACompound(p, p)(&sem)
			r.Semantics = &sem
			r.SetDynamicArray("FUNCS", func(*Runner) []string { return []string{"p", "q"} })
		})
		if want := "[p][q] n=2\n"; out != want || st != 0 {
			t.Errorf("%v: = %q status %d, want %q — the producer answers, not the scalar", p, out, st, want)
		}
	}
}

// A table already declared is not a scalar to be converted either, and the
// probe that shows it needs the axis a table's *scalar view* is asked under.
//
// `${a[0]}` is empty here and the count is 2. Drop the guard and the
// declaration reads the table back through getVar — which answers a table
// with the join of its values where ArrayScalarIsTheWholeArray says yes — and
// promotes that join into the table as a new entry under the key `0`. The
// table would grow an element made of its own contents every time the letter
// was written again, which nothing in the shipped presets reaches because
// none of them joins a table's values *and* promotes under the table letter,
// and which a name declared twice would reach the moment one did.
func TestATableDeclarationOverATableDoesNotRePromoteItsOwnContents(t *testing.T) {
	src := `typeset -A a; a[k]=v; a[j]=w; typeset -A a; echo "n=${#a[@]} zero=[${a[0]}]"`
	out, st := runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		withScalarUnderACompound(ScalarUnderACompoundBecomesTheFirstElement,
			ScalarUnderACompoundBecomesTheFirstElement)(&sem)
		// The join, which is the reading that makes a table answer a plain
		// `$a` with something rather than nothing.
		sem.ArrayScalarIsTheWholeArray = Yes
		r.Semantics = &sem
	})
	if want := "n=2 zero=[]\n"; out != want || st != 0 {
		t.Errorf("= %q status %d, want %q — the table kept its own two entries", out, st, want)
	}
}
