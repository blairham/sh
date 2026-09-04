// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// render summarizes a token stream so a test can state its expectation as one
// readable string rather than as a struct literal nobody checks.
//
// A word renders its spans, so quoting is visible: a"b c"d is
// word(a|"b c"|d) with the quote characters marking the quoted spans.
func render(toks []Token) string {
	var b strings.Builder
	for i, t := range toks {
		if t.Kind == TokEOF {
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		switch t.Kind {
		case TokWord:
			b.WriteString("word(")
			for j, s := range t.Spans {
				if j > 0 {
					b.WriteByte('|')
				}
				if s.Kind != Literal {
					// Substitutions render as kind{inner}, with a leading "
					// when they sit inside double quotes, because the quoting
					// decides whether the result is split.
					if s.Quoting == DoubleQuoted {
						b.WriteByte('"')
					}
					switch s.Kind {
					case CommandSubst:
						b.WriteString("cmd{" + s.Value + "}")
					case ArithSubst:
						b.WriteString("arith{" + s.Value + "}")
					case ParamExp:
						b.WriteString("param{" + s.Value + "}")
					}
					continue
				}
				switch s.Quoting {
				case SingleQuoted:
					b.WriteString("'" + s.Value + "'")
				case DoubleQuoted:
					b.WriteString(`"` + s.Value + `"`)
				case DollarSingleQuoted:
					b.WriteString("$'" + s.Value + "'")
				case BackslashQuoted:
					b.WriteString(`\` + s.Value)
				default:
					b.WriteString(s.Value)
				}
			}
			b.WriteByte(')')
		case TokIONumber:
			b.WriteString("io(" + t.Text + ")")
		case TokNewline:
			b.WriteString("nl")
		case TokArithCmd:
			b.WriteString("arith-cmd{" + t.Text + "}")
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
			// The escaped character is its own span, because the protection
			// has to survive: a field that forgot it would glob.
			"an unquoted backslash protects one character",
			`a\$HOME`, `word(a|\$|HOME)`,
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
		{"case continue where the flag is on", `a;;&b`, withCaseContinue(), `word(a) ;;& word(b)`},
		{"case continue absent from core", `a;;&b`, Core(), `word(a) ;; & word(b)`},
		{"dollar-single in core", `$'a\nb'`, Core(), `word($'a\nb')`},
		// The `$` of a translatable string contributes nothing: the spans
		// are exactly what a bare `"` produces, expansions included.
		{"dollar-double where the flag is on", `echo $"a b"`, withDollarDoubleQuote(), `word(echo) word("a b")`},
		{
			"dollar-double expands inside", `echo $"a $x b"`, withDollarDoubleQuote(),
			`word(echo) word("a "|"param{x}|" b")`,
		},
		// Without the flag the same text still lexes — the `$` is a literal
		// byte in front of an ordinary double-quoted string, which is what
		// dash and zsh do with it. A different word, not an error.
		{"dollar-double absent from core", `echo $"a b"`, Core(), `word(echo) word($|"a b")`},
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
			for _, d := range []Dialect{Core(), POSIX(), withCaseContinue()} {
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

func TestSubstitutionsDoNotEndTheWord(t *testing.T) {
	// $(printf a)b is one word. A lexer that emitted the substitution as its
	// own token could not reconstruct that, and the paren operator used to
	// win here — `echo $(cat b)` lexed as word($) ( word(cat) word(b) ).
	tests := []struct{ src, want string }{
		{`$(printf a)b`, `word(cmd{printf a}|b)`},
		{`x$(printf a)y`, `word(x|cmd{printf a}|y)`},
		{`echo $(cat b)`, `word(echo) word(cmd{cat b})`},
		{"echo `cat b`", `word(echo) word(cmd{cat b})`},
		{`echo ${x:-y}`, `word(echo) word(param{x:-y})`},
		{`echo $((1+2))`, `word(echo) word(arith{1+2})`},
	}
	for _, tc := range tests {
		if got := lex(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestClosingDelimiterIsNotFoundByCounting(t *testing.T) {
	// The rule that decides the implementation: a ) inside quotes does not
	// close the substitution. Counting parens truncates it and silently
	// changes the program.
	tests := []struct{ name, src, want string }{
		{"paren inside double quotes", `$(echo ")" )`, `word(cmd{echo ")" })`},
		{"paren inside a quoted word", `$(echo "a)b")`, `word(cmd{echo "a)b"})`},
		{"paren inside single quotes", `$(echo ')' )`, `word(cmd{echo ')' })`},
		{"escaped paren", `$(echo \) )`, `word(cmd{echo \) })`},
		{"nesting", `$(echo $(echo deep))`, `word(cmd{echo $(echo deep)})`},
		{"subshell needs the space", `$( (echo sub) )`, `word(cmd{ (echo sub) })`},
		{"arithmetic with inner parens", `$(( (1+2)*3 ))`, `word(arith{ (1+2)*3 })`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lex(t, tc.src, Core()); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSubstitutionsInsideDoubleQuotes(t *testing.T) {
	// "$(cmd)" is how most scripts spell a substitution, so a double-quoted
	// section yields several spans rather than one literal.
	tests := []struct{ src, want string }{
		{`"$(cat b)"`, `word("cmd{cat b})`},
		{`"[$(cat b)]"`, `word("["|"cmd{cat b}|"]")`},
		{`"$(echo ")" )"`, `word("cmd{echo ")" })`},
		{`"${x}"`, `word("param{x})`},
		{`"$((1+2))"`, `word("arith{1+2})`},
	}
	for _, tc := range tests {
		if got := lex(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
	// Single quotes protect everything, so a substitution inside them is text.
	if got, want := lex(t, `'$(cat b)'`, Core()), `word('$(cat b)')`; got != want {
		t.Errorf("single-quoted: got %s, want %s", got, want)
	}
}

func TestUnterminatedSubstitutionsAreIncomplete(t *testing.T) {
	for _, src := range []string{`$(echo`, `${x`, "`echo", `$((1+2`} {
		l := NewLexer(src, Core())
		l.Tokens()
		if !l.Incomplete() {
			t.Errorf("%q: want Incomplete", src)
		}
	}
}

func TestArithmeticCommand(t *testing.T) {
	tests := []struct {
		name, src string
		d         Dialect
		want      string
	}{
		{"basic", `(( 1+1 ))`, Core(), `arith-cmd{ 1+1 }`},
		{
			// The point of scanning raw: lexing this > as a redirection would
			// lose the program.
			"comparison stays inside", `(( 2 > 1 ))`, Core(), `arith-cmd{ 2 > 1 }`,
		},
		{"inner parens", `(( (1+2)*3 ))`, Core(), `arith-cmd{ (1+2)*3 }`},
		{
			// Measured: bash, ksh and zsh all treat this as arithmetic even
			// with no space, so no command-position knowledge is needed.
			"no space is still arithmetic", `((echo nested))`, Core(), `arith-cmd{echo nested}`,
		},
		{
			// A space makes it a subshell containing a subshell. The
			// distinction is purely textual.
			"a space makes it subshells", `( (echo sub) )`, Core(),
			`( ( word(echo) word(sub) ) )`,
		},
		{
			// Where the dialect lacks it, the text still lexes and means
			// something else — nested subshells running 1+1 as a command,
			// which is what dash does.
			"absent from posix", `(( 1+1 ))`, POSIX(), `( ( word(1+1) ) )`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lex(t, tc.src, tc.d); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestArithmeticCommandUnterminatedIsIncomplete(t *testing.T) {
	l := NewLexer(`(( 1+1`, Core())
	l.Tokens()
	if !l.Incomplete() {
		t.Error("want Incomplete for an unterminated arithmetic command")
	}
}

func TestDoubleBracketIsLeftToTheParser(t *testing.T) {
	// `[[ ]]` deliberately gets no lexer mode. Inside it, < and > are
	// comparisons rather than redirections, which sounds like a lexer
	// concern — but `[[` is only special in command position (`echo [[ a ]]`
	// prints `[[ a ]]`), and the lexer does not know where commands begin.
	//
	// Lexing < as an operator loses nothing: the parser knows it is inside
	// `[[ ]]` and reinterprets the token. This test pins the contract between
	// the two layers so a later "fix" in the lexer has to argue with it.
	if got, want := lex(t, `[[ a < b ]]`, Core()), `word([[) word(a) < word(b) word(]])`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// And the case a lexer mode would have broken.
	if got, want := lex(t, `echo [[ a ]]`, Core()), `word(echo) word([[) word(a) word(]])`; got != want {
		t.Errorf("echo case: got %s, want %s", got, want)
	}
}

// withCaseContinue is Core plus the one flag under test. A core test names a
// flag rather than a shell: which shells set it is the dialect packages'
// business, and this file must not know they exist.
func withCaseContinue() Dialect {
	d := Core()
	d.CaseContinue = true
	return d
}

// withDollarDoubleQuote is Core plus the one flag under test, for the same
// reason: which shells set it is the dialect packages' business.
func withDollarDoubleQuote() Dialect {
	d := Core()
	d.DollarDoubleQuote = true
	return d
}

// TestHeredocSpansTreatQuotesAsOrdinary: a here-document body is not a word.
// It is closest to a double-quoted string, and the difference is exactly the
// quote: there is nothing for one to quote, so it is content.
//
// The word lexer was used here instead, and the result had no error and no
// non-zero status — `don't` simply arrived as `dont`.
func TestHeredocSpansTreatQuotesAsOrdinary(t *testing.T) {
	literal := func(spans []Span) string {
		var b strings.Builder
		for _, s := range spans {
			if s.Kind != Literal {
				b.WriteString("<" + s.Kind.String() + ">")
				continue
			}
			b.WriteString(s.Value)
		}
		return b.String()
	}
	for _, tc := range []struct{ body, want string }{
		// Quotes are content, single and double alike.
		{`don't`, `don't`},
		{`say "hi"`, `say "hi"`},
		{`'wholly quoted'`, `'wholly quoted'`},
		// A backslash escapes only these three, and disappears doing it.
		{`\$x`, `$x`},
		{"\\`x", "`x"},
		{`\\`, `\`},
		// Before anything else it stays, and so does what follows.
		{`\n`, `\n`},
		{`\'`, `\'`},
		{`\"`, `\"`},
		{`\*`, `\*`},
		// A backslash before a newline joins the lines with nothing between.
		{"abc\\\ndef", "abcdef"},
		// And the substitutions are spans of their own.
		{`$x`, `<parameter expansion>`},
		{`${x}`, `<parameter expansion>`},
		{`$(echo hi)`, `<command substitution>`},
		{`$((1+2))`, `<arithmetic substitution>`},
		{"`echo hi`", `<command substitution>`},
		// A `$` with nothing a name can start with is just a dollar.
		{`$ end`, `$ end`},
	} {
		if got := literal(HeredocSpans(tc.body, Core())); got != tc.want {
			t.Errorf("%q gave %q, want %q", tc.body, got, tc.want)
		}
	}
}
