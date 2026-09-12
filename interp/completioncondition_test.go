// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A completion-context condition reached where it may not run is refused, and
// the refusal is fatal: the statement after it does not run and the status is
// 1.
//
// The flag is named and the shell that sets it is not. It is not an axis on
// Semantics and that is deliberate: an axis records a *disagreement* about
// identical syntax, and one panel column has the operator at all — the other
// four have no `[[ -prefix ]]` to answer about. Measured against zsh 5.9.2 on
// 2026-09-12 and the end-to-end half lives in dialect/zsh (#1879).
func TestACompletionConditionIsRefusedWhereItCannotRun(t *testing.T) {
	completion := func(d *syntax.Dialect) {
		d.CompletionConditions = true
		d.PatternAlternation = true
	}
	for _, src := range []string{
		"[[ -prefix : ]]; echo after",
		"[[ -suffix : ]]; echo after",
		"[[ -prefix //(a|b)/ ]]; echo after",
		"if [[ -prefix : ]]; then echo T; else echo F; fi; echo after",
	} {
		out, st := runGrammar(t, src, completion, nil)
		if strings.Contains(out, "after") {
			t.Errorf("%q ran on after the refusal: %q", src, out)
		}
		if !strings.Contains(out, "condition can only be used in completion function") {
			t.Errorf("%q said %q", src, out)
		}
		if st != 1 {
			t.Errorf("%q: status = %d, want 1", src, st)
		}
	}
	// And with no operand the word is ordinary, so nothing is refused: the
	// condition is the bare-word test and it holds.
	out, st := runGrammar(t, `[[ -prefix ]]; echo "st=$?"`, completion, nil)
	if strings.TrimSpace(out) != "st=0" || st != 0 {
		t.Errorf("`[[ -prefix ]]` said %q at %d, want %q at 0", out, st, "st=0")
	}
}
