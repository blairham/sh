// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"strings"
)

// Lexer turns shell source into tokens.
//
// It is written from docs/spec/grammar/tokenization.md. Two properties are
// requirements rather than niceties, because a shell re-parses the current
// line on every keypress to highlight it:
//
//   - It never panics, on any input.
//   - Incomplete input is distinguishable from invalid input, because a half
//     typed line is the normal case at a prompt and is not an error yet.
type Lexer struct {
	src     string
	dialect Dialect

	off  int
	line int
	col  int

	err        error
	incomplete bool

	// inRegex is set while the token being read is the operand of `=~`. Its
	// parentheses belong to the regular expression rather than to the shell,
	// and in two of the three dialects that have `[[ ]]` so does a bare `|`.
	inRegex bool

	// inCondition is set while the parser is inside `[[ ]]`. One dialect
	// reads pattern groups there and nowhere else, and the lexer is what has
	// to know: whether `(` ends the word is decided before any parser sees a
	// token.
	inCondition bool

	// pending holds here-documents whose bodies have not been read yet.
	//
	// A body starts after the *next newline*, not after the operator — the
	// rest of the line is ordinary input — so the parser registers the
	// redirection when it sees the delimiter and the lexer fills the body in
	// when it reaches the newline. Several on one line are collected in
	// operator order.
	pending       []*Redirect
	pendingQuoted []bool
}

// queueHeredoc registers a redirection whose body is still to be read. The
// parser calls it; the lexer fills r.Heredoc at the next newline.
//
// quoted comes from the parser because it is a property of how the delimiter
// was *written*, which only the raw token still knows: `\EOF` produces the
// same spans as `EOF`, and both make the body literal.
func (l *Lexer) queueHeredoc(r *Redirect, quoted bool) {
	l.pending = append(l.pending, r)
	l.pendingQuoted = append(l.pendingQuoted, quoted)
}

// NewLexer returns a Lexer over src.
func NewLexer(src string, d Dialect) *Lexer {
	return &Lexer{src: src, dialect: d, line: 1, col: 1}
}

// Err reports why lexing stopped early, or nil.
func (l *Lexer) Err() error { return l.err }

// Incomplete reports whether the input ended in the middle of something that
// could still be finished — an unclosed quote, a trailing line continuation.
// A prompt should ask for another line; a script should report an error.
func (l *Lexer) Incomplete() bool { return l.incomplete }

func (l *Lexer) pos() Pos { return Pos{Offset: l.off, Line: l.line, Col: l.col} }

func (l *Lexer) eof() bool { return l.off >= len(l.src) }

func (l *Lexer) peek() byte {
	if l.eof() {
		return 0
	}
	return l.src[l.off]
}

func (l *Lexer) peekAt(n int) byte {
	if l.off+n >= len(l.src) {
		return 0
	}
	return l.src[l.off+n]
}

func (l *Lexer) advance() byte {
	c := l.src[l.off]
	l.off++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *Lexer) fail(p Pos, format string, args ...any) {
	if l.err == nil {
		l.err = fmt.Errorf("%s: %s", p, fmt.Sprintf(format, args...))
	}
}

// isBlank reports whether c separates tokens. TokNewline does not: it is a token.
func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// Next returns the next token. At the end of input it returns TokEOF forever.
func (l *Lexer) Next() Token {
	l.skipBlanksAndComments()
	start := l.pos()

	if l.eof() {
		return Token{Kind: TokEOF, Pos: start, End: start}
	}

	if l.peek() == '\n' {
		l.advance()
		// The newline is where any pending here-document bodies begin.
		l.readHeredocs()
		return Token{Kind: TokNewline, Pos: start, End: l.pos(), Text: "\n"}
	}

	// An IO number is digits *immediately* followed by a redirection. The
	// adjacency is the whole rule: `echo 1>b` writes an empty file because the
	// 1 is a descriptor, and `echo 1 >b` writes "1" because it is an argument.
	if tok, ok := l.tryIONumber(); ok {
		return tok
	}

	// `((` is an arithmetic command; `( (` is a subshell containing one. The
	// distinction is purely textual, which is measured rather than assumed:
	// `((echo nested))` is an arithmetic error in bash, ksh and zsh even with
	// no space, so no knowledge of command position is needed here.
	if l.dialect.ArithCommand && l.peek() == '(' && l.peekAt(1) == '(' {
		return l.scanArithCommand(start)
	}

	// A regular expression's operand owns its parentheses even at the start
	// of it — `[[ x =~ (b) ]]` — and the operator table would otherwise take
	// the `(` before the word scanner ever saw it.
	if l.inRegex && (l.peek() == '(' || l.peek() == ')') {
		return l.scanWord(start)
	}

	// `<(` and `>(` begin a *word* rather than a redirection, so they have to
	// be seen before the operator table takes the `<`. The adjacency is the
	// whole rule, the same way it is for an IO number: `cat <(echo hi)` is a
	// process substitution and `cat < (echo hi)` is a redirection to a
	// subshell, which is a syntax error in every shell that has either.
	if l.startsProcSubst() {
		return l.scanWord(start)
	}

	if k, ok := l.matchOperator(); ok {
		s := text[k]
		for range s {
			l.advance()
		}
		return Token{Kind: k, Pos: start, End: l.pos(), Text: s}
	}

	return l.scanWord(start)
}

// startsProcSubst reports whether the cursor is on `<(` or `>(` in a dialect
// that has process substitution.
func (l *Lexer) startsProcSubst() bool {
	if !l.dialect.ProcessSubstitution {
		return false
	}
	c := l.peek()
	return (c == '<' || c == '>') && l.peekAt(1) == '('
}

// skipBlanksAndComments consumes what separates tokens: blanks, line
// continuations, and comments.
func (l *Lexer) skipBlanksAndComments() {
	for !l.eof() {
		switch c := l.peek(); {
		case isBlank(c):
			l.advance()
		case c == '\\' && l.peekAt(1) == '\n':
			// Removed before tokens are formed, so it can split anything.
			l.advance()
			l.advance()
		case c == '#':
			// Only where a word could begin, which is the case here: mid-word
			// this function is not running. `echo a#b` prints a#b.
			for !l.eof() && l.peek() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

// tryIONumber matches digits followed with no gap by a redirection operator.
func (l *Lexer) tryIONumber() (Token, bool) {
	if c := l.peek(); c < '0' || c > '9' {
		return Token{}, false
	}
	n := 0
	for {
		c := l.peekAt(n)
		if c < '0' || c > '9' {
			break
		}
		n++
	}
	// Strict adjacency: anything but a redirection here and these digits are
	// an ordinary word.
	if c := l.peekAt(n); c != '<' && c != '>' {
		return Token{}, false
	}
	start := l.pos()
	digits := l.src[l.off : l.off+n]
	for range n {
		l.advance()
	}
	return Token{
		Kind:  TokIONumber,
		Pos:   start,
		End:   l.pos(),
		Text:  digits,
		Spans: []Span{{Kind: Literal, Value: digits, Quoting: Unquoted, Pos: start}},
	}, true
}

// operators, longest first. Longest match wins, so the order is the algorithm
// and not merely tidiness: `>>` must be found before `>`.
var operators = []Kind{
	TokDSemiAmp, TokTLess, TokAmpDGreat, TokDLessDash, // 3 bytes
	TokAndAnd, TokOrOr, TokDSemi, TokSemiAmp, TokDGreat, TokLessAmp, TokGreatAmp,
	TokLessGreat, TokClobber, TokDLess, TokAmpGreat, // 2 bytes
	TokAmp, TokPipe, TokSemi, TokLeftParen, TokRightParen, TokLess, TokGreat, // 1 byte
}

// enabled reports whether the dialect has this operator at all.
func (l *Lexer) enabled(k Kind) bool {
	switch k {
	case TokAmpGreat, TokAmpDGreat:
		return l.dialect.AmpersandRedirect
	case TokSemiAmp:
		return l.dialect.CaseFallthrough
	case TokDSemiAmp:
		return l.dialect.CaseContinue
	case TokTLess:
		return l.dialect.Herestring
	}
	return true
}

// matchOperator finds the longest operator the dialect has at the cursor.
//
// Falling back to a shorter match when the longest is disabled is not a
// convenience: it is what the shells do. Where `&>` does not exist, `&>b` is
// `&` followed by `>b`, so the text still lexes and means something else.
func (l *Lexer) matchOperator() (Kind, bool) {
	rest := l.src[l.off:]
	for _, k := range operators {
		if !l.enabled(k) {
			continue
		}
		if strings.HasPrefix(rest, text[k]) {
			return k, true
		}
	}
	return 0, false
}

// isWordEnd reports whether c ends an unquoted word.
func (l *Lexer) isWordEnd(c byte) bool {
	if isBlank(c) || c == '\n' {
		return true
	}
	switch c {
	case '&', '|', ';', '(', ')', '<', '>':
		return true
	}
	return false
}

// endsWord reports whether c closes the word being read.
//
// It is isWordEnd with the one exception that depends on where we are: a `(`
// that opens a pattern group belongs to the word instead of ending it, and the
// switch below cannot see it unless this lets it through.
func (l *Lexer) endsWord(c byte) bool {
	if !l.isWordEnd(c) {
		return false
	}
	// `<(` and `>(` belong to the word rather than ending it, the same way a
	// pattern group's `(` does. Without this the word scanner returns nothing
	// at all where Next has just decided a word starts here, which is not a
	// wrong answer so much as no answer: the cursor never moves.
	if l.startsProcSubst() {
		return false
	}
	if l.inRegex {
		// A regular expression owns its parentheses — a group is taken whole
		// by the scanner above — and owns a bare `|` where the dialect says
		// so: `[[ ab =~ a|b ]]` matches in two of the three shells with
		// `[[ ]]` and is a parse error in the third.
		switch c {
		case '(', ')':
			return false
		case '|':
			return !l.dialect.RegexTakesAlternation
		}
	}
	return c != '(' || !l.opensPatternGroup()
}

// opensPatternGroup reports whether a `(` here belongs to the word.
//
// Two dialects allow it and they allow different things. One takes a group
// only behind a quantifier — `@(`, `?(`, `+(`, `*(`, `!(` — and the other
// takes a bare `(` anywhere inside a word.
//
// Nothing here tests for being mid-word, and it looked as though something
// should: a `(` that *starts* a word opens a subshell. It cannot reach here
// to start one. `(` is in the operator table, so a token beginning with it is
// taken as an operator and scanWord is never entered on one — a guard for it
// would be a line no test could distinguish.
func (l *Lexer) opensPatternGroup() bool {
	// An empty `()` is a function definition and not a group, which is how
	// `f() { … }` survives the rule: the shell that takes bare groups rejects
	// `a()` as a pattern outright, so nothing is lost by leaving it alone.
	if l.peekAt(1) == ')' {
		return false
	}
	// And a `(` straight after `=` opens an array literal, never a group —
	// measured, because it is the same shell: `a=(b|c)` is a parse error
	// there rather than a pattern, so the assignment always wins.
	if l.off > 0 && l.src[l.off-1] == '=' {
		return false
	}
	if l.dialect.PatternAlternation {
		return true
	}
	extended := l.dialect.ExtendedPattern ||
		(l.inCondition && l.dialect.ExtendedPatternInCondition)
	if !extended || l.off == 0 {
		return false
	}
	switch l.src[l.off-1] {
	case '@', '?', '+', '*', '!':
		return true
	}
	return false
}

// scanPatternGroup consumes `( … )` and returns it as written, nesting and
// all. Nothing is interpreted here: the group is text until the matcher reads
// it, exactly as a bracket expression is.
func (l *Lexer) scanPatternGroup() string {
	start := l.off
	depth := 0
	for !l.eof() {
		c := l.advance()
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return l.src[start:l.off]
			}
		}
	}
	// Unterminated: the caller reports the word as unfinished, the same as an
	// unclosed quote.
	l.incomplete = true
	l.fail(l.pos(), "unterminated pattern group")
	return l.src[start:l.off]
}

// scanWord reads a word as a sequence of spans, one per run of uniform
// quoting. The spans are the point: a"b c"d is one word of three spans, and
// only the unquoted ones are subject to splitting and globbing later.
func (l *Lexer) scanWord(start Pos) Token {
	var spans []Span
	var lit strings.Builder
	litPos := start

	flush := func() {
		if lit.Len() > 0 {
			spans = append(spans, Span{Kind: Literal, Value: lit.String(), Quoting: Unquoted, Pos: litPos})
			lit.Reset()
		}
	}

	for !l.eof() {
		c := l.peek()
		if l.endsWord(c) {
			break
		}
		switch {
		case c == '\\' && l.peekAt(1) == '\n':
			l.advance()
			l.advance()

		case c == '\\':
			escPos := l.pos()
			l.advance()
			if l.eof() {
				// A trailing backslash is unfinished rather than wrong.
				l.incomplete = true
				l.fail(l.pos(), "input ends after a backslash")
				break
			}
			// Its own span: the protection must outlive the lexer, because a
			// later stage decides whether the character is a metacharacter.
			flush()
			spans = append(spans, Span{
				Kind:    Literal,
				Value:   string(l.advance()),
				Quoting: BackslashQuoted,
				Pos:     escPos,
			})

		case c == '(' && l.inRegex:
			// A regular expression's group is taken whole, balanced, with
			// whatever is inside it — an alternation in there belongs to the
			// group in all three shells that have `[[ ]]`, so it needs no
			// dialect. Only a *bare* `|` outside one does.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			lit.WriteString(l.scanPatternGroup())

		case c == '(' && l.opensPatternGroup():
			// A parenthesised group belongs to the word rather than ending
			// it. Only mid-word: a leading `(` opens a subshell, or is the
			// paren a `case` arm may carry, and neither is a pattern.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			lit.WriteString(l.scanPatternGroup())

		case c == '\'':
			flush()
			if s, ok := l.scanSingle(); ok {
				spans = append(spans, s)
			}

		case c == '"':
			flush()
			spans = append(spans, l.scanDouble()...)

		case c == '$' && l.peekAt(1) == '\'' && l.dialect.DollarSingleQuote:
			flush()
			if s, ok := l.scanDollarSingle(); ok {
				spans = append(spans, s)
			}

		case c == '$' && l.peekAt(1) == '(' && l.peekAt(2) == '(':
			// `$((` is arithmetic. A command substitution whose first
			// construct is a subshell has to be written `$( (`, which is the
			// only disambiguation available and is decided here: by the time
			// the parser sees tokens the choice has been made.
			flush()
			spans = append(spans, l.scanParens(ArithSubst, Unquoted))

		case c == '$' && l.peekAt(1) == '(':
			flush()
			spans = append(spans, l.scanParens(CommandSubst, Unquoted))

		case l.startsProcSubst():
			// Unquoted only, and that is not an omission: `"<(cmd)"` is the
			// five characters in every shell that has the construct, because
			// what it produces is a *path* and a quoted path is still a path
			// — there would be nothing for the quoting to change.
			flush()
			spans = append(spans, l.scanParens(procSubstKind(c), Unquoted))

		case c == '$' && l.peekAt(1) == '{':
			flush()
			spans = append(spans, l.scanBraces(Unquoted))

		case c == '$' && isBareParam(l.peekAt(1)):
			flush()
			spans = append(spans, l.scanBareParam(Unquoted))

		case c == '`':
			flush()
			spans = append(spans, l.scanBackticks(Unquoted))

		default:
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			lit.WriteByte(l.advance())
		}
		if l.err != nil {
			break
		}
	}
	flush()

	return Token{
		Kind:  TokWord,
		Pos:   start,
		End:   l.pos(),
		Text:  l.src[start.Offset:l.off],
		Spans: spans,
	}
}

// scanSingle reads '...'. Single quotes protect everything, and no escape
// exists inside them — a backslash is an ordinary character.
func (l *Lexer) scanSingle() (Span, bool) {
	open := l.pos()
	l.advance() // '
	var b strings.Builder
	for {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated single quote")
			return Span{Kind: Literal, Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
		}
		if l.peek() == '\'' {
			l.advance()
			return Span{Kind: Literal, Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
		}
		b.WriteByte(l.advance())
	}
}

// dquoteEscapes are the only characters a backslash escapes inside double
// quotes. Before anything else the backslash is literal, so "a\nb" is
// backslash-then-n and not a newline — the rule C intuition gets wrong.
const dquoteEscapes = "$`\"\\"

// heredocEscapes are the only characters a backslash escapes in an unquoted
// here-document body. It is the double-quote set without the quote, because a
// quote there is an ordinary character with nothing to escape.
const heredocEscapes = "$`\\"

// HeredocSpans splits an unquoted here-document body into spans.
//
// A body is not a word and not a double-quoted string, though it is much
// closer to the second: `$name`, `${ }`, `$( )`, `$(( ))` and backticks
// expand, a backslash escapes only those and itself and a newline, and
// everything else is literal — **quotes included**.
//
// Running the word lexer over it instead is what this replaces, and the
// difference is not subtle: quoting rules applied, so `don't` came out as
// `dont` and `\n` as `n`. It printed no error and exited 0, which is the
// worst way to be wrong.
//
// A quoted delimiter is not this. That body is literal throughout and never
// reaches here.
func HeredocSpans(body string, d Dialect) []Span {
	l := NewLexer(body, d)
	return l.heredocSpans()
}

func (l *Lexer) heredocSpans() []Span {
	var out []Span
	var b strings.Builder
	litPos := l.pos()
	flush := func() {
		if b.Len() > 0 {
			out = append(out, Span{Kind: Literal, Value: b.String(), Quoting: DoubleQuoted, Pos: litPos})
			b.Reset()
		}
	}
	for !l.eof() {
		c := l.peek()
		switch {
		// The substitutions, which are the whole reason an unquoted body is
		// treated differently from a quoted one. Marked as double-quoted
		// because that is what stops the result being split: a body is one
		// blob of input, not a list of fields.
		case c == '$' && l.peekAt(1) == '(' && l.peekAt(2) == '(':
			flush()
			out = append(out, l.scanParens(ArithSubst, DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1) == '(':
			flush()
			out = append(out, l.scanParens(CommandSubst, DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1) == '{':
			flush()
			out = append(out, l.scanBraces(DoubleQuoted))
			litPos = l.pos()
		case c == '$' && isBareParam(l.peekAt(1)):
			flush()
			out = append(out, l.scanBareParam(DoubleQuoted))
			litPos = l.pos()
		case c == '`':
			flush()
			out = append(out, l.scanBackticks(DoubleQuoted))
			litPos = l.pos()

		case c == '\\' && l.peekAt(1) == '\n':
			// A continued line, joined with no newline between.
			l.advance()
			l.advance()
		case c == '\\' && strings.IndexByte(heredocEscapes, l.peekAt(1)) >= 0:
			l.advance()
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		default:
			// Everything else, and there is a lot of it: a backslash before
			// an ordinary character stays, both of it, and so does a quote.
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		}
	}
	flush()
	return out
}

func (l *Lexer) scanDouble() []Span {
	open := l.pos()
	l.advance() // "

	var out []Span
	var b strings.Builder
	litPos := l.pos()
	flush := func() {
		if b.Len() > 0 {
			out = append(out, Span{Kind: Literal, Value: b.String(), Quoting: DoubleQuoted, Pos: litPos})
			b.Reset()
		}
	}

	for {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated double quote")
			flush()
			return out
		}
		c := l.peek()
		switch {
		case c == '"':
			l.advance()
			flush()
			if len(out) == 0 {
				// An empty "" still produced a span: it is an empty field,
				// not the absence of one.
				out = append(out, Span{Kind: Literal, Quoting: DoubleQuoted, Pos: open})
			}
			return out

		// Substitutions happen inside double quotes — "$(cmd)" is how most
		// scripts spell a substitution — so they are spans of their own here
		// too. The quoting is carried on them because it decides whether the
		// result is split afterwards, which is the only thing it changes.
		case c == '$' && l.peekAt(1) == '(' && l.peekAt(2) == '(':
			flush()
			out = append(out, l.scanParens(ArithSubst, DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1) == '(':
			flush()
			out = append(out, l.scanParens(CommandSubst, DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1) == '{':
			flush()
			out = append(out, l.scanBraces(DoubleQuoted))
			litPos = l.pos()
		case c == '$' && isBareParam(l.peekAt(1)):
			flush()
			out = append(out, l.scanBareParam(DoubleQuoted))
			litPos = l.pos()
		case c == '`':
			flush()
			out = append(out, l.scanBackticks(DoubleQuoted))
			litPos = l.pos()

		case c == '\\' && l.peekAt(1) == '\n':
			l.advance()
			l.advance()
		case c == '\\' && strings.IndexByte(dquoteEscapes, l.peekAt(1)) >= 0:
			l.advance()
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		default:
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		}
	}
}

// scanDollarSingle reads $'...'.
//
// The escape sequences inside are *not* resolved here. docs/spec does not yet
// say what the table is — which escapes exist, and what the shells disagree
// about — and inventing one in the implementation is exactly what CLEANROOM.md
// forbids. The raw text is preserved so that resolving it later loses nothing.
func (l *Lexer) scanDollarSingle() (Span, bool) {
	open := l.pos()
	l.advance() // $
	l.advance() // '
	var b strings.Builder
	for {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated $' quote")
			return Span{Kind: Literal, Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
		}
		c := l.peek()
		switch {
		case c == '\'':
			l.advance()
			return Span{Kind: Literal, Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
		case c == '\\' && !l.eofAt(1):
			// A backslash consumes the next character whatever it is, so an
			// escaped quote does not end the span. Both bytes are kept.
			b.WriteByte(l.advance())
			b.WriteByte(l.advance())
		default:
			b.WriteByte(l.advance())
		}
	}
}

func (l *Lexer) eofAt(n int) bool { return l.off+n >= len(l.src) }

// Tokens reads the whole input. It is a convenience for tests and for callers
// that are not streaming; it stops at TokEOF or at the first error.
func (l *Lexer) Tokens() []Token {
	var out []Token
	var heredoc Kind
	for {
		t := l.Next()
		out = append(out, t)
		if t.Kind == TokEOF || l.err != nil {
			return out
		}
		// A here-document body is not lexical: where it ends is decided by a
		// delimiter the *parser* normally registers, and without that the
		// body is read as ordinary words. That is wrong in a way this helper
		// used to hide — a body with an apostrophe in it reported an
		// unterminated quote — so the queueing the parser would do is done
		// here too, from the same two tokens it uses.
		switch {
		case t.Kind.IsHeredoc():
			heredoc = t.Kind
		case heredoc != 0 && t.Kind == TokWord:
			// Only the delimiter's text is needed to find the body's end,
			// so the word is the token's spans as they stand: nothing here
			// expands, and a delimiter never does.
			l.queueHeredoc(&Redirect{
				Op:   heredoc,
				Word: &Word{Spans: t.Spans, Start: t.Pos, Stop: t.End},
			}, t.Text != t.Literal())
			heredoc = 0
		default:
			heredoc = 0
		}
	}
}

// scanParens reads $( … ) or $(( … )).
//
// The closing delimiter is not found by counting parens. A `)` inside quotes
// does not close the substitution — `$(echo ")" )` yields `)` in every shell
// in the panel — so the scan tracks quoting as it goes, using the same rules
// as the rest of the lexer. Counting alone truncates the substitution and
// silently changes the program.
func (l *Lexer) scanParens(kind SpanKind, q Quoting) Span {
	open := l.pos()
	l.advance() // $
	l.advance() // (
	depth := 1
	if kind == ArithSubst {
		l.advance() // the second (
		depth = 2
	}

	start := l.off
	for depth > 0 {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated %s", kind)
			break
		}
		switch c := l.peek(); c {
		case '\'':
			l.skipQuoted('\'', false)
		case '"':
			l.skipQuoted('"', true)
		case '`':
			l.skipBackticks()
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '(':
			depth++
			l.advance()
		case ')':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}

	// Trim the closing delimiters the loop consumed.
	end := l.off
	for n := 1; n <= closers(kind) && end > start && l.src[end-1] == ')'; n++ {
		end--
	}
	return Span{Kind: kind, Value: l.src[start:end], Quoting: q, Pos: open}
}

// procSubstKind says which end of the pipe the word will name.
func procSubstKind(c byte) SpanKind {
	if c == '>' {
		return ProcSubstOut
	}
	return ProcSubstIn
}

func closers(k SpanKind) int {
	if k == ArithSubst {
		return 2
	}
	return 1
}

// scanBraces reads ${ … }. Same rule as scanParens: a `}` inside quotes does
// not close it. What the operators inside mean is a separate specification;
// this only finds the end.
func (l *Lexer) scanBraces(q Quoting) Span {
	open := l.pos()
	l.advance() // $
	l.advance() // {
	start := l.off
	depth := 1
	for depth > 0 {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated parameter expansion")
			break
		}
		switch c := l.peek(); c {
		case '\'':
			l.skipQuoted('\'', false)
		case '"':
			l.skipQuoted('"', true)
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '{':
			depth++
			l.advance()
		case '}':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}
	end := l.off
	if end > start && l.src[end-1] == '}' {
		end--
	}
	return Span{Kind: ParamExp, Value: l.src[start:end], Quoting: q, Pos: open}
}

// scanBackticks reads ` … `, the older command substitution. It nests only
// with backslash escaping, which is why $( ) exists and why this form is
// supported but never recommended.
func (l *Lexer) scanBackticks(q Quoting) Span {
	open := l.pos()
	l.advance() // `
	start := l.off
	for {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated backquote substitution")
			return Span{Kind: CommandSubst, Value: unescapeBackquoted(l.src[start:l.off]), Quoting: q, Pos: open}
		}
		switch l.peek() {
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '`':
			end := l.off
			l.advance()
			return Span{Kind: CommandSubst, Value: unescapeBackquoted(l.src[start:end]), Quoting: q, Pos: open}
		default:
			l.advance()
		}
	}
}

// unescapeBackquoted removes the one layer of backslashes the older
// substitution form requires, so the value handed on is the command text.
//
// This belongs to the lexer rather than to whoever evaluates the span,
// because it is part of *recognizing* the construct: POSIX gives the
// backslash its literal meaning inside backquotes except before `$`, a
// backquote, or another backslash. Doing it here is also what makes nesting
// work at all — the inner `\“ becomes a plain backquote, and re-lexing the
// result finds the nested substitution that `$( )` would have made obvious.
func unescapeBackquoted(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '$', '`', '\\':
				i++
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// skipQuoted consumes a quoted run while scanning inside a substitution. It
// does not build a span: the inner text is kept verbatim and re-lexed later by
// whoever parses the substitution.
func (l *Lexer) skipQuoted(quote byte, escapes bool) {
	l.advance() // opening quote
	for !l.eof() {
		c := l.peek()
		if c == quote {
			l.advance()
			return
		}
		if escapes && c == '\\' {
			l.advance()
			if !l.eof() {
				l.advance()
			}
			continue
		}
		l.advance()
	}
}

func (l *Lexer) skipBackticks() {
	l.advance()
	for !l.eof() {
		switch l.peek() {
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '`':
			l.advance()
			return
		default:
			l.advance()
		}
	}
}

// peekIsArithCommand reports whether an arithmetic command begins here.
func (l *Lexer) peekIsArithCommand() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return l.dialect.ArithCommand && i+1 < len(l.src) && l.src[i] == '(' && l.src[i+1] == '('
}

// scanArithCommand reads `(( expr ))` used as a command.
//
// The expression is kept as raw text rather than tokenized, because what is
// inside is an arithmetic expression and not a command list: `(( 2 > 1 ))`
// compares, and lexing that `>` as a redirection would lose the program. The
// operator set inside is a separate specification, so nothing here interprets
// it.
//
// The closing `))` is found the same way substitutions find theirs — tracking
// quoting and nesting rather than counting — so `(( (1+2)*3 ))` works.
func (l *Lexer) scanArithCommand(start Pos) Token {
	l.advance() // (
	l.advance() // (
	depth := 2
	exprStart := l.off

	for depth > 0 {
		if l.eof() {
			l.incomplete = true
			l.fail(start, "unterminated arithmetic command")
			break
		}
		switch l.peek() {
		case '\'':
			l.skipQuoted('\'', false)
		case '"':
			l.skipQuoted('"', true)
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '(':
			depth++
			l.advance()
		case ')':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}

	end := l.off
	for n := 0; n < 2 && end > exprStart && l.src[end-1] == ')'; n++ {
		end--
	}
	expr := l.src[exprStart:end]
	return Token{
		Kind:  TokArithCmd,
		Pos:   start,
		End:   l.pos(),
		Text:  expr,
		Spans: []Span{{Kind: ArithSubst, Value: expr, Quoting: Unquoted, Pos: start}},
	}
}

// peekIsFuncParens reports whether `()` follows the current position with only
// blanks between, which is how a POSIX function definition announces itself.
//
// The parser needs this because `name` and `name()` are indistinguishable
// until the paren: a word at command position is a command name right up to
// the point where it is a function being defined.
// peekIsLeftParen reports whether the next thing is a `(`, whatever follows
// it. Two dialects commit to a function definition there and complain about
// what they find next; the other two never get that far.
func (l *Lexer) peekIsLeftParen() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return i < len(l.src) && l.src[i] == '('
}

func (l *Lexer) peekIsFuncParens() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	if i >= len(l.src) || l.src[i] != '(' {
		return false
	}
	i++
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return i < len(l.src) && l.src[i] == ')'
}

// readHeredocs consumes the bodies of every here-document queued on the line
// just ended, in the order their operators appeared.
func (l *Lexer) readHeredocs() {
	queued, quoted := l.pending, l.pendingQuoted
	l.pending, l.pendingQuoted = nil, nil
	for i, r := range queued {
		l.readOneHeredoc(r, quoted[i])
	}
}

func (l *Lexer) readOneHeredoc(r *Redirect, quoted bool) {
	strip := r.Op == TokDLessDash
	delim := r.Word.Literal()
	start := l.pos()
	var body strings.Builder
	for {
		if l.eof() {
			// Reaching the end without the delimiter is unfinished input
			// rather than a syntax error: bash warns and carries on, and the
			// others take it silently.
			l.incomplete = true
			l.fail(start, "here-document delimited by end of input, wanted %q", delim)
			break
		}
		line, done := l.heredocLine(strip)
		if done == delim {
			break
		}
		body.WriteString(line)
	}

	q := Unquoted
	if quoted {
		q = SingleQuoted
	}
	// The body is kept raw. An unquoted body is subject to expansion, which
	// re-reads it later — the same treatment $( ) and ${ } get, and for the
	// same reason: what is inside is not this stage's to interpret.
	r.Heredoc = &Word{
		Spans: []Span{{Kind: Literal, Value: body.String(), Quoting: q, Pos: start}},
		Start: start,
		Stop:  l.pos(),
	}
}

// heredocLine reads one line. It returns the line including its newline, and
// separately the line's content with tabs stripped when the operator asked for
// it, so the caller can compare that against the delimiter.
func (l *Lexer) heredocLine(strip bool) (line, content string) {
	begin := l.off
	for !l.eof() && l.peek() != '\n' {
		l.advance()
	}
	content = l.src[begin:l.off]
	if !l.eof() {
		l.advance() // the newline
	}
	line = l.src[begin:l.off]
	if strip {
		// Tabs only. Spaces are not stripped, which is why a delimiter
		// indented with spaces never matches.
		line = strings.TrimLeft(line, "\t")
		content = strings.TrimLeft(content, "\t")
	}
	return line, content
}

// bareParamSpecials are the one-character parameters that may follow a `$`
// without braces.
const bareParamSpecials = "@*#?-$!"

// isBareParam reports whether c can begin a `$name` expansion written without
// braces. `$x` and `${x}` mean the same thing, and the short form is by far
// the commoner one, so the lexer has to produce the same span for both.
func isBareParam(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || strings.IndexByte(bareParamSpecials, c) >= 0
}

// scanBareParam reads `$name`, `$1` or `$?` and friends.
//
// A digit is a *single* positional parameter here: `$12` is `$1` followed by
// the character 2, which is why the multi-digit form needs braces. A special
// character is likewise exactly one.
func (l *Lexer) scanBareParam(q Quoting) Span {
	open := l.pos()
	l.advance() // $
	begin := l.off
	switch c := l.peek(); {
	case c >= '0' && c <= '9':
		l.advance()
	case strings.IndexByte(bareParamSpecials, c) >= 0:
		l.advance()
	default:
		for !l.eof() {
			c := l.peek()
			ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9' && l.off > begin)
			if !ok {
				break
			}
			l.advance()
		}
	}
	return Span{Kind: ParamExp, Value: l.src[begin:l.off], Quoting: q, Pos: open}
}
