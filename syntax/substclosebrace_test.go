// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// A `}` that begins a word inside a `$( )` body, read as the substitution
// runs, is the reserved word in the dialect that answers
// [Dialect.CloseBraceStartingAWordIsReservedInACommandSubstitution] — and with
// no brace group open in the body it is refused as one (#6960).
//
// Each body is the text an interpreter hands this package at expansion time:
// the inside of the parentheses, read with both facts it is told about it.

// closeBraceInBody is the grammar these rows need, named by the rule.
func closeBraceInBody() Dialect {
	d := Core()
	d.CloseBraceStartingAWordIsReservedInACommandSubstitution = true
	return d
}

// readBodyAsItRuns parses src as a `$( )` body read when the substitution
// runs, and returns what the read refused, if anything.
func readBodyAsItRuns(src string, d Dialect, asItRuns bool) error {
	p := NewParser(src, d)
	p.InsideASubstitution()
	p.InsideProgramParentheses()
	if asItRuns {
		p.ReadAsACommandSubstitutionRuns()
	}
	p.Parse()
	return p.Err()
}

func TestACloseBraceBeginningAWordInASubstitutionBodyIsReserved(t *testing.T) {
	for _, src := range []string{
		"echo }",
		"echo a }",
		"echo }a",
		"echo }}",
		`echo "a" }`,
		"echo a >}",
		"echo a>}",
		"cat <<}\nx\n}\n",
		"case } in *) echo y;; esac",
		"case x in }) echo y;; esac",
		"for i in }; do echo $i; done",
		"echo ${v:-} }",
		"echo a\n}",
		// A group that has closed takes nothing with it.
		"{ echo a; }; echo }",
		// And a subshell or a compound command is not a group.
		"( echo } )",
		"if :; then echo }; fi",
		"while :; do echo }; break; done",
	} {
		err := readBodyAsItRuns(src, closeBraceInBody(), true)
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnexpected || se.Token != "}" {
			t.Errorf("%q: %v, want `}' refused", src, err)
		}
	}
}

// The readings that keep the brace a word, each with the same body under the
// rule to show the rule is what it is up against.
func TestACloseBraceInASubstitutionBodyIsAWordWhereTheRuleDoesNotReach(t *testing.T) {
	for _, src := range []string{
		// A brace group open in the body, a function's included.
		"{ echo }; }",
		"{ echo }a; }",
		"{ echo } }; }",
		"{ { echo }; }; echo }; }",
		"{ ( echo } ); }",
		"( { echo }; } )",
		"f() { echo }; }; f",
		"{ case x in x) echo };; esac; }",
		// Not at the start of the word, or not bare.
		"echo a}",
		"echo {a}}",
		"echo x=}",
		"x=}; echo $x",
		`echo \}`,
		`echo "}"`,
		"echo ''}",
		"echo $v}",
		`echo a\ }`,
		// Not a word at all.
		"echo a # }\n",
		"cat <<E\n}\nE\n",
	} {
		if err := readBodyAsItRuns(src, closeBraceInBody(), true); err != nil {
			t.Errorf("%q: %v, want it read", src, err)
		}
	}
	for _, src := range []string{"echo }", "echo a >}", "case } in *) :;; esac"} {
		// The read with the line, which never asks: the refusal is the
		// running read's alone.
		if err := readBodyAsItRuns(src, closeBraceInBody(), false); err != nil {
			t.Errorf("%q read with its line: %v, want it read", src, err)
		}
		// And a dialect that does not answer the question.
		if err := readBodyAsItRuns(src, Core(), true); err != nil {
			t.Errorf("%q without the rule: %v, want it read", src, err)
		}
	}
}
