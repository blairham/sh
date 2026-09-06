// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// This shell alone lets a `case` arm's pattern list carry an alternative
// written as nothing. Measured 2026-09-06 on zsh 5.9.2 with `-n` over a script
// file: bash 5.3.15, the same binary as `sh`, bash 3.2.57, ksh93u+ and dash all
// refuse every line here.

// The preset answers the flag, which is the half a parse cannot reach: the
// parser is handed a Dialect and would take the flag from any dialect that set
// it, so a test that only parsed would pass with this shell answering no.
func TestThisShellTakesAnEmptyCasePatternAlternative(t *testing.T) {
	if !zsh.Dialect().CasePatternMayBeEmpty {
		t.Error("zsh takes a `case` pattern list with an alternative written as nothing")
	}
	// And the one it does not answer, which is what makes the two branches
	// of one pattern position unreachable together: an operator standing
	// where a pattern belongs is dash's and not this shell's.
	if zsh.Dialect().CasePatternAcceptsOperator {
		t.Error("zsh refuses an operator where a case pattern belongs")
	}
}

// Running one, so the flag is asserted through what the shell does rather than
// only through the field. The subject that matches only because the
// alternative is there is the empty one.
func TestAnEmptyAlternativeMatchesTheEmptySubjectHere(t *testing.T) {
	for _, tc := range []struct{ subject, want string }{
		{"", "empty-matched\n"},
		{"git", "named-matched\n"},
		{"ftp", "unmatched\n"},
	} {
		const src = `case $proto in (|https|git) [[ -z $proto ]] && echo empty-matched || echo named-matched;; *) echo unmatched;; esac`
		out, st, err := preset.Combined(t, dialecttest.Base{
			Vars: map[string]string{"proto": tc.subject},
		}, src)
		if err != nil {
			t.Fatalf("proto=%q: %v", tc.subject, err)
		}
		if out != tc.want {
			t.Errorf("proto=%q: out = %q, want %q", tc.subject, out, tc.want)
		}
		if st != 0 {
			t.Errorf("proto=%q: status = %d, want 0", tc.subject, st)
		}
	}
}

// `()` is refused here too, which is the boundary the flag must not be widened
// past — and the whole rendered refusal, because this shell words a parse
// failure with the line inside the sentence rather than in front of it.
func TestAPatternListWithNoSeparatorIsRefusedHereToo(t *testing.T) {
	const src = "case a in ) echo m;; *) echo no;; esac\n"
	const want = "s.sh:1: parse error near `)'\n"
	_, err := syntax.Parse(src, zsh.Dialect())
	if err == nil {
		t.Fatal("parsed; this shell refuses it")
	}
	d := zsh.Diagnostics().ForScript()
	if got := d.ParseDiagnostic("s.sh", "", err, src); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := d.SyntaxStatus(), 1; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}
