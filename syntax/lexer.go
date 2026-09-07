// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
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
	// remarks are what the parser has to say about input it accepted anyway.
	// See Remark.
	remarks []Remark

	// openWord is what the input was inside when it ran out — a quote, an
	// expansion, a here-document. The first one wins: a quote inside a
	// substitution ends the input once, and it is the quote that is waiting.
	openWord string

	// wordStart is where the word being read began, kept for the diagnostic
	// that quotes it back.
	//
	// One dialect names the whole word a construct ran out inside rather
	// than the construct — `v=$(echo hi` and not `$(echo hi` — and the word
	// is not something the text can be searched backwards for. Quoting is
	// what makes it so: `echo "a b"$(echo hi` is one word and names all of
	// it, `echo a\ b$(echo hi` equally, and a scan back to the previous
	// blank would cut both in half. The scanner is the only thing that knows
	// where a word started, so it says (#1022).
	//
	// Zero outside a word, which is a real state rather than an unset one: a
	// here-document body that runs out is inside no word, and that dialect
	// quotes nothing there.
	wordStart Pos

	// inRegex is set while the token being read is the operand of `=~`. Its
	// parentheses belong to the regular expression rather than to the shell,
	// and in two of the three dialects that have `[[ ]]` so does a bare `|`.
	inRegex bool

	// inCondition is set while the parser is inside `[[ ]]`. One dialect
	// reads pattern groups there and nowhere else, and the lexer is what has
	// to know: whether `(` ends the word is decided before any parser sees a
	// token. It also suspends the arithmetic command, whose `((` is two
	// grouping parentheses in a condition.
	inCondition bool

	// inPattern is set while the token being read is the operand of a
	// pattern operator — `==`, `=` or `!=`. Its `(` opens an alternation
	// group rather than anything of the shell's, in the dialect that has
	// bare groups, and that has to be known before the operator table sees
	// the character: `(` is an operator, so a token beginning with one never
	// reaches the word scanner. Exactly the shape inRegex has, and for
	// exactly the same reason.
	inPattern bool

	// inArgument is set while the token being read stands where an
	// *argument* may, rather than where a command may begin. One dialect
	// reads a `(` there as part of the word — a pattern with a list of glob
	// qualifiers — and command position is the whole of the difference:
	// `( x )` written first is a subshell and written after a word is one
	// argument. The lexer cannot tell which on its own, because `(` is in
	// the operator table and a token beginning with one never reaches the
	// word scanner, so the parser sets this before the token is read.
	// Exactly the shape inPattern has, for exactly the same reason.
	inArgument bool

	// inCaseArm is set while the token that *begins* a `case` arm is read.
	//
	// Two things are different there, and both are about a `(` the arm may
	// carry. `((` is not an arithmetic command — `case x in ((a|b))` is an
	// arm's paren in front of a group in the dialect that takes it, where
	// this lexer read the whole of `((a|b))` as one expression and the
	// parser then complained about an arithmetic command that the script
	// never wrote. And a leading `(` belongs to the *pattern* rather than
	// being the arm's own paren exactly when it opens a glob flag: `(#i)a)`
	// is the pattern `(#i)a` with the arm's `)` behind it, while `((#i)a)` is
	// the arm's paren in front of that same pattern. Which of the two it is
	// cannot be seen from the character alone, and `case x in (a)` — one
	// paren, an ordinary pattern — is the row that says so.
	//
	// Exactly the shape inCondition has: a position only the parser knows
	// about, told to the lexer before the token is read.
	inCaseArm bool

	// pending holds here-documents whose bodies have not been read yet.
	//
	// A body starts after the *next newline*, not after the operator — the
	// rest of the line is ordinary input — so the parser registers the
	// redirection when it sees the delimiter and the lexer fills the body in
	// when it reaches the newline. Several on one line are collected in
	// operator order.
	pending       []*Redirect
	pendingQuoted []bool

	// heredocEnd is where the last here-document body read here finished:
	// the line that closed it, which is the delimiter's own line, or the
	// last line there was where the input ended before the delimiter did.
	//
	// It exists because a here-document's body and its delimiter are
	// physical lines of the *command* that owns them, and nothing else in a
	// token says so. The newline token that triggers the read is positioned
	// where the command's first line ended, and the body is consumed behind
	// it — so a caller counting the lines a command occupied sees one line
	// where a script shows three. `set -v` is the caller that minds: it
	// writes the input back as it is read, and it echoed the delimiter after
	// running the command rather than with it.
	heredocEnd Pos
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

// Open is what the lexer was inside when the input ran out, spelled as it is
// written: `'`, `"`, `${`, “ ` “, `<<`. Empty when the input was whole, or
// when what ran out was the parser's rather than the lexer's.
//
// One thing rather than a stack. What the lexer is inside nests through
// recursion — a substitution runs a parser of its own — so the innermost is
// the one this level knows about, and the outer ones are the callers'.
func (l *Lexer) Open() string { return l.openWord }

// ranOut records that the input ended inside something, and what.
//
// The first call wins. Once the input has ended, everything after it is a
// consequence rather than another thing left open.
func (l *Lexer) ranOut(word string) {
	if !l.incomplete {
		l.openWord = word
	}
	l.incomplete = true
}

func (l *Lexer) pos() Pos { return Pos{Offset: l.off, Line: l.line, Col: l.col} }

// shiftLines moves the line counter on without moving through any input.
//
// One caller: an alias body whose newlines the dialect counts as lines of the
// program. The text substituted for the alias word is not in this source at
// all, so nothing here can advance over it, and everything read afterwards
// still has to be numbered as though it had been.
func (l *Lexer) shiftLines(n int) { l.line += n }

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

// failUnmatched records input that ran out inside a quoted or substituted
// region, carrying everything a dialect might name: the opener, its closer,
// the text from the opener to the end of its line, and the line the input
// ran out on in both conventions.
func (l *Lexer) failUnmatched(open Pos, opener, closer, msg string) {
	if l.err != nil && !l.replacesUnmatched() {
		return
	}
	// From the start of the word rather than from the opener, which is what
	// the dialect that quotes this names: the construct is part of a word and
	// the word is what was being read. An assignment prefix is inside it —
	// `v=$(echo hi` is one word — and so is anything quoted, which is why
	// this comes from the scanner rather than from a search backwards
	// through the text (#1022).
	//
	// No comparison between the two positions: the word *contains* the
	// opener, so its start is never past it, and a guard saying so was a
	// mutant nothing could kill.
	from := open.Offset
	if l.wordStart.IsValid() {
		from = l.wordStart.Offset
	}
	near := l.src[from:]
	if i := strings.IndexByte(near, '\n'); i >= 0 {
		near = near[:i]
	}
	after := l.line
	if len(l.src) > 0 && l.src[len(l.src)-1] != '\n' {
		// The text stopped mid-line, so the end of it is the line after —
		// the same convention the parser's unterminated() uses.
		after++
	}
	l.err = &Error{
		Pos: open, Kind: ErrUnmatched, Msg: msg,
		Token: opener, Expected: closer, LastToken: near,
		EndLine: after, EofLine: l.line,
	}
}

// replacesUnmatched reports whether a construct noticing that the input ran
// out should take the complaint from one *inside* it that noticed first.
//
// Which construct is blamed when they nest is a dialect question, and it is
// answered by the order the reports arrive in rather than by anything having
// to look around: the scanners recurse, so the innermost one to run out
// returns first and the enclosing ones follow it outwards. Keeping the first
// report blames the innermost, and letting each replace the last blames the
// outermost.
//
// Measured 2026-09-07, `-n` over a script file, on constructs nested both
// ways round:
//
//	echo $( echo "hi          bash, dash, ksh93 blame the `"`
//	echo "${x:-"$( echo hi    the same three blame the `$(`
//	echo $(( 1 + `echo 2      and the backquote
//
// so those three name the innermost in every arrangement. zsh names the
// outermost in the same rows, which is why `echo "$( echo hi` is `unmatched
// "` there and `unexpected EOF while looking for matching )` in bash.
//
// Only an unmatched construct may be replaced. Anything else that has
// already failed is a different diagnosis and the first one stands, which is
// what keeps this from turning a real refusal into a report about a
// delimiter that was merely still open when it happened.
func (l *Lexer) replacesUnmatched() bool {
	if !l.dialect.UnmatchedBlamesTheOutermost {
		return false
	}
	var se *Error
	return errors.As(l.err, &se) && se.Kind == ErrUnmatched
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
		if len(l.pending) > 0 {
			// Input that ends without a newline still has a here-document
			// waiting, and what it never got is an *empty* body rather than
			// no body: `sh -c 'cat <<X'` runs cat with nothing on its input
			// in every shell. Reading them here is also what records the
			// remark about the delimiter that never arrived, which was
			// otherwise missing for exactly this shape.
			l.readHeredocs()
		}
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

	// `{name}` under the same adjacency rule is a descriptor the shell will
	// pick, with the variable receiving its number. The braces survive into
	// the token so the interpreter can tell the two apart.
	if tok, ok := l.tryFdVariable(); ok {
		return tok
	}

	// A pattern's operand owns a group that *starts* it — `[[ $k == (a|b) ]]`
	// — where the dialect reads bare groups. Only the first character needs
	// saying: mid-word the scanner already takes one, which is why
	// `[[ $k == a(b|c) ]]` worked while this did not (#826).
	//
	// Before the arithmetic command below, not after it: a nested group
	// starts `((`, and `[[ $k == ((a|b)|x) ]]` is a pattern rather than the
	// one place in the grammar where two parentheses are one token.
	if l.inPattern && l.peek() == '(' && l.opensPatternGroup() {
		return l.scanWord(start)
	}

	// A `(` where an argument may stand belongs to the word in the dialect
	// that reads glob qualifiers. It has to be seen before the operator
	// table, which would otherwise take it — and before the arithmetic
	// command below, because `echo ((1))` is one word there and not an
	// expression: measured, `no matches found: ((1))`.
	if l.dialect.GlobQualifiers && l.leadingParenBelongsToTheWord() && l.peek() == '(' &&
		l.opensPatternGroup() {
		return l.scanWord(start)
	}

	// `((` is an arithmetic command; `( (` is a subshell containing one. The
	// distinction is textual wherever a command may begin: `((echo nested))`
	// is an arithmetic error in bash, ksh and zsh even with no space, so no
	// knowledge of *which* command position this is is needed there.
	//
	// Inside `[[ ]]` no command may begin at all, so there is no arithmetic
	// command to be had and `((` is two grouping parentheses — unanimous in
	// bash 3.2 and 5.3, bash-as-sh, ksh93 and zsh, which all take
	// `[[ ((1 -eq 1)) ]]` and read it exactly as the spaced `[[ ( (1 -eq 1) ) ]]`.
	// The condition is the only context in the grammar that suspends the
	// rule, which is why the flag rather than a command-position test is
	// what asks (#859).
	// A `case` arm suspends it for the same reason a condition does: no
	// command may begin where a pattern belongs, so there is no arithmetic
	// command to be had and `((` is the arm's paren in front of a group.
	// Measured on zsh 5.9.2: `case x in ((a|b))`, `case x in ((a))` and
	// `case x in ((1))` are all accepted there and were all `parse error
	// near 'arithmetic command'` here (#1161).
	if l.dialect.ArithCommand && !l.inCondition && !l.inCaseArm &&
		l.peek() == '(' && l.peekAt(1) == '(' {
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

	// `<->` and its bounded spellings begin a *word* rather than a
	// redirection, and like `<(` they have to be seen before the operator
	// table takes the `<`.
	if l.startsNumericRange() {
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
//
// Adjacency is the grammar and not a convention: `cat < (echo hi)` is a
// syntax error in every shell in the panel, the three that have the
// construct included. Specified in docs/spec/grammar/substitutions.md.
func (l *Lexer) startsProcSubst() bool {
	if !l.dialect.ProcessSubstitution {
		return false
	}
	c := l.peek()
	return (c == '<' || c == '>') && l.peekAt(1) == '('
}

// startsNumericRange reports whether the cursor is on a numeric range
// pattern — `<->`, `<1-9>`, `<2->`, `<-9>` — in a dialect that has one.
func (l *Lexer) startsNumericRange() bool {
	_, ok := l.numericRangeAt(0)
	return ok
}

// numericRangeAt measures a numeric range starting n bytes ahead of the
// cursor, reporting how many bytes it spans.
//
// The shape is the entire disambiguation and it is exact: `<`, digits, `-`,
// digits, `>`, with either run of digits allowed to be empty. Anything else
// and the `<` is the redirection it is everywhere else — which is why this
// answers a width rather than a bool for the scanner, and a bool for the two
// places that only need to know a redirection is not starting here.
func (l *Lexer) numericRangeAt(n int) (int, bool) {
	if !l.dialect.NumericRangePattern || l.peekAt(n) != '<' {
		return 0, false
	}
	i := n + 1
	for isDigitByte(l.peekAt(i)) {
		i++
	}
	if l.peekAt(i) != '-' {
		return 0, false
	}
	i++
	for isDigitByte(l.peekAt(i)) {
		i++
	}
	if l.peekAt(i) != '>' {
		return 0, false
	}
	return i + 1 - n, true
}

// isDigitByte is the ASCII digit test the range shape is written in.
func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

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
	// And the `<` has to be one. Where the dialect has numeric ranges the
	// operator these digits would attach to may be a pattern instead, and
	// then the digits belong to the word in front of it: `echo 2<->` is one
	// word rather than a redirection of descriptor 2.
	if _, ok := l.numericRangeAt(n); ok {
		return Token{}, false
	}
	// And width, where the dialect reads only one digit as a number. The
	// digits are then an ordinary word and the operator a redirection with no
	// number of its own, which is what makes `exec 10>f` a command called
	// `10` in three of the five shells.
	if n > 1 && !l.dialect.MultiDigitFdNumber {
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

// tryFdVariable matches `{name}` followed with no gap by a redirection
// operator, in a dialect where the shell picks the descriptor and the name
// receives its number.
//
// The name must be one a variable could have; `{a,b}>f` has a comma and is a
// word, which is what keeps this clear of brace expansion. Where the dialect
// says so it may be a subscripted one — `{a[1]}>&-` names an element — and
// the comma rule survives that, because a subscript ends at its own `]` and
// what follows has to be the closing brace.
func (l *Lexer) tryFdVariable() (Token, bool) {
	if !l.dialect.FdVariableRedirections || l.peek() != '{' {
		return Token{}, false
	}
	n := 1
	for {
		c := l.peekAt(n)
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(n > 1 && c >= '0' && c <= '9') {
			n++
			continue
		}
		break
	}
	if n == 1 {
		return Token{}, false
	}
	if l.peekAt(n) == '[' {
		sub, ok := l.fdVariableSubscript(n)
		if !ok {
			return Token{}, false
		}
		n = sub
	}
	if l.peekAt(n) != '}' {
		return Token{}, false
	}
	// The same strict adjacency an IO number has: anything but a
	// redirection after the brace and this is an ordinary word.
	if c := l.peekAt(n + 1); c != '<' && c != '>' {
		return Token{}, false
	}
	// And the same exception: `{a}<->` is a word, not a descriptor the shell
	// would pick.
	if _, ok := l.numericRangeAt(n + 1); ok {
		return Token{}, false
	}
	start := l.pos()
	text := l.src[l.off : l.off+n+1]
	for range n + 1 {
		l.advance()
	}
	return Token{
		Kind:  TokIONumber,
		Pos:   start,
		End:   l.pos(),
		Text:  text,
		Spans: []Span{{Kind: Literal, Value: text, Quoting: Unquoted, Pos: start}},
	}, true
}

// fdVariableSubscript reads the `[...]` of `{a[1]}`, reporting how far the
// token now reaches and whether there was a subscript there at all.
//
// It is deliberately narrow about what may stand between the brackets. The
// whole token becomes one literal span, so nothing in it is ever expanded —
// and a subscript that says `$i` and means the two characters would be worse
// than one that is not a subscript at all. What is left is what needs no
// expansion: a name, a numeral, an expression built from them. `{a[$i]}` is
// therefore an ordinary word here, which is the one spelling bash reads and
// this does not; ksh93 takes the token and then refuses the `$` in the
// arithmetic, so there is no answer that is everyone's.
//
// A subscript is also never empty and never nested: `{a[]}` and `{a[b[1]]}`
// are words, so a bracket that opens one has to close it before the brace.
func (l *Lexer) fdVariableSubscript(open int) (int, bool) {
	if !l.dialect.FdVariableSubscript {
		return 0, false
	}
	n := open + 1
	for {
		c := l.peekAt(n)
		if c == ']' {
			break
		}
		if c == '_' || c == '+' || c == '-' || c == '*' || c == '/' || c == '%' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			n++
			continue
		}
		return 0, false
	}
	if n == open+1 {
		return 0, false
	}
	return n + 1, true
}

// operators, longest first. Longest match wins, so the order is the algorithm
// and not merely tidiness: `>>` must be found before `>`.
var operators = []Kind{
	TokAmpDGreatClobber, TokAmpDGreatBang, // 4 bytes
	TokDSemiAmp, TokTLess, TokAmpDGreat, TokDLessDash,
	TokDGreatClobber, TokDGreatBang, TokAmpGreatClobber, TokAmpGreatBang, // 3 bytes
	TokAndAnd, TokOrOr, TokDSemi, TokSemiAmp, TokDGreat, TokLessAmp, TokGreatAmp,
	TokLessGreat, TokClobber, TokClobberBang, TokDLess, TokAmpGreat,
	TokAmpBang, TokAmpPipe, TokPipeAmp, TokSemiPipe, // 2 bytes
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
	case TokSemiPipe:
		return l.dialect.CaseContinuePipe
	case TokTLess:
		return l.dialect.Herestring
	case TokAmpBang, TokAmpPipe:
		return l.dialect.BackgroundAndDisown
	case TokPipeAmp:
		// Either reading lexes the two bytes as one token; which construct
		// they are is the parser's. Where neither is set the operator table
		// falls back to `|` then `&`, which is what bash 3.2 and dash lex.
		return l.dialect.PipeBothStreams || l.dialect.CoprocPipeOperator
	case TokClobberBang, TokDGreatClobber, TokDGreatBang:
		return l.dialect.ClobberOverrideMarker
	case TokAmpGreatClobber, TokAmpGreatBang, TokAmpDGreatClobber, TokAmpDGreatBang:
		// The marker on the both-streams operators needs those operators
		// first: where `&>` is not read at all, `&>|` cannot be the marker on
		// it, and the fallback that matters is `&` then `>|` rather than a
		// four-byte operator nobody wrote.
		return l.dialect.ClobberOverrideMarker && l.dialect.AmpersandRedirect
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
	// A numeric range does too, which is what keeps `<->\" \"<->` one word:
	// the pattern a real prompt theme compares a terminal size against.
	if l.startsNumericRange() {
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
// leadingParenBelongsToTheWord reports whether a `(` at the *front* of a
// token is part of the word rather than an operator.
//
// Where an argument may stand it always is, which is what `inArgument` says.
// At the start of a `case` arm both readings are available at the same
// character — the arm carries an optional paren of its own, and a pattern
// may be a group — and what separates them is not the character but whether
// the arm is left a `)` to close it. Measured on zsh 5.9.2, 2026-09-07, each
// probe in a script file of its own under `env -i`:
//
//	case x in (a) …          the arm's paren, pattern `a`
//	case x in ((a|b)) …      the arm's paren, pattern `(a|b)`
//	case x in ((#i)a) …      the arm's paren, pattern `(#i)a`
//	case x in (a|b)) …       *no* arm paren, pattern `(a|b)`
//	case x in (a)b) …        *no* arm paren, pattern `(a)b`
//	case x in (#i)a) …       *no* arm paren, pattern `(#i)a`
//	case x in (a|b) ) …      *no* arm paren; a blank before the arm's `)`
//	case x in (a) b) …       refused: the arm's paren, then `b` is not `)`
//
// The `#` looked like the discriminator and is not — rows four, five and
// seven have no `#` in them and are the pattern's paren all the same, and
// the previous note here recorded `case x in (a)b)` as refused where that
// shell answers it. The `#` rows are subsumed rather than special: what
// decides them is the same thing that decides the rest.
//
// A group and a two-element pattern list match identically, so the two
// readings can only be told apart where the group carries text of its own —
// `(a)b)`, `(a|b)x)` — or where the arm reading runs out of parentheses.
// That is why the rule is about what follows the list and not about it.
//
// Read only where the *operator table* would otherwise take the paren. Once
// the token is known to be a word, `inArgument` alone decides how a leading
// group is scanned; see the note at that call.
func (l *Lexer) leadingParenBelongsToTheWord() bool {
	if l.inArgument {
		return true
	}
	if !l.inCaseArm || l.peek() != '(' {
		return false
	}
	// A glob flag is decided by the `#` alone, because the arm reading is
	// not available behind one at all: `#` where a word may begin opens a
	// *comment*, so `case x in (#i*)` read as the arm's paren swallows the
	// rest of the line. That shape is refused either way — this shell
	// answers `bad pattern: #i*` and reads the pattern — and the `#` is
	// what keeps the refusal pointing at the pattern rather than at the
	// arm's own paren.
	if l.peekAt(1) == '#' {
		return true
	}
	return l.caseArmParenOpensAGroup()
}

// caseArmParenOpensAGroup reports whether the `(` beginning a `case` arm is
// the *pattern's* rather than the arm's own, by reading the pattern list it
// would open and asking whether a `)` is left to close the arm.
//
// The lookahead is a throwaway lexer over the rest of the source, driven as
// an argument, rather than a hand-written scan for the matching `)`. That is
// the point of it: a group is taken whole — nesting, blanks and glob flags
// alike — by the scanner that already knows how, so there is no second
// implementation of "where does this group end" to drift from the first.
// Its diagnostics are discarded, and a probe that fails to read a list at
// all answers no, which leaves the arm's own paren as it was.
//
// It inherits that scanner's blind spot for a quoted `)` (#1241) rather than
// working around it, so the two answer the same shape the same way.
//
// Two of its guards are equivalent mutants, measured rather than assumed,
// and recorded so the next reader does not go looking for the row that would
// kill them. Both survive because the *caller* asks more than this does:
// nothing reaches here unless `opensPatternGroup` also says yes.
//
//	dropping the TokWord test      the leading `(` is folded into a word
//	                               exactly when a group opens there, and
//	                               where it is not — `()`, `((`) — the
//	                               caller has already declined
//	dropping the probe's Err test  the probe only fails where the real scan
//	                               fails on the same bytes, so the reading
//	                               it would have chosen never runs
func (l *Lexer) caseArmParenOpensAGroup() bool {
	probe := NewLexer(l.src[l.off:], l.dialect)
	// An argument is exactly the position a pattern list stands in once the
	// arm's paren is out of the way, which is also what the parser sets for
	// the patterns it reads after taking one.
	probe.inArgument = true
	for {
		if probe.Next().Kind != TokWord || probe.Err() != nil {
			return false
		}
		switch probe.Next().Kind {
		case TokRightParen:
			// The arm has its `)`, so the paren this started at was the
			// pattern's.
			return true
		case TokPipe:
			// So is a `|`, and it settles the question rather than leaving
			// it open. Were the leading `(` the arm's own, everything up to
			// the matching `)` would already be inside its pattern list and
			// the `)` would have closed the arm — so a `|` standing *after*
			// that `)` could only begin a body, and no body begins with one.
			// The group was one alternative of a longer list.
			//
			// Requiring another word after the `|` was wrong on five
			// measured rows, and found by mutation. `case a in (a|b)|)` is
			// the plain one: this dialect writes an alternative as nothing,
			// so there need not be a word there at all, and zsh 5.9.2
			// matches `a`. The other four are refusals whose *position*
			// moved — `(a|b)|c echo`, `(a|b)| echo`, `(a|b)|(c) echo` and
			// `(a|b)|c d)` are all refused there at the token after the
			// list, and falling back to the arm reading blamed the `|`.
			return true
		default:
			return false
		}
	}
}

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
	l.ranOut("pattern")
	l.fail(l.pos(), "unterminated pattern group")
	return l.src[start:l.off]
}

// scanArgumentGroup reads a `( … )` that stands for a whole word, stopping
// where an operator ends the word rather than at the closing parenthesis.
//
// It is scanPatternGroup with one clause added, and the clause is measured:
// `;`, `<`, `>` and `&` end the word where they appear, so `echo ( a <b )`
// leaves the `)` to the parser and is `parse error near `)”. A `|` does not,
// because a pattern group may hold an alternation — `echo ( a|b )` is one
// word, which the shell then reports as matching nothing.
//
// **It is the scanner for every group that starts a word**, which is three
// routes rather than a generalization of one. A *pattern* operand's leading
// group ends at the same four characters — measured on zsh 5.9.2, 2026-09-07,
// from a script file under `env -i`, each probe in a file of its own so the
// first refusal does not hide the rest:
//
//	[[ $k == (a<b) ]]    parse error near `<'
//	[[ $k == (a>b) ]]    parse error near `>'
//	[[ $k == (a;b) ]]    parse error near `;'
//	[[ $k == (a&b) ]]    parse error near `&'
//	[[ $k == (a|b) ]]    matches — the `|` is the group's
//
// A *regular expression's* operand is the other answer, and it never comes
// through here: scanWord takes a regex group whole at its own case above, and
// the four shells with `=~` agree it should — `[[ 'a<b' =~ (a<b) ]]` and
// `[[ 'a;b' =~ (a;b) ]]` both match in bash 5.3, bash 3.2, bash-as-`sh` and
// ksh93, where zsh refuses the `<` while parsing. So a regex operand owns its
// operators and a pattern operand does not, which is a difference between two
// constructs rather than the accident it looked like (#1175).
//
// Unterminated input is scanPatternGroup's business rather than an operator's,
// so it delegates the whole scan when nothing stops it.
func (l *Lexer) scanArgumentGroup() string {
	start := l.off
	depth := 0
	for !l.eof() {
		c := l.peek()
		// A numeric range's `<` is pattern text and not the operator that
		// ends the word, so it is taken whole before the four characters
		// below get to see it — the same precedence `next` and `endsWord`
		// already give it outside a group. Without this the group ended at
		// the `<` and left its `)` to the parser, which is why every
		// powerlevel10k config died on `(5.<1->*|<6->.*)` (#1217).
		//
		// `numericRangeAt` is the whole disambiguation and it is exact, so
		// the plain operator keeps every byte that is not a range: measured
		// on zsh 5.9.2, `(a<b)`, `(<a-b>)`, `(<1-2-3>)`, `(<-->)`, `(<>)`
		// and `(<1)` are all still parse errors there.
		//
		// Adding `&& depth > 0` here survives the suite, for the same
		// reason `depth >= 0` does below and recorded for the same reason:
		// the only pass with depth zero is the first one and its byte is
		// the `(`, which is not a `<`. It is an equivalent mutant rather
		// than a gap.
		if width, ok := l.numericRangeAt(0); ok {
			for range width {
				l.advance()
			}
			continue
		}
		// `depth > 0` cannot be false at one of those four bytes and is
		// kept for what it says rather than for what it decides: this is
		// entered on a `(`, so the only pass with depth zero is the first
		// one and its byte is that `(`. Mutating it to `depth >= 0` survives
		// the suite, and that is an equivalent mutant rather than a gap —
		// recorded here so the next reader does not go looking for the row
		// that would kill it.
		if depth > 0 && strings.IndexByte(";<>&", c) >= 0 {
			return l.src[start:l.off]
		}
		l.advance()
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
	l.off = start
	return l.scanPatternGroup()
}

// scanWord reads a word as a sequence of spans, one per run of uniform
// quoting. The spans are the point: a"b c"d is one word of three spans, and
// only the unquoted ones are subject to splitting and globbing later.
func (l *Lexer) scanWord(start Pos) Token {
	// Cleared on the way out rather than left behind: a failure raised after
	// the word is read belongs to no word, and a stale start would quote one
	// that had already finished.
	l.wordStart = start
	defer func() { l.wordStart = Pos{} }()

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
				l.ranOut("\\")
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

		case c == '<' && l.startsNumericRange():
			// Taken whole, because the `>` that ends it would otherwise end
			// the word: the range is literal pattern text and the matcher
			// reads it later.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			width, _ := l.numericRangeAt(0)
			for range width {
				lit.WriteByte(l.advance())
			}

		case c == '(' && l.opensPatternGroup():
			// A parenthesised group belongs to the word rather than ending
			// it. Mid-word everywhere, and at the *start* of one only where
			// the parser has said a word may begin with one: elsewhere a
			// leading `(` opens a subshell, or is the paren a `case` arm may
			// carry, and neither is a pattern.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			// **Position, not route.** A group that starts a word ends the
			// word at a shell operator whichever of the three ways it got
			// here, which is measured rather than assumed — see
			// scanArgumentGroup, where the pattern operand's four probes
			// are.
			//
			// An `l.inArgument` guard used to stand in this condition, so
			// only the argument route reached scanArgumentGroup and the
			// pattern route swallowed operators. Removing it survived the
			// whole suite, which is what #1175 was filed about; the shells
			// do distinguish the two, and the row that says so is the
			// second one in scanArgumentGroup's list. What #1161 measured
			// still holds — a `case` arm cannot tell the two scanners
			// apart, `case x in (#i;a)b)` and `case x in (#i<a)b)` being
			// refused under both readings — so that route is unaffected
			// either way and is no reason to keep a guard the pattern route
			// answers wrong.
			if lit.Len() == 0 {
				lit.WriteString(l.scanArgumentGroup())
				continue
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

		case c == '$' && l.peekAt(1) == '"' && l.dialect.DollarDoubleQuote:
			// `$"..."` marks the string for locale translation. With no
			// message catalog every shell that has the form reads it as a
			// plain double-quoted string — same escapes, same expansions — so
			// the `$` contributes nothing and the spans are exactly what a
			// bare `"` produces. The printer therefore writes them back as
			// plain quotes: the two spellings parse to identical trees, and
			// recording the `$` would be keeping a byte the tree has no
			// question for. Where the flag is off, the `$` falls through to
			// the literal path, which is what dash and zsh do with it.
			flush()
			l.advance() // $
			spans = append(spans, l.scanDouble()...)

		case c == '$' && l.peekAt(1) == '(' && l.peekAt(2) == '(':
			// `$((` is arithmetic. A command substitution whose first
			// construct is a subshell has to be written `$( (`, which is the
			// only disambiguation available and is decided here: by the time
			// the parser sees tokens the choice has been made.
			flush()
			spans = append(spans, l.scanParens(ArithSubst, Unquoted))

		case c == '$' && l.peekAt(1) == '[' && l.dialect.DollarBracketArith:
			// The older spelling of the case above. Where the flag is off
			// this falls through to the literal path, which leaves a `$` and
			// a bracket expression — what ksh93 and dash do with it.
			flush()
			spans = append(spans, l.scanBracket(Unquoted))

		case c == '$' && l.peekAt(1) == '(':
			flush()
			spans = append(spans, l.scanParens(CommandSubst, Unquoted))

		case l.startsProcSubst():
			// Unquoted only, and that is not an omission: `"<(echo hi)"` is
			// its own ten characters of text in every shell in the panel,
			// dash included, because what it produces is a *path* and a
			// quoted path is still a path — there would be nothing for the
			// quoting to change. Measured, and pinned by the corpus case
			// procsub/quoted-is-not-a-substitution; see
			// docs/spec/grammar/substitutions.md.
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
			l.ranOut("'")
			if l.dialect.CloseQuotesAtEOF {
				// The end of input is as good as the closing mark here;
				// what was read is the string. ranOut still marks the
				// input incomplete, so a prompt continues the line.
				return Span{Kind: Literal, Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
			}
			l.failUnmatched(open, "'", "'", "unterminated single quote")
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
		case c == '$' && l.peekAt(1) == '[' && l.dialect.DollarBracketArith:
			flush()
			out = append(out, l.scanBracket(DoubleQuoted))
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
	return l.scanDoubleBody(open, true)
}

// scanDoubleBody reads double-quoted content from the cursor.
//
// `closing` says a `"` ends it, which is the ordinary run scanDouble opens.
// Without one the content runs to the end of the input and a `"` in it is an
// ordinary character — which is what the operand of a `${ }` written inside
// double quotes is. The quote that put it in this context is outside the text,
// so there is none to find and running out is not a failure.
func (l *Lexer) scanDoubleBody(open Pos, closing bool) []Span {
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
			if !closing {
				flush()
				return out
			}
			l.ranOut("\"")
			if !l.dialect.CloseQuotesAtEOF {
				l.failUnmatched(open, "\"", "\"", "unterminated double quote")
			}
			flush()
			return out
		}
		c := l.peek()
		switch {
		// A `"` written inside an operand opens a run of its own rather than
		// standing for a character, which is the half of this that is *not*
		// like a single quote: `"${u:-"a b"}"` is `a b` in every shell in the
		// panel, quotes removed, where `"${u:-'a b'}"` keeps them. So the two
		// quote characters part company here and each keeps its own rule.
		case c == '"' && !closing:
			flush()
			out = append(out, l.scanDouble()...)
			litPos = l.pos()
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
		case c == '$' && l.peekAt(1) == '[' && l.dialect.DollarBracketArith:
			flush()
			out = append(out, l.scanBracket(DoubleQuoted))
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
// The escape sequences inside are *not* resolved here, and that is a division
// of labor rather than a gap: the table is written down — see the `$'...'`
// section of docs/spec/grammar/tokenization.md, which measures it escape by
// escape — and interp's expandDollarSingle applies it. What this stage owes
// the later ones is the span's *quoting*, since a decoded tab must not be
// split on, and the raw text, so that decoding it later loses nothing.
func (l *Lexer) scanDollarSingle() (Span, bool) {
	open := l.pos()
	l.advance() // $
	l.advance() // '
	var b strings.Builder
	for {
		if l.eof() {
			l.ranOut("$'")
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
	if kind == CommandSubst {
		// Where the contents end is a question about the grammar, not about
		// how many parentheses have been seen: a `case` arm's `)` closes
		// nothing, so counting stops early and takes half an arm with it.
		//
		//	x=$(case a in a) echo yes;; esac)
		//
		// Every shell in the panel runs that. Counting made it a syntax
		// error here — and worse, made it *parse into the wrong tree* where
		// the leftovers happened to be a command, which is how two scripts
		// on this machine parsed and could not be printed back.
		//
		// The contents of `$( )` are the contents of a subshell, so the
		// answer is the one the parser already knows: read a list, and stop
		// where it stops.
		if end, remarks, ok := l.parseToClose(start); ok {
			// What that read had to say comes back with it. A parse inside a
			// parse otherwise says nothing — the reason takeRemarks exists
			// below — and this is the *other* route into a substitution's
			// contents, reached when the grammar does find the `)`.
			//
			// Which is what a nested substitution looks like: the inner one
			// swallows the here-document and leaves a `)` for the outer one,
			// so the outer read succeeds where the un-nested shape's fails,
			// and the remark the innermost lexer raised was dropped with the
			// parser that noticed it (#1024). Only the dialects that read a
			// body as ending at the closing parenthesis get here at all: in
			// the others the inner construct is refused, this read fails
			// with it, and the refusal is the whole answer.
			l.remarks = append(l.remarks, remarks...)
			for l.off < end {
				l.advance()
			}
			value := l.src[start:l.off]
			l.advance() // the )
			return Span{Kind: kind, Value: value, Quoting: q, Pos: open}
		}
		// Not something the parser could read — half a line at a prompt,
		// most often. Counting is the older answer and is kept for it: it
		// gets the common shapes right and reports the rest as unterminated,
		// which is what an unfinished substitution is.
	}
	for depth > 0 {
		if l.eof() {
			l.ranOut(openingOf(kind))
			l.failedToClose(open, kind)
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

	// Where the loop stopped, kept before anything below moves the cursor.
	//
	// Only the refusal moves it, and a refused span is nobody's to read
	// today — a mutant taking the span from the moved cursor is
	// indistinguishable end to end. The span is still cut here, because the
	// alternative is a Value that silently becomes the rest of the file for
	// the first caller that reads a tree after a parse failure.
	stop := l.off

	if holdsCommands(kind) {
		// Counting found the end, which for a command substitution means the
		// read above could not — and the commonest reason is the shape #785
		// is about: a here-document
		// whose delimiter is only there because the `)` follows it, so the
		// body ran past the substitution and took the closing parenthesis
		// with it. The document *did* end at end of input, and the one shell
		// that says so says it here.
		//
		// Read from the substitution's own text, closing parenthesis
		// included, because that is what the here-document's input is: the
		// delimiter line is `EOF)` and matches nothing, and the last line of
		// the substitution is the last line the body could have. The trimmed
		// text would say the opposite — `EOF` alone is the delimiter, so
		// there would be nothing to remark on, which is also why the
		// substitution itself runs and yields `a`.
		remarks, bodyRanOut := l.takeRemarks(l.src[start:l.off], open.Line)
		// And whether that read is an answer at all is a dialect question,
		// which nothing here used to ask. Counting the parentheses finds the
		// `)` whatever shell this is, so all four dialects accepted a program
		// dash and zsh refuse.
		//
		// Reading the body from between the parentheses is one answer to
		// where it ends and reading it from the whole input is the other.
		// Under the second, the `)` counting just found is a line of the
		// body, nothing ever closed the construct, and the right complaint
		// is the one an unterminated `(` already gets — which is exactly
		// what dash and zsh say here, word for word.
		//
		// `depth == 0` because the loop above may have run out of input
		// instead of finding the `)`, which is already reported and is not
		// this. Nothing can observe the difference — ranOut keeps the first
		// call and the fail helpers keep the first error, so the second pass
		// would change nothing — and a mutant without it is byte-identical
		// across every unterminated shape in all four dialects. It stays
		// because "already reported" is the reason, not the idempotence.
		//
		// The remark is kept either way, and deliberately. It is what says a
		// body reached the end of this text, so the nesting depends on it:
		// `$(echo $(cat <<E` … `E))` is refused only because the inner
		// construct's remark reaches the outer one's read. It cannot be seen
		// where it is refused — a dialect that reads a body from the whole
		// input has no wording for a here-document at end of file, so there
		// is nothing to print beside the complaint — and suppressing it here
		// silently made that nested shape parse again.
		l.remarks = append(l.remarks, remarks...)
		if bodyRanOut && depth == 0 && !l.dialect.HeredocEndsAtClosingParen {
			// The body took the `)` and everything after it, so that is
			// where the cursor belongs: the input ran out inside this
			// construct and there is nothing left for anyone to read.
			//
			// Not bookkeeping. The line a refusal is located on is the line
			// the input ended on, and both shells that refuse this name it —
			// leaving the cursor at the `)` blamed the delimiter's line and
			// every one of the six corpus rows said so.
			for !l.eof() {
				l.advance()
			}
			l.ranOut(openingOf(kind))
			l.failedToClose(open, kind)
		}
	}
	// Trim the closing delimiters the loop consumed.
	end := stop
	for n := 1; n <= closers(kind) && end > start && l.src[end-1] == ')'; n++ {
		end--
	}
	return Span{Kind: kind, Value: l.src[start:end], Quoting: q, Pos: open}
}

// failedToClose records a parenthesised construct the input ran out inside.
//
// The two kinds part here, and on the line holdsCommands already draws.
// Everything that holds a *program* goes through failUnmatched, which carries
// what a dialect words from: the opener as written, the closer that never
// came, the text from the opener, and the line the input ran out on in both
// conventions. Before this, only `$(` did — the two process-substitution
// spellings took a plain formatted error instead, so all four dialects said
// the lexer's own sentence, none of them said what its shell says, and
// `ParseFailureLine` answered 0 because there was no *syntax.Error to read a
// line from (#1023).
//
// The one that holds an *expression* goes through it too, since #1086, and
// its line is why that took a second measurement rather than following on
// from the first. A dialect that reports an unmatched `$(` at the line after
// the input's last reports `$((` at the **opener's**: `echo $((1+2` with a
// trailing newline answers `line 1` where `v=$(echo hi` answers `line 2`.
//
// What settles it is that the line convention is already keyed on the opener
// rather than on the kind. ParseFailureLine reads CmdSubstUnmatchedAtEnd for
// `$(`, `<(` and `>(` and UnmatchedReportedAtOpener for everything else, and
// `$((` wants the second — which is the same answer a quote gets, in the one
// dialect that parts them, and is what that dialect prints. So the routing
// needed no new switch at all, and adding `$((` to the first set is the
// mistake that would have moved the line: measured per dialect afterwards,
// all four agree with the panel unchanged.
func (l *Lexer) failedToClose(open Pos, kind SpanKind) {
	l.failUnmatched(open, openingOf(kind), closingOf(kind), "unterminated "+kind.String())
}

// scanBracket reads `$[ … ]`, the older spelling of `$(( … ))`.
//
// It produces an ArithSubst span, because that is what it is: everything
// downstream — the expression parser, evaluation, every diagnostic — is the
// same, and only the delimiters differ. Span.Bracketed carries the spelling
// for the printer.
//
// The closing `]` is found the way scanParens finds its `)`: by tracking
// quoting and nesting rather than by taking the first one. Nesting is not
// theoretical here — a subscript is arithmetic too, and `$[a[1]+1]` answers
// in both shells that have the construct, so the inner `]` has to be counted
// past.
func (l *Lexer) scanBracket(q Quoting) Span {
	open := l.pos()
	l.advance() // $
	l.advance() // [
	depth := 1
	start := l.off

	for depth > 0 {
		if l.eof() {
			l.ranOut("$[")
			// The older spelling names its own delimiters rather than
			// borrowing ArithSubst's: the dialect that echoes the closer
			// back says `]` here and `)` for `$((`, measured, and the kind
			// is the same for both spellings because everything downstream
			// of the delimiters is.
			l.failUnmatched(open, "$[", "]", "unterminated "+ArithSubst.String())
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
		case '[':
			depth++
			l.advance()
		case ']':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}

	end := l.off
	if end > start && l.src[end-1] == ']' {
		end--
	}
	return Span{Kind: ArithSubst, Value: l.src[start:end], Quoting: q, Pos: open, Bracketed: true}
}

// parseToClose reads the contents of a command substitution and returns the
// offset of the `)` that ends them.
//
// A parser of its own over the rest of the input, which is what makes this
// answer the grammar's question rather than a counting one. It reports
// failure rather than a guess: an unfinished substitution has no closing
// parenthesis to find, and the caller has an older answer for that.
// takeRemarks reads text as a program of its own and keeps what it had to say
// about input it accepted anyway, with the positions moved into this source.
//
// A parse inside a parse otherwise says nothing: a *failure* there is reported
// as this parse failing, and a remark — the one thing a parser accepts and
// remarks on — was dropped with the parser that noticed it. `v=$(cat <<EOF` …
// `EOF)` runs, `v` is `a`, and the warning about the document ending at end of
// file went nowhere (#785).
//
// from is the line text begins on, so a remark names a line of the program
// rather than of the substitution. The offsets are relative to text and are
// left that way: nothing reads a remark's offset, and moving it would claim a
// correspondence this text does not have — it is a slice of the source here
// and is not, for the routes that hand this package a fragment.
//
// Only remarks are taken. Whatever else the read found — an error, a tree — is
// the caller's own business and it has already decided what to do about it.
//
// The remarks are returned rather than kept, because whether they are the
// caller's to keep is now a question: a here-document that ran to the end of
// this text is a body that would have run straight past the parentheses had
// it been read from the whole input, and in a dialect that reads it that way
// the construct is refused instead of remarked on. See
// Dialect.HeredocEndsAtClosingParen. The second return says which of those it
// was.
func (l *Lexer) takeRemarks(text string, from int) ([]Remark, bool) {
	sub := NewParserAt(text, l.dialect, from)
	sub.parseList()
	for _, r := range sub.lex.remarks {
		if r.Kind == RemarkHeredocAtEOF {
			return sub.lex.remarks, true
		}
	}
	return sub.lex.remarks, false
}

// It also returns what the read had to say about input it accepted, for the
// same reason takeRemarks does: a remark is the one thing a parser produces
// that its caller cannot recover from the tree or the error.
//
// The sub-parse is told which line it starts on, so those remarks name a line
// of the program rather than of the substitution. `$(` holds no newline, so
// the lexer's current line is the opener's, and the offsets the caller uses
// are untouched by it.
func (l *Lexer) parseToClose(from int) (int, []Remark, bool) {
	sub := NewParserAt(l.src[from:], l.dialect, l.line)
	sub.parseList()
	if sub.err != nil || !sub.at(TokRightParen) {
		return 0, nil, false
	}
	return from + sub.tok.Pos.Offset, sub.lex.remarks, true
}

// procSubstKind says which end of the pipe the word will name.
func procSubstKind(c byte) SpanKind {
	if c == '>' {
		return ProcSubstOut
	}
	return ProcSubstIn
}

// holdsCommands reports whether what is between the parentheses is a program
// rather than an expression.
//
// It decides which spans are read again for what that read has to say about
// them, and the line it draws is not decoration: `$(( a << b ))` is a left
// shift, and reading it as a program makes `<<` a here-document whose
// delimiter `b` never arrives — a warning about a script that has none. The
// two process-substitution kinds are on this side of it, which is measured
// rather than assumed: bash remarks on `<(cat <<EOF` … `EOF)` exactly as it
// does on `$(cat <<EOF` … `EOF)`.
func holdsCommands(k SpanKind) bool {
	return k == CommandSubst || k == ProcSubstIn || k == ProcSubstOut
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
			l.ranOut("${")
			if q == DoubleQuoted {
				// The `${` began inside a double quote, and three of the
				// panel blame the quote for the whole thing — the fourth
				// blames the quote character itself, which its wording of
				// this same failure carries.
				l.failUnmatched(open, "\"", "\"", "unterminated parameter expansion")
			} else {
				l.failUnmatched(open, "${", "}", "unterminated parameter expansion")
			}
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
			// A substitution written in the body is stepped over whole, so
			// a `}` inside it does not close this expansion and a `)` or a
			// backquote it never closes is blamed on the substitution
			// rather than on the brace. `${x:-$(echo hi` is `unexpected EOF
			// while looking for matching )` in bash, dash and ksh93, and the
			// backquoted spelling the same — both were `${` here, which is
			// the same cause as the nested case #1151 is about, one scanner
			// over.
			//
			// This is the *body* of the expansion holding a program, which
			// is a different question from what skipSubstitution's own note
			// declines to answer: that one is about a `${ }` written inside
			// a quote being skipped as text, where the panel disagrees over
			// what quoting means. What a `$( )` contains is not in dispute.
			if l.skipSubstitution() {
				continue
			}
			l.advance()
		}
	}
	end := l.off
	if end > start && l.src[end-1] == '}' {
		end--
	}
	if l.dialect.CurrentShellSubstitution && start < len(l.src) && isBraceCommandStart(l.src[start]) {
		// `${ cmd;}` is a command and `${x}` is a parameter, and the space
		// is the whole of the difference — which is why it is decided here,
		// on the character after the brace, rather than by trying to read
		// what follows as a name and failing.
		//
		// The body is taken exactly as the parameter form takes it: a `}`
		// inside quotes does not close either, and both nest.
		return Span{Kind: CommandSubst, CurrentShell: true, Value: l.src[start:end], Quoting: q, Pos: open}
	}
	return Span{Kind: ParamExp, Value: l.src[start:end], Quoting: q, Pos: open}
}

// isBraceCommandStart reports whether what follows `${` makes it a command
// rather than a parameter. Measured: a space, a tab and a newline all do, and
// nothing else can — a parameter name may not begin with any of them.
func isBraceCommandStart(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n'
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
			l.ranOut("`")
			if !l.dialect.CloseQuotesAtEOF {
				l.failUnmatched(open, "`", "`", "unterminated backquote substitution")
			}
			return Span{Kind: CommandSubst, Backquoted: true, Value: unescapeBackquoted(l.src[start:l.off]), Quoting: q, Pos: open}
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
			return Span{Kind: CommandSubst, Backquoted: true, Value: unescapeBackquoted(l.src[start:end]), Quoting: q, Pos: open}
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
//
// A double-quoted run is not a run of text. A substitution written inside one
// holds a *program*, and a program brings its own quoting: the single quotes
// in `"$(grep '"')"` quote that `"`, so it is not the one that ends the run.
// Skipping the run character by character reads it as the closing quote,
// leaves the cursor on the second `'`, and takes the rest of the file as a
// single-quoted string — which is why `${x:-"$(grep '"')"}` was refused with
// the brace never found, in all four dialects at once (#1140). So the skip
// steps over a nested substitution whole, exactly as scanDouble does when it
// is building spans rather than skipping them.
//
// Single quotes need none of this: nothing inside them is special, which is
// the whole of their specification, and `escapes` is the flag that already
// separates the two.
func (l *Lexer) skipQuoted(quote byte, escapes bool) {
	open := l.pos()
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
		if escapes && l.skipSubstitution() {
			continue
		}
		l.advance()
	}
	// The input ran out inside this quote, and saying so is the whole of
	// what the skippers were missing: a delimiter scan used to return
	// quietly here, so the construct *enclosing* it reached end of input and
	// named its own opener — which meant the outermost was blamed in every
	// dialect, including the three that name the innermost (#1151).
	//
	// ranOut is deliberately not called here, and the two questions part on
	// exactly this point. What a *diagnostic* blames is the innermost
	// construct in three of the four dialects; what a **prompt** is still
	// waiting on is the outermost, which is what Lexer.openWord answers and
	// what Parser.Open reports — `echo $( echo 'x` is a substitution that is
	// still open, and the quote inside it is not the thing a continuation
	// prompt is for. The enclosing scanner records it on its way past.
	l.failUnmatched(open, string(quote), string(quote), unterminatedQuoteMsg(quote))
}

// unterminatedQuoteMsg is the substrate's own sentence for a quote the input
// ran out inside, matching what scanWord says for the same failure so the two
// routes into it cannot drift apart.
func unterminatedQuoteMsg(quote byte) string {
	if quote == '\'' {
		return "unterminated single quote"
	}
	return "unterminated double quote"
}

// skipSubstitution steps over a substitution beginning at the cursor and
// reports whether one did. It is the skipping counterpart of the spans
// scanWord builds, and it exists for the scanners that only need to find a
// delimiter: `$( )`, `$(( ))` and the backquoted form all hold input whose
// quoting is its own.
//
// `${ }` is deliberately not one of them. Its body is a word rather than a
// program, and what quoting means inside it is exactly where the panel stops
// agreeing — `${x:-"${y:-'"'}"}` is accepted by bash alone and refused by the
// other five — so it is a dialect question rather than a delimiter one, and
// nothing here should answer it.
func (l *Lexer) skipSubstitution() bool {
	if l.peek() == '`' {
		l.skipBackticks()
		return true
	}
	if l.peek() != '$' || l.peekAt(1) != '(' {
		return false
	}
	open := l.pos()
	l.advance() // $
	l.advance() // (
	// The arithmetic spelling needs no case of its own. Its second `(` is
	// the next character skipToDepth reads, and counting it there is what
	// makes both `)` at the other end belong to the construct — so one loop
	// serves `$( )` and `$(( ))` alike.
	if !l.skipToDepth(1) {
		// It ran out rather than finding the `)`, and it is the construct
		// three of the four dialects blame — `echo "${x:-"$( echo hi` is
		// `unexpected EOF while looking for matching )` in bash, exactly as
		// the un-nested shape is. Before this the skip returned quietly and
		// the `${` around it was blamed instead (#1151).
		//
		// Named as the plain substitution and not the arithmetic one: this
		// skip does not know which it stepped over, and the two spellings
		// are told apart by their closers, which it never reached. The
		// scanners that *do* know still report their own.
		l.failUnmatched(open, "$(", ")", "unterminated "+CommandSubst.String())
	}
	return true
}

// skipToDepth consumes input until the given number of parentheses have been
// closed, tracking quoting and further nesting on the way. It is the scanning
// rule scanParens uses to find its own `)`, factored out for the skippers,
// and it stops at end of input rather than complaining: whoever called it is
// inside a construct of its own and already has the unterminated one to
// report.
//
// It does not call skipSubstitution on the way, and that is not an omission:
// a nested `$( )` reached from here is already counted correctly — the `$` is
// an ordinary byte and the parentheses balance — and `$(( ))` for the same
// reason, its two opening parens counted the same as its two closing ones.
// Recurring was tried and every mutant of it read the same, in all four
// dialects and across the corpus, because it can only arrive at the state
// counting arrives at.
func (l *Lexer) skipToDepth(depth int) bool {
	for depth > 0 && !l.eof() {
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
	return depth == 0
}

func (l *Lexer) skipBackticks() {
	open := l.pos()
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
	// Ran out rather than finding the closing mark. The dialect that ends an
	// unterminated quote at end of input and runs is asked here for the same
	// reason scanBackticks asks it: there is nothing unfinished there.
	if !l.dialect.CloseQuotesAtEOF {
		l.failUnmatched(open, "`", "`", "unterminated backquote substitution")
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
			l.ranOut("$((")
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

// peekIsRightParen reports whether the next thing in the input is a `)`,
// blanks aside. Used where the `(` is already the current token, which is
// what tells an anonymous function's empty parameter list from a subshell.
func (l *Lexer) peekIsRightParen() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return i < len(l.src) && l.src[i] == ')'
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
	// Where the last line of the body began, which is where the input ran
	// out as far as the one shell that remarks on this is concerned. Not
	// l.pos() at the end: a body whose last line ends in a newline leaves
	// the lexer at the start of the line *after* it, and the warning names
	// the last line that had something on it. A body with no lines at all
	// names the here-document's own line, which is measured.
	lastLine := r.OpPos
	var body strings.Builder
	for {
		if l.eof() {
			// Reaching the end without the delimiter is unfinished input and
			// not a syntax error: every shell in the panel takes the body as
			// everything to the end and runs the command, one of them with a
			// warning and three in silence.
			//
			// Marked incomplete all the same, because *where* the input ended
			// is a different question at a prompt: a here-document still open
			// when the line ends should ask for another line rather than run
			// with what it has. The parser reports both, and each front end
			// reads the one it needs.
			l.ranOut("<<")
			// Said out loud by one shell and passed over by three, so it is
			// recorded here and worded — or not — by the front end.
			l.remarks = append(l.remarks, Remark{
				Kind:  RemarkHeredocAtEOF,
				Pos:   lastLine,
				At:    r.OpPos,
				Token: delim,
			})
			// The body ran to the end of the input, so the last line there
			// was is the last line this command occupied.
			l.markHeredocEnd(lastLine)
			break
		}
		linePos := l.pos()
		line, done := l.heredocLine(strip)
		if done == delim {
			// The delimiter's own line is the command's last, and it is not
			// part of the body.
			l.markHeredocEnd(linePos)
			break
		}
		lastLine = linePos
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

// markHeredocEnd records how far a here-document reached, keeping the furthest
// of several on one line: `cat <<A <<B` is closed by B's delimiter and not by
// A's, and they are read in the order their operators appeared.
func (l *Lexer) markHeredocEnd(at Pos) {
	if at.Offset > l.heredocEnd.Offset {
		l.heredocEnd = at
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
	if l.dialect.BareSubscript && l.peek() == '#' && isBareLengthTarget(l.peekAt(1)) {
		// `$#name` is that parameter's length, so the `#` is an operator here
		// and the parameter is what follows it. Without the flag — and with
		// it, where nothing a length can be taken of follows — the `#` is
		// itself the parameter, and the rest of the word is literal text.
		l.advance()
	}
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
	l.bareSubscript(l.src[begin:l.off], q)
	return Span{Kind: ParamExp, Value: l.src[begin:l.off], Quoting: q, Pos: open}
}

// isBareLengthTarget reports whether c may follow the `#` of `$#name`.
//
// A name, a digit, `@` or `*`. Measured on zsh 5.9.2: `$#a` is an array's
// count, `$#0` and `$#1` are the lengths of those parameters, and `$#@` and
// `$#*` are the number of positional parameters — while `$##` prints the
// count and then a `#`, and `$#!` the count and then a `!`, so the two
// specials that would make the inner text ambiguous are also the two the
// shell itself leaves out.
func isBareLengthTarget(c byte) bool {
	return c == '@' || c == '*' || (c >= '0' && c <= '9') || isNameStart(c)
}

// takesBareSubscript reports whether the parameter a bare `$` just named may
// carry a `[ … ]` after it.
//
// name is the inner text scanned so far, which is the parameter with the `#`
// of a length still on the front of it: `$#a[2]` is the length of the second
// element, so the subscript belongs to `a` and is read here.
//
// A name, `@` or `*`. The positional digits are excluded because the shell
// excludes them — `set -- abcd; echo $1[2]` prints `abcd[2]` — and the other
// specials because the span could not record the result: `#` and `!` already
// mean an operator at the front of a `${ … }`, and the rest are single-valued
// parameters no script subscripts.
func takesBareSubscript(name string) bool {
	name = strings.TrimPrefix(name, "#")
	if name == "@" || name == "*" {
		return true
	}
	if name == "" || !isNameStart(name[0]) {
		return false
	}
	return true
}

// bareSubscript reads the `[ … ]` a bare parameter carries where the dialect
// has them, leaving the cursor after the closing bracket.
//
// The brackets are balanced, because a subscript may hold another — `$a[$b[1]]`
// — and the scan gives up where the word would end: an unquoted `[` that never
// closes before the word does is not a subscript at all, and its characters
// stay literal. That is one step short of the shell, which commits to the
// subscript and reports an invalid one at run time; the shape still fails
// either way, and the difference is the wording of a diagnostic for input
// nobody writes.
//
// Where the word ends is the *quoting's* question and not a fixed set of
// characters, which is measured on both sides: inside double quotes a blank
// and a **newline** are ordinary text and `"$a[1\n]"` is the first element,
// while unquoted either one ends the word and the brackets are literal. A
// scan that stopped at every newline would have been wrong for the quoted
// half and was — nothing caught it until the line was mutated away and the
// shell was asked.
func (l *Lexer) bareSubscript(name string, q Quoting) {
	if !l.dialect.BareSubscript || l.peek() != '[' || !takesBareSubscript(name) {
		return
	}
	depth := 0
	// A subscript may open with a flag group of its own, and the `(` that
	// opens one would otherwise end the word: `$a[(r)b]` was a syntax error
	// naming the parenthesis, which is worse than a wrong answer because it
	// takes the whole file with it. The group is stepped over as a unit, so
	// only a group the grammar can actually read is protected — anything
	// else still ends the word exactly where it did.
	past := -1
	if l.dialect.ArraySubscriptFlags {
		if _, rest, isGroup := scanSubscriptFlags(l.src[l.off+1:]); isGroup {
			past = len(l.src) - len(rest)
		}
	}
	for i := l.off; i < len(l.src); i++ {
		if i > l.off && i < past {
			continue
		}
		switch c := l.src[i]; {
		case c == '[':
			depth++
		case c == ']':
			if depth--; depth == 0 {
				for l.off <= i {
					l.advance()
				}
				return
			}
		case q == DoubleQuoted && c == '"':
			return
		case q != DoubleQuoted && l.isWordEnd(c):
			return
		}
	}
}

// openingOf is how a span's kind is written, for saying what is unfinished.
//
// A kind describes itself in words for a diagnostic — "command substitution"
// — and a caller drawing what is still open wants the characters that opened
// it instead.
//
// Only the kinds that reach here have a case. A parameter expansion runs out
// in a scanner of its own and names itself there, and a branch for it here
// was dead: a mutation of it changed nothing, which is how it was found.
func openingOf(kind SpanKind) string {
	switch kind {
	case ArithSubst:
		return "$(("
	case CommandSubst:
		return "$("
	case ProcSubstIn:
		return "<("
	case ProcSubstOut:
		return ">("
	}
	return kind.String()
}

// closingOf is the delimiter that never came, for the dialect whose sentence
// names it rather than the opener.
//
// One parenthesis for every kind that reaches here, the arithmetic one
// included, and that is measured rather than a simplification: the dialect
// that words this as "looking for matching `)'" says `)` for `$((1+2` and not
// `))`, so echoing the two characters the construct was opened with would be
// wrong. The bracketed spelling is the exception and does not come through
// here — scanBracket names its own `]`, which is what that dialect prints
// for it.
func closingOf(kind SpanKind) string {
	switch kind {
	case ArithSubst, CommandSubst, ProcSubstIn, ProcSubstOut:
		return ")"
	}
	return kind.String()
}
