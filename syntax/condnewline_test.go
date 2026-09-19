// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A newline inside `[[ ]]` continues the condition rather than ending a
// command, at every point where the grammar is still waiting for something.
//
// Found by the wild sweep in two unrelated files — bats-core's tracing.bash
// and vim's shell syntax test inputs — which is what makes it ordinary
// formatting rather than a curiosity. Nothing multi-line parsed before: not
// only after `&&`, but after `[[` itself and before `]]`.
func TestANewlineContinuesACondition(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.DoubleBracket = true
	for _, c := range []struct{ name, src string }{
		{"after &&", "[[ 1 == 1 &&\n2 == 2 ]]"},
		{"after ||", "[[ 1 == 2 ||\n2 == 2 ]]"},
		{"after [[", "[[\n1 == 1 ]]"},
		{"before ]]", "[[ 1 == 1\n]]"},
		{"after !", "[[ !\n1 == 2 ]]"},
		{"after a group opens", "[[ (\n1 == 1 ) ]]"},
		{"before a group closes", "[[ ( 1 == 1\n) ]]"},
		{"a group spanning lines", "[[ ( 1 == 1 &&\n2 == 2 ) ]]"},
		{"several at once", "[[\n1 == 1 &&\n(\n2 == 2 ||\n3 == 3\n)\n]]"},
		// Blank lines between, not just one newline.
		{"more than one newline", "[[ 1 == 1 &&\n\n\n2 == 2 ]]"},
		// And the other side of the operator: a condition that is complete
		// at the end of a line, with the `&&` leading the continuation. It
		// is the shape a real prompt theme is written in, and the one this
		// list was missing — the newline has to be skipped before it is
		// known whether an operator or the `]]` follows it.
		{"before &&", "[[ -n x\n&& -z \"\" ]]"},
		{"before ||", "[[ -n x\n|| -z q ]]"},
		{"before && indented", "[[ -n x\n      && -z \"\" ]]"},
		{"before && with a blank line", "[[ -n x\n\n&& -z \"\" ]]"},
		{"before && then !", "[[ -n x\n&& ! -z q ]]"},
		{"before && three deep", "[[ -n x\n&& -n y\n&& -n z ]]"},
		{"before || after &&", "[[ -n x && -n y\n|| -z q ]]"},
		{"before && inside a group", "[[ ( -n x\n&& -z \"\" ) ]]"},
		{"a comment then the operator", "[[ -n x # c\n&& -z \"\" ]]"},
		{"both sides of the operator", "[[ -n x\n&&\n-z \"\" ]]"},
		// A closed group with the newline outside it, which is the same
		// speculative position one level up: the `)` is complete and either
		// an operator or the `]]` may follow.
		{"after a group closes", "[[ ( -n x )\n]]"},
		{"after a group closes then &&", "[[ ( -n x )\n&& -n y ]]"},
		{"nested groups closing across lines", "[[ ( ( -n x )\n)\n]]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, d)
			p.Parse()
			if err := p.Err(); err != nil {
				t.Errorf("parse %q: %v", c.src, err)
			}
		})
	}
}

// Not around a binary operator, on either side. `[[ 1 ==` and `[[ 1` each
// followed by a newline are errors in bash and ksh93, and only zsh takes
// them; refused is what the two agree on.
//
// Both sides, because they are separate places in the parser: one is the
// operator having nothing after it, the other is the left operand having no
// operator yet. Testing only the first left the second ungraded.
func TestANewlineDoesNotSurroundABinaryOperator(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.DoubleBracket = true
	for _, c := range []struct{ name, src string }{
		{"after the operator", "[[ 1 ==\n1 ]]"},
		{"before the operator", "[[ 1\n== 1 ]]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, d)
			p.Parse()
			if p.Err() == nil {
				t.Errorf("%q parsed, want it refused", c.src)
			}
		})
	}
}

// Skipping a newline before an operator must not skip one before anything
// else. The condition is complete at the end of the line either way, so the
// only thing that says the parser did not simply start ignoring newlines is
// that a word, a second operand or a `;` on the next line is still refused —
// unanimously, in every shell that has the construct.
func TestANewlineDoesNotJoinTwoConditions(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.DoubleBracket = true
	for _, c := range []struct{ name, src string }{
		{"a second operand", "[[ -n x\n-z \"\" ]]"},
		{"a bare word", "[[ -n x\ny ]]"},
		{"a semicolon", "[[ -n x\n; ]]"},
		{"a pipe", "[[ -n x\n| y ]]"},
		{"a single ampersand", "[[ -n x\n& y ]]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, d)
			p.Parse()
			if p.Err() == nil {
				t.Errorf("%q parsed, want it refused", c.src)
			}
		})
	}
}

// A term whose first word has been read and whose shape is not yet settled —
// a binary operator may still follow it — will not take a newline in two of
// the three columns, and the third adds it.
//
// Measured 2026-09-18, script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`: `[[ y` with the `]]`, a `&&`, a `||`, a `)` or a `== z` on the
// next line runs in zsh 5.9.2 and is refused by bash 5.3.20, bash 3.2 and
// ksh93u+, all three naming the **newline**. So the core refuses and the
// flag adds it.
func TestWhetherATermsFirstWordMayEndItsLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src string
		// Refused under the flag as well, which is what says the flag
		// reaches this position and not newlines in a condition generally:
		// an operator with no operand behind it, and input that ran out.
		refusedEitherWay bool
	}{
		{"[[ y\n]]", false},
		{"[[ y\n&& -n z ]]", false},
		{"[[ y\n|| -n z ]]", false},
		{"[[ ( y\n) ]]", false},
		{"[[ -n x && y\n]]", false},
		{"[[ y\n== z ]]", true},
		{"[[ y\n", true},
	} {
		for _, may := range []bool{false, true} {
			d := syntax.Core()
			d.DoubleBracket = true
			d.ConditionNewlineMayFollowATermsFirstWord = may
			p := syntax.NewParser(c.src, d)
			p.Parse()
			err := p.Err()
			want := !may || c.refusedEitherWay
			if (err != nil) != want {
				t.Errorf("may=%v %q: err=%v, want refused=%v", may, c.src, err, want)
			}
		}
	}
}

// And the refusal names the newline rather than whatever stands behind it,
// located at the **last** of the blank lines rather than the first. Both
// halves are measured: ksh93u+ writes “ `newline' unexpected “ on the
// `echo`'s line for three blank lines between them.
func TestARefusedTermNewlineIsNamedAndLocatedAtTheLastOfThem(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.DoubleBracket = true
	for _, c := range []struct {
		src  string
		line int32
	}{
		{"[[ y\n]]", 1},
		{"[[ y\necho after", 1},
		{"[[ y\n\n\necho after", 3},
		{"[[ y\n\n", 2},
	} {
		p := syntax.NewParser(c.src, d)
		p.Parse()
		var se *syntax.Error
		if !errors.As(p.Err(), &se) {
			t.Fatalf("%q: err=%v, want a syntax error", c.src, p.Err())
		}
		if se.Token != "newline" {
			t.Errorf("%q: Token=%q, want %q", c.src, se.Token, "newline")
		}
		if !se.CondTermUndecided {
			t.Errorf("%q: CondTermUndecided is false, want true", c.src)
		}
		if se.Pos.Line != c.line {
			t.Errorf("%q: the newline is on line %d, want %d", c.src, se.Pos.Line, c.line)
		}
	}
}

// The question is asked at that one position and nowhere else: a term that is
// **finished** takes a newline in every column, which is what says this is not
// a rule about newlines inside a condition.
func TestAFinishedTermStillTakesANewline(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.DoubleBracket = true
	for _, src := range []string{
		"[[ -n x\n]]",
		"[[ y == z\n]]",
		"[[ ( -n x )\n]]",
		"[[ -n x\n&& -n y ]]",
	} {
		p := syntax.NewParser(src, d)
		p.Parse()
		if err := p.Err(); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
}

// An unterminated condition is still unterminated: skipping newlines must not
// swallow the end of the input and call it a complete clause.
func TestANewlineDoesNotHideAnUnterminatedCondition(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.DoubleBracket = true
	for _, src := range []string{"[[ 1 == 1\n", "[[\n", "[[ 1 == 1 &&\n"} {
		p := syntax.NewParser(src, d)
		p.Parse()
		if p.Err() == nil {
			t.Errorf("%q parsed, want it refused as unfinished", src)
		}
	}
}
