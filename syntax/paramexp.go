// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// ParamOp is the operator inside `${ … }`.
type ParamOp uint8

const (
	// ParamNone is a plain reference, `${x}`.
	ParamNone ParamOp = iota
	// ParamDefault is `-`: substitute the word when the test fires.
	ParamDefault
	// ParamAssign is `=`: assign the word, then substitute it. The only
	// expansion with a side effect.
	ParamAssign
	// ParamError is `?`: write the word as an error and exit.
	ParamError
	// ParamAlternate is `+`: substitute the word when the test does *not*
	// fire.
	ParamAlternate
	// ParamTrimPrefix is `#`, ParamTrimPrefixLong `##`. Doubling the
	// operator is what selects the longer match; there is no greediness
	// syntax inside the pattern.
	ParamTrimPrefix
	ParamTrimPrefixLong
	// ParamTrimSuffix is `%`, ParamTrimSuffixLong `%%`.
	ParamTrimSuffix
	ParamTrimSuffixLong
	// ParamReplace is `/`, with All and Anchor saying which spelling.
	ParamReplace
	// ParamSubstring is `${x:off:len}`.
	ParamSubstring
	// ParamUpperFirst is `^`, ParamLowerFirst `,` and ParamToggleFirst `~`:
	// the first character only, and only when it matches the operator's
	// pattern. `${x^b}` on `abc` is `abc`, because the first character is
	// not a `b`.
	ParamUpperFirst
	ParamLowerFirst
	ParamToggleFirst
	// ParamToggle is `~~`, which swaps the case of every character the
	// pattern matches, as ParamUpper and ParamLower do in one direction.
	ParamToggle
	// ParamUpper is `^^` and ParamLower `,,`. bash alone, so they are
	// rejected unless the dialect has them.
	ParamUpper
	ParamLower
)

func (o ParamOp) String() string {
	switch o {
	case ParamDefault:
		return "-"
	case ParamAssign:
		return "="
	case ParamError:
		return "?"
	case ParamAlternate:
		return "+"
	case ParamTrimPrefix:
		return "#"
	case ParamTrimPrefixLong:
		return "##"
	case ParamTrimSuffix:
		return "%"
	case ParamTrimSuffixLong:
		return "%%"
	case ParamReplace:
		return "/"
	case ParamSubstring:
		return ":"
	case ParamUpperFirst:
		return "^"
	case ParamLowerFirst:
		return ","
	case ParamToggleFirst:
		return "~"
	case ParamToggle:
		return "~~"
	case ParamUpper:
		return "^^"
	case ParamLower:
		return ",,"
	}
	return ""
}

// ParamExpr is a parsed `${ … }`.
type ParamExpr struct {
	// Name is the parameter: a name, a digit, or a special such as @ * # ?.
	Name string
	// Index is the `[…]` subscript, nil when absent. Its base is a dialect
	// question — bash and ksh93 count from 0, zsh from 1 — so nothing here
	// interprets it.
	Index *Word
	// Length is `${#x}`.
	Length bool
	// Indirect is `${!x}`. bash alone means indirection by it; ksh93 means
	// something else and does not error, so a dialect that lacks it must
	// refuse rather than guess.
	Indirect bool
	// Prefix is `${!name@}` or `${!name*}`, which yields the *names* of the
	// variables beginning with name rather than any value. It carries the
	// `@` or the `*`, which differ exactly as `$@` and `$*` do: one field
	// each, or one field joined.
	Prefix byte

	Op ParamOp
	// Colon records the `:` that extends the test from "unset" to "unset or
	// empty". It is the whole difference between the two rows of
	// conditionals.
	Colon bool

	// Arg is the operand: the word for `-=?+`, the pattern for `#%/`, the
	// offset for a substring. It is a *word* and not a string, because it is
	// itself expanded — `${u:-$(echo sub)}` yields sub.
	Arg *Word
	// Arg2 is the replacement for `/`, or the length for a substring.
	Arg2 *Word
	// All is `//`, replacing every match rather than the first.
	All bool
	// Anchor is '#' for `/#` or '%' for `/%`, and 0 otherwise.
	Anchor byte

	// Bad marks an expansion whose operator the grammar did not recognize,
	// in a dialect that diagnoses that when the expansion is reached rather
	// than when it is read. Src holds the inside of the braces for the
	// report; every other field on a Bad node is meaningless.
	Bad bool
	// Src is the raw text between the braces, kept for the diagnostic.
	Src string

	Start Pos
	Stop  Pos
}

func (p *ParamExpr) Pos() Pos { return p.Start }
func (p *ParamExpr) End() Pos { return p.Stop }

// specialParams are the one-character parameters that are not names.
const specialParams = "@*#?-$!0123456789"

// parseParamExp parses the text between `${` and `}`.
//
// The lexer already found the matching brace, tracking quoting so a `}` inside
// quotes did not end it early, so src here is exactly the inside.
func (p *Parser) parseParamExp(src string, start Pos) *ParamExpr {
	e := &ParamExpr{Start: start, Stop: start}
	s := src

	switch {
	case strings.HasPrefix(s, "#") && len(s) > 1:
		e.Length = true
		s = s[1:]
	case strings.HasPrefix(s, "!") && len(s) > 1:
		if !p.dialect.ParamIndirection {
			// Refused rather than guessed: ksh93 accepts this and means
			// something else, so a dialect without it cannot pretend.
			p.failKind(ErrBadSubstitution, "${!name} is not available in this dialect")
			return e
		}
		e.Indirect = true
		s = s[1:]
	}

	e.Name, s = scanParamName(s)
	if e.Name == "" {
		p.failKind(ErrBadSubstitution, "expected a parameter name in ${%s}", src)
		return e
	}

	// `${!name@}` and `${!name*}` are the names beginning with name, not a
	// value at all. Only after `!`, and only when the whole rest is the one
	// character — `${!name@U}` is an operator on an indirection and not this.
	if e.Indirect && (s == "@" || s == "*") {
		e.Prefix = s[0]
		return e
	}

	if p.dialect.ArraySubscript && strings.HasPrefix(s, "[") {
		if i := closingBracket(s); i > 0 {
			e.Index = p.wordFrom(s[1:i], start)
			s = s[i+1:]
		}
	}
	if s == "" {
		return e
	}

	op, rest, ok := p.scanParamOp(s, e)
	if !ok {
		if !p.dialect.BadSubstitutionAtParseTime {
			// The majority defers: the node is kept, marked, and diagnosed
			// only if the expansion is ever reached — an unrecognized
			// operator in a branch never taken is not an error at all.
			e.Bad, e.Src = true, src
			return e
		}
		// The operator itself travels with the failure: one dialect does not
		// call this a bad substitution at all, but a syntax error naming the
		// character it could not read.
		p.failKind(ErrBadSubstitution, "unknown operator in ${%s}", src)
		if pe, isErr := p.err.(*Error); isErr {
			pe.Token = firstRune(s)
		}
		return e
	}
	e.Op = op
	p.fillParamArgs(e, rest, start)
	return e
}

// scanParamName reads the parameter, which is either a name or one of the
// single characters that are parameters in their own right.
func scanParamName(s string) (name, rest string) {
	if s == "" {
		return "", ""
	}
	if strings.IndexByte(specialParams, s[0]) >= 0 && !isNameStart(s[0]) {
		// A digit may begin a multi-digit positional parameter.
		if s[0] >= '0' && s[0] <= '9' {
			i := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			return s[:i], s[i:]
		}
		return s[:1], s[1:]
	}
	i := 0
	for i < len(s) && isNameByte(s[i], i) {
		i++
	}
	return s[:i], s[i:]
}

// closingBracket finds the `]` that closes the subscript s opens with, or -1.
//
// Matched rather than *last*, which is what this used to take. A colon-less
// operator whose value holds another subscripted expansion —
// `${tags[@]+${tags[@]}}`, the standard way to expand a possibly-empty array
// under `set -u` — put a second `]` in the string, and taking the last one
// swallowed everything between. The simple `${a[@]+x}` worked, which is why
// it went unnoticed until a real script used the nested form.
//
// Depth counting, because a subscript may itself hold one: `${a[b[0]]}`.
func closingBracket(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameByte(c byte, i int) bool {
	return isNameStart(c) || (i > 0 && c >= '0' && c <= '9')
}

// scanParamOp reads the operator and reports the text after it.
func (p *Parser) scanParamOp(s string, e *ParamExpr) (ParamOp, string, bool) {
	// The colon-prefixed conditionals first, so `:-` is not read as a
	// substring whose offset begins with a minus.
	if len(s) >= 2 && s[0] == ':' && strings.IndexByte("-=?+", s[1]) >= 0 {
		e.Colon = true
		return condOp(s[1]), s[2:], true
	}
	switch {
	case s[0] == ':':
		if !p.dialect.ParamSubstring {
			return 0, "", false
		}
		return ParamSubstring, s[1:], true
	case strings.IndexByte("-=?+", s[0]) >= 0:
		return condOp(s[0]), s[1:], true
	case strings.HasPrefix(s, "##"):
		return ParamTrimPrefixLong, s[2:], true
	case s[0] == '#':
		return ParamTrimPrefix, s[1:], true
	case strings.HasPrefix(s, "%%"):
		return ParamTrimSuffixLong, s[2:], true
	case s[0] == '%':
		return ParamTrimSuffix, s[1:], true
	case s[0] == '/':
		if !p.dialect.ParamSubstitution {
			return 0, "", false
		}
		rest := s[1:]
		switch {
		case strings.HasPrefix(rest, "/"):
			e.All = true
			rest = rest[1:]
		case strings.HasPrefix(rest, "#"), strings.HasPrefix(rest, "%"):
			e.Anchor = rest[0]
			rest = rest[1:]
		}
		return ParamReplace, rest, true
	case strings.HasPrefix(s, "^"), strings.HasPrefix(s, ","), strings.HasPrefix(s, "~"):
		if !p.dialect.ParamCaseChange {
			return 0, "", false
		}
		// Doubled is every character, single is the first — and what follows
		// either is a *pattern* saying which characters count, so the rest is
		// handed on rather than required to be empty.
		if len(s) > 1 && s[1] == s[0] {
			switch s[0] {
			case '^':
				return ParamUpper, s[2:], true
			case ',':
				return ParamLower, s[2:], true
			}
			return ParamToggle, s[2:], true
		}
		switch s[0] {
		case '^':
			return ParamUpperFirst, s[1:], true
		case ',':
			return ParamLowerFirst, s[1:], true
		}
		return ParamToggleFirst, s[1:], true
	}
	return 0, "", false
}

func condOp(c byte) ParamOp {
	switch c {
	case '-':
		return ParamDefault
	case '=':
		return ParamAssign
	case '?':
		return ParamError
	}
	return ParamAlternate
}

// fillParamArgs splits the operand text according to the operator.
func (p *Parser) fillParamArgs(e *ParamExpr, rest string, start Pos) {
	switch e.Op {
	case ParamReplace:
		// The separator is an unquoted slash, so a slash inside quotes or
		// after a backslash belongs to the pattern.
		if i := indexUnquoted(rest, '/'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start)
			e.Arg2 = p.wordFrom(rest[i+1:], start)
		} else {
			// Omitting the replacement deletes the match.
			e.Arg = p.wordFrom(rest, start)
		}
	case ParamSubstring:
		if i := indexUnquoted(rest, ':'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start)
			e.Arg2 = p.wordFrom(rest[i+1:], start)
		} else {
			e.Arg = p.wordFrom(rest, start)
		}
	case ParamUpper, ParamLower, ParamToggle,
		ParamUpperFirst, ParamLowerFirst, ParamToggleFirst:
		// What follows is the pattern saying which characters to convert.
		// Empty means every one of them, which is what `?` would say.
		if rest != "" {
			e.Arg = p.wordFrom(rest, start)
		}
	default:
		if rest != "" {
			e.Arg = p.wordFrom(rest, start)
		}
	}
}

// indexUnquoted finds c outside quotes and not backslash-escaped.
func indexUnquoted(s string, c byte) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case ch == '\\' && quote != '\'':
			i++
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == c:
			return i
		}
	}
	return -1
}

// wordFrom lexes text as a word, so an operand keeps its structure: the word
// in `${x:-word}` is itself expanded, and `${u:-$(echo sub)}` yields sub.
func (p *Parser) wordFrom(text string, at Pos) *Word {
	if text == "" {
		return &Word{Start: at, Stop: at}
	}
	if p.depth >= maxParamDepth {
		p.fail("expansions nested too deeply")
		return &Word{Start: at, Stop: at}
	}
	p.depth++
	defer func() { p.depth-- }()

	sub := NewLexer(text, p.dialect)
	w := &Word{Start: at, Stop: at}
	// Whatever the lexer stepped over between two tokens is text here, not a
	// separator: `${u:-a b}` is the two words `a b` and not `ab`. A lexer
	// reading a command is right to drop the blank between two words, and an
	// operand is not a command — everything up to the closing brace belongs
	// to it. The gap is put back by offset rather than guessed at, so a run
	// of spaces, a tab, or a `#` the lexer read as a comment all survive as
	// what they were.
	last := 0
	gap := func(upto int) {
		if upto > last && last >= 0 && upto <= len(text) {
			w.Spans = append(w.Spans, Span{Kind: Literal, Value: text[last:upto], Pos: at})
		}
	}
	for {
		t := sub.Next()
		if t.Kind == TokEOF {
			gap(len(text))
			break
		}
		gap(t.Pos.Offset)
		last = t.End.Offset
		if t.Kind == TokWord {
			// Through newWord, so a nested ${ } in an operand is parsed too.
			nested := p.newWord(t.Spans, at, at)
			w.Spans = append(w.Spans, nested.Spans...)
			continue
		}
		// An operator inside an operand is ordinary text there — `${x:-a>b}`
		// has no redirection in it — so it is kept as a literal span rather
		// than being dropped.
		w.Spans = append(w.Spans, Span{Kind: Literal, Value: t.Text, Pos: at})
	}
	return w
}

// firstRune is the one character a diagnostic names when the operator it
// could not read is longer than one — `${x^^}` is `^` unexpected.
func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}
