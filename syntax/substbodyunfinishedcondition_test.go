// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `$( … )` body refused at the **closing parenthesis** refuses the line that
// holds it, and one shape is carved back out of that where a dialect says so:
// an `if` or `elif` still short of its `then`. See
// [Dialect.SubstitutionBodyRefusesAnUnfinishedCondition], where the grid is,
// and [Lexer.bodyRefusalRefusesTheLine], which is the rule.

// closerLeavesAnUnfinishedConditionAlone is the state in which a condition
// short of its `then` is the one body the closer does not refuse the line for.
func closerLeavesAnUnfinishedConditionAlone() Dialect {
	d := Core()
	d.SubstitutionBodyRefusalEndsTheRead = true
	d.SubstitutionBodyRefusesAnUnfinishedCondition = false
	return d
}

// closerRefusesAnUnfinishedCondition differs in that one field.
func closerRefusesAnUnfinishedCondition() Dialect {
	d := closerLeavesAnUnfinishedConditionAlone()
	d.SubstitutionBodyRefusesAnUnfinishedCondition = true
	return d
}

func TestAnUnfinishedConditionSettlesTheReadWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		// refusedEitherWay is whether the line is refused with the flag
		// *off*. The flag only ever adds, so a row true here is true with it
		// on as well; a row false here is what `settled` then answers.
		refusedEitherWay bool
		// settled is whether the line is refused once the flag is on.
		settled bool
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
		// are an unfinished `if` **inside** something — which the flag moves
		// nothing about, because what decides is the outermost construct. A
		// rule about "an unfinished construct" takes all three the same way
		// as the rows above and is wrong about every one of them.
		//
		// **They are refused either way since #4859**, which is the rule
		// this flag is carved out of rather than the flag: a body the
		// grammar refused at the closer is the line's answer, so the text
		// written in front of the substitution never runs. Measured
		// 2026-09-27 on zsh 5.9.2 over `echo b; v=$(X); echo a` as a script
		// file, with `setopt shortloops` and with `unsetopt shortloops` on
		// the line above it — no `b` in any of the six columns.
		{"a then already read", "if true; then", true, true},
		{"an if inside a then", "if true; then if", true, true},
		{"an if inside a loop's body", "while true; do if", true, true},
		// And the rest of the closer's population, which neither state
		// moves and which was #4859's opening measurement: `for`, `case`,
		// `{` and `echo |` are refused with the line in that shell under
		// every emulation mode.
		{"a for with no name", "for", true, true},
		{"a case with no in", "case x", true, true},
		{"a brace group", "{", true, true},
		{"a pipeline wanting a command", "echo |", true, true},
		// The control on the other side: a body with nothing wrong in it is
		// untouched in both states.
		{"a body that parses", "echo hi", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "echo b; v=$(" + tc.body + "); echo a\n"
			_, off := Parse(src, closerLeavesAnUnfinishedConditionAlone())
			if got := off != nil; got != tc.refusedEitherWay {
				t.Errorf("with the flag off: refused=%v (%v), want %v", got, off, tc.refusedEitherWay)
			}
			_, on := Parse(src, closerRefusesAnUnfinishedCondition())
			if got := on != nil; got != tc.settled {
				t.Errorf("with the flag on: refused=%v (%v), want %v", got, on, tc.settled)
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
		for i, d := range []Dialect{closerLeavesAnUnfinishedConditionAlone(), closerRefusesAnUnfinishedCondition()} {
			if _, err := Parse(src, d); err == nil {
				t.Errorf("$(%s) parsed under dialect %d, so the rule this is carved out of is not in force",
					body, i)
			}
		}
	}
}
