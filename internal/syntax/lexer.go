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

// isBlank reports whether c separates tokens. Newline does not: it is a token.
func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// Next returns the next token. At the end of input it returns EOF forever.
func (l *Lexer) Next() Token {
	l.skipBlanksAndComments()
	start := l.pos()

	if l.eof() {
		return Token{Kind: EOF, Pos: start, End: start}
	}

	if l.peek() == '\n' {
		l.advance()
		return Token{Kind: Newline, Pos: start, End: l.pos(), Text: "\n"}
	}

	// An IO number is digits *immediately* followed by a redirection. The
	// adjacency is the whole rule: `echo 1>b` writes an empty file because the
	// 1 is a descriptor, and `echo 1 >b` writes "1" because it is an argument.
	if tok, ok := l.tryIONumber(); ok {
		return tok
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
		Kind:  IONumber,
		Pos:   start,
		End:   l.pos(),
		Text:  digits,
		Spans: []Span{{Value: digits, Quoting: Unquoted, Pos: start}},
	}, true
}

// operators, longest first. Longest match wins, so the order is the algorithm
// and not merely tidiness: `>>` must be found before `>`.
var operators = []Kind{
	DSemiAmp, TLess, AmpDGreat, DLessDash, // 3 bytes
	AndAnd, OrOr, DSemi, SemiAmp, DGreat, LessAmp, GreatAmp,
	LessGreat, Clobber, DLess, AmpGreat, // 2 bytes
	Amp, Pipe, Semi, LeftParen, RightParen, Less, Great, // 1 byte
}

// enabled reports whether the dialect has this operator at all.
func (l *Lexer) enabled(k Kind) bool {
	switch k {
	case AmpGreat, AmpDGreat:
		return l.dialect.AmpersandRedirect
	case SemiAmp:
		return l.dialect.CaseFallthrough
	case DSemiAmp:
		return l.dialect.CaseContinue
	case TLess:
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

// scanWord reads a word as a sequence of spans, one per run of uniform
// quoting. The spans are the point: a"b c"d is one word of three spans, and
// only the unquoted ones are subject to splitting and globbing later.
func (l *Lexer) scanWord(start Pos) Token {
	var spans []Span
	var lit strings.Builder
	litPos := start

	flush := func() {
		if lit.Len() > 0 {
			spans = append(spans, Span{Value: lit.String(), Quoting: Unquoted, Pos: litPos})
			lit.Reset()
		}
	}

	for !l.eof() {
		c := l.peek()
		if l.isWordEnd(c) {
			break
		}
		switch {
		case c == '\\' && l.peekAt(1) == '\n':
			l.advance()
			l.advance()

		case c == '\\':
			l.advance()
			if l.eof() {
				// A trailing backslash is unfinished rather than wrong.
				l.incomplete = true
				l.fail(l.pos(), "input ends after a backslash")
				break
			}
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			lit.WriteByte(l.advance())

		case c == '\'':
			flush()
			if s, ok := l.scanSingle(); ok {
				spans = append(spans, s)
			}

		case c == '"':
			flush()
			if s, ok := l.scanDouble(); ok {
				spans = append(spans, s)
			}

		case c == '$' && l.peekAt(1) == '\'' && l.dialect.DollarSingleQuote:
			flush()
			if s, ok := l.scanDollarSingle(); ok {
				spans = append(spans, s)
			}

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
		Kind:  Word,
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
			return Span{Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
		}
		if l.peek() == '\'' {
			l.advance()
			return Span{Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
		}
		b.WriteByte(l.advance())
	}
}

// dquoteEscapes are the only characters a backslash escapes inside double
// quotes. Before anything else the backslash is literal, so "a\nb" is
// backslash-then-n and not a newline — the rule C intuition gets wrong.
const dquoteEscapes = "$`\"\\"

func (l *Lexer) scanDouble() (Span, bool) {
	open := l.pos()
	l.advance() // "
	var b strings.Builder
	for {
		if l.eof() {
			l.incomplete = true
			l.fail(open, "unterminated double quote")
			return Span{Value: b.String(), Quoting: DoubleQuoted, Pos: open}, true
		}
		c := l.peek()
		switch {
		case c == '"':
			l.advance()
			return Span{Value: b.String(), Quoting: DoubleQuoted, Pos: open}, true
		case c == '\\' && l.peekAt(1) == '\n':
			l.advance()
			l.advance()
		case c == '\\' && strings.IndexByte(dquoteEscapes, l.peekAt(1)) >= 0:
			l.advance()
			b.WriteByte(l.advance())
		default:
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
			return Span{Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
		}
		c := l.peek()
		switch {
		case c == '\'':
			l.advance()
			return Span{Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
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
// that are not streaming; it stops at EOF or at the first error.
func (l *Lexer) Tokens() []Token {
	var out []Token
	for {
		t := l.Next()
		out = append(out, t)
		if t.Kind == EOF || l.err != nil {
			return out
		}
	}
}
