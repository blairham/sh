// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a declaration does with a subscripted operand that carries **no
// value** has three answers, and only one of them leaves the brackets unread.
//
// They were unread in every dialect, so `typeset 'a[b c]'` was silent at 0
// where zsh and ksh93 both end the script, and `typeset 'a[1]'` left a zsh
// array untouched where zsh empties the element (#3501).
func TestAValuelessSubscriptedOperandIsReadWhereTheDialectReadsIt(t *testing.T) {
	for _, c := range []struct {
		name   string
		p      ValuelessSubscriptedOperandPolicy
		want   string
		status int
	}{
		// The brackets say the name is an array and nothing else, so the
		// array standing in front of the declaration is untouched.
		{
			"the name is declared", ValuelessSubscriptedOperandDeclaresTheName,
			"same=0 a=[1 2 3]\n", 0,
		},
		// The subscript is read and no element is written: `a[1]` is a valid
		// expression, so nothing complains and nothing changes.
		{
			"the subscript is read", ValuelessSubscriptedOperandReadsTheSubscript,
			"same=0 a=[1 2 3]\n", 0,
		},
		// The empty-value form under another spelling, so element 1 is
		// emptied exactly as `typeset 'a[1]='` empties it.
		{
			"the element is written", ValuelessSubscriptedOperandWritesTheElement,
			"same=0 a=[1  3]\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := valuelessSubRun(t, c.p,
				"a=(1 2 3)\n"+`typeset 'a[1]'; echo "same=$? a=[${a[*]}]"`)
			if out != c.want || st != c.status {
				t.Errorf("out %q (status %d), want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// And the whole point of reading them: a subscript that will not evaluate is
// only reachable at all in the two columns that read it, and then it gives up
// as much as Semantics.BadSubscriptToADeclaration says (#3495).
func TestAValuelessOperandsBadSubscriptGivesUpWhereItIsRead(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`typeset 'a[b c]'; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	for _, c := range []struct {
		name  string
		p     ValuelessSubscriptedOperandPolicy
		reads bool
	}{
		{"unread", ValuelessSubscriptedOperandDeclaresTheName, false},
		{"read and discarded", ValuelessSubscriptedOperandReadsTheSubscript, true},
		{"read and written through", ValuelessSubscriptedOperandWritesTheElement, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := valuelessSubRun(t, c.p, src)
			if !c.reads {
				if want := "same=0\nnext=0 a=[1 2 3]\n"; out != want {
					t.Errorf("out %q, want %q — the brackets are not read here", out, want)
				}
				return
			}
			if !strings.Contains(out, "b c") {
				t.Errorf("out %q says nothing about the expression it read", out)
			}
			if strings.Contains(out, "same=") || strings.Contains(out, "next=") {
				t.Errorf("out %q ran on past a give-up that ends the script", out)
			}
		})
	}
}

// A **table's** brackets hold a key and not an expression, in every column —
// so the two columns that read the brackets put the key in with an empty
// value and neither of them reaches the arithmetic. The third does nothing at
// all.
//
// Measured 2026-09-17: `typeset -A m; typeset 'm[b c]'` is silent at 0 in bash
// 5.3.20, and leaves `['b c']` empty in zsh 5.9.2 and ksh93u+ alike. This
// refused the operand as `cannot convert associative to indexed array` in all
// three, and ended the script for it in the ksh column.
func TestAValuelessSubscriptedOperandOnATableWritesTheKey(t *testing.T) {
	const src = "typeset -A m\n" +
		`typeset 'm[b c]'; echo "same=$?"` + "\n" +
		`echo "key=[${m[b c]-none}]"`
	for _, c := range []struct {
		name string
		p    ValuelessSubscriptedOperandPolicy
		want string
	}{
		{"the name is declared", ValuelessSubscriptedOperandDeclaresTheName, "same=0\nkey=[none]\n"},
		{"the subscript is read", ValuelessSubscriptedOperandReadsTheSubscript, "same=0\nkey=[]\n"},
		{"the element is written", ValuelessSubscriptedOperandWritesTheElement, "same=0\nkey=[]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := valuelessSubRun(t, c.p, src)
			if out != c.want || st != 0 {
				t.Errorf("out %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// The column that writes the element grows the array to reach the subscript,
// which is the half a status cannot show — and it is the measured tell that
// the valueless form really is the empty-value form there.
func TestAValuelessOperandWritingTheElementGrowsTheArray(t *testing.T) {
	// Sparse is the axis that decides how far: the column that writes the
	// element is also the column that fills the gap, so the array reaches
	// the subscript rather than gaining one element.
	dense := func(s *Semantics) { s.ArraysAreSparse = No }
	out, st := valuelessSubRunWith(t, ValuelessSubscriptedOperandWritesTheElement, dense,
		"a=(1 2 3)\n"+`typeset 'a[6]'; echo "n=${#a[@]}"`)
	if want := "n=7\n"; out != want || st != 0 {
		t.Errorf("out %q (status %d), want %q at 0", out, st, want)
	}
	// And the column that only reads it leaves the array the length it was.
	out, st = valuelessSubRunWith(t, ValuelessSubscriptedOperandReadsTheSubscript, dense,
		"a=(1 2 3)\n"+`typeset 'a[6]'; echo "n=${#a[@]}"`)
	if want := "n=3\n"; out != want || st != 0 {
		t.Errorf("out %q (status %d), want %q at 0", out, st, want)
	}
}

// All four spellings go through the one door, because the answer is about the
// declaration and not about which word spells it — and a second copy that
// omitted a case is what this repository keeps doing to itself.
func TestEverySpellingOfAValuelessSubscriptedOperandReadsIt(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"typeset", `typeset 'a[b c]'`},
		{"local in a function", `f() { local 'a[b c]'; }; f`},
		{"readonly", `readonly 'a[b c]'`},
		{"export", `export 'a[b c]'`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "a=(1 2 3)\n" + c.src + "\n" + `echo "next=$?"`
			out, _ := valuelessSubRun(t, ValuelessSubscriptedOperandReadsTheSubscript, src)
			if !strings.Contains(out, "b c") {
				t.Errorf("out %q says nothing about the expression", out)
			}
			if strings.Contains(out, "next=") {
				t.Errorf("out %q ran on past a give-up that ends the script", out)
			}
			// And the column that never reads the brackets reaches none of
			// it, at any spelling.
			out, _ = valuelessSubRun(t, ValuelessSubscriptedOperandDeclaresTheName, src)
			if strings.Contains(out, "b c") {
				t.Errorf("out %q read the brackets where the dialect does not", out)
			}
		})
	}
}

// An axis nobody answered is refused by name rather than guessed.
func TestAValuelessSubscriptedOperandRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := valuelessSubRun(t, ValuelessSubscriptedOperandUnspecified,
		"a=(1 2 3)\n"+`typeset 'a[1]'; echo "same=$?"`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out %q is not a refusal naming the axis", out)
	}
	if !strings.Contains(out, "same=2") {
		t.Errorf("out %q does not leave the refusal's status behind", out)
	}
}

// A give-up at one operand stops the builtin there: the operands *before* it
// stand and the ones after it are never reached.
//
// `read 'r[1/0]' b` already answered that (#3494) and the other two sites did
// not, so a command bash abandons still did work bash never does — and the
// declaration half arrived with the abandon itself (#3495, #3506). Measured
// 2026-09-17 in bash 5.3.20 and 3.2.57 alike.
func TestAGiveUpAtOneOperandStopsTheBuiltinThere(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a declaration",
			"a=(1 2 3)\n" + `typeset 'a[b c]'=v x=1 y=2` + "\n" +
				`echo "x=[${x-unset}] y=[${y-unset}]"`,
			"x=[unset] y=[unset]\n",
		},
		{
			"unset, with an operand on each side",
			"a=(1 2 3)\nx=1\ny=2\n" + `unset x 'a[b c]' y` + "\n" +
				`echo "x=[${x-unset}] y=[${y-unset}]"`,
			"x=[unset] y=[2]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := valuelessSubRunAbandoning(t, c.src)
			// The complaint goes to the same buffer, so the assertion is on
			// the line the script printed rather than on the whole of it.
			line := out[strings.LastIndex(out, "x=["):]
			if line != c.want {
				t.Errorf("out %q ended %q, want %q", out, line, c.want)
			}
		})
	}
}

func valuelessSubRun(t *testing.T, p ValuelessSubscriptedOperandPolicy, src string) (string, int) {
	t.Helper()
	return valuelessSubRunWith(t, p, nil, src)
}

// valuelessSubRunWith is the same with one further axis moved, for the rows
// that need a second answer to see what the first one did.
func valuelessSubRunWith(t *testing.T, p ValuelessSubscriptedOperandPolicy,
	also func(*Semantics), src string,
) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		valuelessSubSemantics(s)
		s.ValuelessSubscriptedOperand = p
		if also != nil {
			also(s)
		}
	}, Diagnostics{}, src, RouteUnspecified)
}

// valuelessSubRunAbandoning is the same shell with the give-up answered as the
// column that abandons the command, which is the only column the
// operand-after-a-give-up question can be put to.
func valuelessSubRunAbandoning(t *testing.T, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		valuelessSubSemantics(s)
		s.ValuelessSubscriptedOperand = ValuelessSubscriptedOperandReadsTheSubscript
		s.BadSubscriptToUnset = BadSubscriptAbandonsTheCommand
		s.BadSubscriptToADeclaration = BadSubscriptAbandonsTheCommand
		s.UnsetTakesASubscript = Yes
		s.UnsetSubscriptSkippedWhenNameUnset = No
	}, Diagnostics{}, src, RouteUnspecified)
}

// valuelessSubSemantics answers everything a declaration of an element needs
// except the axis under test, so a row varies that one alone.
func valuelessSubSemantics(s *Semantics) {
	arraySemantics(s)
	s.TypesetTakesASubscript = Yes
	s.DeclarationTakesASubscript = Yes
	s.SubscriptedOperandTakesALocalDeclaration = Yes
	s.SubscriptedOperandTakesTheContainerAttribute = Yes
	s.ReadonlyElement = ReadonlyElementWritten
	s.DeclaredNameWithoutValueIsEmpty = No
	s.TypesetLocalNeedsKeywordFunction = No
	s.CompoundElementsGoThroughTheAttribute = Yes
	s.TableLetterReachesItsOwnOperandsSubscript = Yes
	// The two columns that do not write the element declare the *name*
	// instead, so a row lands on the valueless-declaration listing on its
	// way past. Answered flat: what a listing does is a question of its own
	// and every row below is about the subscript.
	s.ValuelessDeclarationOfAHeldNameListsIt = No
	// The give-up is the axis one question over, and every row above that
	// reaches it is written for the column that ends the script.
	s.BadSubscriptToADeclaration = BadSubscriptEndsTheScript
	s.FatalErrorStatusIsOne = Yes
}
