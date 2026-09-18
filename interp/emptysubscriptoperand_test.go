// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An **empty** subscript in a builtin's operand — `unset 'a[]'`, `read 'a[]'`
// and `printf -v 'a[]'`, which is what `unset "a[$i]"` and the rest are once a
// blank `$i` has gone in, the word being expanded before the builtin sees it.
//
// Every dialect acted on element **zero** here and said nothing, so a computed
// subscript that came out blank quietly deleted or replaced the array's *first*
// element at status 0 — the worst shape a wrong answer takes, because nothing
// downstream can tell (#3513).
//
// Semantics.EmptyArithSubscript is the axis, read rather than a field of its
// own: every shell gives a builtin's operand the disposition it gives the same
// brackets in an expression. The three rows below are the three columns.

// emptyOpUnsetSrc is the pair of lines the `unset` rows are measured over: the
// status on the *same* line as the builtin and the array on the line after it,
// because "gave up the line" and "ended the script" print the same nothing
// when there is only one line to look at.
const emptyOpUnsetSrc = "a=(1 2 3)\n" +
	`unset 'a[]'; echo "same=$?"` + "\n" +
	`echo "next=$? a=[${a[*]}]"`

// emptyOpReadSrc is the same pair one builtin over. A group with a
// here-document rather than a pipeline, because a pipeline's element already
// runs somewhere a give-up unwinds out of.
const emptyOpReadSrc = "r=(1 2 3)\n" +
	`{ read 'r[]'; echo "same=$?"; } <<EOF` + "\n" +
	"Y\nEOF\n" +
	`echo "next=$? r=[${r[*]}]"`

// A delete is the one route where the column that complains has nothing to
// complain about: the brackets name no element, and removing no element is not
// a failure.
//
// Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin /dev/null,
// a script file and repeated under `( … )` and `-c`, `a=(1 2 3)`:
//
//	bash 5.3.20   silent at 0, all three elements standing
//	zsh 5.9.2     `invalid subscript` at 1, the array whole, the line goes on
//	ksh93u+       element zero removed, silent at 0
func TestAnEmptySubscriptToUnsetIsAnsweredLikeAnEmptyOneInAnExpression(t *testing.T) {
	for _, c := range []struct {
		name  string
		p     EmptyArithSubscriptPolicy
		want  string
		says  bool
		after string
	}{
		// The brackets hold an expression that happens to be empty, which is
		// zero, so the operand is element zero and it goes.
		{"the empty expression", EmptyArithSubscriptIsTheEmptyExpression, "same=0", false, "next=0 a=[2 3]"},
		// Nothing removed and nothing said: there was no element to remove.
		{"reported", EmptyArithSubscriptIsReported, "same=0", false, "next=0 a=[1 2 3]"},
		// Refused, and the refusal leaves a failed builtin behind — how much
		// more it gives up is the route's own axis, pinned below.
		{"invalid", EmptyArithSubscriptIsInvalid, "same=1", true, "next=0 a=[1 2 3]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := emptyOpRun(t, c.p, Diagnostics{}, emptyOpUnsetSrc)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
			if !strings.Contains(out, c.after) {
				t.Errorf("out %q is missing %q", out, c.after)
			}
			said := strings.Contains(out, "subscript") || strings.Contains(out, "identifier")
			if said != c.says {
				t.Errorf("out %q, want a complaint: %v", out, c.says)
			}
		})
	}
}

// The refusing column writes the **read's** sentence of an `unset` operand:
// the builtin reads the brackets to find the element it is to remove, and zsh
// says `invalid subscript` of them where the same shell says `not an
// identifier: r[]` of a store's. The two fields the expression already holds
// are what carry it, so a route's wording is read rather than copied.
func TestAnEmptySubscriptToUnsetWritesTheReadsSentence(t *testing.T) {
	out, _ := emptyOpRun(t, EmptyArithSubscriptIsInvalid,
		Diagnostics{ArithEmptySubscript: "READ", ArithEmptySubscriptTarget: "WRITE"},
		emptyOpUnsetSrc)
	if !strings.Contains(out, "READ") {
		t.Errorf("out %q does not write the read's sentence", out)
	}
	if strings.Contains(out, "WRITE") {
		t.Errorf("out %q writes the write's sentence at a delete", out)
	}
}

// And the store writes the **write's**, which is the other half of the same
// measurement: one construct, two sentences, and the pair of rows is what
// keeps either from being written at both.
func TestAnEmptySubscriptToAStoreWritesTheWritesSentence(t *testing.T) {
	out, _ := emptyOpRun(t, EmptyArithSubscriptIsInvalid,
		Diagnostics{ArithEmptySubscript: "READ", ArithEmptySubscriptTarget: "WRITE"},
		emptyOpReadSrc)
	if !strings.Contains(out, "WRITE") {
		t.Errorf("out %q does not write the write's sentence", out)
	}
	if strings.Contains(out, "READ") {
		t.Errorf("out %q writes the read's sentence at a store", out)
	}
}

// How much a refused operand gives up is the *route's* axis and not this one,
// which is why the empty subscript is not answered by the give-up policy
// itself: bash abandons the command for a subscript that will not evaluate and
// only reports one that is empty.
func TestAnEmptySubscriptToUnsetGivesUpAsMuchAsTheRouteDoes(t *testing.T) {
	for _, c := range []struct {
		name   string
		giveUp BadSubscriptPolicy
		want   []string
		absent []string
	}{
		{"a failed builtin", BadSubscriptReported, []string{"same=1", "next=0"}, nil},
		{"the command and its line", BadSubscriptAbandonsTheCommand, []string{"next=1"}, []string{"same="}},
		{"the script", BadSubscriptEndsTheScript, nil, []string{"same=", "next="}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := emptyOpRunWith(t, EmptyArithSubscriptIsInvalid, Diagnostics{},
				func(s *Semantics) { s.BadSubscriptToUnset = c.giveUp }, emptyOpUnsetSrc)
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("out %q is missing %q", out, want)
				}
			}
			for _, absent := range c.absent {
				if strings.Contains(out, absent) {
					t.Errorf("out %q ran %q, which this answer gives up", out, absent)
				}
			}
		})
	}
}

// A store has nowhere to put the value, so the column that acts on nothing
// refuses the **whole operand as a name** — and that is the builtin's own
// bad-name refusal rather than a sentence of this axis's: measured, bash
// 5.3.20 answers `read 'r[]'` and `read '1x'` with one sentence at 1.
func TestAnEmptySubscriptToAStoreIsRefusedAsTheBuiltinsName(t *testing.T) {
	for _, c := range []struct {
		name  string
		p     EmptyArithSubscriptPolicy
		want  []string
		wrote bool
	}{
		{
			"the empty expression", EmptyArithSubscriptIsTheEmptyExpression,
			[]string{"same=0", "next=0 r=[Y 2 3]"},
			true,
		},
		{
			"reported", EmptyArithSubscriptIsReported,
			[]string{"read: `r[]': not a valid identifier", "same=1", "next=0 r=[1 2 3]"},
			false,
		},
		{
			"invalid", EmptyArithSubscriptIsInvalid,
			[]string{"not an identifier: r[]"},
			false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := emptyOpRun(t, c.p, Diagnostics{}, emptyOpReadSrc)
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("out %q is missing %q", out, want)
				}
			}
			if strings.Contains(out, "Y 2 3") != c.wrote {
				t.Errorf("out %q, want the element written: %v", out, c.wrote)
			}
		})
	}
}

// `printf -v` reaches the same refusal and carries **its own** status: bash
// counts the identical sentence as a usage error there and as a failure at
// `read`. See Diagnostics.BuiltinBadNameStatusFor.
func TestAnEmptySubscriptToPrintfCarriesPrintfsOwnBadNameStatus(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`printf -v 'a[]' X; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	out, _ := emptyOpRun(t, EmptyArithSubscriptIsReported,
		Diagnostics{BuiltinBadNameStatusFor: map[string]int{"printf": 2}}, src)
	if !strings.Contains(out, "printf: `a[]': not a valid identifier") {
		t.Errorf("out %q is not printf's bad-name refusal", out)
	}
	for _, want := range []string{"same=2", "next=0 a=[1 2 3]"} {
		if !strings.Contains(out, want) {
			t.Errorf("out %q is missing %q", out, want)
		}
	}
	// Without an entry the builtin takes the dialect's own number, which is
	// what `read` takes in the same column.
	out, _ = emptyOpRun(t, EmptyArithSubscriptIsReported, Diagnostics{}, src)
	if !strings.Contains(out, "same=1") {
		t.Errorf("out %q does not fall back to the dialect's status", out)
	}
}

// The operand is answered **ahead of the freeze**, which is measured: bash's
// and zsh's `readonly a; unset 'a[]'` never mention the freeze, and neither
// does either shell's `readonly r; read 'r[]'`. A good subscript still reaches
// it, which is the control row.
func TestAnEmptySubscriptOperandIsAnsweredAheadOfTheFreeze(t *testing.T) {
	const unsetSrc = "a=(1 2 3); readonly a\n" +
		`unset 'a[]'; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	out, _ := emptyOpRun(t, EmptyArithSubscriptIsReported, Diagnostics{}, unsetSrc)
	if strings.Contains(out, "readonly") || strings.Contains(out, "read only") {
		t.Errorf("out %q refused the frozen array where the brackets name no element", out)
	}
	if !strings.Contains(out, "same=0") || !strings.Contains(out, "a=[1 2 3]") {
		t.Errorf("out %q, want a silent 0 with the array whole", out)
	}
	// The control: a subscript that names an element does reach the freeze.
	const goodSrc = "a=(1 2 3); readonly a\n" +
		`unset 'a[0]'; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	out, _ = emptyOpRun(t, EmptyArithSubscriptIsReported, Diagnostics{}, goodSrc)
	if !strings.Contains(out, "readonly") {
		t.Errorf("out %q lost the freeze refusal for a subscript that names an element", out)
	}
	if strings.Contains(out, "a=[2 3]") {
		t.Errorf("out %q removed an element from a frozen array", out)
	}
	const readSrc = "r=(1 2 3); readonly r\n" +
		`{ read 'r[]'; echo "same=$?"; } <<EOF` + "\n" + "Y\nEOF\n" +
		`echo "next=$? r=[${r[*]}]"`
	out, _ = emptyOpRun(t, EmptyArithSubscriptIsReported, Diagnostics{}, readSrc)
	if !strings.Contains(out, "not a valid identifier") {
		t.Errorf("out %q is not the name refusal", out)
	}
	if strings.Contains(out, "onl") {
		t.Errorf("out %q refused the freeze in front of the name", out)
	}
}

// A **store** is answered in front of the table's key and a **delete** is not,
// which is measured rather than tidy: bash's and zsh's `read 'm[]'` on a table
// are refused where `unset 'm[]'` on the same table is silent at 0 — zsh keeps
// an empty key standing through it.
func TestAnEmptySubscriptToAStoreIsAnsweredAheadOfTheTablesKey(t *testing.T) {
	const readSrc = "typeset -A m; m[k]=v\n" +
		`{ read 'm[]'; echo "same=$?"; } <<EOF` + "\n" + "Y\nEOF\n" +
		`echo "next=$? n=${#m[@]}"`
	out, _ := emptyOpRun(t, EmptyArithSubscriptIsReported, Diagnostics{}, readSrc)
	if !strings.Contains(out, "not a valid identifier") {
		t.Errorf("out %q reached the table's key with an empty subscript", out)
	}
	if !strings.Contains(out, "n=1") {
		t.Errorf("out %q put a key in the table", out)
	}
	const unsetSrc = "typeset -A m; m[k]=v\n" +
		`unset 'm[]'; echo "same=$?"` + "\n" +
		`echo "next=$? n=${#m[@]}"`
	out, _ = emptyOpRun(t, EmptyArithSubscriptIsInvalid, Diagnostics{}, unsetSrc)
	if strings.Contains(out, "subscript") || strings.Contains(out, "identifier") {
		t.Errorf("out %q refused a table's empty key at a delete", out)
	}
	if !strings.Contains(out, "same=0") || !strings.Contains(out, "n=1") {
		t.Errorf("out %q, want a silent 0 with the table as it was", out)
	}
}

// A subscript that is **blank** rather than empty is the neighboring axis and
// keeps its own answer: measured, bash's `unset 'a[ ]'` removes element 0 where
// `unset 'a[]'` removes nothing.
func TestABlankOperandSubscriptIsNotAnEmptyOneAtTheseRoutes(t *testing.T) {
	src := "a=(1 2 3)\n" +
		`unset 'a[ ]'; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	out, _ := emptyOpRunWith(t, EmptyArithSubscriptIsReported, Diagnostics{},
		func(s *Semantics) { s.BlankArithSubscriptIsTheEmptyExpression = Yes }, src)
	if !strings.Contains(out, "a=[2 3]") {
		t.Errorf("out %q answered a blank subscript as an empty one", out)
	}
}

// And a subscript whose **text expanded** to nothing is a third construct,
// measured apart from this one: `i=; unset 'a[$i]'`, single-quoted so the `$i`
// reaches the builtin, removes element **zero** in bash 5.3.20 where
// `unset 'a[]'` removes nothing. Semantics.EmptySubscriptTextIsAMathError is
// that neighbor and this must not answer for it.
func TestASubscriptThatExpandedToNothingIsNotAWrittenEmptyOne(t *testing.T) {
	src := "a=(1 2 3); i=\n" +
		`unset 'a[$i]'; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	out, _ := emptyOpRun(t, EmptyArithSubscriptIsReported, Diagnostics{}, src)
	if !strings.Contains(out, "a=[2 3]") {
		t.Errorf("out %q answered an expanded emptiness as a written one", out)
	}
}

// An axis nobody answered is refused by name rather than guessed at, at both
// routes: one column acts on element zero, one acts on nothing and one refuses,
// and no two of those can stand in for each other.
func TestAnEmptySubscriptOperandRefusesAnUnspecifiedAxis(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"unset", emptyOpUnsetSrc},
		{"a store", emptyOpReadSrc},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := emptyOpRun(t, EmptyArithSubscriptUnspecified, Diagnostics{}, c.src)
			if !strings.Contains(out, "no dialect was chosen") {
				t.Errorf("out %q is not a refusal naming the axis", out)
			}
			if !strings.Contains(out, "same=2") {
				t.Errorf("out %q does not leave the refusal's status behind", out)
			}
		})
	}
}

func emptyOpRun(t *testing.T, p EmptyArithSubscriptPolicy, dg Diagnostics, src string) (string, int) {
	t.Helper()
	return emptyOpRunWith(t, p, dg, nil, src)
}

// emptyOpRunWith answers everything an operand's subscript needs except the
// axis under test, so a row varies that one alone.
func emptyOpRunWith(t *testing.T, p EmptyArithSubscriptPolicy, dg Diagnostics,
	also func(*Semantics), src string,
) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		// `printf -v` is an axis of its own, and a row about the store behind
		// it needs the option to exist before it can reach one.
		s.PrintfAssignsWithV = Yes
		// The two give-ups are the routes' own axes, answered flat here so
		// that a row varies the emptiness alone; one row below varies them.
		s.BadSubscriptToUnset = BadSubscriptReported
		s.BadSubscriptToAnOutputOperand = BadSubscriptReported
		// The neighboring emptinesses are answered flat for the same
		// reason: a subscript that is blank, and one whose text expanded to
		// nothing, are constructs of their own and measured apart from this.
		s.BlankArithSubscriptIsTheEmptyExpression = No
		s.EmptySubscriptTextIsAMathError = No
		s.EmptyArithSubscript = p
		// A give-up takes the dialect's own fatal status, so this is answered
		// for the reason the rows next door answer it.
		s.FatalErrorStatusIsOne = Yes
		if also != nil {
			also(s)
		}
	}, dg, src, RouteUnspecified)
}
