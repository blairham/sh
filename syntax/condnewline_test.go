// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
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

// Not after a binary operator: `[[ 1 ==` then a newline is an error in bash
// and ksh93, and only zsh takes it. Refused is what the two agree on.
func TestANewlineDoesNotFollowABinaryOperator(t *testing.T) {
	d := syntax.Core()
	d.DoubleBracket = true
	p := syntax.NewParser("[[ 1 ==\n1 ]]", d)
	p.Parse()
	if p.Err() == nil {
		t.Error("parsed, want an operator with nothing after it to be refused")
	}
}

// An unterminated condition is still unterminated: skipping newlines must not
// swallow the end of the input and call it a complete clause.
func TestANewlineDoesNotHideAnUnterminatedCondition(t *testing.T) {
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
