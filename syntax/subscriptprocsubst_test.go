// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// ProcSubstInSubscriptAsText turns a process substitution standing inside a
// subscript into the characters it was written with, and leaves one that is
// not inside a subscript alone.
//
// The helper's own contract, asserted over a word the lexer really produced
// rather than over one built by hand: the span kinds are what the two readings
// part on, and a test that made its own spans could not tell a rewrite from a
// word that never held a substitution.
func TestProcSubstInSubscriptIsReadAsTheTextItWasWrittenWith(t *testing.T) {
	d := syntax.Dialect{ProcessSubstitution: true, ArraySubscript: true}
	for _, tc := range []struct {
		name        string
		src         string
		isSubscript bool
		want        string
		wantKinds   []syntax.SpanKind
	}{
		{
			// A word that *is* a subscript: every span in it stands inside
			// one, so the substitution is text.
			name: "a whole word that is a subscript", src: "x<(2)",
			isSubscript: true, want: "x<(2)",
			wantKinds: []syntax.SpanKind{syntax.Literal, syntax.Literal},
		},
		{
			// The same word read as an ordinary one: no brackets, so nothing
			// is inside a subscript and the substitution stands.
			name: "the same text with no brackets around it", src: "x<(2)",
			isSubscript: false, want: "x",
			wantKinds: []syntax.SpanKind{syntax.Literal, syntax.ProcSubstIn},
		},
		{
			// Brackets the word writes itself, which is the reading a
			// comparison's operand needs.
			name: "brackets the word writes itself", src: "a[1<(2)]",
			isSubscript: false, want: "a[1<(2)]",
			wantKinds: []syntax.SpanKind{syntax.Literal, syntax.Literal, syntax.Literal},
		},
		{
			// In front of any bracket, which is the control: `[[ -e <(cmd) ]]`
			// is a substitution on the same route.
			name: "in front of any bracket", src: "<(2)a[1]",
			isSubscript: false, want: "a[1]",
			wantKinds: []syntax.SpanKind{syntax.ProcSubstIn, syntax.Literal},
		},
		{
			// The output spelling, and the temp-file one, are the same rule.
			name: "the output spelling", src: "x>(2)",
			isSubscript: true, want: "x>(2)",
			wantKinds: []syntax.SpanKind{syntax.Literal, syntax.Literal},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := wordOf(t, d, tc.src)
			got := syntax.ProcSubstInSubscriptAsText(w, tc.isSubscript)
			var kinds []syntax.SpanKind
			var text string
			for _, sp := range got.Spans {
				kinds = append(kinds, sp.Kind)
				if sp.Kind == syntax.Literal {
					text += sp.Value
				}
			}
			if len(kinds) != len(tc.wantKinds) {
				t.Fatalf("spans = %v, want %v", kinds, tc.wantKinds)
			}
			for i, k := range kinds {
				if k != tc.wantKinds[i] {
					t.Fatalf("spans = %v, want %v", kinds, tc.wantKinds)
				}
			}
			if text != tc.want {
				t.Errorf("literal text = %q, want %q", text, tc.want)
			}
		})
	}
}

// A word with no process substitution in it is handed back as it stands, which
// keeps every subscript a script really writes off the copying path.
func TestAWordWithNoProcSubstIsHandedBackUntouched(t *testing.T) {
	d := syntax.Dialect{ProcessSubstitution: true, ArraySubscript: true}
	w := wordOf(t, d, "1+1")
	if got := syntax.ProcSubstInSubscriptAsText(w, true); got != w {
		t.Errorf("= %p, want the word itself at %p", got, w)
	}
	if got := syntax.ProcSubstInSubscriptAsText(nil, true); got != nil {
		t.Errorf("nil = %v, want nil", got)
	}
}

// wordOf lexes one word out of a snippet by parsing `: <word>` and taking the
// argument, so the spans are the ones a program really produces.
func wordOf(t *testing.T, d syntax.Dialect, src string) *syntax.Word {
	t.Helper()
	p := syntax.NewParser(": "+src, d)
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("parse %q: got %T, want a one-command pipeline", src, f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Args) != 2 {
		t.Fatalf("parse %q: no argument word", src)
	}
	return cmd.Args[1]
}
