// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `$( … )` body whose read stopped at the **closing parenthesis** leaves the
// construct closed, and one shape is carved back out of that where a dialect
// says so: an `if` or `elif` still short of its `then`. See
// [Dialect.SubstitutionBodyRefusesAnUnfinishedCondition], where the grid is.

// closerLeavesTheConstructClosed is every dialect's answer: the closer is the
// token the read was looking for, so finding it there is the body ending.
func closerLeavesTheConstructClosed() Dialect {
	d := Core()
	d.SubstitutionBodyRefusalEndsTheRead = true
	d.SubstitutionBodyRefusesAnUnfinishedCondition = false
	return d
}

// closerRefusesAnUnfinishedCondition differs in that one field.
func closerRefusesAnUnfinishedCondition() Dialect {
	d := closerLeavesTheConstructClosed()
	d.SubstitutionBodyRefusesAnUnfinishedCondition = true
	return d
}

func TestAnUnfinishedConditionSettlesTheReadWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		// settled is whether the line is refused once the flag is on. Every
		// row reads with it off, which is the control on each of them.
		settled bool
	}{
		// `if true &&` belongs to this list too and is in the dialect's own
		// test instead: whether a dangling `&&` ends the body at all is
		// another dialect's answer, and a fixture that got it wrong would
		// fail this row for a reason that is not this flag.
		{"the keyword alone", "if", true},
		{"a condition with nothing after it", "if true", true},
		{"a condition that is itself an if", "if if true", true},
		{"an elif's condition", "if true; then :; elif", true},
		// The `then` is the line, and the nesting does not move it: the
		// first is the same construct one keyword later, and the last two
		// are an unfinished `if` **inside** something — which settles
		// nothing, because what decides is the outermost construct. A rule
		// about "an unfinished construct" takes all three the same way as
		// the rows above and is wrong about every one of them.
		{"a then already read", "if true; then", false},
		{"an if inside a then", "if true; then if", false},
		{"an if inside a loop's body", "while true; do if", false},
		// And the rest of the closer's population, which neither state moves:
		// it is the carve-out this sits inside and is #4859's.
		{"a for with no name", "for", false},
		{"a case with no in", "case x", false},
		{"a brace group", "{", false},
		{"a pipeline wanting a command", "echo |", false},
		// The control on the other side: a body with nothing wrong in it is
		// untouched in both states.
		{"a body that parses", "echo hi", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "echo b; v=$(" + tc.body + "); echo a\n"
			if _, err := Parse(src, closerLeavesTheConstructClosed()); err != nil {
				t.Errorf("the dialect that leaves the construct closed refused the line: %v", err)
			}
			_, err := Parse(src, closerRefusesAnUnfinishedCondition())
			if got := err != nil; got != tc.settled {
				t.Errorf("with the flag on: refused=%v (%v), want %v", got, err, tc.settled)
			}
		})
	}
}

// And the flag is asked only where the refusal is on the closer: a body
// refused at a token settles the read in both states, which is the rule this
// one is carved out of.
func TestAnUnfinishedConditionDoesNotWidenTheRuleItSitsIn(t *testing.T) {
	for _, body := range []string{"&&", ";;", "fi", "done", "esac"} {
		src := "echo b; v=$(" + body + "); echo a\n"
		for i, d := range []Dialect{closerLeavesTheConstructClosed(), closerRefusesAnUnfinishedCondition()} {
			if _, err := Parse(src, d); err == nil {
				t.Errorf("$(%s) parsed under dialect %d, so the rule this is carved out of is not in force",
					body, i)
			}
		}
	}
}
