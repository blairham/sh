// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// With Dialect.NegationAfterABarTogglesThePipeline, each `!` after a bar
// toggles the one flag the pipeline carries, and `!`s with nothing after them
// end the pipeline with an element that runs nothing. Without the flag the
// word is refused there (#5256, #5272).
func TestANegationAfterABarTogglesWhereTheDialectSays(t *testing.T) {
	d := syntax.Core()
	d.NegationAfterABarTogglesThePipeline = true
	for _, c := range []struct {
		src     string
		negated bool
		cmds    int
	}{
		{"echo | ! true", true, 2},
		{"! true | ! true", false, 2},
		{"echo | ! ! true", false, 2},
		{"echo | ! true | false", true, 3},
		{"echo | !", true, 2},
	} {
		f, err := syntax.Parse(c.src, d)
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		pl, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
		if !ok || pl.Negated != c.negated || len(pl.Cmds) != c.cmds {
			t.Errorf("%q: got %#v, want negated=%v over %d commands", c.src, f.Stmts[0].Expr, c.negated, c.cmds)
		}
	}
	if _, err := syntax.Parse("echo | ! true", syntax.Core()); err == nil {
		t.Error("without the flag `echo | ! true` parsed, want the refusal")
	}
}
