// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `$( … )` body refused at the **closing parenthesis** settles the read like
// any other token, and one shape is carved back out of that where a dialect
// says so: an `if` or `elif` still short of its `then`. See
// [Dialect.SubstitutionBodyRefusesAnUnfinishedCondition], where the grid is.

// closerDefersAnUnfinishedCondition is the reading of the shell whose
// short-body option is on: the one shape is left to the moment it runs.
func closerDefersAnUnfinishedCondition() Dialect {
	d := Core()
	d.SubstitutionBodyRefusalEndsTheRead = true
	d.SubstitutionBodyRefusesAnUnfinishedCondition = false
	return d
}

// closerRefusesAnUnfinishedCondition differs in that one field, and settles
// the one shape with everything else.
func closerRefusesAnUnfinishedCondition() Dialect {
	d := closerDefersAnUnfinishedCondition()
	d.SubstitutionBodyRefusesAnUnfinishedCondition = true
	return d
}

func TestAnUnfinishedConditionSettlesTheReadWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		// refused with the flag off and with it on. The flag moves a row only
		// where the two differ, and that is the whole population it reaches.
		off, on bool
	}{
		// `if true &&` belongs to this list too and is in the dialect's own
		// test instead: whether a dangling `&&` ends the body at all is
		// another dialect's answer, and a fixture that got it wrong would
		// fail this row for a reason that is not this flag.
		{"the keyword alone", "if", false, true},
		{"a condition with nothing after it", "if true", false, true},
		{"a condition that is itself an if", "if if true", false, true},
		{"an elif's condition", "if true; then :; elif", false, true},
		// The `then` is the line, and the nesting does not move it: the
		// first is the same construct one keyword later, and the last two
		// are an unfinished `if` **inside** something — which the flag does
		// not reach, because what decides is the outermost construct. A rule
		// about "an unfinished construct" takes all three the same way as
		// the rows above and is wrong about every one of them.
		{"a then already read", "if true; then", true, true},
		{"an if inside a then", "if true; then if", true, true},
		{"an if inside a loop's body", "while true; do if", true, true},
		// And the rest of the closer's population, which settles in both
		// states: a refusal on the closer is the body's like any other, and
		// the rows above are the one shape carved back out of that.
		{"a for with no name", "for", true, true},
		{"a case with no in", "case x", true, true},
		{"a brace group", "{", true, true},
		{"a pipeline wanting a command", "echo |", true, true},
		// The control on the other side: a body with nothing wrong in it is
		// untouched in both states, so a flag that had broken the read
		// rather than settled it would show here and nowhere else.
		{"a body that parses", "echo hi", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "echo b; v=$(" + tc.body + "); echo a\n"
			if _, err := Parse(src, closerDefersAnUnfinishedCondition()); (err != nil) != tc.off {
				t.Errorf("with the flag off: refused=%v, want %v (%v)", err != nil, tc.off, err)
			}
			if _, err := Parse(src, closerRefusesAnUnfinishedCondition()); (err != nil) != tc.on {
				t.Errorf("with the flag on: refused=%v, want %v (%v)", err != nil, tc.on, err)
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
		for i, d := range []Dialect{closerDefersAnUnfinishedCondition(), closerRefusesAnUnfinishedCondition()} {
			if _, err := Parse(src, d); err == nil {
				t.Errorf("$(%s) parsed under dialect %d, so the rule this is carved out of is not in force",
					body, i)
			}
		}
	}
}
