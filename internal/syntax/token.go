// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// Kind classifies a token.
//
// Operators get a kind each rather than a shared kind with the text attached.
// The parser switches on them, and a dialect decides whether some of them
// exist at all, so both want a value rather than a string comparison.
type Kind uint8

//go:generate stringer -type=Kind
const (
	// EOF is returned once the input is exhausted, repeatedly.
	EOF Kind = iota

	// Word is a word, possibly built from differently quoted spans. Its
	// Spans field is what carries the quoting; see [Token].
	Word

	// IONumber is a run of digits immediately followed by a redirection
	// operator, which makes it a file descriptor rather than an argument.
	// `echo 1>b` writes an empty file and `echo 1 >b` writes "1"; one space
	// changes what the digit is.
	IONumber

	// Newline is significant: it terminates a command like `;` does, and it
	// is where a pending here-document's body begins.
	Newline

	// ArithCmd is `(( expr ))` used as a command. Its Text is the expression,
	// scanned raw: what is inside is an arithmetic expression rather than a
	// command list, so tokenizing it as commands would lose it.
	ArithCmd

	// Control operators.
	Amp        // &
	AndAnd     // &&
	Pipe       // |
	OrOr       // ||
	Semi       // ;
	DSemi      // ;;
	SemiAmp    // ;&   fall through to the next case body
	DSemiAmp   // ;;&  keep testing later case patterns
	LeftParen  // (
	RightParen // )

	// Redirection operators.
	Less      // <
	Great     // >
	DGreat    // >>
	LessAmp   // <&
	GreatAmp  // >&
	LessGreat // <>
	Clobber   // >|
	DLess     // <<
	DLessDash // <<-
	TLess     // <<<  herestring
	AmpGreat  // &>   both streams
	AmpDGreat // &>>  both streams, appending
)

// text is the source spelling of each operator, and the table the lexer
// matches against. Longest match wins, which is why callers must not assume
// this is ordered by anything but Kind.
var text = map[Kind]string{
	Amp: "&", AndAnd: "&&", Pipe: "|", OrOr: "||",
	Semi: ";", DSemi: ";;", SemiAmp: ";&", DSemiAmp: ";;&",
	LeftParen: "(", RightParen: ")",
	Less: "<", Great: ">", DGreat: ">>", LessAmp: "<&", GreatAmp: ">&",
	LessGreat: "<>", Clobber: ">|", DLess: "<<", DLessDash: "<<-",
	TLess: "<<<", AmpGreat: "&>", AmpDGreat: "&>>",
}

// String returns the operator's spelling, or a name for the non-operators.
func (k Kind) String() string {
	if s, ok := text[k]; ok {
		return s
	}
	switch k {
	case EOF:
		return "end of input"
	case Word:
		return "word"
	case IONumber:
		return "IO number"
	case Newline:
		return "newline"
	case ArithCmd:
		return "arithmetic command"
	}
	return "unknown token"
}

// IsRedirect reports whether k is a redirection operator. A redirection may
// appear anywhere in a simple command — before the command name or between
// its arguments — so the parser lifts these out of the word list wherever it
// finds them rather than expecting a suffix.
func (k Kind) IsRedirect() bool {
	switch k {
	case Less, Great, DGreat, LessAmp, GreatAmp, LessGreat, Clobber,
		DLess, DLessDash, TLess, AmpGreat, AmpDGreat:
		return true
	}
	return false
}

// IsHeredoc reports whether k begins a here-document, whose body is read from
// the lines after the current one rather than from the token stream.
func (k Kind) IsHeredoc() bool { return k == DLess || k == DLessDash }

// Quoting says how a span of a word was written, which decides what happens
// to it later: only unquoted spans of an expansion result are subject to
// field splitting and pathname expansion.
type Quoting uint8

const (
	// Unquoted text: expansions apply, and the result is split and globbed.
	Unquoted Quoting = iota
	// Single quotes protect everything, including backslash. No escape
	// exists inside them.
	SingleQuoted
	// Double quotes protect everything except $, `, \ and ". A backslash is
	// an escape only before those four and newline; before anything else it
	// is a literal backslash, so "a\nb" is backslash-then-n.
	DoubleQuoted
	// Dollar-single quotes, $'...', where backslash escapes are interpreted.
	// Absent from dash.
	DollarSingleQuoted
)

// SpanKind says what a span is, as distinct from how it was quoted. A
// substitution inside double quotes is still a substitution, and the quoting
// only decides whether its result is split afterwards.
type SpanKind uint8

const (
	// Literal text.
	Literal SpanKind = iota
	// CommandSubst is $(...) or `...`. Value is the inner source, unparsed:
	// what is inside is a program, and parsing it is the parser's job.
	CommandSubst
	// ArithSubst is $((...)). Value is the inner expression, unparsed.
	ArithSubst
	// ParamExp is ${...}. Value is the inner text, unparsed — the operator
	// set inside is a separate specification.
	ParamExp
)

func (k SpanKind) String() string {
	switch k {
	case CommandSubst:
		return "command substitution"
	case ArithSubst:
		return "arithmetic substitution"
	case ParamExp:
		return "parameter expansion"
	}
	return "literal"
}

// Span is a run of a word written with uniform quoting.
//
// A word is a sequence of these rather than a string, because quoting is
// recorded per span and not per word: in a"b c"d the three spans are
// unquoted, double-quoted and unquoted, and the whole is a single word. Later
// stages act only on the unquoted spans, so a lexer that flattened this would
// make field splitting and globbing unimplementable.
type Span struct {
	// Kind says whether this is literal text or a substitution.
	Kind SpanKind
	// Value is the span's text with its own delimiters removed — quotes for a
	// literal, `$(` and `)` for a substitution — and with no expansion
	// performed and no escapes resolved. Quote removal proper happens at the
	// end of expansion, not here.
	Value string
	// Quoting is the quoting this span sits in. For a substitution it decides
	// only whether the result is split afterwards, not what the span is.
	Quoting Quoting
	// Pos is where the span starts, including its opening delimiter.
	Pos Pos
}

// Token is one lexical unit.
type Token struct {
	Kind Kind
	Pos  Pos
	End  Pos

	// Text is the exact source of the token, quotes and all. It is what a
	// caller echoes back when reporting an error, and what a highlighter
	// measures.
	Text string

	// Spans is set for [Word] tokens and is empty otherwise.
	Spans []Span
}

// IsQuoted reports whether any part of the word was quoted. A word can be
// partly quoted, so this is not the same as "the word was written in quotes".
func (t Token) IsQuoted() bool {
	for _, s := range t.Spans {
		if s.Quoting != Unquoted {
			return true
		}
	}
	return false
}

// Literal returns the word's spans joined, which is the word with its quote
// characters removed and nothing else done to it. It is meaningful only when
// the caller has already decided no expansion applies — a here-document
// delimiter, for instance.
func (t Token) Literal() string {
	if len(t.Spans) == 1 {
		return t.Spans[0].Value
	}
	var b []byte
	for _, s := range t.Spans {
		b = append(b, s.Value...)
	}
	return string(b)
}
