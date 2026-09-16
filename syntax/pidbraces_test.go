// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
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
	// The shell the construct comes from also reads a bare `}` as the
	// reserved word wherever one stands, which is what makes the `}` inside
	// a run a real question rather than a hypothetical: without it the rows
	// below pass whether the run is told from an ordinary word or not. See
	// [Dialect.CloseBraceAlwaysReserved].
	d.CloseBraceAlwaysReserved = true
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
		{"a second `$$` inside one", "echo $${a$${b c}d}", []string{"echo", "$${a$${b c}d}"}},
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
// measured on, and asserts the *marking* rather than "it parsed": every line
// here is one word whether the run opened or not, so a row that only counted
// words would pass either way. Each of them brace-expands in the shell this
// comes from — `$!{a,b}` is `0a 0b`, `$$x{a,b}` is `<pid>xa <pid>xb` — which
// is what says the braces stayed syntax.
func TestOnlyTheBarePidOpensTheRun(t *testing.T) {
	d := pidBraceDialect()
	for _, tc := range []struct{ name, src string }{
		{"another special parameter", "echo $!{a,b}"},
		{"the length sigil", "echo $#{a,b}"},
		{"the option string", "echo $-{a,b}"},
		{"a name", "echo $x{a,b}"},
		{"the braced spelling of the same name", "echo ${$}{a,b}"},
		{"text between", "echo $$x{a,b}"},
		{"no dollar at all", "echo x{a,b}"},
		{"the run's own spelling, as the control", "echo $${a,b}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, d)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
			marked := 0
			for _, sp := range c.Args[1].Spans {
				if sp.PidBrace {
					marked++
				}
			}
			want := 0
			if tc.name == "the run's own spelling, as the control" {
				want = 2
			}
			if marked != want {
				t.Errorf("%s: %d marked braces, want %d", tc.src, marked, want)
			}
		})
	}
}

// TestABlankBeforeTheBraceLeavesTheReservedWord is the other half of the
// adjacency, and it is a refusal in the shell this is measured from: with no
// run to be part of, the `}` is the reserved word standing where a command
// does. Measured — `printf '[%s]' $$ {a b}` and `$$x{a b}` are both refused
// there, along with `x{a b}`, where every other column splits them in two.
func TestABlankBeforeTheBraceLeavesTheReservedWord(t *testing.T) {
	d := pidBraceDialect()
	for _, src := range []string{
		"echo $$ {a b}",
		"echo $$x{a b}",
		"echo x{a b}",
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, and the shell this is measured from refuses it", src)
		}
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
	const src = "echo $${a\necho two\n"
	_, err := Parse(src, d)
	if err == nil {
		t.Fatal("an unmatched run parsed, and the shell this is measured from refuses it")
	}
	// And it points at the `{` that never closed, which is what every other
	// unmatched opener here points at. The two lines the run swallowed are
	// behind it, so a refusal carrying the end of the input instead would
	// name a line the script has no brace on.
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("%v is not a parse error", err)
	}
	if se.Kind != ErrUnmatched {
		t.Errorf("kind = %v, want an unmatched opener", se.Kind)
	}
	if got, want := int(se.Pos.Offset), strings.IndexByte(src, '{'); got != want {
		t.Errorf("refused at offset %d, want the `{` at %d", got, want)
	}
	if se.Token != "{" || se.Expected != "}" {
		t.Errorf("opener %q closer %q, want `{` and `}`", se.Token, se.Expected)
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

// TestARunDoesNotNest. A second `$$` inside a run opens nothing: its braces
// are the ordinary text the run already makes inert. Measured — `$${a$${b,c}d}`
// is two words on the shell this comes from, the inner `{b,c}` having expanded
// as a list, where a nested run would have left it one. So the marking is on
// one pair and the interior is untouched, which is what the counts here say.
func TestARunDoesNotNest(t *testing.T) {
	d := pidBraceDialect()
	f, err := Parse("echo $${a$${b,c}d}", d)
	if err != nil {
		t.Fatalf("echo $${a$${b,c}d}: %v", err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	marked, params := 0, 0
	var unmarked string
	for _, sp := range c.Args[1].Spans {
		switch {
		case sp.PidBrace:
			marked++
		case sp.Kind == ParamExp:
			params++
		case sp.Kind == Literal:
			unmarked += sp.Value
		}
	}
	if marked != 2 {
		t.Errorf("%d marked braces, want the outer pair alone", marked)
	}
	if params != 2 {
		t.Errorf("%d parameter expansions, want both `$$`", params)
	}
	if unmarked != "a{b,c}d" {
		t.Errorf("unmarked literal text %q, want the inner braces left as text", unmarked)
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

// TestThePrinterLeavesTheRunsRawModeAtTheMatch is the half the rows above
// cannot reach: every one of them prints back identically whether the raw
// mode is put back at the closing `}` or left on for the rest of the word,
// because nothing after their run needs protecting. A `$` that begins no
// expansion does — it is written `\$` under the ordinary rules — so this is
// where leaving the mode on becomes visible.
func TestThePrinterLeavesTheRunsRawModeAtTheMatch(t *testing.T) {
	d := pidBraceDialect()
	const src = "echo $${a b}$%"
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	got := strings.TrimRight(Print(f), "\n")
	if want := `echo $${a b}\$%`; got != want {
		t.Errorf("%q printed back as %q, want %q", src, got, want)
	}
	// The control: the same tail with no run in front of it is written the
	// same way, which is what says the escape is the ordinary rule and not
	// something this construct added.
	f, err = Parse("echo $%", d)
	if err != nil {
		t.Fatalf("echo $%%: %v", err)
	}
	if got, want := strings.TrimRight(Print(f), "\n"), `echo \$%`; got != want {
		t.Errorf("the control printed back as %q, want %q", got, want)
	}
}
