// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `<(...)` and `>(...)` are a word, not an operator — which is the whole of
// the grammar question. The parser has no rule for them; the lexer has to
// decide, before it reaches the operator table, that a `<` followed by `(` is
// the start of a word rather than a redirection.
func TestProcessSubstitutionScansAsAWord(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		kind      syntax.SpanKind
		inner     string
	}{
		{"read from", `cat <(echo hi)`, syntax.ProcSubstIn, "echo hi"},
		{"write to", `echo x > >(tr a-z A-Z)`, syntax.ProcSubstOut, "tr a-z A-Z"},
		{"parens inside", `cat <(echo a; (echo b))`, syntax.ProcSubstIn, "echo a; (echo b)"},
		{"a substitution inside", `cat <(echo $(date))`, syntax.ProcSubstIn, "echo $(date)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			span, ok := findSpan(f, tc.kind)
			if !ok {
				t.Fatalf("no %v span in %q", tc.kind, tc.src)
			}
			// Unparsed, exactly as a command substitution is: what is inside
			// is a script, and it is the interpreter that decides which
			// dialect to read it in.
			if span.Value != tc.inner {
				t.Errorf("inner = %q, want %q", span.Value, tc.inner)
			}
		})
	}
}

// The dialect that does not have it reads the same text as a redirection whose
// target is another redirection, and says so. Not a special case anywhere —
// the lexer simply never takes the word branch.
func TestProcessSubstitutionIsADialectFeature(t *testing.T) {
	d := syntax.Core()
	d.ProcessSubstitution = false
	for _, src := range []string{`cat <(echo hi)`, `echo x > >(cat)`} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("parse %q succeeded, want a refusal where the dialect has no such construct", src)
		}
	}
	if _, err := syntax.Parse(`cat <(echo hi)`, syntax.Core()); err != nil {
		t.Errorf("parse with the feature on: %v", err)
	}
}

// findSpan looks in the two places a word can be: an argument, and the target
// of a redirection.
func findSpan(f *syntax.File, kind syntax.SpanKind) (syntax.Span, bool) {
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	words := append([]*syntax.Word(nil), sc.Args...)
	for _, rd := range sc.Redirs {
		words = append(words, rd.Word)
	}
	for _, w := range words {
		if w == nil {
			continue
		}
		for _, s := range w.Spans {
			if s.Kind == kind {
				return s, true
			}
		}
	}
	return syntax.Span{}, false
}
