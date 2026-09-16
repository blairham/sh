// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `case` pattern carrying `=(` (#3040).
//
// A `(` straight after an `=` is an array literal and never a pattern group
// — where an *assignment* may be written. A `case` pattern is not such a
// place: no assignment may stand there, so the `=` is an ordinary character
// of the pattern and the `(` behind it is the group it looks like.
//
// The guard had been read everywhere but inside `[[ ]]`, which was too wide
// by exactly this position. A shipped completion function whose pattern
// captures on both sides of an `=` is what it stopped at: read as an array
// literal, the word ended at the `(` and the parenthesis was then an
// operator with nowhere to go.
//
// Measured 2026-09-15 on zsh 5.9.2, each probe in a script file of its own,
// against the subject `a=b`:
//
//	| probe                 | zsh 5.9.2   |
//	| `((a)=(b))`           | the arm runs |
//	| `(a=(b))`             | the arm runs |
//	| `(x(a)=(b))`          | no arm runs — the pattern parsed and missed |
//	| `((*)=(*))`           | the arm runs |
//
// The third row is the discriminator between "it parsed" and "it parsed as
// the right thing": a pattern that reached the matcher and did not match is
// a different outcome from a line that was refused, and a reading that
// dropped the group would have matched it.
//
// The other five columns have neither construct — no bare groups in a
// pattern and no array literal spelled this way — so there is no panel split
// to settle. This is one shell against our reading of it: a plain bug rather
// than an axis.

func casePatternAssignGrammar(d *Dialect) {
	// Bare groups in a pattern, which is what the `(` after the `=` is.
	d.PatternAlternation = true
	// And a group at the front of a word, for the rows whose pattern opens
	// with one.
	d.GlobQualifiers = true
	// The array literal this is told apart from.
	d.ArrayLiteral = true
}

func TestACasePatternMayCarryAnEqualsBeforeAGroup(t *testing.T) {
	d := Core()
	casePatternAssignGrammar(&d)
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"a group on each side", "case x in ((a)=(b)) :;; esac", []string{"(a)=(b)"}},
		{"a bare name and a group", "case x in (a=(b)) :;; esac", []string{"a=(b)"}},
		{"text in front of the first group", "case x in (x(a)=(b)) :;; esac", []string{"x(a)=(b)"}},
		{"stars in both groups", "case x in ((*)=(*)) :;; esac", []string{"(*)=(*)"}},
		{"an alternation inside the second", "case x in (a=(b|c)) :;; esac", []string{"a=(b|c)"}},
		{
			// The control that says the ordinary reading is untouched: a
			// pattern with an `=` and no group behind it never went near
			// this.
			"an equals with no group behind it",
			"case x in (a=b) :;; esac",
			[]string{"a=b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := firstArm(t, tc.src, d)
			if len(got) != len(tc.want) {
				t.Fatalf("%s: %d patterns %q, want %d %q", tc.src, len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%s: pattern %d is %q, want %q", tc.src, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestAnArrayLiteralIsStillAnArrayLiteral is the half the widening could have
// cost, and it is the case the guard was written for: where an assignment may
// be written, `a=(x y)` is two elements and not a word with a group in it.
func TestAnArrayLiteralIsStillAnArrayLiteral(t *testing.T) {
	d := Core()
	casePatternAssignGrammar(&d)
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"at command position", "a=(x y)", 2},
		{"with one element", "a=(x)", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, d)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
			if !ok {
				t.Fatalf("%s: first command is %T, want a simple command", tc.src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
			}
			if len(c.Assigns) != 1 {
				t.Fatalf("%s: %d assignments, want 1 — the `(` was not read as an array literal", tc.src, len(c.Assigns))
			}
			if !c.Assigns[0].IsArray {
				t.Fatalf("%s: the assignment is not an array literal", tc.src)
			}
			if got := len(c.Assigns[0].Elems); got != tc.want {
				t.Errorf("%s: %d elements, want %d", tc.src, got, tc.want)
			}
		})
	}
}

// TestAnArrayLiteralWinsOverTheGroupReading is the sharpest form of the same
// half, and the one an element count cannot make: where an assignment may be
// written, `a=(x|y)` is *refused* — the array literal takes the parentheses
// and an alternation is not an element. Measured on the shell this is read
// from, where the same characters in a `case` pattern are a group.
func TestAnArrayLiteralWinsOverTheGroupReading(t *testing.T) {
	d := Core()
	casePatternAssignGrammar(&d)
	if _, err := Parse("a=(x|y)", d); err == nil {
		t.Error("`a=(x|y)` parsed — the `(` after the `=` was read as a group where an assignment may stand")
	}
}
