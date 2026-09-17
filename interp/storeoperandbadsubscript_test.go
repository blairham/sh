// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// storeOperandSrc is the pair of lines the give-up rows are measured over: a
// `read` whose subscript will not evaluate, something after it on the same
// line, and something on the line after that.
//
// A group rather than a pipeline, because a pipeline's element already runs
// somewhere a give-up unwinds out of — so the two answers that give something
// up would print the same thing, which is the trap the whole of #3485 is
// about. The here-document is what feeds `read` without one.
const storeOperandSrc = "r=(1 2 3)\n" +
	`{ read 'r[1/0]'; echo "same=$?"; } <<EOF` + "\n" +
	"Y\nEOF\n" +
	`echo "next=$? r=[${r[*]}]"`

// The store a subscripted operand walks into divides the panel three ways,
// and it does not divide it where `unset` does: the column that leaves a
// failed builtin behind for `unset` ends the script here.
//
// It ended the script for **everybody** before this, which is the bug: a
// `read 'r[i]'` whose computed subscript came out blank stopped a script that
// two of the three columns run to the end, with no trap and no `||` able to
// see it.
func TestAStoreThroughAnOperandGivesUpAsMuchAsTheDialectDoes(t *testing.T) {
	for _, c := range []struct {
		name   string
		giveUp BadSubscriptPolicy
		want   []string
		absent []string
	}{
		// The array is untouched under all three, and the value never
		// arrives: `read` gives up at the name it could not resolve.
		{
			"a failed builtin", BadSubscriptReported,
			[]string{"same=1", "next=0 r=[1 2 3]"},
			nil,
		},
		{
			"the command and its line", BadSubscriptAbandonsTheCommand,
			[]string{"next=1 r=[1 2 3]"},
			[]string{"same="},
		},
		{
			"the script", BadSubscriptEndsTheScript,
			nil,
			[]string{"same=", "next="},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeOperandRun(t, c.giveUp, Diagnostics{}, storeOperandSrc)
			if !strings.Contains(out, "1/0") && !strings.Contains(out, "division") {
				t.Errorf("output %q says nothing about the expression", out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("output %q is missing %q", out, want)
				}
			}
			for _, absent := range c.absent {
				if strings.Contains(out, absent) {
					t.Errorf("output %q ran %q, which this answer gives up", out, absent)
				}
			}
		})
	}
}

// The names behind the one that failed keep what they held, which is the half
// a status alone cannot show: the builtin gives up at the bad operand rather
// than filling the rest from the line it already read.
func TestAStoreThroughAnOperandGivesUpTheNamesBehindIt(t *testing.T) {
	const src = "r=(1 2 3)\nb=preset\n" +
		`{ read 'r[1/0]' b; echo "same=$?"; } <<EOF` + "\n" +
		"x y\nEOF\n" +
		`echo "next=$? b=[$b]"`
	out, _ := storeOperandRun(t, BadSubscriptReported, Diagnostics{}, src)
	if !strings.Contains(out, "same=1") {
		t.Errorf("output %q does not report the failed builtin", out)
	}
	if !strings.Contains(out, "b=[preset]") {
		t.Errorf("output %q filled a name behind the one that failed", out)
	}
}

// `printf -v` reaches the same store, so it answers the same axis — and its
// own status is the store's rather than the formatting's, which reported 0
// over a write that never happened.
func TestPrintfIntoAnOperandGivesUpTheSameWay(t *testing.T) {
	const src = "r=(1 2 3)\n" +
		`printf -v 'r[1/0]' %s Q; echo "same=$?"` + "\n" +
		`echo "next=$? r=[${r[*]}]"`
	out, _ := storeOperandRun(t, BadSubscriptReported, Diagnostics{}, src)
	if !strings.Contains(out, "same=1") {
		t.Errorf("output %q does not report the failed builtin", out)
	}
	if !strings.Contains(out, "r=[1 2 3]") {
		t.Errorf("output %q wrote through a subscript that would not evaluate", out)
	}
	out, _ = storeOperandRun(t, BadSubscriptAbandonsTheCommand, Diagnostics{}, src)
	if strings.Contains(out, "same=") {
		t.Errorf("output %q ran the rest of the line the command gave up", out)
	}
	if !strings.Contains(out, "next=1") {
		t.Errorf("output %q does not leave the give-up's status behind", out)
	}
}

// The answer that gives up a command gives up a **command string** whole,
// exactly as it does one builtin over.
func TestAStoreGivingUpACommandGivesUpACommandStringWhole(t *testing.T) {
	out, st := storeOperandRunAs(t, BadSubscriptAbandonsTheCommand, Diagnostics{},
		storeOperandSrc, RouteCommandString)
	if strings.Contains(out, "next=") {
		t.Errorf("output %q ran on past the failure", out)
	}
	if st == 0 {
		t.Error("status 0, want a failure")
	}
}

// An axis nobody answered is refused by name rather than guessed, and the
// sentence about the subscript is not written under the refusal: a shell that
// was never told what to do here has not decided to complain.
func TestAStoreThroughAnOperandRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := storeOperandRun(t, BadSubscriptUnspecified, Diagnostics{}, storeOperandSrc)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("output %q is not a refusal naming the axis", out)
	}
	if strings.Contains(out, "division") {
		t.Errorf("output %q wrote the complaint under the refusal", out)
	}
	if !strings.Contains(out, "same=2") {
		t.Errorf("output %q does not leave the refusal's status behind", out)
	}
}

// One column names the builtin that was handed the operand in front of the
// sentence, where the other two write the sentence the language writes about
// the same text anywhere else.
func TestAStoreThroughAnOperandNamesTheBuiltinWhereTheDialectDoes(t *testing.T) {
	out, _ := storeOperandRun(t, BadSubscriptReported,
		Diagnostics{StoreOperandBadSubscript: "%[1]s: %[2]s"}, storeOperandSrc)
	if !strings.Contains(out, "read: ") {
		t.Errorf("output %q does not name the builtin", out)
	}
	out, _ = storeOperandRun(t, BadSubscriptReported, Diagnostics{}, storeOperandSrc)
	if strings.Contains(out, "read: ") {
		t.Errorf("output %q names the builtin where the dialect does not", out)
	}
}

func storeOperandRun(t *testing.T, p BadSubscriptPolicy, dg Diagnostics, src string) (string, int) {
	t.Helper()
	return storeOperandRunAs(t, p, dg, src, RouteUnspecified)
}

// storeOperandRunAs answers everything the store walk needs except the axis
// under test, so a row varies that one alone.
func storeOperandRunAs(t *testing.T, p BadSubscriptPolicy, dg Diagnostics, src string, route Route) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		// `printf -v` is an axis of its own, and a row about the store
		// behind it needs the option to exist before it can reach one.
		s.PrintfAssignsWithV = Yes
		s.BadSubscriptToAnOutputOperand = p
		// The give-up takes the dialect's own fatal status, so this is
		// answered for the same reason the `unset` rows answer it.
		s.FatalErrorStatusIsOne = Yes
	}, dg, src, route)
}
