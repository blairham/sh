// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Four causes behind the five shapes of #3898, each named by the flag it
// belongs to rather than by the shell that holds the flag. What the reference
// prints for every row is in dialect/zsh/zshacceptedshapes_test.go.
//
// They were filed as five parse errors and they are not five bugs: the first
// two shapes share one cause and the other three have one each.

// shapesDialect is the core with the flags these causes live under: the
// reserved `}`, the short-form body, the lenient and-or, and the declaration
// utilities whose operand is an assignment.
func shapesDialect() Dialect {
	d := Core()
	d.CloseBraceAlwaysReserved = true
	d.ShortForm = true
	d.OpenEndedAndOr = true
	// Two more the joining-operator rows need: the second pipeline spelling,
	// and the second short-form loop.
	d.PipeBothStreams = true
	d.Repeat = true
	d.DeclarationUtilities = map[string]bool{"typeset": true, "export": true, "readonly": true}
	return d
}

// A `}` that ends a declaration utility's operand is text, because the operand
// is an assignment — which is the carve-out [Dialect.CloseBraceAlwaysReserved]
// already makes for one written as a command of its own.
func TestACloseBraceEndsADeclarationOperand(t *testing.T) {
	t.Parallel()
	d := shapesDialect()
	for _, src := range []string{
		`export z=a}`,
		`typeset z=a}`,
		`readonly z=a}`,
		`export z=a} w=b}`,
		`export z=}`,
		// The bare assignment, which worked before and must still: it is the
		// row that says what moved is the *position* rather than the brace.
		`z=a}`,
	} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	// And the rows that must keep refusing, which are what make this the
	// assignment rather than the command word. An operand that is not an
	// assignment, a utility that takes none, and an ordinary argument behind
	// a prefix are all still the reserved word.
	for _, src := range []string{
		`export -- a}`,
		`echo z=a}`,
		`x=1 echo a}`,
		`echo a}`,
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, want the reserved `}`", src)
		}
	}
}

// A `}` inside a parenthesized `case` pattern is text. The parentheses are
// what decide it, not the `case`.
func TestACloseBraceInsideAParenthesizedCasePattern(t *testing.T) {
	t.Parallel()
	d := shapesDialect()
	for _, src := range []string{
		`case x in (a}) echo P;; (*) echo D;; esac`,
		`case x in (a}|b) echo P;; (*) echo D;; esac`,
		// Already accepted before, and kept: the brace mid-word, and a
		// pattern that is nothing but one.
		`case x in (a}b) echo P;; esac`,
		`case x in (}) echo P;; esac`,
	} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	// The control: an arm written without the opening parenthesis keeps the
	// reserved reading, so this is the paren list and not the `case`.
	if _, err := Parse(`case x in a}) echo P;; *) echo D;; esac`, d); err == nil {
		t.Errorf("a bare case arm parsed, want the reserved `}`")
	}
}

// An and-or list may end on its operator where a substitution's body ends,
// because the closing delimiter is cut off before the body is parsed and
// [Parser.atListEnd] therefore never sees it.
//
// [Parser.InsideASubstitution] is what tells this parser the text it has is
// such a body; without it the end of the input is the end of a program, which
// is the route question [Dialect.OpenEndedAndOr] parks.
func TestAnAndOrMayEndOnItsOperatorWhereASubstitutionDoes(t *testing.T) {
	t.Parallel()
	d := shapesDialect()
	body := func(src string) error {
		p := NewParser(src, d)
		p.InsideASubstitution()
		p.Parse()
		return p.Err()
	}
	for _, src := range []string{`echo x &&`, `echo x ||`, `echo hi; echo x &&`, ` : || `} {
		if err := body(src); err != nil {
			t.Errorf("as a substitution body, %q: %v", src, err)
		}
		// The same text read as a program is still unfinished, which is the
		// half nothing here answers.
		if _, err := Parse(src, d); err == nil {
			t.Errorf("as a program, %q parsed — the route question is not this one", src)
		}
	}
	// A pipeline never takes it, in any shell, and neither does a stray
	// terminator: two rows that must still be refused as a body.
	for _, src := range []string{`echo x |`, `echo x ;;`} {
		if err := body(src); err == nil {
			t.Errorf("as a substitution body, %q parsed, want a refusal", src)
		}
	}
	// And the flag is what it hangs on.
	off := shapesDialect()
	off.OpenEndedAndOr = false
	p := NewParser(`echo x &&`, off)
	p.InsideASubstitution()
	p.Parse()
	if p.Err() == nil {
		t.Errorf("with OpenEndedAndOr off, a body ending on its operator parsed")
	}
}

// A short-form body may be empty where the token standing in its place is one
// that joins two commands, so the construct is the operator's left-hand side.
func TestAShortFormBodyMayBeEmptyBeforeAJoiningOperator(t *testing.T) {
	t.Parallel()
	d := shapesDialect()
	for _, src := range []string{
		`select o in a b c; | cat`,
		`select o in a b c; && echo x`,
		`for i in a b; | cat`,
		`for i in a b; |& cat`,
		`for i in a b; || echo x`,
		`repeat 2; | cat`,
	} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	// Four controls. The two tokens that are *not* joining operators are
	// still refused there; a body that is really written is still read; and
	// the **long** form is not lenient, which is what says what moved is a
	// body nobody wrote rather than what a bar may follow.
	for _, src := range []string{
		`for i in a b; & echo x`,
		`for i in a b; ;; echo x`,
		`for i in a b; do :; done; | cat`,
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, want a refusal", src)
		}
	}
	if _, err := Parse(`for i in a b; echo $i`, d); err != nil {
		t.Errorf("a short body that is written: %v", err)
	}
}
