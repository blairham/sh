// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// render summarises a token stream so a test can state its expectation as one
// readable string rather than as a struct literal nobody checks.
//
// A word renders its spans, so quoting is visible: a"b c"d is
// word(a|"b c"|d) with the quote characters marking the quoted spans.
func render(toks []Token) string {
	var b strings.Builder
	for i, t := range toks {
		if t.Kind == EOF {
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		switch t.Kind {
		case Word:
			b.WriteString("word(")
			for j, s := range t.Spans {
				if j > 0 {
					b.WriteByte('|')
				}
				switch s.Quoting {
				case SingleQuoted:
					b.WriteString("'" + s.Value + "'")
				case DoubleQuoted:
					b.WriteString(`"` + s.Value + `"`)
				case DollarSingleQuoted:
					b.WriteString("$'" + s.Value + "'")
				default:
					b.WriteString(s.Value)
				}
			}
			b.WriteByte(')')
		case IONumber:
			b.WriteString("io(" + t.Text + ")")
		case Newline:
			b.WriteString("nl")
		default:
			b.WriteString(t.Kind.String())
		}
	}
	return b.String()
}

func lex(t *testing.T, src string, d Dialect) string {
	t.Helper()
	return render(NewLexer(src, d).Tokens())
}

func TestSpansWithinAWord(t *testing.T) {
	// The case docs/spec/grammar/expansion.md's per-span requirement rests on:
	// one word, three spans, only the unquoted ones split later.
	got := lex(t, `a"b c"d`, Core())
	want := `word(a|"b c"|d)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	toks := NewLexer(`a"b c"d`, Core()).Tokens()
	if n := len(toks[0].Spans); n != 3 {
		t.Fatalf("want 3 spans, got %d", n)
	}
	if !toks[0].IsQuoted() {
		t.Error("a partly quoted word must report as quoted")
	}
	if lit := toks[0].Literal(); lit != "ab cd" {
		t.Errorf("Literal() = %q, want %q", lit, "ab cd")
	}
}

func TestQuoting(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{
			"double quotes escape their own quote",
			`"a\"b"`, `word("a"b")`,
		},
		{
			// The rule C intuition gets wrong: \n inside double quotes is
			// backslash-then-n, not a newline.
			"backslash is literal before n inside double quotes",
			`"a\nb"`, `word("a\nb")`,
		},
		{
			"backslash is literal before anything else too",
			`"a\qb"`, `word("a\qb")`,
		},
		{
			"single quotes protect everything, including the dollar",
			`'a$HOME'`, `word('a$HOME')`,
		},
		{
			"single quotes have no escapes at all",
			`'a\b'`, `word('a\b')`,
		},
		{
			"an unquoted backslash protects one character",
			`a\$HOME`, `word(a$HOME)`,
		},
		{
			"adjacent quoting concatenates into one word",
			`'a'"b"c`, `word('a'|"b"|c)`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lex(t, tc.src, Core()); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestOperatorsDelimitWithoutWhitespace(t *testing.T) {
	// a>b is three tokens. A lexer that split on whitespace would be wrong
	// before it started.
	if got, want := lex(t, `a>b`, Core()), `word(a) > word(b)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLongestMatchWins(t *testing.T) {
	tests := []struct{ src, want string }{
		{`a>>b`, `word(a) >> word(b)`},
		{`a&&b`, `word(a) && word(b)`},
		{`a||b`, `word(a) || word(b)`},
		{`a;;b`, `word(a) ;; word(b)`},
		{`a<<<b`, `word(a) <<< word(b)`},
	}
	for _, tc := range tests {
		if got := lex(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestIONumberNeedsStrictAdjacency(t *testing.T) {
	// One space changes what the digit *is*: a file descriptor or an argument.
	if got, want := lex(t, `echo 1>b`, Core()), `word(echo) io(1) > word(b)`; got != want {
		t.Errorf("adjacent: got %s, want %s", got, want)
	}
	if got, want := lex(t, `echo 1 >b`, Core()), `word(echo) word(1) > word(b)`; got != want {
		t.Errorf("separated: got %s, want %s", got, want)
	}
	// Digits must *start* the word, and a digit run not followed by a
	// redirection is an ordinary word.
	if got, want := lex(t, `a1>b`, Core()), `word(a1) > word(b)`; got != want {
		t.Errorf("mid-word digits: got %s, want %s", got, want)
	}
	if got, want := lex(t, `12 34`, Core()), `word(12) word(34)`; got != want {
		t.Errorf("bare digits: got %s, want %s", got, want)
	}
}

func TestCommentsNeedAWordBoundary(t *testing.T) {
	if got, want := lex(t, `echo a#b`, Core()), `word(echo) word(a#b)`; got != want {
		t.Errorf("mid-word: got %s, want %s", got, want)
	}
	if got, want := lex(t, `echo a #b`, Core()), `word(echo) word(a)`; got != want {
		t.Errorf("at boundary: got %s, want %s", got, want)
	}
}

func TestLineContinuationJoinsAWord(t *testing.T) {
	if got, want := lex(t, "ab\\\ncd", Core()), `word(abcd)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// It is removed before tokens form, so it can split an operator too.
	if got, want := lex(t, "a>\\\n>b", Core()), `word(a) > > word(b)`; got != want {
		t.Errorf("split operator: got %s, want %s", got, want)
	}
}

func TestNewlineIsAToken(t *testing.T) {
	if got, want := lex(t, "a\nb", Core()), `word(a) nl word(b)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestAmpersandRedirectIsADialectDecision(t *testing.T) {
	// The dangerous axis. With the operator, `&>` redirects both streams.
	// Without it the same text still lexes — as `&` then `>` — which is a
	// background command followed by a truncating redirection. Nothing errors;
	// it simply means something else, which is why the lexer has to know its
	// dialect rather than accept the union.
	if got, want := lex(t, `echo hi &>b`, Core()), `word(echo) word(hi) &> word(b)`; got != want {
		t.Errorf("with the operator: got %s, want %s", got, want)
	}
	if got, want := lex(t, `echo hi &>b`, POSIX()), `word(echo) word(hi) & > word(b)`; got != want {
		t.Errorf("without it: got %s, want %s", got, want)
	}
}

func TestDialectGatesOtherConstructs(t *testing.T) {
	tests := []struct {
		name, src string
		d         Dialect
		want      string
	}{
		{"herestring in core", `a<<<b`, Core(), `word(a) <<< word(b)`},
		// Without it, <<< is a heredoc followed by a redirection — again a
		// different meaning rather than an error.
		{"herestring absent", `a<<<b`, POSIX(), `word(a) << < word(b)`},
		{"case fallthrough in core", `a;&b`, Core(), `word(a) ;& word(b)`},
		{"case fallthrough absent", `a;&b`, POSIX(), `word(a) ; & word(b)`},
		{"case continue is bash only", `a;;&b`, Bash(), `word(a) ;;& word(b)`},
		{"case continue absent from core", `a;;&b`, Core(), `word(a) ;; & word(b)`},
		{"dollar-single in core", `$'a\nb'`, Core(), `word($'a\nb')`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lex(t, tc.src, tc.d); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestIncompleteIsNotInvalid(t *testing.T) {
	// A half-typed line is the normal case at a prompt. The caller needs to
	// tell "ask for another line" from "this is wrong".
	for _, src := range []string{`"abc`, `'abc`, `$'abc`, `abc\`} {
		l := NewLexer(src, Core())
		l.Tokens()
		if !l.Incomplete() {
			t.Errorf("%q: want Incomplete", src)
		}
		if l.Err() == nil {
			t.Errorf("%q: want an error describing what is unfinished", src)
		}
	}
}

func TestPositionsAreTracked(t *testing.T) {
	toks := NewLexer("ab\ncd", Core()).Tokens()
	if p := toks[0].Pos; p.Line != 1 || p.Col != 1 || p.Offset != 0 {
		t.Errorf("first token at %v (offset %d), want 1:1 offset 0", p, p.Offset)
	}
	if p := toks[2].Pos; p.Line != 2 || p.Col != 1 {
		t.Errorf("token after newline at %v, want 2:1", p)
	}
	if (Pos{}).IsValid() {
		t.Error("the zero Pos must not claim to be valid")
	}
}

func TestNeverPanics(t *testing.T) {
	// This runs on the keystroke path, so malformed and half-typed input is
	// the normal case rather than the exception.
	inputs := []string{
		"", " ", "\n", "\\", "\\\n", `"`, `'`, `$'`, `$`, "#", "#no newline",
		"a>", ">>>", "<<<<", "&&&", ";;;", "|||", "()", "a\\", `"a\`,
		"\x00", "\xff\xfe", strings.Repeat("<", 100), strings.Repeat(`"`, 50),
		"a" + strings.Repeat("\\\n", 30) + "b",
	}
	for _, src := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			for _, d := range []Dialect{Core(), POSIX(), Bash()} {
				NewLexer(src, d).Tokens()
			}
		}()
	}
}

func FuzzLexerNeverPanics(f *testing.F) {
	for _, s := range []string{`a"b c"d`, "echo 1>b", "a\\\nb", `$'x'`, "a;;&b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		toks := NewLexer(src, Core()).Tokens()
		// Whatever the input, the stream ends and positions never run backwards.
		var last Pos
		for _, tk := range toks {
			if tk.Pos.Offset < last.Offset {
				t.Fatalf("positions went backwards at %v in %q", tk.Pos, src)
			}
			last = tk.Pos
		}
	})
}
