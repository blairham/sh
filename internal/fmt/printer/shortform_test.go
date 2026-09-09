// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The brace-spelled bodies, which are the one thing the four dialects answer
// differently — and which the tree cannot tell apart, so nothing here can be
// checked by parsing alone. See docs/spec/style.md.

// TestZshKeepsTheSpellingTheAuthorWrote pins the preserving answer. Every case
// is source zsh 5.9.2 accepts; `make check` does not run zsh, so the spellings
// were measured once and written down rather than probed here.
func TestZshKeepsTheSpellingTheAuthorWrote(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "if with a brace body",
			src:  "if [[ -n $x ]] {\necho a\n}\n",
			want: "if [[ -n $x ]] {\n  echo a\n}\n",
		},
		{
			name: "if else",
			src:  "if [[ -n $x ]] {\necho a\n} else {\necho b\n}\n",
			want: "if [[ -n $x ]] {\n  echo a\n} else {\n  echo b\n}\n",
		},
		{
			// The conditions are `[[ ]]` and not bare commands on purpose:
			// zsh refuses `if a { b }`, because a bare command swallows the
			// `{` and nothing says where the condition stopped. Measured
			// 2026-09-09 against zsh 5.9.2, both spellings and both
			// line shapes. See docs/spec/style.md.
			name: "if elif else",
			src:  "if [[ a ]] {\nb\n} elif [[ c ]] {\nd\n} else {\ne\n}\n",
			want: "if [[ a ]] {\n  b\n} elif [[ c ]] {\n  d\n} else {\n  e\n}\n",
		},
		{
			name: "for keeps its parenthesized list",
			src:  "for f ( a b c ) {\nprint $f\n}\n",
			want: "for f ( a b c ) {\n  print $f\n}\n",
		},
		{
			name: "while",
			src:  "while (( i-- )) {\nprint $i\n}\n",
			want: "while (( i-- )) {\n  print $i\n}\n",
		},
		{
			name: "until",
			src:  "until (( done )) {\nsleep 1\n}\n",
			want: "until (( done )) {\n  sleep 1\n}\n",
		},
		{
			name: "repeat",
			src:  "repeat 3 {\necho hi\n}\n",
			want: "repeat 3 {\n  echo hi\n}\n",
		},
		{
			name: "select",
			src:  "select f ( a b ) {\nprint $f\n}\n",
			want: "select f ( a b ) {\n  print $f\n}\n",
		},
		{
			name: "one line stays one line",
			src:  "repeat 3 { echo hi }\n",
			want: "repeat 3 { echo hi }\n",
		},
		{
			name: "the keyword spelling is left alone too",
			src:  "if true; then\necho a\nfi\n",
			want: "if true; then\n  echo a\nfi\n",
		},
		{
			name: "a group inside a keyword body is not a short form",
			src:  "if true; then\n{ echo a; }\nfi\n",
			want: "if true; then\n  { echo a; }\nfi\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := format(t, c.src, zsh.Dialect(), zsh.Style()); got != c.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}
}

// TestExpandingIsTheOtherAnswer runs the same source under a style that does
// not preserve, which is what every dialect but zsh answers.
//
// This is the test that makes Style load-bearing: same dialect, same input,
// different layout, because the layout came from the vector and not from the
// grammar. Without it the field could be ignored by the printer and nothing
// here would notice.
func TestExpandingIsTheOtherAnswer(t *testing.T) {
	expand := zsh.Style()
	expand.BraceShortForm = syntax.ExpandShortForm
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "if",
			src:  "if [[ -n $x ]] {\necho a\n}\n",
			want: "if [[ -n $x ]]; then\n  echo a\nfi\n",
		},
		{
			name: "if else",
			src:  "if [[ a ]] {\nb\n} else {\nc\n}\n",
			want: "if [[ a ]]; then\n  b\nelse\n  c\nfi\n",
		},
		{
			name: "for loses its parentheses with its braces",
			src:  "for f ( a b c ) {\nprint $f\n}\n",
			want: "for f in a b c; do\n  print $f\ndone\n",
		},
		{
			name: "repeat",
			src:  "repeat 3 {\necho hi\n}\n",
			want: "repeat 3; do\n  echo hi\ndone\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatStyle(t, c.src, zsh.Dialect(), expand); got != c.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}
}

// TestTheTwoSpellingsAreOneProgram is the safety net under the choice: whether
// a body is written with braces or with keywords, it is the same program, so
// preserving and expanding are a matter of taste rather than of meaning.
func TestTheTwoSpellingsAreOneProgram(t *testing.T) {
	expand := zsh.Style()
	expand.BraceShortForm = syntax.ExpandShortForm
	for _, src := range []string{
		"if [[ -n $x ]] {\necho a\n} else {\necho b\n}\n",
		"for f ( a b c ) {\nprint $f\n}\n",
		"while (( i-- )) {\nprint $i\n}\n",
		"repeat 3 {\necho hi\n}\n",
	} {
		t.Run(src, func(t *testing.T) {
			kept := format(t, src, zsh.Dialect(), zsh.Style())
			widened := formatStyle(t, src, zsh.Dialect(), expand)
			if kept == widened {
				t.Fatalf("the two styles agreed, so this case proves nothing:\n%s", kept)
			}
			a, err := syntax.Parse(kept, zsh.Dialect())
			if err != nil {
				t.Fatalf("preserved output does not parse: %v\n%s", err, kept)
			}
			b, err := syntax.Parse(widened, zsh.Dialect())
			if err != nil {
				t.Fatalf("expanded output does not parse: %v\n%s", err, widened)
			}
			if why, ok := syntax.SameProgram(a, b); !ok {
				t.Errorf("the spellings are different programs at %s\nkept:\n%s\nwidened:\n%s",
					why, kept, widened)
			}
		})
	}
}
