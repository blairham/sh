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
	// TokEOF is returned once the input is exhausted, repeatedly.
	TokEOF Kind = iota

	// TokWord is a word, possibly built from differently quoted spans. Its
	// Spans field is what carries the quoting; see [Token].
	TokWord

	// TokIONumber is a run of digits immediately followed by a redirection
	// operator, which makes it a file descriptor rather than an argument.
	// `echo 1>b` writes an empty file and `echo 1 >b` writes "1"; one space
	// changes what the digit is.
	TokIONumber

	// TokNewline is significant: it terminates a command like `;` does, and it
	// is where a pending here-document's body begins.
	TokNewline

	// TokArithCmd is `(( expr ))` used as a command. Its Text is the expression,
	// scanned raw: what is inside is an arithmetic expression rather than a
	// command list, so tokenizing it as commands would lose it.
	TokArithCmd

	// Control operators.
	TokAmp        // &
	TokAmpBang    // &!   background it and let go of it
	TokAmpPipe    // &|   the same, spelled the other way
	TokAndAnd     // &&
	TokPipe       // |
	TokOrOr       // ||
	TokPipeAmp    // |&   pipe both streams; two bytes with no blank between
	TokSemi       // ;
	TokDSemi      // ;;
	TokSemiAmp    // ;&   fall through to the next case body
	TokDSemiAmp   // ;;&  keep testing later case patterns
	TokSemiPipe   // ;|   the same, spelled zsh's way
	TokLeftParen  // (
	TokRightParen // )

	// Redirection operators.
	TokLess      // <
	TokGreat     // >
	TokDGreat    // >>
	TokLessAmp   // <&
	TokGreatAmp  // >&
	TokLessGreat // <>
	TokClobber   // >|
	// The clobber-override marker in its other spellings. Core has `|` on
	// `>` alone; one dialect takes `|` or `!` after any of the four write
	// operators, which is [Dialect.ClobberOverrideMarker] and these seven
	// tokens. Each is its own kind because the printer writes a redirection
	// back with the spelling it was read with.
	TokClobberBang      // >!
	TokDGreatClobber    // >>|
	TokDGreatBang       // >>!
	TokAmpGreatClobber  // &>|
	TokAmpGreatBang     // &>!
	TokAmpDGreatClobber // &>>|
	TokAmpDGreatBang    // &>>!
	TokDLess            // <<
	TokDLessDash        // <<-
	TokTLess            // <<<  herestring
	TokAmpGreat         // &>   both streams
	TokAmpDGreat        // &>>  both streams, appending
)

// text is the source spelling of each operator, and the table the lexer
// matches against. Longest match wins, which is why callers must not assume
// this is ordered by anything but Kind.
var text = map[Kind]string{
	TokAmp: "&", TokAmpBang: "&!", TokAmpPipe: "&|",
	TokAndAnd: "&&", TokPipe: "|", TokOrOr: "||", TokPipeAmp: "|&",
	TokSemi: ";", TokDSemi: ";;", TokSemiAmp: ";&", TokDSemiAmp: ";;&",
	TokSemiPipe:  ";|",
	TokLeftParen: "(", TokRightParen: ")",
	TokLess: "<", TokGreat: ">", TokDGreat: ">>", TokLessAmp: "<&", TokGreatAmp: ">&",
	TokLessGreat: "<>", TokClobber: ">|", TokClobberBang: ">!",
	TokDGreatClobber: ">>|", TokDGreatBang: ">>!",
	TokAmpGreatClobber: "&>|", TokAmpGreatBang: "&>!",
	TokAmpDGreatClobber: "&>>|", TokAmpDGreatBang: "&>>!",
	TokDLess: "<<", TokDLessDash: "<<-",
	TokTLess: "<<<", TokAmpGreat: "&>", TokAmpDGreat: "&>>",
}

// String returns the operator's spelling, or a name for the non-operators.
func (k Kind) String() string {
	if s, ok := text[k]; ok {
		return s
	}
	switch k {
	case TokEOF:
		return "end of input"
	case TokWord:
		return "word"
	case TokIONumber:
		return "IO number"
	case TokNewline:
		return "newline"
	case TokArithCmd:
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
	case TokLess, TokGreat, TokDGreat, TokLessAmp, TokGreatAmp, TokLessGreat, TokClobber,
		TokClobberBang, TokDGreatClobber, TokDGreatBang,
		TokAmpGreatClobber, TokAmpGreatBang, TokAmpDGreatClobber, TokAmpDGreatBang,
		TokDLess, TokDLessDash, TokTLess, TokAmpGreat, TokAmpDGreat:
		return true
	}
	return false
}

// IsHeredoc reports whether k begins a here-document, whose body is read from
// the lines after the current one rather than from the token stream.
func (k Kind) IsHeredoc() bool { return k == TokDLess || k == TokDLessDash }

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
	// BackslashQuoted is a single character protected by an unquoted
	// backslash. It is its own value rather than folded into the literal
	// text around it, because the protection has to survive: `\*` and `'*'`
	// behave identically, and a field that forgot the backslash would glob.
	BackslashQuoted
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
	// ProcSubstIn is <(...) and ProcSubstOut is >(...). Value is the inner
	// source, unparsed, exactly as for a command substitution: what is inside
	// is a program.
	//
	// Two kinds rather than one with a direction on it, because the direction
	// is not a property of the substitution so much as which end of the pipe
	// the word names. `<(cmd)` gives a path to read cmd's output from and
	// `>(cmd)` a path to write cmd's input to, and nothing that handles one
	// handles the other by changing a flag.
	ProcSubstIn
	ProcSubstOut
	// ProcSubstFile is `=(...)`: the same construct with a *file* where the
	// other two have a pipe. Value is the inner source, unparsed, as for the
	// other two.
	//
	// A third kind rather than a flag on the first, for the reason there are
	// two already: nothing that handles one handles another by changing a
	// field. `<(cmd)` hands over a path that is read *while* cmd writes, and
	// the shell carries on; `=(cmd)` runs cmd to completion, puts its output
	// in a regular file, and hands over that path — so the word is seekable
	// and reopenable, which is the whole reason the construct exists where a
	// pipe will not do. One dialect has it; see
	// Dialect.ProcessSubstitutionToFile.
	ProcSubstFile
)

func (k SpanKind) String() string {
	switch k {
	case CommandSubst:
		return "command substitution"
	case ArithSubst:
		return "arithmetic substitution"
	case ParamExp:
		return "parameter expansion"
	case ProcSubstIn, ProcSubstOut, ProcSubstFile:
		return "process substitution"
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
//
// The field order is deliberate and is not the order to read them in. Wide
// fields first, then the one-byte ones together at the end: written in
// reading order — a kind, a value, a quoting — Go padded the struct out to
// 88 bytes, of which 15 were holes. There are about 185,000 of these in the
// tree a real interactive startup leaves behind, so the holes were 3MB of
// resident memory holding nothing (#2073). Reordering for legibility costs
// that again.
type Span struct {
	// Value is the span's text with its own delimiters removed — quotes for a
	// literal, `$(` and `)` for a substitution — and with no expansion
	// performed and no escapes resolved. Quote removal proper happens at the
	// end of expansion, not here.
	Value string

	// Arith is the parsed form of an ArithSubst span, for the same reason.
	Arith ArithExpr

	// Param is the parsed form of a ParamExp span, filled by the parser. The
	// lexer leaves it nil: finding the closing brace and understanding the
	// operators are different jobs, and only the second needs a dialect.
	Param *ParamExpr

	// Pos is where the span starts, including its opening delimiter.
	Pos Pos

	// Kind says whether this is literal text or a substitution.
	Kind SpanKind

	// Quoting is the quoting this span sits in. For a substitution it decides
	// only whether the result is split afterwards, not what the span is.
	Quoting Quoting

	// CurrentShell says a command substitution was written `${ cmd;}`, which
	// runs in the shell that read it rather than in a subshell — so what it
	// assigns survives, which is the only reason the spelling exists.
	//
	// A flag rather than a kind of its own for the same reason Backquoted is
	// one: everything that *runs* a substitution treats the three alike, and
	// only the reading, the writing back and the choice of shell differ.
	CurrentShell bool

	// Backquoted says a command substitution was written `` `like this` ``
	// rather than as `$( … )`.
	//
	// The two are one node to everything that runs them, and they are not
	// the same syntax: a backquoted one ends at its closing backquote and
	// the other ends where its contents end. So a substitution holding a
	// here-document whose delimiter never matches — which happens, in a
	// script installed on this machine — is terminated by the backquote and
	// would not be terminated by a parenthesis. Anything writing one back
	// has to write the spelling that was read.
	Backquoted bool

	// Comments is the rule a `#` in this span's body was read under, for the
	// spans that hold a program and are read again by whatever runs them.
	// The zero value is [CommentsSkipped], the ordinary rule.
	//
	// It travels for the reason Backquoted does: a substitution's body is
	// kept as the text it was written as and parsed a second time when it
	// runs, and a second parse under a different rule is a second shell. The
	// front end is the one that can set the rule — see [Dialect.Comments] —
	// so without this the *typed* line reads a `#` as a character and the
	// body inside it reads the same `#` as a comment.
	//
	// Measured on zsh 5.9.2, 2026-09-12, `-f -i` on a pipe with the option
	// off: `echo M-$(echo a #b)` answers `M-a #b`, and so does the same line
	// with the substitution nested twice or written with backquotes. The
	// rule reaches the whole of the text that was typed, at every depth.
	Comments CommentMode

	// Bracketed says an arithmetic substitution was written `$[ … ]` rather
	// than `$(( … ))`.
	//
	// The same reasoning Backquoted carries, one construct along: the two are
	// one node to everything that evaluates them — the expression grammar is
	// the same and so is every diagnostic it produces — and they are not the
	// same syntax, because one ends at a `]` and the other at a `))`. So
	// anything writing one back has to write the spelling that was read, and
	// a printer that normalized `$[1+1]` into `$((1+1))` would be editing a
	// script rather than printing it.
	Bracketed bool

	// PatternGroup says this literal span is the text of a parenthesised
	// group that belongs to the word — `(a|b)` in `echo (a|b)`, `@(a|b)`
	// or a `case` arm's pattern — so its `(`, `|` and `)` are the
	// pattern's rather than the shell's.
	//
	// The note has to be in the tree because a character cannot be read
	// off the value: an unquoted `(` in a word is a group's where the
	// group grammar accepted one and a literal parenthesis where a
	// backslash or a quote put it there, and both arrive as the same byte.
	// Nothing that *runs* a pattern needs this — the matcher reads the
	// group out of the text, as it reads a bracket expression — and
	// anything writing the word back does: printing `\(a\|b\)` for
	// `(a|b)` leaves the tree identical and turns a group into three
	// literal characters, which is a different program at exit 0 (#1221).
	//
	// Set on the unquoted spans of a group and on those alone. A quoted
	// span inside one carries its own quoting and is written back with it,
	// which is what says the protection was the script's rather than the
	// printer's.
	//
	// Setting it on the quoted spans as well is an equivalent mutant, and
	// is recorded here so the next reader does not go looking for the row
	// that would kill it: the printer is the only thing that reads this
	// field, and it reads it in the arm it takes for unquoted text alone —
	// a quoted span is written from its quoting before the question is
	// asked.
	PatternGroup bool

	// Bare says a parameter expansion was written `$name` rather than
	// `${name}`.
	//
	// The two are the same node to everything that expands one, which is why
	// the flag is here and not a kind of its own — and they are not the same
	// *text*, because the short form has no closing brace to stop it. So a
	// `[` after the name belongs to the expansion only where a grammar flag
	// puts it there, and whether it is then read as a subscript is a
	// question the run answers: a shell can have the construct and still be
	// told, at run time, that the brackets after an unbraced name are
	// ordinary characters. Nothing can ask that once the two spellings are
	// indistinguishable, which is what this records.
	//
	// Only the parser reads it, and only to hand ParamExpr.BareIndexText the
	// subscript's text.
	Bare bool
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

	// Spans is set for [TokWord] tokens and is empty otherwise.
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
