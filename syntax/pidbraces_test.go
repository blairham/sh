// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A `{ … }` run written immediately after `$$` (#3091, the last of #3040).
//
// The braces are characters there, and the run to the matching `}` is
// lexically atomic: a blank, a newline, a `;`, a `<` or a `>` inside it is
// text rather than an operator. Measured 2026-09-15 on zsh 5.9.2, each probe
// in a script file of its own and printed one argument per `[%s]` so the
// word count is visible:
//
//	| probe         | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
//	| `$${a b}`     | `{a b}`   | two words | two words | two words | two words | two words | two words |
//	| `$${a;b}`     | `{a;b}`   | refused  | refused    | refused  | refused | refused | refused |
//	| `$${a>b}`     | `{a>b}`   | redirects | redirects | redirects | redirects | redirects | redirects |
//	| `$${a{b,c}d}` | two words | one word | one word  | one word | one word | one word | one word |
//
// Six columns against one, so it is a dialect's grammar. The rows assert the
// *word* — how many the line has and what the first one reads as — rather
// than "it parsed", because parsing is the cheap half: a reading that ended
// the word at the blank parses `echo $${a b}` perfectly well and hands the
// command two arguments.

func pidBraceDialect() Dialect {
	d := Core()
	d.PidBraceGroupIsText = true
	return d
}

// words is the argument list of the first command in src, each rendered as
// the source it occupied.
func pidBraceWords(t *testing.T, src string, d Dialect) []string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("%s: first command is %T, want a simple command", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	out := make([]string, 0, len(c.Args))
	for _, w := range c.Args {
		out = append(out, src[w.Pos().Offset:w.End().Offset])
	}
	return out
}

func TestAPidBraceRunIsOneWord(t *testing.T) {
	d := pidBraceDialect()
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"a blank", "echo $${a b}", []string{"echo", "$${a b}"}},
		{"a semicolon", "echo $${a;b}", []string{"echo", "$${a;b}"}},
		{"a redirection operator", "echo $${a>b}", []string{"echo", "$${a>b}"}},
		{"an input redirection", "echo $${a<b}", []string{"echo", "$${a<b}"}},
		{"a pipe", "echo $${a|b}", []string{"echo", "$${a|b}"}},
		{"an ampersand", "echo $${a&b}", []string{"echo", "$${a&b}"}},
		{"parentheses", "echo $${(s<,>)x}", []string{"echo", "$${(s<,>)x}"}},
		{"a newline", "echo $${a\nb}", []string{"echo", "$${a\nb}"}},
		{"a nested pair", "echo $${a{b c}d}", []string{"echo", "$${a{b c}d}"}},
		{"text behind the run", "echo $${a b}rest", []string{"echo", "$${a b}rest"}},
		{"two runs in one word", "echo $${a b}$${c d}", []string{"echo", "$${a b}$${c d}"}},
		{"the run ends at its match", "echo $${a} b", []string{"echo", "$${a}", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pidBraceWords(t, tc.src, d)
			if len(got) != len(tc.want) {
				t.Fatalf("%q: %d words %q, want %d %q", tc.src, len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%q: word %d is %q, want %q", tc.src, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestThePidBraceRunEndsAtTheMatch is the half the atomicity could cost: what
// follows the matching `}` is read under the ordinary rules again, so the
// `>` in the second row is a redirection and the `;` in the third ends the
// command.
func TestThePidBraceRunEndsAtTheMatch(t *testing.T) {
	d := pidBraceDialect()
	f, err := Parse("echo $${a}>out", d)
	if err != nil {
		t.Fatalf("echo $${a}>out: %v", err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(c.Redirs) != 1 {
		t.Errorf("echo $${a}>out: %d redirections, want 1", len(c.Redirs))
	}
	f, err = Parse("echo $${a}; echo two", d)
	if err != nil {
		t.Fatalf("echo $${a}; echo two: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Errorf("echo $${a}; echo two: %d statements, want 2", len(f.Stmts))
	}
}

// TestOnlyTheBarePidOpensTheRun keeps the reading to the one spelling it was
// measured on. Every line here brace-expands or splits in the shell this
// comes from, so a flag that fired on any of them would be the wrong rule
// with the right test passing.
func TestOnlyTheBarePidOpensTheRun(t *testing.T) {
	d := pidBraceDialect()
	for _, tc := range []struct{ name, src string }{
		{"another special parameter", "echo $!{a b}"},
		{"the length sigil", "echo $#{a b}"},
		{"the option string", "echo $-{a b}"},
		{"a name", "echo $x{a b}"},
		{"the braced spelling of the same name", "echo ${$}{a b}"},
		{"a blank between", "echo $$ {a b}"},
		{"text between", "echo $$x{a b}"},
		{"no dollar at all", "echo x{a b}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pidBraceWords(t, tc.src, d)
			if len(got) < 3 {
				t.Errorf("%q: %d words %q, want the blank to end the word", tc.src, len(got), got)
			}
		})
	}
}

// TestThePidBraceRunIsOffWithoutTheFlag is the control for every row above:
// the same lines under the core grammar end the word at the blank, which is
// what the other six columns do with them.
func TestThePidBraceRunIsOffWithoutTheFlag(t *testing.T) {
	d := Core()
	got := pidBraceWords(t, "echo $${a b}", d)
	if len(got) != 3 {
		t.Errorf("echo $${a b} without the flag: %d words %q, want 3", len(got), got)
	}
	// The `;` is an operator there, so the line is two statements rather
	// than the one it is with the flag — which is the reading every other
	// column takes, each of them then refusing the `}` it is left holding.
	f, err := Parse("echo $${a;b}", d)
	if err != nil {
		t.Fatalf("echo $${a;b} without the flag: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Errorf("echo $${a;b} without the flag: %d statements, want 2", len(f.Stmts))
	}
}

// TestAnUnmatchedPidBraceIsRefused. The shell this is measured from swallows
// every line after the `{` looking for the match and then says `closing brace
// expected`, so an unmatched one is a refusal rather than text.
func TestAnUnmatchedPidBraceIsRefused(t *testing.T) {
	d := pidBraceDialect()
	if _, err := Parse("echo $${a\necho two\n", d); err == nil {
		t.Error("an unmatched run parsed, and the shell this is measured from refuses it")
	}
}

// TestThePidBracesAreMarkedAndNothingElseIs. The outer pair carries the note
// and the contents do not, which is the whole of what tells brace expansion
// that `$${a,b}` is one word and the `{b,c}` inside `$${a{b,c}d}` is still a
// list.
func TestThePidBracesAreMarkedAndNothingElseIs(t *testing.T) {
	d := pidBraceDialect()
	f, err := Parse("echo $${a{b,c}d}", d)
	if err != nil {
		t.Fatalf("echo $${a{b,c}d}: %v", err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	var marked []string
	var unmarked string
	for _, s := range c.Args[1].Spans {
		if s.PidBrace {
			marked = append(marked, s.Value)
			continue
		}
		if s.Kind == Literal {
			unmarked += s.Value
		}
	}
	if len(marked) != 2 || marked[0] != "{" || marked[1] != "}" {
		t.Errorf("marked spans %q, want the outer `{` and `}`", marked)
	}
	if unmarked != "a{b,c}d" {
		t.Errorf("unmarked literal text %q, want the contents with their own braces", unmarked)
	}
}

// TestAPidBraceRunPrintsBackAsItCame. The blanks and operators inside were
// characters, so escaping them would leave the tree identical and make the
// word a different program — the failure #1221 names for a pattern group.
func TestAPidBraceRunPrintsBackAsItCame(t *testing.T) {
	d := pidBraceDialect()
	for _, src := range []string{
		"echo $${a b}",
		"echo $${a;b}",
		"echo $${a>b}",
		"echo $${(s<,>)x}",
		"echo $${a b}rest",
		"echo $${a{b,c}d}",
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		got := strings.TrimRight(Print(f), "\n")
		if got != src {
			t.Errorf("%q printed back as %q", src, got)
		}
	}
}
