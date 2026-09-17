// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// declSubSrc is the pair of lines the give-up rows are measured over: a
// declaration whose operand's subscript will not evaluate, something after it
// on the same line, and something on the line *after* that.
//
// Two lines on purpose. On one line "gave up the rest of the line" and "ended
// the script" print the same nothing, which is how bash was recorded as
// ending a script it runs to the end at the two neighbouring sites (#3485),
// and this site was left ending the script for everybody on the same reading.
const declSubSrc = "a=(1 2 3)\n" +
	`typeset 'a[b c]'=v; echo "same=$?"` + "\n" +
	`echo "next=$? a=[${a[*]}]"`

// A declaration is the third site of the one failure, and the panel splits at
// it a third way: see Semantics.BadSubscriptToADeclaration.
//
// It reached Runner.fatal directly before this, so a script bash runs to the
// end stopped here — the same shape #3485 found one builtin over, at a site
// #3485 did not cover.
func TestADeclarationGivesUpAsMuchAsTheDialectDoes(t *testing.T) {
	for _, c := range []struct {
		name   string
		giveUp BadSubscriptPolicy
		want   []string
		absent []string
	}{
		// The array is untouched under all three: nothing is written through
		// a subscript that would not evaluate.
		{
			"a failed builtin", BadSubscriptReported,
			[]string{"same=1", "next=0 a=[1 2 3]"},
			nil,
		},
		{
			"the command and its line", BadSubscriptAbandonsTheCommand,
			[]string{"next=1 a=[1 2 3]"},
			[]string{"same="},
		},
		{
			"the script", BadSubscriptEndsTheScript,
			nil,
			[]string{"same=", "next="},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := declSubRun(t, c.giveUp, declSubSrc)
			if !strings.Contains(out, "b c") {
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

// Giving up the command gives up whatever the command is *inside* — the whole
// function, list, `if`, loop or subshell — and the script carries on at the
// next top-level command with the give-up's status behind it.
//
// Measured 2026-09-17 at every one of these in bash 5.3.20 and 3.2.57: none
// of them reaches a second command and the line after each of them runs.
func TestADeclarationGivingUpACommandUnwindsOutOfAConstruct(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"an && list", `typeset 'a[b c]'=v && echo yes || echo no`},
		{"an if condition", `if typeset 'a[b c]'=v; then echo t; else echo f; fi`},
		{"a loop body", `while :; do typeset 'a[b c]'=v; echo body; break; done`},
		{"a subshell", `( typeset 'a[b c]'=v; echo x )`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "a=(1 2 3)\n" + c.src + "\n" + `echo "next=$? a=[${a[*]}]"`
			out, st := declSubRun(t, BadSubscriptAbandonsTheCommand, src)
			for _, absent := range []string{"yes", "no", "t\n", "f\n", "body", "x\n"} {
				if strings.Contains(out, absent) {
					t.Errorf("output %q ran %q inside the construct that was given up", out, absent)
				}
			}
			if !strings.Contains(out, "next=1 a=[1 2 3]") {
				t.Errorf("output %q does not carry on at the next line with the give-up's status", out)
			}
			if st != 0 {
				t.Errorf("status %d, want the script to finish at the last command's 0", st)
			}
		})
	}
}

// A command substitution is the one shape where the enclosing *assignment*
// still completes: measured, `v=$( typeset 'a[b c]'=v; echo x )` leaves `v`
// empty at 1 in bash and the script runs on.
func TestADeclarationGivingUpACommandLeavesTheAssignmentAroundIt(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`v=$( typeset 'a[b c]'=q; echo x )` + "\n" +
		`echo "assigned=$? v=[$v]"`
	out, _ := declSubRun(t, BadSubscriptAbandonsTheCommand, src)
	if !strings.Contains(out, "assigned=1 v=[]") {
		t.Errorf("output %q does not complete the assignment at the give-up's status", out)
	}
}

// The answer that gives up a command gives up a **command string** whole,
// exactly as it does at the two sites next door. Only the column that
// abandons can reach the question, so it is written on the constant rather
// than asked as an axis of its own.
func TestADeclarationGivingUpACommandGivesUpACommandStringWhole(t *testing.T) {
	out, st := declSubRunAs(t, BadSubscriptAbandonsTheCommand, declSubSrc, RouteCommandString)
	if strings.Contains(out, "next=") {
		t.Errorf("output %q ran on past the failure", out)
	}
	if st == 0 {
		t.Error("status 0, want a failure")
	}
}

// `local` reaches the same declaration and so answers the same axis — which
// is what the column that abandons does with it: measured, a `local
// 'a[b c]'=v` inside a function gives up the function whole and the script
// goes on.
func TestALocalDeclarationGivesUpTheSameWay(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`f() { local 'a[b c]'=v; echo inside; }` + "\n" +
		"f\n" +
		`echo "next=$? a=[${a[*]}]"`
	out, _ := declSubRun(t, BadSubscriptAbandonsTheCommand, src)
	if strings.Contains(out, "inside") {
		t.Errorf("output %q ran on inside the function that was given up", out)
	}
	if !strings.Contains(out, "next=1 a=[1 2 3]") {
		t.Errorf("output %q does not carry on at the next line with the give-up's status", out)
	}
	out, _ = declSubRun(t, BadSubscriptEndsTheScript, src)
	if strings.Contains(out, "next=") {
		t.Errorf("output %q ran on past a give-up that ends the script", out)
	}
}

// `readonly` and `export` reach it too, in the dialects whose declaration
// builtins take a subscript at all — the axis is about the declaration and
// not about which word spells it.
func TestAFrozenOrExportedElementGivesUpTheSameWay(t *testing.T) {
	for _, name := range []string{"readonly", "export"} {
		t.Run(name, func(t *testing.T) {
			src := "a=(1 2 3)\n" + name + ` 'a[b c]'=v; echo "same=$?"` + "\n" +
				`echo "next=$? a=[${a[*]}]"`
			out, _ := declSubRun(t, BadSubscriptAbandonsTheCommand, src)
			if strings.Contains(out, "same=") {
				t.Errorf("output %q ran the rest of the line the command gave up", out)
			}
			if !strings.Contains(out, "next=1 a=[1 2 3]") {
				t.Errorf("output %q does not carry on at the next line with the give-up's status", out)
			}
			out, _ = declSubRun(t, BadSubscriptEndsTheScript, src)
			if strings.Contains(out, "next=") {
				t.Errorf("output %q ran on past a give-up that ends the script", out)
			}
		})
	}
}

// An axis nobody answered is refused by name rather than guessed, and the
// sentence about the subscript is not written under the refusal: a shell that
// was never told what to do here has not decided to complain about it.
func TestADeclarationRefusesAnUnspecifiedBadSubscriptAxis(t *testing.T) {
	out, _ := declSubRun(t, BadSubscriptUnspecified, declSubSrc)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("output %q is not a refusal naming the axis", out)
	}
	if strings.Contains(out, "arithmetic") || strings.Contains(out, "expression") {
		t.Errorf("output %q wrote the complaint under the refusal", out)
	}
	if !strings.Contains(out, "same=2") {
		t.Errorf("output %q does not leave the refusal's status behind", out)
	}
}

func declSubRun(t *testing.T, p BadSubscriptPolicy, src string) (string, int) {
	t.Helper()
	return declSubRunAs(t, p, src, RouteUnspecified)
}

// declSubRunAs answers everything a declaration of an element needs except
// the axis under test, so a row varies that one alone.
func declSubRunAs(t *testing.T, p BadSubscriptPolicy, src string, route Route) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		// The two builtin-strictness axes: `typeset`/`declare`/`local` and
		// `readonly`/`export` are asked separately, and a row here needs
		// both to reach the subscript at all.
		s.TypesetTakesASubscript = Yes
		s.DeclarationTakesASubscript = Yes
		s.SubscriptedOperandTakesALocalDeclaration = Yes
		s.ReadonlyElement = ReadonlyElementWritten
		// Not what these rows are about, and answered flat so an unanswered
		// axis cannot stand in for the give-up a row is looking for.
		s.DeclaredNameWithoutValueIsEmpty = No
		s.TypesetLocalNeedsKeywordFunction = No
		s.CompoundElementsGoThroughTheAttribute = Yes
		s.BadSubscriptToADeclaration = p
		// The give-up takes the dialect's own fatal status, so this is
		// answered for the same reason the two sibling sites answer it.
		s.FatalErrorStatusIsOne = Yes
	}, Diagnostics{}, src, route)
}
