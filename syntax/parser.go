// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"strings"
)

// Parser builds a syntax tree from tokens.
//
// It is written from docs/spec/grammar/commands.md and carries the lexer's two
// requirements, for the same reason: a shell re-parses the current line on
// every keypress, so it never panics, and unfinished input is distinguishable
// from wrong input because `if x; then` mid-typing is normal.
type Parser struct {
	lex     *Lexer
	dialect Dialect

	tok Token

	// Aliases answers what a word stands for, and is the caller's table
	// rather than one this package keeps — see alias.go. Nil expands
	// nothing.
	Aliases Aliases

	// GlobalAliases answers for the kind expanded wherever a word stands
	// rather than only where a command word does, and SuffixAliases for the
	// kind keyed on a command word's extension. Both are the caller's
	// tables, both are nil in a dialect without the kind, and one dialect in
	// the panel has either — see interp.Semantics.GlobalAliases.
	GlobalAliases Aliases
	SuffixAliases Aliases

	// pending are tokens an alias expansion put in front of the lexer, and
	// aliasNextWord says the last expansion ended in a space, so the word
	// after it is eligible in turn.
	pending       []Token
	aliasNextWord bool
	// aliasSpliced counts the tokens of the current expansion still in hand,
	// so the trailing-space rule can tell a word that *came from* the value
	// from the word that follows it.
	aliasSpliced int
	// aliasLineShift totals the lines every expansion so far has added to
	// the input, where the dialect counts an alias body's newlines. Reported
	// by LineShift, for a caller that parses a program in pieces and has to
	// carry the numbering from one piece to the next.
	aliasLineShift int
	// aliasDone are the names already expanded in the command being read. It
	// is a field rather than a local because the command word is not always
	// reached from one place: assignment prefixes stand in front of it, so
	// the word after them is expanded in parseSimple while the first word was
	// expanded in parseCommand, and the two have to share a set or
	// `alias al='y=1 al'` expands forever. Measured: that alias is
	// `al: not found` in ksh93 and zsh alike, so the inner name is spent.
	aliasDone map[string]bool
	// globalDone is the same set for one word's *global* expansion, which is
	// per word rather than per command. Kept on the parser and cleared
	// rather than allocated each time, because it is reached from next.
	globalDone map[string]bool
	// aliasPrimed records that the first token has been offered to the
	// global-alias hook. See primeAliases.
	aliasPrimed bool

	err        error
	incomplete bool

	// open is the constructs the parser is inside, innermost last, and
	// lastText the token before the current one. Both exist for one reason:
	// when the input runs out, the panel names four different parts of that
	// state and this is where they come from.
	open     []opener
	lastText string

	// openAtEnd is what open held when the input ran out, kept because the
	// stack is unwound by the time Parse returns: opens closes by deferring,
	// so a caller that asked afterwards would always be told nothing was
	// open. A prompt asking what it is waiting for asks afterwards.
	openAtEnd []opener

	// funcBody says the command about to be parsed is a function's body, so
	// that a brace group standing as one is recorded as the function rather
	// than as a group. They are the same syntax and not the same thing to
	// someone being told what is still open.
	funcBody bool

	// depth bounds nesting while parsing operands, which are themselves
	// words and may hold further expansions. Pathological input is the
	// normal case on the keystroke path, so this is a bound rather than a
	// trust.
	depth int

	// bodyTookTerm is where a command's own body took the separator that
	// would otherwise have ended the statement around it, and it is the one
	// way a statement is terminated without holding the terminator itself.
	//
	// A short loop body is a whole statement, terminator and all: the `;` in
	// `for i (a b) echo $i; echo end` belongs to `echo $i`, and there is
	// nothing left between the loop and `echo end` — so the loop is
	// terminated by it too, and the list may go on. A `do … done` or a brace
	// body is closed by its own word instead and leaves the statement
	// unterminated, which is measured: the shell with short loops parses
	// `for i (a b) echo $i; echo end` and refuses
	// `for i (a b) { echo $i; } echo end`.
	bodyTookTerm Pos

	// separatorStood is where a `;` the dialect stepped over stands, while
	// it is still the innermost thing the parse is inside.
	//
	// Measured on ksh93u+ 2026-09-12, `-n` over a script file ending where
	// it is shown. A separator that was stepped over becomes the token that
	// shell names when the input then runs out, in place of the keyword it
	// otherwise names:
	//
	//	{ :                     `{' unmatched
	//	{ : ;                   `{' unmatched   — a *terminator* is not this
	//	{ ;                     `;' unmatched
	//	{ ; :                   `;' unmatched   — and more input does not clear it
	//	( ;                     `;' unmatched
	//	if :; then ;            `;' unmatched
	//	while :; do ;           `;' unmatched
	//	x() { ; 	        `;' unmatched
	//	{ false || ;            `;' unmatched
	//	{ false && ;            `;' unmatched
	//	{ : |& ;                `;' unmatched
	//	if false || ; then      `;' unmatched   — a clause does not displace it
	//	{ ; } ; if :; then      `then' unmatched — but its construct closing does
	//
	// #1207 filed this as the `;` that admitted an empty **and-or** operand.
	// It is not: `{ ; ` has no and-or in it and answers the same, so it is
	// the step-over and not the operator.
	//
	// **A `case` arm is the exception and is measured, not assumed.**
	// `case x in x) ;` and `case x in x) false || ;` both answer
	// `` `case' unmatched `` there. Two neighbors of that are left
	// unmodeled deliberately: once the arm's `;;` has been read the `;` is
	// named again, and with a newline between them the `;;` is named — and
	// that last answer comes back for `case x in x) : ;` + newline + `;;`,
	// which has no stepped-over separator in it at all, so it is a fact
	// about the terminator rather than about this.
	//
	// **Diagnostic only.** It is deliberately not an entry on p.open, so it
	// never reaches Parser.Open() and never reaches a continuation prompt.
	// That shell has no open-state prompt escape, so what it prompts here
	// cannot be measured, and #1207 is explicit that guessing both halves at
	// once is how a wrong rule gets into the tables.
	separatorStood Pos

	// inCondition says the list about to be read is a keyword's condition,
	// where one dialect refuses the `;` it steps over elsewhere. Set by
	// parseCondition and cleared by the parseList that reads it.
	inCondition bool
}

// maxParamDepth is how far `${x:-${y:-…}}` may nest before the parser stops.
// Deep enough that no real script reaches it, shallow enough that no input
// can exhaust the stack.
const maxParamDepth = 64

// NewParser returns a parser over src.
func NewParser(src string, d Dialect) *Parser { return NewParserAt(src, d, 1) }

// NewParserAt returns a parser over src whose first line is numbered first.
//
// Input does not always arrive whole. A shell reading its program from a
// descriptor takes a piece of it, runs that, and comes back for more — the
// piece is all the parser has, and a diagnostic still has to name the line of
// the *input* rather than the line of the piece. Nothing else changes: an
// offset stays relative to src, because it is only ever used to quote the text
// a node was written as, and that text is here.
//
// first is the number to give src's first line, so 1 is NewParser.
func NewParserAt(src string, d Dialect, first int) *Parser {
	lex := NewLexer(src, d)
	if first > 0 {
		// Before the first token is read: the lookahead carries a position,
		// and a line set afterwards would leave that one token behind.
		lex.line = first
	}
	return newParserOn(lex, d)
}

// newParserOn wraps a lexer the caller has already set up.
//
// It exists because the first token is read here, before anything outside can
// reach the lexer: a caller with something to say to it — [ShellWords], which
// has to install its recorder and stop here-document bodies being read — has
// to say it before that, or the very first token is lexed under the wrong
// settings and never recorded.
func newParserOn(lex *Lexer, d Dialect) *Parser {
	p := &Parser{lex: lex, dialect: d}
	p.next()
	return p
}

// EndsWithContinuation reports whether text ends with a backslash joining it to
// a line that has not arrived yet.
//
// A parser cannot answer this, and is right not to. `echo one \` at the end of
// a *file* is a finished command — the continuation joins it to nothing — while
// the same text at the end of what has been read so far is a command still
// being written. Which one it is depends on whether there is more input, which
// only the thing doing the reading knows.
func EndsWithContinuation(text string) bool {
	text = strings.TrimSuffix(text, "\n")
	n := 0
	for i := len(text) - 1; i >= 0 && text[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// Parse parses src completely.
func Parse(src string, d Dialect) (*File, error) {
	p := NewParser(src, d)
	f := p.Parse()
	return f, p.Err()
}

// Err reports why parsing stopped, or nil.
func (p *Parser) Err() error { return p.err }

// SetDialect replaces the dialect for input that has not been read yet.
//
// A shell can change its own grammar while it runs: a builtin executed on one
// line decides whether a quantified group is a group on the next. The parser
// cannot know that — the builtin runs in an interpreter this package has never
// heard of — so the front end, which is the only place the two meet, reads the
// interpreter's answer between lines and hands it in here. The same joint as
// Aliases, for the same reason.
//
// It applies to what has not been tokenized. The parser holds one token of
// lookahead, so the very first token of the next line may have been read under
// the old dialect; every construct a runtime toggle governs sits deeper in a
// line than its first token, which is what makes the boundary safe.
func (p *Parser) SetDialect(d Dialect) {
	p.dialect = d
	p.lex.dialect = d
}

// Incomplete reports whether the input ended part-way through a construct that
// could still be finished. A prompt should ask for another line.
func (p *Parser) Incomplete() bool { return p.incomplete || p.lex.Incomplete() }

// LineShift is how many lines the alias bodies this parser expanded added to
// the numbering, where [Dialect.AliasBodyCountsLines] says they count.
//
// A caller that reads a program in pieces — one that arrives on a descriptor —
// retires a piece by counting its newlines and starting the next parser after
// them. That count is of the text, and an expanded alias contributed lines
// that were never in the text, so without this the numbering resets at the
// first refill.
func (p *Parser) LineShift() int { return p.aliasLineShift }

// slice returns the source between two positions, which is how a node keeps
// the text it was written as. A diagnostic quotes what the author typed, and
// no reconstruction from the tree can be relied on to match it.
func (p *Parser) slice(from, to Pos) string {
	if from.Offset < 0 || int(to.Offset) > len(p.lex.src) || from.Offset > to.Offset {
		return ""
	}
	return p.lex.src[from.Offset:to.Offset]
}

func (p *Parser) next() {
	if p.tok.Kind != TokEOF && p.tok.Text != "" {
		p.lastText = p.tok.Text
	}
	if p.aliasSpliced > 0 {
		p.aliasSpliced--
	}
	if len(p.pending) > 0 {
		// An alias expansion is still being handed out. Nothing else about
		// the input has moved, so the lexer is not touched.
		p.tok, p.pending = p.pending[0], p.pending[1:]
		// A global alias inside an alias body is expanded in turn — measured
		// `alias -g B=x; alias -g H='a B'` gives `a x` — so the tokens being
		// handed out are asked as well as the ones being read. Not a fresh
		// chain: these tokens are the expansion's, and the names it has
		// already spent are spent for them.
		p.expandGlobalAlias(false)
		return
	}
	p.tok = p.lex.Next()
	if p.err == nil && p.lex.Err() != nil {
		p.err = p.lex.Err()
	}
	// A global alias is expanded wherever a word stands, so the question is
	// asked of every token rather than at the handful of places a command
	// word is read. A token read here begins a chain of its own. See
	// expandGlobalAlias.
	p.expandGlobalAlias(true)
	if p.lex.Incomplete() {
		// The lexer ran out inside a quote or an expansion. The parser may
		// never fail over it — a word that never finished is still a word —
		// so the snapshot has to be taken here, while the constructs around
		// it are still on the stack. Taken once, like every other.
		p.ranOut()
	}
}

// primeAliases offers the *first* token of the input to the global-alias
// hook, which could not have seen it when it was read.
//
// [newParserOn] reads that token in the constructor, deliberately — a caller
// with something to say to the lexer has to say it before anything is lexed —
// and the three alias hooks are plain fields the caller sets afterwards. Every
// later token passes through [Parser.next] with the tables in hand; this one
// is the exception, and without this a global alias written as the first word
// of a parse is the one word that never expands. Command position is not
// affected, because the table there is read when the command is parsed rather
// than when its word was lexed.
//
// Once, because a second pass over a token already expanded would spend its
// names again: `alias -g S='S x'` would come out `S x x`.
func (p *Parser) primeAliases() {
	if p.aliasPrimed {
		return
	}
	p.aliasPrimed = true
	p.expandGlobalAlias(true)
}

func (p *Parser) at(k Kind) bool { return p.tok.Kind == k }

// atWord reports whether the current token is the given reserved word, written
// unquoted. Quoting removes the reservation: `"if"` is a command name.
func (p *Parser) atWord(s string) bool {
	return p.tok.Kind == TokWord && !p.tok.IsQuoted() && p.tok.Literal() == s
}

// stopWords end a list. They are only reserved where a command may begin,
// which is exactly where parseList tests them: `echo if then` passes them
// through as arguments because parseSimple never asks.
var stopWords = map[string]bool{
	"then": true, "elif": true, "else": true, "fi": true,
	"do": true, "done": true, "esac": true, "}": true,
}

// reservedWords is every word the grammar reserves, which is the stop words
// plus the ones that open a construct. It is the class distinction one
// dialect's wording turns on: `fi` is quoted there and `echo` is "word".
var reservedWords = map[string]bool{
	"if": true, "then": true, "elif": true, "else": true, "fi": true,
	"for": true, "while": true, "until": true, "do": true, "done": true,
	"case": true, "in": true, "esac": true, "{": true, "}": true,
	"function": true, "select": true, "time": true,
}

// atReservedWord reports whether the current token is one of the words the
// grammar reserves, written unquoted. Quoting removes the reservation exactly
// as it does for [Parser.atWord]: `"if"` is a command name.
func (p *Parser) atReservedWord() bool {
	return p.tok.Kind == TokWord && !p.tok.IsQuoted() && reservedWords[p.tok.Literal()]
}

// atReservedPrecommand reports whether the current token is one of the words
// the dialect takes away in front of a command — see
// [Dialect.ReservedPrecommands]. Quoting removes the reservation, exactly as
// it does for `if`: `\nocorrect echo hi` is `command not found`.
func (p *Parser) atReservedPrecommand() bool {
	if p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	return p.dialect.ReservedPrecommands[p.tok.Literal()]
}

func (p *Parser) atStopWord() bool {
	if p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	// `end` closes a `foreach`, and it is reserved wherever a command may
	// begin rather than only inside one: measured, `end` alone and
	// `end() { :; }` are both parse errors in the shell that has the loop,
	// while `echo end` and `end=5` are not. So it is a stop word for that
	// dialect and an ordinary word for the other four.
	if p.dialect.Foreach && p.tok.Literal() == "end" {
		return true
	}
	return stopWords[p.tok.Literal()]
}

// atListEnd reports whether the current token closes the list the parser is
// reading, rather than being something a command could begin with.
//
// The tokens are the ones every enclosing construct stops on: a stop word,
// the `)` of a subshell or a `case` arm's pattern list, and a `case`
// terminator. The end of input is deliberately not one of them — a caller
// asking this is deciding whether a list *ended*, and input that ran out has
// not ended, it has stopped, which is a separate answer with a continuation
// prompt attached to it.
//
// A terminator is not one either. `;` and `&` end a statement rather than the
// list holding it, and the shell that takes an and-or with no right-hand side
// splits exactly there: measured 2026-09-07 on zsh 5.9.2, `{ true || ⏎ }`,
// `( true || )`, `if x; then true || fi`, `while …; do true || done` and
// `case x in x) true || ;; esac` all run, and `true || & b` is a parse error
// naming the `&`.
func (p *Parser) atListEnd() bool {
	switch p.tok.Kind {
	case TokRightParen, TokDSemi, TokSemiAmp, TokDSemiAmp, TokSemiPipe:
		return true
	}
	return p.atStopWord()
}

// tokenText names a token the way a diagnostic should: the word itself when
// there is one, and the operator's spelling otherwise.
func tokenText(tok Token) string {
	if tok.Kind == TokWord {
		return `"` + tok.Literal() + `"`
	}
	return `"` + tok.Kind.String() + `"`
}

// opener is a construct or clause the parser is currently inside.
//
// It is kept so that running out of input can be *described* rather than just
// reported: the panel names four different parts of that state — see
// ErrUnterminated — and all four are here.
type opener struct {
	word string
	line int
	// construct marks a compound command rather than a clause of one. `if`
	// is a construct and the `then` inside it is not, which is the
	// distinction one shell's wording turns on.
	construct bool
}

// opens records a construct and returns the function that closes it.
//
// The close truncates rather than pops, so a clause opened inside it — `then`,
// `else` — needs no unwinding of its own and an early return cannot leave the
// stack out of step with the parse.
func (p *Parser) opens(word string) func() {
	depth := len(p.open)
	// A separator stood over inside this construct belongs to it and goes
	// with it. `{ ; } ; if :; then` names the `then` in the shell that names
	// one of these, where `{ ; ` on its own names the `;` — measured.
	stood := p.separatorStood
	p.separatorStood = Pos{}
	p.open = append(p.open, opener{word: word, line: int(p.tok.Pos.Line), construct: true})
	return func() {
		p.open = p.open[:depth]
		p.separatorStood = stood
	}
}

// opensClause records a keyword that is itself awaiting a partner.
//
// A clause replaces the clause before it rather than stacking on it: `then`
// and `else` are alternatives within one `if`, not one inside the other, and
// a list saying both were open would be describing a state the parser was
// never in. Constructs below are untouched, so `for` holding an `if` holding
// an `else` still reads as the three of them.
func (p *Parser) opensClause(word string) {
	for len(p.open) > 0 && !p.open[len(p.open)-1].construct {
		p.open = p.open[:len(p.open)-1]
	}
	p.open = append(p.open, opener{word: word, line: int(p.tok.Pos.Line)})
}

// ranOut records that the input ended with something unfinished, and keeps
// what was open at that moment.
//
// One place rather than an assignment at each site. The snapshot has to be
// taken while the stack is still standing, and a site that set the flag
// without taking it would leave a prompt with nothing to say — a failure that
// looks exactly like nothing having been open.
func (p *Parser) ranOut() {
	if !p.incomplete {
		p.openAtEnd = append([]opener(nil), p.open...)
	}
	p.incomplete = true
}

// Open is what the parser was still inside when the input ran out, outermost
// first.
//
// Empty when the input was complete, or when it was wrong in some way other
// than ending too soon. A caller drawing a continuation prompt wants this: it
// is the difference between "there is more to type" and "there is more to
// type and it is the `for` from three lines up".
//
// The words are the shell's own keywords, which is all this package knows.
// What a dialect calls them at a prompt is the dialect's business — one of
// them says `for` where another would say the clause inside it.
func (p *Parser) Open() []Open {
	out := make([]Open, 0, len(p.openAtEnd)+1)
	for _, o := range p.openAtEnd {
		out = append(out, Open{Word: o.word, Line: o.line, Construct: o.construct})
	}
	// The lexer's own, innermost: a quote or an expansion is inside whatever
	// construct the parser had reached, and it is what the next line goes on
	// with. It arrives even when the parser recorded nothing, because a word
	// that never finished can end the input without the parser having asked
	// for anything.
	if w := p.lex.Open(); w != "" {
		out = append(out, Open{Word: w})
	}
	return out
}

// Open is one thing the parser is inside.
type Open struct {
	// Word is the keyword that opened it: `if`, `for`, `case`, `{`, or a
	// clause's own word such as `then` or `do`.
	Word string
	// Line is where it was opened, which is what a diagnostic names when the
	// construct began further up than the failure.
	Line int
	// Construct distinguishes a compound command from a clause of one. `if`
	// is a construct and the `then` inside it is not.
	Construct bool
}

// unterminated describes the state the parser gave up in.
func (p *Parser) unterminated(expected string) *Error {
	e := &Error{
		Pos: p.tok.Pos, Kind: ErrUnterminated,
		Expected: expected, LastToken: p.lastText,
	}
	if n := len(p.open); n > 0 {
		e.Innermost = p.open[n-1].word
		if p.separatorStood.IsValid() {
			// A `;` this dialect stepped over is what the one shell naming
			// an innermost keyword names instead, and it outlasts a clause:
			// `if false || ; then` with the input ending there is
			// `` `;' unmatched `` in ksh93u+ where `if :; then` is
			// `` `then' unmatched ``. See Parser.separatorStood.
			e.Innermost = ";"
		}
		for i := n - 1; i >= 0; i-- {
			if p.open[i].construct {
				e.Construct, e.ConstructLine = p.open[i].word, p.open[i].line
				break
			}
		}
	}
	e.EndLine = int(p.tok.Pos.Line)
	if !strings.HasSuffix(p.lex.src, "\n") {
		// The text stopped mid-line, so the end of it is the line after.
		e.EndLine++
	}
	e.Msg = "unexpected end of input"
	if e.Construct != "" {
		e.Msg = "unterminated " + e.Construct
	}
	return e
}

// failUnexpected records a token the grammar did not want, with what would
// have been valid where the parser knows it.
//
// The class travels because one dialect names it rather than the token: an
// ordinary word is "word unexpected" there, where a reserved word and an
// operator are quoted.
// failUnexpectedOperand is failUnexpected where a *command* could not begin,
// so no word there is reserved.
//
// A reserved word is only reserved where the grammar could take a command:
// `esac` in the pattern position of `case a in a) echo x;;& esac` is an
// ordinary word, and the dialect that names a word's class calls it one —
// "word unexpected", not `"esac" unexpected`. Treating the list of reserved
// words as reserved everywhere got that wrong in the one place it shows.
func (p *Parser) failUnexpectedOperand(expected string) {
	p.failUnexpectedAs(expected, true)
}

func (p *Parser) failUnexpected(expected string) {
	p.failUnexpectedAs(expected, false)
}

func (p *Parser) failUnexpectedAs(expected string, plain bool) {
	p.failUnexpectedAt(p.tok, expected, plain)
}

// failUnexpectedAt records a token the grammar did not want when the parser
// has already read past it.
//
// A rule that can only be checked once more input has been consumed — the
// function body that has to be compound, which is not known to be simple
// until it has been parsed — still has to name the token where the trouble
// began, and point the echo of the offending line at *its* line rather than
// at wherever the parser ended up. So the token travels rather than being
// read off the parser's current position.
func (p *Parser) failUnexpectedAt(tok Token, expected string, plain bool) {
	if p.err != nil {
		return
	}
	if tok.Kind == TokEOF {
		p.ranOut()
		p.err = p.unterminated(expected)
		return
	}
	literal, text := tokenLiteral(tok), tokenText(tok)
	source := tokenSource(tok)
	if p.emptyParensStartAt(tok) {
		// The dialect that reads `()` as one token names the pair wherever a
		// refusal falls on the first of them, and not only inside the
		// definition production that consumes them. See
		// [Dialect.EmptyParensAreOneToken].
		literal, text, source = "()", `"()"`, ""
	}
	p.err = &Error{
		Pos: tok.Pos, Kind: ErrUnexpected,
		Token: literal, TokenOpener: tokenOpener(tok),
		TokenSource: source, TokenHoldsExpansion: tokenHoldsAnExpansion(tok),
		Class: tokenClass(tok, plain), Expected: expected,
		Redirect: tok.Kind.IsRedirect(),
		Msg:      text + " unexpected",
	}
}

// emptyParensStartAt reports whether tok is the `(` of an adjacent `()`, in a
// dialect that reads the pair as one token.
//
// Adjacency is the rule and it is measured: `x=1 f () { … }` is a parse
// error naming `()` on zsh 5.9.2, and `x=1 f ( ) { … }` — the same
// line with one blank inside the parentheses — is blamed at the `}` instead,
// the two characters then being a subshell. So this reads the source rather
// than skipping blanks the way the definition path's lookahead does.
func (p *Parser) emptyParensStartAt(tok Token) bool {
	if !p.dialect.EmptyParensAreOneToken || tok.Kind != TokLeftParen {
		return false
	}
	i := int(tok.End.Offset)
	return i >= 0 && i < len(p.lex.src) && p.lex.src[i] == ')'
}

// tokenLiteral is the token as a diagnostic writes it, without the quotes a
// message may add of its own.
func (p *Parser) tokenLiteral() string { return tokenLiteral(p.tok) }

func tokenLiteral(tok Token) string {
	switch tok.Kind {
	case TokWord:
		return tok.Literal()
	case TokArithCmd:
		// The expression the construct held, blanks and all — ` 2 ` for
		// `(( 2 ))` — which is what two of the panel's four quote back.
		// Kind.String() answers `arithmetic command`, a description written
		// for prose, and standing it where a diagnostic quotes what it read
		// produced a sentence no shell writes (#2013).
		return tok.Text
	}
	return tok.Kind.String()
}

// tokenSource is the word as it was written, quotes and all, for the dialects
// that echo a refused word back rather than naming what it comes to. See
// Error.TokenSource.
//
// Only a word has two spellings: every other token's source and its name are
// the same characters, and TokArithCmd's Text is already the expression rather
// than the source, so answering it here would name the same thing twice.
func tokenSource(tok Token) string {
	if tok.Kind != TokWord {
		return ""
	}
	return tok.Text
}

// tokenOpener is the second spelling of a token whose text is not what it was
// written as. See Error.TokenOpener; only the arithmetic command has one.
func tokenOpener(tok Token) string {
	if tok.Kind == TokArithCmd {
		return "(("
	}
	return ""
}

func (p *Parser) tokenClass(plain bool) TokenClass { return tokenClass(p.tok, plain) }

func tokenClass(tok Token, plain bool) TokenClass {
	if tok.Kind == TokNewline {
		return ClassNewline
	}
	if tok.Kind != TokWord {
		return ClassOperator
	}
	if !plain && !tok.IsQuoted() && reservedWords[tok.Literal()] {
		return ClassReserved
	}
	return ClassWord
}

func (p *Parser) fail(format string, args ...any) {
	p.failKind(ErrSyntax, format, args...)
}

// failKind records a parse failure with a classification, so a dialect can
// word it without matching on the message text.
func (p *Parser) failKind(kind ErrorKind, format string, args ...any) {
	if p.err != nil {
		return
	}
	if p.at(TokEOF) {
		// Running out of input is unfinished rather than wrong.
		p.ranOut()
	}
	p.err = &Error{Pos: p.tok.Pos, Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// expectWord consumes a reserved word or records what was missing.
func (p *Parser) expectWord(s string) Pos {
	pos := p.tok.Pos
	if !p.atWord(s) {
		if p.at(TokEOF) {
			// The input ended with something still open, which every shell in
			// the panel reports as its own kind of failure rather than as a
			// word in the wrong place.
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated(s)
			}
			return pos
		}
		p.failUnexpected(s)
		return pos
	}
	p.next()
	return pos
}

// Parse reads the whole input.
func (p *Parser) Parse() *File {
	f := &File{}
	for {
		line, ok := p.NextLine()
		if !ok {
			break
		}
		f.Stmts = append(f.Stmts, line.Stmts...)
	}
	f.Last = p.tok.Pos
	return f
}

// NextLine parses one logical line: the statements up to the newline that ends
// them, which is more than one line of text when a construct is still open.
//
// It exists because a shell runs what it has read rather than reading
// everything first. `echo one` on line 1 runs before line 3 fails to parse,
// which is unanimous across the panel and is why a script that ends badly
// still does what its good lines said.
//
// The *line* is the unit and not the statement, which is measured: with
// `echo one; { fi; }` on one line, nothing runs. So everything up to the
// newline is parsed before any of it is run, and a failure anywhere in it
// discards the whole line.
//
// The second result is false at the end of the input, and when parsing has
// already failed.
func (p *Parser) NextLine() (*File, bool) {
	p.primeAliases()
	p.skipNewlines()
	if p.at(TokEOF) || p.err != nil {
		return nil, false
	}
	f := &File{}
	for p.err == nil {
		// A `;` where a command belongs, for the dialects that step over one:
		// `; echo two` and `true ; ; echo two` alike, since the second
		// statement of a line begins here as much as the first does.
		if p.skipSeparators(false, false) {
			p.skipNewlines()
		}
		st := p.parseStmt()
		if st == nil {
			if p.err == nil && !p.at(TokEOF) {
				// Nothing here can begin a command: a stop word with no
				// construct open, most often. Every shell in the panel calls
				// that a syntax error — `}` alone is one in all four — where
				// this used to stop quietly and silently truncate the rest of
				// the script. `function f { ...; }` in a dialect without the
				// keyword is the case that found it: the `}` ended parsing,
				// and the commands after it never ran.
				p.failUnexpected("")
			}
			break
		}
		f.Stmts = append(f.Stmts, st)
		if p.at(TokNewline) || p.at(TokEOF) {
			break
		}
		if !st.Semi.IsValid() {
			// Nothing terminated the statement, so the line ended with it:
			// two commands need a `;`, a newline or an `&` between them.
			// Only a command that ends itself gets this far — a simple
			// command absorbs the next word as an argument — which is why it
			// shows on `{ :; } echo x` and never on `true echo x`.
			p.failUnexpected("")
			break
		}
	}
	f.Last = p.lineEnd()
	return f, true
}

// lineEnd is where the logical line just parsed ends in the input.
//
// The token that ended it, ordinarily — and the line that closed a
// here-document where one was read, because its body and its delimiter are
// physical lines of the command that owns them and the token says nothing
// about them. The newline that triggers the read sits at the end of the
// command's *first* line and the body is consumed behind it, so `cat <<END`
// over three lines of script otherwise reports one.
//
// The furthest of the two, which is the whole rule: a line with no
// here-document is unaffected, and one with several is closed by the last
// delimiter rather than the first.
func (p *Parser) lineEnd() Pos {
	at := p.tok.Pos
	if e := p.lex.heredocEnd; e.Offset > at.Offset {
		return e
	}
	return at
}

func (p *Parser) skipNewlines() {
	for p.at(TokNewline) {
		p.next()
	}
}

// skipSeparators steps over a `;` written where a command belongs, as far as
// the dialect goes, and reports whether it stepped over any.
//
// A newline *before* one is stepped over by the caller and always was —
// `a || ⏎ b` is core — so `a || ⏎ ; b` runs wherever `a || ; b` does. A
// newline **after** one is the dialect's own question and the two shells
// answer it differently, which is measured:
//
//	true || ; ⏎ echo two   →  two        ksh93
//	                       →  (nothing)  zsh
//
// zsh reads on and makes `echo two` the right-hand side, so the `true`
// short-circuits past it. ksh93 stops at the newline, the and-or ends with
// nothing on its right, and `echo two` is the next statement — which is why
// only the wider value steps over what follows.
//
// afterBar says the caller is a pipeline looking for the command after its
// bar, and inCondition that it is a keyword's condition list looking for a
// statement of its own. Those are the two positions ksh93 will not take, and
// they are both the caller's to know: the wider value takes them both.
func (p *Parser) skipSeparators(afterBar, inCondition bool) bool {
	limit := 0
	crossNewlines := false
	switch p.dialect.SeparatorWhereACommandBelongs {
	case OneSeparatorExceptAfterABarOrBeforeACondition:
		if afterBar || inCondition {
			return false
		}
		limit = 1
	case AnySeparatorWhereACommandBelongs:
		limit = -1
		crossNewlines = true
	default:
		return false
	}
	skipped := false
	for limit != 0 && p.at(TokSemi) {
		if !p.insideACaseArm() {
			// What the dialect that names an innermost keyword names from
			// here on. See Parser.separatorStood, where the arm exception is
			// measured too.
			p.separatorStood = p.tok.Pos
		}
		p.next()
		if crossNewlines {
			p.skipNewlines()
		}
		skipped = true
		if limit > 0 {
			limit--
		}
	}
	return skipped
}

// parseList reads statements until a stop word, a closing paren, or the end —
// and until a statement that was written with no terminator after it.
//
// That last one is the list's real boundary and nothing else marks it: two
// statements need a `;`, a newline or an `&` between them, so a command
// standing straight after one that ended itself — `(( i < 2 )) echo hi` — is
// not a second statement of the same list. Every shell in the panel refuses
// that text where it is only a list; the one with short loops reads the second
// command as a loop *body*, which is why the boundary has to be visible here
// rather than guessed at afterwards.
//
// The list stops rather than failing, and the caller says what was wrong,
// because the caller is the one that knows what it was waiting for: the panel
// names the token it met and one shell also names the closer it wanted, which
// is `)` for a subshell and `done` for a loop. A simple command hides the gap
// entirely, since its words absorb whatever follows — `true echo x` is one
// command with an argument — so this only ever shows after a compound.
func (p *Parser) parseList() []*Stmt {
	// Read and cleared at the top, so a list nested inside a condition — a
	// brace group written as one, a substitution in one — is an ordinary
	// list again. Only the two calls in this function are what the rule is
	// about: an and-or's right-hand side and a pipeline's are their own
	// positions, and ksh93 answers those differently. Measured.
	inCondition := p.inCondition
	p.inCondition = false
	var out []*Stmt
	p.skipNewlines()
	if p.skipSeparators(false, inCondition) {
		// A newline after the separator ends nothing here: between two
		// statements it is an ordinary terminator, which every shell takes.
		// It is only where an and-or's right-hand side belongs that one shell
		// stops at it.
		p.skipNewlines()
	}
	for p.err == nil && !p.at(TokEOF) && !p.atStopWord() && !p.at(TokRightParen) {
		st := p.parseStmt()
		if st == nil {
			if p.err == nil && len(out) > 0 && p.at(TokSemi) {
				// A `;` the dialect would not step over, standing where the
				// *next* statement of this list begins. The list stopped
				// silently and left it for whatever the caller wanted a
				// separator before, which took it: `if :; ; then :; fi`
				// parsed in all four dialects where dash, bash 5.3 and ksh93
				// each name the second `;` and only zsh runs it (#2023).
				//
				// Only once the list has something in it. An empty one is
				// [Parser.requireBody]'s question — the dialect that allows
				// an empty body allows `if ; then :; fi` with it — and
				// failing here would answer it twice and differently.
				p.failUnexpected("")
			}
			break
		}
		out = append(out, st)
		if !st.Semi.IsValid() {
			break
		}
		p.skipNewlines()
		if p.skipSeparators(false, inCondition) {
			p.skipNewlines()
		}
	}
	return out
}

// parseBody is parseList where the grammar requires the list to have
// something in it, which is every compound command's body and every
// condition — but not a `case` arm, whose body may be empty in all four, and
// not a command substitution, which is a program rather than a body.
//
// The check is here rather than in parseList because parseList is also how
// the places that *may* be empty read their contents, and because the answer
// is one answer: an empty body is refused by dash, bash and ksh93 in every
// shape and taken by zsh in every shape, so a shell asks once.
//
// The token the parser stopped on is what is named, which is what the panel
// names: `}` for a brace group, `)` for a subshell, `fi` and `done` for the
// keyword forms. Nothing is named as expected alongside it — an empty body
// has nothing to be in the middle of, which is the same distinction
// parseGroup already draws for a reserved word it cannot use.
func (p *Parser) parseBody() []*Stmt {
	return p.requireBody(p.parseList())
}

// insideACaseArm reports whether the innermost construct the parse is inside
// is a `case`. See Parser.separatorStood for why one position is excepted.
func (p *Parser) insideACaseArm() bool {
	for i := len(p.open) - 1; i >= 0; i-- {
		if p.open[i].construct {
			return p.open[i].word == "case"
		}
	}
	return false
}

// parseCondition is parseBody for the list a keyword takes as its *condition*
// — `if`, `elif`, `while`, `until` — where one dialect will not step over a
// `;` that it steps over everywhere else.
//
// Measured on ksh93u+ 2026-09-12: `if; then :; fi` and `if :; ; then :; fi`
// are both “ `;' unexpected “ there, while `if :; then : ; ; fi` and
// `if false || ; then :; fi` run — so the position is where a *statement of
// the condition list* begins, and neither the header as a whole nor the token
// after the keyword. See Dialect.SeparatorWhereACommandBelongs, where the
// nine rows are (#2023).
func (p *Parser) parseCondition() []*Stmt {
	p.inCondition = true
	list := p.parseBody()
	p.inCondition = false
	return list
}

// requireBody is parseBody for a caller that read its list some other way —
// a loop header, where whether the list stops at the body is a dialect's
// answer of its own.
func (p *Parser) requireBody(list []*Stmt) []*Stmt {
	if len(list) != 0 || p.dialect.EmptyCompoundBody || p.err != nil {
		return list
	}
	if p.dialect.SteppedOverSeparatorIsABody && p.separatorStood.IsValid() &&
		(p.atStopWord() || p.at(TokRightParen)) {
		// The body was written as a `;` this dialect steps over, and the
		// construct's closer is what came next: `{ ; }` runs where `{ }` is
		// refused. The closer has to be there for it — `{ ; ; }` leaves the
		// parse sitting on the second `;`, which is one more than the
		// dialect's count steps over and is refused there too.
		//
		// End of input is not a closer, so it falls through to the caller
		// below, which is what reports an unterminated construct.
		return list
	}
	if p.at(TokEOF) {
		// The input ran out rather than the body being empty, and what is
		// waiting for more is the caller's to report — it knows which
		// construct is open.
		return list
	}
	p.failUnexpected("")
	return list
}

// parseStmt reads one and-or list and its terminator.
func (p *Parser) parseStmt() *Stmt {
	// Cleared here rather than after it is read, so that the statement being
	// started asks about its own body and never about an earlier one's.
	p.bodyTookTerm = Pos{}
	expr := p.parseAndOr()
	if expr == nil {
		return nil
	}
	st := &Stmt{Expr: expr, Semi: p.bodyTookTerm}
	switch p.tok.Kind {
	case TokPipeAmp:
		// Only where the dialect reads the operator as a coprocess. Where it
		// reads it as a pipe the token was consumed by parsePipeline and
		// never arrives here, and where it has neither reading the lexer
		// never made the token at all.
		//
		// Which makes the guard **equivalent rather than decisive today**,
		// and it is kept for what it says: mutating it to `if false`
		// survives the suite, and a probe of both other dialects says why —
		// with the pipe reading, `cat |&`, `cat |& b`, `a && cat |&` and a
		// trailing newline all answer exactly as they do without the mutant,
		// because parsePipeline never leaves this token behind; with neither
		// reading the lexer makes `|` and `&`. A dialect setting both flags
		// is the only reachable difference, no preset does, and the pipe
		// reading would win there anyway. Recorded so the next reader does
		// not go looking for the row that would kill it.
		if !p.dialect.CoprocPipeOperator {
			break
		}
		// `&` plus two pipes, and it terminates the same thing `&` does: the
		// whole and-or, measured — `echo A && cat |&` sends `echo A`'s
		// output into the coprocess.
		st.Background, st.Coprocess = true, true
		st.Semi = p.tok.Pos
		st.Text = p.textBetween(expr.Pos(), st.Semi)
		p.next()
	case TokAmp, TokAmpBang, TokAmpPipe:
		// `&` belongs to the statement, not the command: `a && b &`
		// backgrounds the whole and-or. `&!` and `&|` are the same
		// terminator with the job let go of, and they reach the same
		// statement for the same reason — `true && false &!` disowns the
		// whole and-or.
		st.Background = true
		st.Disown = p.tok.Kind != TokAmp
		st.Semi = p.tok.Pos
		// What was written, for a `jobs` listing to show. Taken from the
		// input rather than rebuilt from the tree: `jobs` shows what someone
		// typed, spacing and quoting included, and a printer would show what
		// the parser understood — which is a different thing and the wrong
		// one here.
		st.Text = p.textBetween(expr.Pos(), st.Semi)
		p.next()
	case TokSemi:
		st.Semi = p.tok.Pos
		p.next()
	case TokNewline:
		st.Semi = p.tok.Pos
	}
	return st
}

// textBetween is the input from one position up to another, trimmed.
//
// Bounds-checked rather than trusted: a Pos is only as good as whatever
// produced it, and this is on the path a `jobs` listing prints from.
func (p *Parser) textBetween(from, to Pos) string {
	src := p.lex.src
	if from.Offset < 0 || int(to.Offset) > len(src) || from.Offset >= to.Offset {
		return ""
	}
	return strings.TrimSpace(src[from.Offset:to.Offset])
}

// parseAndOr reads pipelines joined by && and ||.
//
// One precedence level, left-associative. Building a right-leaning tree here,
// or giving && a tighter binding, is C's rule and is wrong: every shell prints
// B for `true || echo A && echo B`, which needs `(true || echo A) && echo B`.
func (p *Parser) parseAndOr() Expr {
	left := p.parsePipeline()
	if left == nil {
		return nil
	}
	depth := len(p.open)
	for p.at(TokAndAnd) || p.at(TokOrOr) {
		op, pos := p.tok.Kind, p.tok.Pos
		// Open while the command after it is looked for, the same way a
		// pipeline's bar is: input ending on `&&` is a line waiting for its
		// other half rather than a line that merely stopped.
		p.open = append(p.open, opener{word: op.String(), line: int(pos.Line)})
		p.next()
		p.skipNewlines()
		skipped := p.skipSeparators(false, false)
		right := p.parsePipeline()
		if right == nil {
			if skipped && p.dialect.AbsentAndOrOperandIsAnEmptyCommand &&
				(p.at(TokEOF) || p.at(TokNewline) || p.atListEnd()) {
				// A separator stood where the right-hand side belongs and
				// the list ended there, so a command that does nothing and
				// succeeds stands in its place: `false || ;` answers 0.
				//
				// The end of input counts here where it does not for
				// OpenEndedAndOr, and that is measured rather than assumed:
				// through a pty, ksh93 answers `false || ;` at its prompt
				// with **another PS1** and `$?` of 0 — the line is finished —
				// where `false ||` alone draws PS2 and waits. zsh draws PS2
				// for both, which is why its end-of-input answer stays the
				// route question #1174 left open and this one does not.
				//
				// A newline counts too, and only here: `true || ; ⏎ echo two`
				// prints `two` in this shell, so the and-or ended at the
				// newline and the `echo` is the statement after it — where
				// zsh reads on and makes it the right-hand side.
				//
				// A second separator is none of the three, so `a || ; ; b`
				// still fails on the one it found, which is what this shell
				// does.
				//
				// A node rather than an answer the interpreter reads, because
				// an empty command already runs and already succeeds — the
				// difference between this dialect and the one that drops the
				// operator is what is *in* the tree.
				p.open = p.open[:depth]
				empty := &SimpleCmd{Start: p.tok.Pos, Stop: p.tok.Pos}
				left = &BinaryExpr{
					X: left, Op: op, OpPos: pos,
					Y: &Pipeline{Cmds: []Command{empty}},
				}
				continue
			}
			if p.dialect.OpenEndedAndOr && p.atListEnd() {
				// The right-hand side is absent and the list ends here, so
				// the operator is dropped: `{ : || ⏎ }` is `{ : ⏎ }`.
				//
				// Dropped rather than stood in for, which is measured. The
				// status is the left-hand side's — `false ||` answers 1 and
				// `true &&` answers 0 — so an absent operand is neither an
				// implicit success nor an implicit failure. Either stand-in
				// gets exactly one of that pair wrong.
				//
				// No check that parsing is still clean, deliberately: a
				// failure already recorded is never cleared — every fail
				// site returns early once p.err is set — so taking this
				// branch cannot turn a refusal into an acceptance. A guard
				// here was written and then removed as unreachable: nothing
				// distinguished the two, because the only other thing this
				// branch does is give up the openers below, and once a
				// failure is recorded the prompt reads the snapshot ranOut
				// took rather than the live stack.
				p.open = p.open[:depth]
				return left
			}
			// The token that stopped it is named, not the operator behind
			// it — the same rule a bar follows, and wrong here for the same
			// reason it was wrong there: the whole panel quotes what it
			// found and none of them mentions the `&&`, which by now is
			// read and was never the problem. `echo a && fi` is `"fi"
			// unexpected` and `true && & b` blames the `&`.
			//
			// Not the operand form: a command *may* begin after `&&`, so a
			// reserved word standing there is reserved, and the dialect that
			// names a word's class instead of quoting it quotes this one.
			//
			// Input that ran out is unfinished rather than wrong, which
			// failUnexpected already distinguishes: a line ending on `&&`
			// is waiting for its other half, and every shell in the panel
			// draws a continuation prompt for it — measured through a pty
			// on all three that have one.
			p.failUnexpected("")
			return left
		}
		p.open = p.open[:depth]
		left = &BinaryExpr{X: left, Op: op, OpPos: pos, Y: right}
	}
	return left
}

// parsePipeline reads commands joined by `|`, with an optional leading `!`
// that negates the whole pipeline rather than its first command.
//
// `time` is read here rather than in parseCommand because that is what it
// binds: the whole pipeline, on either side of the `!` — `time ! true` and
// `! time true` both parse, and both report.
func (p *Parser) parsePipeline() Expr {
	if p.dialect.TimeKeyword && p.atWord("time") {
		return p.parseTime(false, Pos{})
	}
	pl := &Pipeline{}
	if p.atWord("!") {
		pl.Negated = true
		pl.Bang = p.tok.Pos
		p.next()
		if p.dialect.TimeKeyword && p.atWord("time") {
			return p.parseTime(true, pl.Bang)
		}
	}
	// A bar is recorded while the command after it is being looked for, so
	// that input ending there is describable as a pipeline waiting for its
	// other half and not only as input that ended.
	depth := len(p.open)
	for {
		cmd := p.parseCommand()
		if cmd == nil {
			if len(pl.Cmds) == 0 {
				return nil
			}
			// The token that stopped it is named, not the bar behind it.
			// Every shell in the panel that refuses this quotes what it
			// found — `;`, `&`, the end of input — and none of them says
			// anything about the `|`, which by now is read and was never
			// the problem. It reaches `|&` because a dialect without that
			// operator lexes those two bytes as a bar and an ampersand, so
			// the refusal lands on the `&` where bash 3.2's and dash's do.
			//
			// Not the operand form: a command *may* begin after a bar, so a
			// reserved word standing there is reserved, and the dialect that
			// names a word's class instead of quoting it quotes this one —
			// `echo a | fi` is `"fi" unexpected` there and not `word
			// unexpected`. The only words that reach here are stop words,
			// which are exactly the reserved ones.
			p.failUnexpected("")
			return pl
		}
		// The bar has its command, so the pipeline is whole again: a failure
		// after this has nothing to do with it.
		p.open = p.open[:depth]
		pl.Cmds = append(pl.Cmds, cmd)
		// `|&` joins two commands only where it is the *pipe* reading. Where
		// it is the coprocess operator the pipeline is over and the token
		// belongs to the statement, which is what makes `cat |& | wc -l` a
		// refusal at the second bar — measured, ksh93 says
		// ``syntax error … `|' unexpected``.
		bothStreams := p.at(TokPipeAmp) && p.dialect.PipeBothStreams
		if !p.at(TokPipe) && !bothStreams {
			return pl
		}
		if bothStreams {
			p.mergeStderr(cmd, p.tok.Pos)
		}
		// Pushed rather than opened as a clause: a clause displaces the
		// clause before it, and a bar is not one of those — it belongs to
		// the pipeline and not to whatever construct the pipeline is in. As
		// a clause it evicted the `then` it was written inside, which then
		// reported an `if` waiting for a bar.
		p.open = append(p.open, opener{word: p.tok.Kind.String(), line: int(p.tok.Pos.Line)})
		p.next()
		p.skipNewlines()
		// And a `;` written where the command after the bar belongs, for the
		// one dialect that steps over one there. ksh93 will not: it takes
		// `a || ; b` and refuses `a | ; b`, which is why this asks.
		p.skipSeparators(true, false)
	}
}

// parseTime reads `time [-p] [pipeline]`, the keyword already at hand.
//
// The pipeline is read by parsePipeline itself, so `time` takes everything a
// pipeline takes — the bars, an inner `!`, even another `time` — and nothing
// more: an `&&` past it belongs to the caller. A bare `time` is legitimate
// and reports on nothing, but a bare `time` followed by `|` is a pipe with no
// first element, which is the syntax error bash makes of it.
func (p *Parser) parseTime(negated bool, bang Pos) Expr {
	tc := &TimeClause{Negated: negated, Bang: bang, Time: p.tok.Pos, Stop: p.tok.End}
	p.next()
	if p.dialect.TimePosixFlag && p.atWord("-p") {
		tc.Posix, tc.PosixPos = true, p.tok.Pos
		tc.Stop = p.tok.End
		p.next()
	}
	tc.Pipeline = p.parsePipeline()
	if tc.Pipeline == nil && p.at(TokPipe) {
		p.failUnexpected("")
	}
	return tc
}

// parseCommand dispatches on what begins the command.
func (p *Parser) parseCommand() Command {
	// Taken here and cleared, so that only the command the function opened
	// can be its body: anything nested inside is a group like any other.
	body := p.funcBody
	p.funcBody = false
	// Before the keyword dispatch below, because an alias may hold one:
	// `alias iff='if true; then'` has to produce the `if` the grammar reads.
	// The set is fresh per command, so `e yes; e two` expands `e` twice.
	if p.Aliases != nil || p.SuffixAliases != nil {
		p.aliasNextWord = false
		p.aliasDone = map[string]bool{}
		p.expandCommandWord(p.aliasDone)
	}
	switch {
	case p.at(TokEOF), p.at(TokNewline), p.atStopWord():
		return nil
	case p.at(TokLeftParen) && p.dialect.AnonymousFunction && p.lex.peekIsRightParen():
		// `()` where a command begins is an empty parameter list rather than
		// a subshell with nothing in it — which every dialect refuses, so
		// nothing is taken away by reading it this way.
		return p.withRedirs(p.parseAnonFunc(false))
	case p.at(TokLeftParen):
		return p.withRedirs(p.parseSubshell())
	case p.at(TokArithCmd):
		return p.withRedirs(p.parseArithCmd())
	case p.atWord("{"):
		return p.withRedirs(p.parseGroupOrTry(body))
	case p.atWord("if"):
		return p.withRedirs(p.parseIf())
	case p.atWord("while"), p.atWord("until"):
		return p.withRedirs(p.parseLoop())
	case p.atWord("for"):
		return p.withRedirs(p.parseFor())
	case p.atWord("repeat") && p.dialect.Repeat:
		return p.withRedirs(p.parseRepeat())
	case p.atWord("foreach") && p.dialect.Foreach:
		return p.withRedirs(p.parseForeach())
	case p.atWord("select") && p.dialect.Select:
		return p.withRedirs(p.parseSelect())
	case p.atWord("case"):
		return p.withRedirs(p.parseCase())
	case p.atWord("[[") && p.dialect.DoubleBracket:
		return p.withRedirs(p.parseTestClause())
	case p.atWord("function") && p.dialect.AnonymousFunction && p.peekIsAnonBody():
		return p.withRedirs(p.parseAnonFunc(true))
	case p.atWord("function") && p.dialect.FunctionKeyword:
		return p.parseFuncKeyword()
	case p.atWord("coproc") && p.dialect.Coproc:
		return p.parseCoproc()
	}
	return p.parseSimple()
}

// parseCoproc reads `coproc [NAME] command`.
//
// The name is only a name when a compound command follows it — with a simple
// command the first word is the command, which is why `coproc cat` runs cat
// rather than defining a coprocess called cat that runs nothing. Measured:
// bash reports `MY: command not found` for `coproc MY cat`. The construct is
// specified in the `coproc` section of docs/spec/grammar/commands.md.
//
// A dialect with the word and no CoprocName never reads a name at all, so
// `coproc MY { cat; }` is `MY {` followed by an unexpected `}` there — which
// is the failure zsh reports, and it falls out of the missing flag rather
// than being written down twice.
func (p *Parser) parseCoproc() Command {
	c := &CoprocClause{Coproc: p.tok.Pos}
	p.next()
	if p.dialect.CoprocName && p.at(TokWord) && isPlainName(p.tokenLiteral()) && !stopWords[p.tokenLiteral()] {
		w := p.word()
		if p.startsCompoundCommand() {
			c.Name = w.Literal()
			c.Cmd = p.parseCommand()
		} else {
			// The word was the command after all, and the rest of the
			// simple command is read from here.
			sc, _ := p.parseSimple().(*SimpleCmd)
			if sc == nil {
				sc = &SimpleCmd{Start: w.Pos(), Stop: p.tok.Pos}
			}
			sc.Args = append([]*Word{w}, sc.Args...)
			sc.Start = w.Pos()
			c.Cmd = sc
		}
	} else {
		c.Cmd = p.parseCommand()
	}
	if c.Cmd == nil && p.err == nil {
		p.failUnexpected("")
		return nil
	}
	if c.Cmd != nil {
		c.Stop = c.Cmd.End()
	}
	return c
}

// startsCompoundCommand reports whether the current token opens a compound
// command — the question `coproc NAME …` turns on.
func (p *Parser) startsCompoundCommand() bool {
	if p.at(TokLeftParen) || p.at(TokArithCmd) {
		return true
	}
	if !p.at(TokWord) {
		return false
	}
	switch p.tokenLiteral() {
	case "{", "if", "while", "until", "for", "case", "select", "[[":
		return true
	}
	return false
}

// isPlainName reports whether s could name a variable: letters, digits and
// underscores, not starting with a digit.
func isPlainName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// withRedirs attaches trailing redirections to a compound command, because a
// redirection on one applies to everything inside it.
func (p *Parser) withRedirs(c Command) Command {
	type hasRedirs interface{ addRedir(*Redirect) }
	h, ok := c.(hasRedirs)
	if !ok {
		return c
	}
	for p.tok.Kind.IsRedirect() || p.at(TokIONumber) {
		r := p.parseRedirect()
		if r == nil {
			break
		}
		h.addRedir(r)
	}
	return c
}

func (r *redirs) addRedir(x *Redirect) { r.Redirs = append(r.Redirs, x) }

func (c *SimpleCmd) addRedir(x *Redirect) { c.Redirs = append(c.Redirs, x) }

// addRedir on a definition reaches its body, which is where the parser
// already puts a written one: `f() { :; } 2>&1` reads the redirection as the
// body compound's, and the definition itself has no list to hold it.
func (c *FuncDecl) addRedir(x *Redirect) {
	if h, ok := c.Body.(interface{ addRedir(*Redirect) }); ok {
		h.addRedir(x)
	}
}

// mergeStderr attaches the redirection `|&` stands for to the command before
// it, at the end of that command's own list.
//
// At the end and not the start, which is the whole of the operator: both
// shells that have it show the error for `e 2>/dev/null |& cat` and show
// nothing for `e >/dev/null |& cat`, and only a `2>&1` written *after*
// whatever the command wrote for itself gives that pair of answers.
//
// A command with nowhere to put a redirection is left alone. The one that
// reaches here is a definition whose body never parsed, which is already an
// error, and a definition writes nothing either way.
func (p *Parser) mergeStderr(cmd Command, pos Pos) {
	h, ok := cmd.(interface{ addRedir(*Redirect) })
	if !ok {
		return
	}
	h.addRedir(&Redirect{
		N:        literalWord("2", pos),
		Op:       TokGreatAmp,
		OpPos:    pos,
		Text:     "1",
		Word:     literalWord("1", pos),
		PipeBoth: true,
	})
}

// literalWord is a word the grammar supplies rather than one the script
// wrote, positioned at the operator it stands for.
func literalWord(text string, pos Pos) *Word {
	return &Word{
		Spans: []Span{{Kind: Literal, Value: text, Quoting: Unquoted, Pos: pos}},
		Start: pos, Stop: pos,
	}
}

func (p *Parser) word() *Word {
	if p.tok.Kind != TokWord {
		return nil
	}
	w := p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
	p.next()
	return w
}

// newWord builds a word and parses the expansions inside it. Every word in
// the tree goes through here, so no path can produce one with an unparsed
// ${ } in it.
func (p *Parser) newWord(spans []Span, start, stop Pos) *Word {
	out := make([]Span, len(spans))
	copy(out, spans)
	for i := range out {
		switch {
		case out[i].Kind == ParamExp && out[i].Param == nil:
			out[i].Param = p.parseParamExp(out[i].Value, out[i].Pos, out[i].Quoting, out[i].Bare)
		case out[i].Kind == ArithSubst && out[i].Arith == nil:
			// Deferring, like the arithmetic command: a `$(( ))` whose
			// expression will not read does not refuse the file. Both
			// spellings come through here, so `$[echo hi]` is covered by
			// the same line (#865).
			out[i].Arith = p.parseArithLater(out[i].Value, out[i].Pos)
		}
	}
	return &Word{Spans: out, Start: start, Stop: stop}
}

// parseRedirect reads an optional IO number, an operator and its target.
func (p *Parser) parseRedirect() *Redirect {
	r := &Redirect{}
	if p.at(TokIONumber) {
		r.N = p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
		p.next()
	}
	if !p.tok.Kind.IsRedirect() {
		p.fail("expected a redirection operator")
		return nil
	}
	r.Op, r.OpPos = p.tok.Kind, p.tok.Pos
	// A target is read where a command would otherwise begin, and no
	// assignment may be written there: `> a==(echo hi) cat` is `missing end
	// of string` in the dialect where the same word at command position
	// assigns a path. Written with the redirection *first*, because that is
	// the only one this reaches — a target after a word is already in
	// argument position. Set before the `p.next()` that reads the target,
	// because that is the token the flag has to reach; see
	// Lexer.noAssignment.
	savedNoAssign := p.lex.noAssignment
	p.lex.noAssignment = true
	p.next()
	p.lex.noAssignment = savedNoAssign
	if p.tok.Kind != TokWord {
		// The token that is there, not the one that is missing. `cat <(x)` in
		// a dialect without process substitution is `"(" unexpected` in dash,
		// which is what it says about every other token in the wrong place —
		// and this was the one redirection failure that said something else.
		//
		// Nothing named where a token is there, because dash names what it
		// was waiting for only when that is a keyword. At end of input there
		// is a construct to name, and the wording that reports one always
		// prints the clause.
		if p.at(TokEOF) {
			p.failUnexpectedOperand("a redirection target")
		} else {
			p.failUnexpectedOperand("")
		}
		return nil
	}
	// Built without advancing, because advancing is what reaches the newline
	// where the body is read — and the queue has to be set before that
	// happens. Registering after p.word() looks equivalent and silently
	// collects nothing.
	r.Word = p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
	// As it was written, for the one dialect that names it when the target
	// turns out not to be a single word. Taken from the input rather than
	// rebuilt from the spans: `$e` and `${e}` are the same word and not the
	// same text, and it is the text that goes in the message.
	r.Text = p.textBetween(p.tok.Pos, p.tok.End)
	if r.Op.IsHeredoc() {
		// Any quoting *anywhere* in the delimiter makes the whole body
		// literal, and a backslash counts. Both are detected the same way:
		// if removing quotes changed the text, it was quoted. `\EOF` and
		// `"EOF"` both differ from their literal; a bare `EOF` does not.
		quoted := p.tok.Text != p.tok.Literal()
		// The body starts after the next newline, which the lexer reaches;
		// the delimiter is here, which the parser has. Hence the handoff.
		p.lex.queueHeredoc(r, quoted)
	}
	p.next()
	return r
}

// assignHead is the name half of an assignment, already taken apart: the name,
// the subscript if one was written, whether the `=` was an append, and where
// the value begins.
//
// The subscript is a span sequence rather than a string because it may hold
// expansions — `a[$i]=v` is how a loop ordinarily writes an element — and a
// substitution is not text the parser may flatten. That is also why the head
// carries a position into the token rather than a length: the value's first
// span is whatever is left of the span holding the `=`, and the spans after
// it belong to the value whole.
type assignHead struct {
	name   string
	index  []Span // the subscript's spans, nil when none was written
	append bool
	span   int // the span holding the `=`
	off    int // byte offset just past the `=`, within that span's value
}

// isAssign reports whether a word is `name=…` written so the name is unquoted.
func (p *Parser) isAssign(t Token) (assignHead, bool) {
	var h assignHead
	if t.Kind != TokWord || len(t.Spans) == 0 || t.Spans[0].Quoting != Unquoted ||
		t.Spans[0].Kind != Literal {
		return h, false
	}
	head := t.Spans[0].Value
	eq := strings.IndexByte(head, '=')
	// `name[i]=` is an assignment too, and the bracket has to be looked for
	// before the `=`: a `=` inside the value — `a=b[0]` — is the ordinary
	// scalar case and must not be read as a subscript.
	if open := strings.IndexByte(head, '['); open > 0 && (eq < 0 || open < eq) {
		return p.subscriptedAssign(t, open)
	}
	// Only the first span is looked at here, and that is the rule rather than
	// a shortcut: `a$b=c` is a command name in every shell on the panel, so a
	// name interrupted by an expansion is not a name.
	if eq <= 0 {
		return h, false
	}
	name := head[:eq]
	h.span, h.off = 0, eq+1
	// `name+=value` appends. The `+` is part of neither the name nor the
	// value, so it is taken off here.
	if strings.HasSuffix(name, "+") {
		if !p.dialect.AppendAssign {
			return h, false
		}
		name = name[:len(name)-1]
		h.append = true
	}
	if !isName(name) {
		// A run of digits names a positional parameter where the dialect has
		// the construct. Behind isName rather than inside it, because every
		// other caller of that function is asking about an identifier — a
		// `for` variable, a function's name, a declaration's operand — and
		// none of them takes a digit in any shell on the panel.
		if !p.dialect.PositionalAssignment || !isDigitRun(name) {
			return h, false
		}
	}
	h.name = name
	return h, true
}

// isDigitRun reports whether s is one or more decimal digits and nothing else.
//
// Leading zeros included: `01=z` writes the first positional parameter in the
// shell with the construct, measured 2026-09-07, so the digits are read as a
// number rather than matched against a canonical spelling.
func isDigitRun(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// subscriptedAssign reads the `name[subscript]=` shape, whose subscript may
// hold expansions and so may run across several spans. open is where the `[`
// stands in the token's first span.
//
// The closing `]` is the first one written as unquoted literal text with an
// `=` or a `+=` after it, so that anything a quoted or substituted span
// contributes is data — the subscript is expanded later, and a `]` that
// arrives from an expansion never closes anything.
func (p *Parser) subscriptedAssign(t Token, open int) (assignHead, bool) {
	var h assignHead
	// Without arrays there is no subscript to read, and the word is a command
	// name: the shell without them answers `a[1]=Q: not found`.
	if !p.dialect.ArraySubscript {
		return h, false
	}
	name := t.Spans[0].Value[:open]
	if !isName(name) {
		return h, false
	}
	for i, s := range t.Spans {
		if s.Kind != Literal || s.Quoting != Unquoted {
			continue
		}
		from := 0
		if i == 0 {
			from = open + 1
		}
		for j := from; j < len(s.Value); j++ {
			if s.Value[j] != ']' {
				continue
			}
			rest := s.Value[j+1:]
			appends := false
			if strings.HasPrefix(rest, "+=") {
				if !p.dialect.AppendAssign {
					continue
				}
				appends = true
			} else if !strings.HasPrefix(rest, "=") {
				continue
			}
			h.name = name
			h.append = appends
			h.index = spanRange(t.Spans, 0, open+1, i, j)
			h.span = i
			h.off = j + 1
			if appends {
				h.off++
			}
			h.off++ // the `=` itself
			return h, true
		}
	}
	return h, false
}

// assignIndexFlags reads the flag group an assignment's subscript may open
// with, which is the same group a *read* takes and the same scanner.
//
// `a[(r)y]=Q` replaces the element whose value is `y`, and `a[(i)nomatch]=W`
// appends, because `(i)` missing answers one past the last element and that is
// the index an append writes to. The semantics need nothing new: the group
// names an *index*, which is what a subscript on this side has always been.
//
// Only a group written as unquoted literal text at the front of the subscript
// is one. That is the same rule the read side follows and it is what keeps
// `a["(r)y"]=Q` and `a[$g]=Q` out: a group that arrives from an expansion is
// text, since the operand behind it is lexed as a word of its own and a group
// the source did not write has no operand to lex.
func (p *Parser) assignIndexFlags(spans []Span) *SubscriptFlags {
	if !p.dialect.ArraySubscriptFlags || len(spans) == 0 {
		return nil
	}
	if spans[0].Kind != Literal || spans[0].Quoting != Unquoted {
		return nil
	}
	g, rest, ok := scanSubscriptFlags(spans[0].Value)
	if !ok {
		return nil
	}
	// The operand is the rest of the first span plus every span after it, so
	// a substitution in it is performed exactly as one in the subscript would
	// be — `a[(re)$want]=Q` is the shape worth having.
	operand := append([]Span{{
		Kind: Literal, Value: rest, Quoting: Unquoted, Pos: spans[0].Pos,
	}}, spans[1:]...)
	g.Arg = p.newWord(operand, spans[0].Pos, spans[0].Pos)
	return g
}

// toEnd is spanRange's toOff for "as far as the spans go".
const toEnd = -1

// spanRange takes the run of spans from fromOff in span from through span to,
// stopping at toOff there — or at the end of the run when toOff is toEnd. A
// partial literal keeps its own position advanced by the offset, so a
// diagnostic about a subscript points at the subscript.
//
// Empty pieces are dropped: `a[$i]=v` splits its first span at the bracket
// with nothing before the expansion, and a zero-length literal span would be
// a word the printer writes back and the expander has to carry.
func spanRange(spans []Span, from, fromOff, to, toOff int) []Span {
	var out []Span
	for i := from; i <= to && i < len(spans); i++ {
		s := spans[i]
		if s.Kind != Literal {
			// A substitution is indivisible, so an offset can only fall
			// before or after it, never inside.
			out = append(out, s)
			continue
		}
		lo, hi := 0, len(s.Value)
		if i == from {
			lo = fromOff
		}
		if i == to && toOff != toEnd {
			hi = toOff
		}
		if lo >= hi {
			continue
		}
		s.Pos.Offset += int32(lo)
		s.Pos.Col += int32(lo)
		s.Value = s.Value[lo:hi]
		out = append(out, s)
	}
	return out
}

// isName reports whether s is a shell name: the production the grammar spells
// `name`, which POSIX defines as an identifier. `for 1 in …` is rejected by
// every shell for this reason.
// funcNamePunctuation is the punctuation a function name may carry where the
// dialect allows any, and it is measured rather than chosen — see
// [Dialect.FunctionNamePunctuation] for the run and for what was left out.
//
// `=` is not here and cannot be: an array assignment is a parenthesis after a
// word too, so admitting it would read `a=()` as a definition of a function
// called `a=`.
const funcNamePunctuation = "!#%+,-./:@]^"

// isFuncName is isName with that punctuation, per the flag, and with the bytes
// above ASCII — a name written in another script is a name to every shell that
// has punctuated ones at all.
// tokenHoldsAnExpansion reports whether a word token has a span the shell
// would expand, which is exactly when [Token.Literal] loses information.
//
// The test the name check needs: `_p_${w}` flattens to `_p_w`, a perfectly
// good name for a different function, so "is the literal a name" cannot see
// the difference and answered yes.
func tokenHoldsAnExpansion(t Token) bool { return spansHoldAnExpansion(t.Spans) }

func spansHoldAnExpansion(spans []Span) bool {
	for _, s := range spans {
		if s.Kind != Literal {
			return true
		}
	}
	return false
}

// keywordFuncName reports whether the word after the `function` keyword is a
// name.
//
// Two readings, and which one runs is [Dialect.FunctionKeywordNameIsAnyWord].
// Without it the name is checked as text, exactly as the POSIX form's is, and
// the two share isFuncName so that they cannot come apart. With it there is no
// text check at all: the shell that has the flag reads a word and the word is
// the name, whatever is in it.
//
// The one thing the flag does not carry is a name the shell would match
// against the *filesystem*. Measured on zsh 5.9.2: `function a*b { :; }` is
// `no matches found: a*b`, `a?b` the same, and `a[b` is `bad pattern: a[b` —
// so those are not definitions there either, and taking them as literal names
// here would put a function called `a*b` in the table at status 0 where the
// shell being modeled defines nothing. A refusal is the visible answer and a
// plausible wrong definition is not, so they stay refused. The quoting is what
// decides it and is read per span: `'a*b'` is ordinary text and is a name,
// `a*'b'` is not, because the `*` in it is still bare.
func (p *Parser) keywordFuncName(t Token) bool {
	if !p.dialect.FunctionKeywordNameIsAnyWord {
		return isFuncName(p.funcNameText(t), p.dialect.FunctionNamePunctuation)
	}
	return !holdsBarePatternCharacter(t)
}

// funcNameText is the text a definition's name is tested as.
//
// One dialect reads the word as it was **written** and the rest read what it
// comes to, which is [Dialect.FunctionNameIsSourceText] and is the whole of
// the difference between defining `f` from `function 'f'` and refusing it.
// The refused word is carried on the declaration as source text either way,
// so this changes which words are refused and never how one is named.
func (p *Parser) funcNameText(t Token) string {
	if p.dialect.FunctionNameIsSourceText {
		return p.textBetween(t.Pos, t.End)
	}
	return t.Literal()
}

// referenceListName reports whether t may stand in the word list a `function`
// keyword's name may be followed by in one dialect.
//
// A name and nothing else: quoting comes off first — `function a "b"` is
// taken there — and an expansion, an assignment and a word that is not an
// identifier are one refusal between them. See
// [Dialect.FunctionKeywordReferenceList], where the rows are.
func (p *Parser) referenceListName(t Token) bool {
	return !tokenHoldsAnExpansion(t) && isName(t.Literal())
}

// holdsBarePatternCharacter reports whether any literal span of t that the
// shell would match against the filesystem carries a pattern character.
//
// Per span rather than over [Token.Literal], because quoting is what decides
// whether a character is a pattern and Literal has already dropped it.
func holdsBarePatternCharacter(t Token) bool {
	return spansHoldBarePatternCharacter(t.Spans)
}

func spansHoldBarePatternCharacter(spans []Span) bool {
	for _, s := range spans {
		if s.Kind != Literal || s.Quoting != Unquoted {
			continue
		}
		if strings.ContainsAny(s.Value, "*?[") {
			return true
		}
	}
	return false
}

func isFuncName(s string, punctuation bool) bool {
	if isName(s) {
		return true
	}
	if !punctuation || s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 0x80 || c == '_' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			strings.IndexByte(funcNamePunctuation, c) >= 0
		if !ok {
			return false
		}
	}
	return true
}

func isName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// parseSimple reads assignments, arguments and redirections in any order.
//
// They interleave in the source — `>b echo hi` and `echo one >b two` both work
// — so redirections are lifted out wherever they appear rather than expected
// as a suffix.
func (p *Parser) parseSimple() Command {
	c := &SimpleCmd{Start: p.tok.Pos}
	seenArg := false
	// Argument position lasts as long as this command does. The token that
	// ends it has already been read by the time this returns, so restoring
	// the flag here is soon enough and is the only place that catches every
	// way out.
	defer func() { p.lex.inArgument = false }()

	for p.err == nil {
		switch {
		case p.at(TokIONumber), p.tok.Kind.IsRedirect():
			if r := p.parseRedirect(); r != nil {
				c.Redirs = append(c.Redirs, r)
			}
		case p.at(TokWord):
			// A function definition announces itself only at the paren —
			// which is why the words before it are read as arguments first
			// and turned into names here, where the dialect gives one body
			// several. An assignment ends that reading and is measured to:
			// `x=1 a b () { … }` is a refusal in the shell that has the
			// list, so the guard is the same one the first word takes.
			if len(c.Assigns) == 0 && !seenArg && p.looksLikeFuncDef() {
				return p.parseFuncPosix()
			}
			if !seenArg && p.atReservedPrecommand() {
				c.Precommands = append(c.Precommands, p.word())
				// The word after it stands where a command word stands, so
				// an alias there expands: `alias e=echo; nocorrect e hi`
				// prints `hi`. parseCommand expanded the *first* word before
				// dispatching here and this is the next one, so the call has
				// to be made again — leaving it out made the alias an
				// ordinary command name and `command not found` the answer.
				if p.Aliases != nil || p.SuffixAliases != nil {
					p.aliasNextWord = false
					p.expandCommandWord(map[string]bool{})
				}
				// And only a simple command may follow. A reserved word has
				// nowhere to go after it — `nocorrect if true; then echo hi;
				// fi` is `parse error near `if'` in the shell that has the
				// word, not an `if` with a modifier in front of it.
				if p.atStopWord() || p.atReservedWord() {
					p.failUnexpected("")
					return c
				}
				continue
			}
			if h, ok := p.isAssign(p.tok); ok && !seenArg {
				c.Assigns = append(c.Assigns, p.parseAssign(h))
				continue
			}
			// The command word is where an alias is expanded, and an
			// assignment prefix does not move it: `alias al=echo; y=2 al HI`
			// prints `HI` in both shells of the panel that expand aliases in
			// a script. parseCommand expanded the word it dispatched on, and
			// that word turned out to be an assignment, so the expansion has
			// to be made again here — where the real command word finally
			// stands. Left out, the word resolved as written and the answer
			// was `command not found` (#1942).
			//
			// aliasSpliced == 0 keeps this to a word of the *input*: an alias
			// value's own second word is not expanded in turn, only the word
			// after a value ending in a blank is, which is the rule the
			// trailing-space branch below carries. The set is
			// parseCommand's, so a name already spent in this command is not
			// expanded again.
			if len(c.Assigns) > 0 && !seenArg && p.aliasSpliced == 0 &&
				(p.Aliases != nil || p.SuffixAliases != nil) {
				p.aliasNextWord = false
				p.expandCommandWord(p.aliasDone)
				if h, ok := p.isAssign(p.tok); ok {
					// The value put an assignment where the command word was
					// — `alias al='x=1 echo'` — and it is a prefix like the
					// ones written out, so the word after it is the command
					// word in turn. Measured `HI`, status 0.
					c.Assigns = append(c.Assigns, p.parseAssign(h))
					continue
				}
				// Otherwise the word stands as the command word, expanded or
				// not, and the readings below are the ones it would have had.
				// Falling through rather than looping is what keeps a word
				// that names no alias from being offered here forever.
			}
			if a, consumed := p.declarationArray(c); consumed {
				if a != nil {
					c.Assigns = append(c.Assigns, a)
				}
				seenArg = true
				continue
			}
			if p.dialect.TimesIsReserved && seenArg && len(c.Args) == 1 &&
				c.Args[0].Literal() == "times" {
				// A reserved word takes no arguments, so the word after it
				// has nowhere to go.
				p.failUnexpected("")
				return c
			}
			if p.dialect.CloseBraceAlwaysReserved && p.atWord("}") {
				// Reserved even here, so it ends the command rather than
				// becoming an argument to it.
				return c
			}
			if p.aliasNextWord && p.aliasSpliced == 0 && p.Aliases != nil {
				// Still the table and not the suffix kind: this is the word
				// *after* a value ending in a blank, which is an argument
				// rather than a command word.
				// The expansion before this one ended in a space, so this
				// word is eligible too — the rule behind `alias sudo='sudo '`.
				// Only once the expansion's own tokens are spent: the space
				// makes the word *after* the value eligible, not the value's
				// own second word. Cleared first so a value that does not end
				// in a space stops the chain here.
				p.aliasNextWord = false
				p.expandAlias(map[string]bool{}, p.Aliases)
				continue
			}
			// A word list in front of `()` is a definition's name list where
			// the dialect gives one body several names — `clipcopy
			// clippaste() { … }`, and `echo hi () { … }`, which is why this
			// stands in the argument loop rather than in looksLikeFuncDef:
			// nothing about the first word announces it.
			//
			// *After* the declaration readings on purpose. `typeset -aU e1=()`
			// is an array declaration and not a definition of `typeset`,
			// `-aU` and `e1=`, and the utility is what decides that: measured
			// 2026-09-10, `a e1=() { echo X; }` defines `a` and `e1=` in the
			// shell that has the list, where the same word after `typeset -a`
			// is the array and the `()` after *it* is a parse error.
			if len(c.Assigns) == 0 && seenArg && p.dialect.FunctionMultipleNames &&
				p.argsCanBeFuncNames(c.Args) &&
				p.canBeFuncName(p.tok.Spans, p.tok.Literal()) &&
				p.lex.peekIsFuncParens() {
				return p.parseFuncPosixNames(c)
			}
			seenArg = true
			// The token *after* this word stands where an argument may, and
			// p.word() is what reads it — so the lexer is told before the
			// call rather than after it. One dialect reads a `(` there as
			// part of a word; everywhere else the flag changes nothing.
			p.lex.inArgument = true
			c.Args = append(c.Args, p.word())
		case p.at(TokLeftParen) && len(c.Assigns) == 0 && len(c.Redirs) > 0 &&
			p.dialect.FunctionMultipleNames && p.argsCanBeFuncNames(c.Args) &&
			p.lex.peekIsRightParen():
			// A redirection written *between* the names and the parentheses,
			// which is a definition too and whose redirection is the body's:
			// `a b >out () { echo "[$0]"; }` sends both calls to the file.
			// The `(` is recognized from the inside here, no word standing in
			// front of it to announce the reading.
			return p.parseFuncPosixNamesAtParen(c)
		case p.at(TokLeftParen) && (seenArg || len(c.Assigns) > 0 || len(c.Redirs) > 0):
			// A `(` in command position opens a subshell; one *after* a word
			// opens nothing. All four shells call it a syntax error, so this
			// is core rather than a dialect question. It is what makes
			// `function f() { … }` an error where the keyword is absent, and
			// `[[ ( -n x ) ]]` an error where `[[` is not a construct — both
			// of which used to run as ordinary commands with surprising
			// arguments.
			p.failUnexpected("")
			return c
		default:
			c.Stop = p.tok.Pos
			// Precommands count towards the command existing. `nocorrect`
			// with nothing after it is a command that runs nothing and
			// succeeds in the shell that has the word, and returning nil
			// here left the word consumed and the command gone — which
			// parseCommand reads as end of input.
			if len(c.Assigns) == 0 && len(c.Args) == 0 && len(c.Redirs) == 0 && len(c.Precommands) == 0 {
				return nil
			}
			return c
		}
	}
	c.Stop = p.tok.Pos
	return c
}

func (p *Parser) parseAssign(h assignHead) *Assign {
	a := &Assign{Name: h.name, Start: p.tok.Pos, Append: h.append}
	if h.index != nil {
		a.Index = p.newWord(h.index, h.index[0].Pos, p.tok.End)
		a.IndexFlags = p.assignIndexFlags(h.index)
	}
	// The value is what is left of the span holding the `=`, plus every span
	// after it — which is why the head reports a position rather than a count.
	// `a[$i]=$v` has an expansion on each side of the `=`, and only the spans
	// say which is which.
	spans := spanRange(p.tok.Spans, h.span, h.off, len(p.tok.Spans)-1, toEnd)
	a.Stop = p.tok.End
	if len(spans) > 0 {
		a.Value = p.newWord(spans, p.tok.Pos, p.tok.End)
	}
	p.next()

	// `a=(1 2)` is an array, and the parenthesis has to be adjacent. With a
	// space it is not a subshell — measured, against the comment that used
	// to stand here: `a= (echo x)` is a syntax error in dash, bash and zsh,
	// and only ksh93 accepts it, reading it as the literal and leaving `a`
	// holding `echo` and `x`. The adjacency check still matters, because it
	// decides *which* error, and the non-adjacent form now falls through to
	// the paren-after-a-word rule in parseSimple. Specified in the array
	// assignment section of docs/spec/grammar/commands.md.
	if a.Value == nil && p.at(TokLeftParen) && p.tok.Pos.Offset == a.Stop.Offset {
		if !p.dialect.ArrayLiteral {
			p.failUnexpected("")
			return a
		}
		a.IsArray = true
		// Between the parentheses an element stands where an argument does,
		// so a `(` that begins one belongs to the *element* — which is the
		// flag the lexer already has for it, set here for the same reason
		// parseSimple sets it before each argument.
		//
		// Without it the assignment found its closing `)` by counting, and a
		// glob flag opens one that is part of a word: `files=( (#i)a )` was
		// `expected ) to close an array assignment` where zsh accepts it, and
		// so was every other parenthesised shape that stands at the *front*
		// of an element. The three that stand after a pattern —
		// `*(-.DN)`, `*~(*/*)` and `*.(zip|tgz)` — were already right,
		// because mid-word the lexer folds the group without being told
		// (#1149).
		//
		// Restored rather than cleared, because an assignment is read at
		// command position as well as after a word, and the token *after*
		// the array is not an argument either way — parseSimple's own defer
		// is what ends argument position for the command.
		saved := p.lex.inArgument
		p.lex.inArgument = true
		p.next()
		p.skipNewlines()
		for p.tok.Kind == TokWord && p.err == nil {
			a.Elems = append(a.Elems, p.word())
			p.skipNewlines()
		}
		p.lex.inArgument = saved
		if !p.at(TokRightParen) {
			p.fail("expected ) to close an array assignment")
			return a
		}
		a.Stop = p.tok.End
		p.next()
	}
	return a
}

// declarationArray reads `name=(x y)` written as an operand of a utility that
// takes assignments, and reports nil when this is not one.
//
// Only the array form. A scalar `local a=1` is an ordinary word and stays one:
// it expands by rules of its own that expandAssignArg already implements, and
// routing it here would change a path nothing asked to change. The array form
// has no such path — it was a syntax error — which is the whole of what this
// adds.
//
// The test for it is that the word ends at its `=`, because that is the only
// shape a `(` can follow: `a=(x y)` reaches the parser as the word `a=` and
// then a parenthesis, where `a=1` is one word. If no parenthesis turns out to
// be there the word is handed back unchanged, so `local a=` is still an
// ordinary operand.
// consumed is separate from the assignment because the word is read either
// way: `local a=` is an ordinary operand and is put back as one, and the
// caller must not read it a second time.
func (p *Parser) declarationArray(c *SimpleCmd) (a *Assign, consumed bool) {
	if len(c.Args) == 0 || len(p.dialect.DeclarationUtilities) == 0 {
		return nil, false
	}
	if !p.dialect.DeclarationUtilities[c.Args[0].Literal()] {
		return nil, false
	}
	h, ok := p.isAssign(p.tok)
	if !ok || !strings.HasSuffix(p.tok.Text, "=") {
		// The suffix test is what keeps a scalar off this path rather than
		// what makes it come out right — the fallback below would hand
		// `local a=1` back unchanged anyway. It is here so the common case
		// does not take a round trip through parseAssign to arrive where it
		// started.
		return nil, false
	}
	tok := p.tok
	a = p.parseAssign(h)
	if a != nil && a.IsArray {
		a.Operand = true
		return a, true
	}
	// Not an array after all, so put the word back the way it came — read
	// once, by this, and not again by the caller.
	c.Args = append(c.Args, p.newWord(tok.Spans, tok.Pos, tok.End))
	return nil, true
}

// looksLikeFuncDef reports whether the current word begins `name()`.
func (p *Parser) looksLikeFuncDef() bool {
	if p.dialect.FunctionNameIsAnyWord {
		return p.anyWordFuncDef()
	}
	// A quoted name is not a definition in most dialects, and quoting is not
	// an expansion: `'q'() { :; }` is refused here as it was before the flag.
	// Two dialects read one, by different routes: the one whose names are any
	// word at all, in the branch above, and the one that reads the name as
	// *source text* and refuses it when the definition runs — where the
	// quotes are in the name rather than around it, so a quoted word is a
	// definition and the name it declares is not one.
	if p.tok.IsQuoted() && !p.dialect.FunctionNameIsSourceText {
		return false
	}
	// One unquoted literal span is the ordinary name. Where the dialect
	// expands a name, several spans are allowed and an expansion among them
	// is the point — and where it reads the name as source text, several
	// spans are allowed too so long as none of them is an expansion:
	// `a\ b()` is three spans there and is the definition bash reads before
	// refusing the name.
	if !p.dialect.FunctionNameExpands && !p.tokenIsPlainText(p.tok) {
		return false
	}
	if p.dialect.FuncDefAtParen {
		// The paren is the whole announcement here, and the word before it is
		// not checked for being a name: `[[ ( -n x ) ]]` is a definition of a
		// function called `[[` to a shell without `[[`, which is how the
		// dialect that does this reaches the diagnosis it reaches.
		//
		// `=` is still excluded, and for the reason below: an assignment of an
		// array is a parenthesis after a word too.
		return !strings.Contains(p.tok.Literal(), "=") && p.lex.peekIsLeftParen()
	}
	// A function name is a name — plus the punctuation the dialect allows —
	// so it cannot contain `=`. Without this, `a=()` — an empty array — was
	// read as a definition of a function called `a=`, because a parenthesis
	// pair follows either way.
	if p.dialect.FunctionNameExpands && tokenHoldsAnExpansion(p.tok) {
		// The name is a word here, so whether its *text* is a name cannot be
		// known until it is expanded. The parentheses are what say this is a
		// definition at all — except where the word is an assignment's name
		// half, because an array assignment is a parenthesis after a word
		// too and the subscript is where a loop ordinarily puts an
		// expansion: `a[$i]=()` empties an element and defines nothing.
		//
		// The reading is lexical, which is what tells the two apart. In
		// `a[$i]=` the `=` follows a name and a subscript written as literal
		// text, so the word is an assignment; in `_p_${w}=` the `=` follows
		// an expansion, so it is not one, and the function it defines is
		// named `_p_foo=` — measured on zsh 5.9.2, which lists it under
		// `${(k)functions}` with no variable `_p_foo` anywhere.
		if _, isAssign := p.isAssign(p.tok); isAssign {
			return false
		}
		return p.lex.peekIsFuncParens()
	}
	if !isFuncName(p.tok.Literal(), p.dialect.FunctionNamePunctuation) {
		return false
	}
	return p.lex.peekIsFuncParens()
}

// tokenIsPlainText reports whether t holds text and nothing the shell would
// expand, which is what a name may be made of where a name is not a word.
//
// One span is the ordinary reading and the only one most dialects allow: a
// second span means quoting, and quoting is what those dialects refuse before
// this is reached. The dialect that reads a name as source text keeps the
// quoting *in* the name, so it has several spans to walk.
func (p *Parser) tokenIsPlainText(t Token) bool {
	if !p.dialect.FunctionNameIsSourceText {
		return len(t.Spans) == 1 && t.Spans[0].Kind == Literal
	}
	if len(t.Spans) == 0 {
		return false
	}
	for _, sp := range t.Spans {
		if sp.Kind != Literal {
			return false
		}
	}
	return true
}

// anyWordFuncDef is looksLikeFuncDef where the word before the parentheses is
// the name whatever is in it — see [Dialect.FunctionNameIsAnyWord].
//
// Nothing about the *text* decides this, so the parentheses are the whole
// announcement and there is no name test to fail. The two exclusions are the
// readings that are not definitions at all rather than names being refused:
//
//	a=()        an empty array, and the `=` has to be bare to make one —
//	            `'a=b'()` and `a\=b()` are definitions of `a=b` there. The
//	            same lexical test [Dialect.FunctionNameExpands] makes.
//	a*b()       matched against the filesystem in the shell this models, so
//	            it defines nothing there and must not define anything here.
//	            Quoted the characters are ordinary text and are taken.
func (p *Parser) anyWordFuncDef() bool {
	if _, isAssign := p.isAssign(p.tok); isAssign {
		return false
	}
	if holdsBarePatternCharacter(p.tok) {
		return false
	}
	if !p.dialect.FunctionNameExpands && tokenHoldsAnExpansion(p.tok) {
		// A name that is not text until the shell runs is a *different*
		// flag's question, and without that flag there is nowhere to keep the
		// word: [FuncDecl.Name] would take the token's literal spelling, so
		// `_p_${w}() { … }` would define `_p_w` — a perfectly good name for a
		// perfectly wrong function, at status 0. The refusal that stood here
		// before this flag existed stands still.
		return false
	}
	return p.lex.peekIsFuncParens()
}

func (p *Parser) parseFuncPosix() Command {
	fn := &FuncDecl{Name: p.tok.Literal(), Start: p.tok.Pos}
	if text := p.funcNameText(p.tok); text != fn.Name &&
		!isFuncName(text, p.dialect.FunctionNamePunctuation) {
		// The dialect that reads the name as source text, given a word whose
		// text is not one: the declaration is read whole and the word is
		// kept as it was written, which is what the complaint quotes when
		// the definition runs. The keyword form takes the same branch in
		// parseFuncKeyword; only these two spellings have a name at all.
		fn.RefusedName = text
	}
	if p.dialect.FunctionNameExpands && tokenHoldsAnExpansion(p.tok) {
		// A name that is not text until the shell runs, kept whole. p.word()
		// consumes it, which is the p.next() the plain path takes.
		fn.NameWord = p.word()
	} else {
		p.next()
	}
	return p.parseFuncParensAndBody(fn)
}

// parseFuncParensAndBody reads `() compound` with the name or names already
// on the declaration and the parser standing at the `(`.
//
// Split out because the names reach it three ways — one word, a word list,
// and a word list with a redirection between it and the parentheses — and
// everything after the `(` is the same production in all three.
func (p *Parser) parseFuncParensAndBody(fn *FuncDecl) Command {
	p.next() // (
	if !p.at(TokRightParen) {
		p.failUnexpectedOperand(")")
		return fn
	}
	p.next()
	// The dialect that commits at the paren without allowing punctuation
	// checks the name once the parens close: dash's `f-g() { :; }` is
	// `Bad function name`, said only after `[[ ( -n x ) ]]`-shaped text has
	// already reached its own unexpected-word diagnosis above.
	if p.dialect.FuncDefAtParen && !p.dialect.FunctionNamePunctuation &&
		fn.NameWord == nil && !isName(fn.Name) {
		p.fail("Bad function name")
		return fn
	}
	if p.dialect.EmptyParensAreOneToken {
		// The parens are empty by construction here — anything between them
		// was refused above — so joining them is a matter of saying so. It
		// stands until the next token is read, which is exactly as long as it
		// is the last thing seen.
		p.lastText = "()"
	}
	// Whether the body was allowed to start on a later line, which one dialect
	// reports differently from a body that never started at all.
	atParens := p.tok.Pos.Line
	p.skipNewlines()
	sameLine := p.tok.Pos.Line == atParens
	// The body's first token, kept before the body is read. Both ways of
	// refusing a body below name it, and neither can be decided until the
	// parser has moved on: `f() >out` is only known not to be compound once
	// the redirection has been parsed as a command of its own.
	body := p.tok
	p.funcBody = true
	if fn.Body = p.parseCommand(); fn.Body == nil {
		hadError := p.err != nil
		p.failUnexpectedAt(body, "", false)
		if se, ok := p.err.(*Error); ok && !hadError && sameLine {
			se.FuncBody = true
		}
		return fn
	}
	if p.dialect.FuncBodyMustBeCompound {
		if _, isSimple := fn.Body.(*SimpleCmd); isSimple {
			p.failUnexpectedAt(body, "", false)
			return fn
		}
	}
	if p.dialect.FuncBodyTakesNoRedirection {
		// The operator rather than the body's first token: this dialect takes
		// the command and objects to what it redirects, so `f() echo hi >out`
		// is refused at the `>` with the `echo` already accepted.
		if simple, isSimple := fn.Body.(*SimpleCmd); isSimple && len(simple.Redirs) > 0 {
			r := simple.Redirs[0]
			p.failRedirectAt(r.OpPos, r.Op)
		}
	}
	return fn
}

// parseFuncPosixNames is parseFuncPosix where the words already read are the
// definition's earlier names — `clipcopy clippaste() { … }`, and `a b () { …
// }` with a blank in front of the parentheses.
//
// The words arrive as a simple command's arguments because that is what they
// are until the parentheses appear, so the declaration is read by the
// ordinary path and the arguments are folded onto the front of its name list
// afterwards. Each is read the way a name after the keyword is read, which is
// what keeps the two spellings from drifting: a word holding an expansion
// stays a word, and a name is its literal text everywhere else.
//
// A redirection written *between* the names and the parentheses is a
// definition there too — `a b >out () { echo "$0"; }` sends both calls to the
// file — and is not read here, because the parentheses then follow the
// redirection's target rather than a name and there is no word in hand when
// the `(` arrives. [Parser.parseFuncPosixNamesAtParen] is that route.
func (p *Parser) parseFuncPosixNames(c *SimpleCmd) Command {
	cmd := p.parseFuncPosix()
	fn, ok := cmd.(*FuncDecl)
	if !ok || p.err != nil {
		return cmd
	}
	last := FuncName{Name: fn.Name, Word: fn.NameWord}
	names := make([]FuncName, 0, len(c.Args)+len(fn.AlsoNamed))
	for _, w := range c.Args[1:] {
		names = append(names, p.funcNameFromWord(w))
	}
	first := p.funcNameFromWord(c.Args[0])
	fn.Name, fn.NameWord = first.Name, first.Word
	fn.AlsoNamed = append(names, append(fn.AlsoNamed, last)...)
	fn.Start = c.Start
	// A redirection read before the names is the body's too, and it arrives
	// the same way: `>out a b () { … }` writes the file in the shell that has
	// the list. Attached once the body exists, for the reason
	// [Parser.parseFuncPosixNamesAtParen] gives.
	for _, r := range c.Redirs {
		fn.addRedir(r)
	}
	return fn
}

// parseFuncPosixNamesAtParen is parseFuncPosixNames where a *redirection*
// stands between the names and the parentheses, so the parser reaches the `(`
// with no word in front of it and the whole name list already read.
//
// The redirection is the **body's**, which is where a definition's written
// one goes everywhere else — `f() { :; } 2>&1` reads it that way — so it is
// attached once the body exists rather than kept on the declaration. Measured
// 2026-09-12 on zsh 5.9.2: `a b >out () { echo "[$0]"; }; a; b; cat out`
// prints nothing at the terminal and leaves `[b]` in the file, so both names
// share one redirected body. Several are taken and so is a leading one:
// `a b >o1 >o2 ()` writes both files and `>o1 a b ()` writes the one.
//
// See #1838; the formatter is the other half, and `printer.funcDecl` has it.
func (p *Parser) parseFuncPosixNamesAtParen(c *SimpleCmd) Command {
	fn := &FuncDecl{Start: c.Start}
	first := p.funcNameFromWord(c.Args[0])
	fn.Name, fn.NameWord = first.Name, first.Word
	for _, w := range c.Args[1:] {
		fn.AlsoNamed = append(fn.AlsoNamed, p.funcNameFromWord(w))
	}
	cmd := p.parseFuncParensAndBody(fn)
	if p.err != nil {
		return cmd
	}
	for _, r := range c.Redirs {
		fn.addRedir(r)
	}
	return cmd
}

// funcNameFromWord reads a word already parsed as one name of a definition.
//
// The same split [FuncDecl.NameWord] records: a word the shell would expand
// is kept whole, because its literal text names a different function, and
// everything else is its text. Which of the two applies is the dialect's
// answer and not this word's.
func (p *Parser) funcNameFromWord(w *Word) FuncName {
	n := FuncName{Name: w.Literal()}
	if p.dialect.FunctionNameExpands && spansHoldAnExpansion(w.Spans) {
		n.Word = w
	}
	return n
}

// canBeFuncName reports whether a word standing in a definition's name list
// after the first may be one of its names — the tests looksLikeFuncDef makes
// of the word in front of the parentheses, minus the one that is not about
// names at all.
//
// One production, so one rule: a bare pattern character is matched against the
// filesystem and names nothing, and a name that is not text until the shell
// runs is kept only where the dialect expands one. What is *not* asked here is
// whether the word is an assignment — that reading belongs to the first word
// of a command and nowhere else, so measured 2026-09-10 on zsh 5.9.2,
// `a c=d () { :; }` defines `a` and `c=d` where `c=d () { :; }` alone is an
// assignment of an empty array. `a*b c() { :; }` is `no matches found: a*b`
// there and `a "b c" () { :; }` defines both names.
func (p *Parser) canBeFuncName(spans []Span, literal string) bool {
	if spansHoldBarePatternCharacter(spans) {
		return false
	}
	if spansHoldAnExpansion(spans) {
		return p.dialect.FunctionNameExpands
	}
	if p.dialect.FunctionNameIsAnyWord {
		return true
	}
	return isFuncName(literal, p.dialect.FunctionNamePunctuation)
}

// wordCanBeFuncName is canBeFuncName asked of a word already parsed.
func (p *Parser) wordCanBeFuncName(w *Word) bool {
	return p.canBeFuncName(w.Spans, w.Literal())
}

// argsCanBeFuncNames is wordCanBeFuncName over every word already read, which
// is what says an argument list standing in front of `()` is a name list.
func (p *Parser) argsCanBeFuncNames(args []*Word) bool {
	if len(args) == 0 {
		return false
	}
	for _, w := range args {
		if !p.wordCanBeFuncName(w) {
			return false
		}
	}
	return true
}

// failRedirectAt records a redirection operator the grammar did not want,
// named by the operator and not by what follows it.
func (p *Parser) failRedirectAt(pos Pos, op Kind) {
	if p.err != nil {
		return
	}
	p.err = &Error{
		Pos: pos, Kind: ErrUnexpected,
		Token: op.String(), Class: ClassOperator, Redirect: true,
		Msg: op.String() + " unexpected",
	}
}

// peekIsAnonBody reports whether `function` is followed straight by a body
// rather than by a name, which is the keyword spelling of an anonymous
// function.
func (p *Parser) peekIsAnonBody() bool {
	i := p.lex.off
	for i < len(p.lex.src) && isBlank(p.lex.src[i]) {
		i++
	}
	return i < len(p.lex.src) && (p.lex.src[i] == '{' || p.lex.src[i] == '(')
}

// parseAnonFunc reads `() body [word …]` and `function body [word …]`.
//
// The words after the body are the call's positional parameters, which is
// what makes this a call and not only a definition: there is no name to
// invoke it by later, so it runs where it stands. They are read the way a
// simple command's arguments are, and stop where a command stops.
func (p *Parser) parseAnonFunc(keyword bool) Command {
	fn := &AnonFunc{Keyword: keyword, Start: p.tok.Pos}
	p.next()
	if !keyword {
		if !p.at(TokRightParen) {
			p.failUnexpectedOperand(")")
			return fn
		}
		p.next()
	}
	p.skipNewlines()
	// A nameless function's body is a function body, which is what says the
	// try-always keyword may not follow it: measured 2026-09-07, `() { echo
	// anon; } always { echo A; }` is a parse error on the `}` in the shell
	// that has both constructs — the `always` and the `{` after it are read
	// as *arguments* to the call, which is what the word loop below already
	// does. Without saying so here, the brace group is offered the keyword
	// the way one standing as a command is (#1216).
	p.funcBody = true
	fn.Body = p.parseCommand()
	if fn.Body == nil {
		if keyword {
			p.fail("expected a body after `function`")
			return fn
		}
		// `()` with nothing after it is the empty subshell it has always
		// been rather than a function with no body — measured, `( ); echo
		// ok` prints `ok` in the shell that has both readings, which is the
		// EmptyCompoundBody rule and not this one. The parentheses have been
		// consumed, so the node is built here rather than parsed again.
		return &Subshell{Start: fn.Start, Stop: p.tok.Pos}
	}
	for p.tok.Kind == TokWord && !p.atStopWord() {
		fn.Args = append(fn.Args, p.word())
	}
	return fn
}

// funcKeywordName reads one name after the `function` keyword and consumes it.
//
// The first name and every name after it go through this, so a definition
// that gives several cannot read its second name by a different rule from its
// first: whether a name may hold an expansion and whether any word at all is
// a name are the dialect's answers, and they are the same answers for every
// name in the list.
//
// ok is false where the word is not a name this dialect takes, and the caller
// raises the diagnostic — the token has not been consumed, so the failure is
// reported at the word that was refused.
func (p *Parser) funcKeywordName() (FuncName, bool) {
	if tokenHoldsAnExpansion(p.tok) {
		// A name that is not text until the shell runs. Where the dialect
		// expands one it is kept as a word; where it does not, it is refused
		// — this used to fall through to Literal(), which turns `_p_${w}`
		// into `_p_w` and defines a function nobody asked for, at status 0.
		if !p.dialect.FunctionNameExpands {
			return FuncName{}, false
		}
		n := FuncName{Name: p.tok.Literal()}
		n.Word = p.word()
		return n, true
	}
	if !p.keywordFuncName(p.tok) {
		return FuncName{}, false
	}
	n := FuncName{Name: p.tok.Literal()}
	p.next()
	return n, true
}

func (p *Parser) parseFuncKeyword() Command {
	fn := &FuncDecl{Keyword: true, Start: p.tok.Pos}
	p.next()
	if p.tok.Kind != TokWord {
		p.fail("expected a name after `function`")
		return fn
	}
	first, ok := p.funcKeywordName()
	switch {
	case ok:
		fn.Name, fn.NameWord = first.Name, first.Word
	case p.dialect.FunctionNameCheckedWhenTheDefinitionRuns:
		// The word is not a name, and this dialect says so where the
		// definition *runs* rather than here — so the declaration is read
		// whole and the word is kept as it was written, which is what the
		// complaint quotes. The name list below is not entered: no dialect
		// has both this and [Dialect.FunctionMultipleNames], and a list of
		// names one of which is refused is a shape no shell in the panel has.
		fn.RefusedName = p.textBetween(p.tok.Pos, p.tok.End)
		p.next()
	default:
		p.fail("expected a name after `function`")
		return fn
	}
	// Every word after the first is another name for the same body, where the
	// dialect has the list. Greedily, the way a loop's name list is taken:
	// the words stop at the body's `{`, at the `()` of the hybrid form, at a
	// stop word and at anything that is not a word at all, and a word that
	// opens a construct is a name like any other — see
	// [Dialect.FunctionMultipleNames], where the readings are measured.
	for p.dialect.FunctionMultipleNames &&
		p.at(TokWord) && !p.atStopWord() && !p.atWord("{") {
		also, ok := p.funcKeywordName()
		if !ok {
			p.fail("expected a name after `function`")
			return fn
		}
		fn.AlsoNamed = append(fn.AlsoNamed, also)
	}
	// And where the dialect takes words after the name that are *not* names
	// for the body, they are read and dropped. The list stops at the end of
	// the line, which is what makes the one-line spelling blame the line
	// after it — see [Dialect.FunctionKeywordReferenceList].
	for p.dialect.FunctionKeywordReferenceList && p.at(TokWord) &&
		!p.atReservedWord() {
		if !p.referenceListName(p.tok) {
			p.fail("invalid reference list")
			return fn
		}
		p.next()
	}
	if p.at(TokLeftParen) {
		// The hybrid `function f() {}`: bash and zsh take it, ksh93 rejects
		// it. Accepting it everywhere the keyword exists meant the ksh
		// dialect ran a definition ksh93 calls a syntax error.
		if !p.dialect.FunctionKeywordParens {
			p.failUnexpected("")
			return fn
		}
		p.next()
		if p.at(TokRightParen) {
			p.next()
		}
	}
	// Where the body is optional a separator may stand in front of it, and
	// the position the names ended at is kept: it is where an absent body is
	// recorded as being. See [Dialect.FunctionKeywordBodyIsOptional].
	afterNames := p.tok.Pos
	p.skipNewlines()
	if p.dialect.FunctionKeywordBodyIsOptional {
		for p.err == nil && p.at(TokSemi) {
			p.next()
			p.skipNewlines()
		}
	}
	body := p.tok
	p.funcBody = true
	if fn.Body = p.funcKeywordBody(); fn.Body == nil {
		if p.dialect.FunctionKeywordBodyIsOptional && p.err == nil {
			// No body at all, which is a declaration rather than a failure:
			// each name is defined with an empty one. An empty group is how
			// that is said, so nothing downstream has a nil body to read as
			// a refusal — and it is what the shell itself reports, printing
			// `a () { }` for a name declared this way.
			fn.Body = &Group{Start: afterNames, Stop: afterNames}
			return fn
		}
		p.failUnexpectedAt(body, "", false)
		return fn
	}
	if !p.funcKeywordBodyIsTakenHere(fn.Body) {
		p.failUnexpectedAt(body, "", false)
	}
	return fn
}

// funcKeywordBody reads the body of a `function` keyword's declaration.
//
// One dialect ends the declaration at a brace group's `}` and lets every
// other body reach to the end of the and-or list — see
// [Dialect.FunctionKeywordBodyIsAnAndOrList], where the order the two
// readings print in is measured. Everywhere else a body is one command, which
// is what [Parser.parseCommand] reads.
//
// A list of more than one pipeline is wrapped in a [Group], because
// [FuncDecl.Body] is a Command and an and-or list is not one. A single
// command is handed back bare: it is the body it was before this flag, and
// wrapping it would change what every existing definition prints back as.
func (p *Parser) funcKeywordBody() Command {
	if !p.dialect.FunctionKeywordBodyIsAnAndOrList || p.atWord("{") {
		return p.parseCommand()
	}
	expr := p.parseAndOr()
	if expr == nil {
		return nil
	}
	if pl, ok := expr.(*Pipeline); ok && !pl.Negated && len(pl.Cmds) == 1 {
		return pl.Cmds[0]
	}
	return &Group{
		List:  []*Stmt{{Expr: expr}},
		Start: expr.Pos(),
		Stop:  expr.End(),
	}
}

// funcKeywordBodyIsTakenHere reports whether the command read after the
// `function` keyword may stand as a body in this dialect.
//
// The question is the body's *shape* and it is asked here rather than in
// [Parser.parseFuncParensAndBody] because the two spellings of a definition
// are not one rule: the shell that wants a compound body after the keyword
// wants one after the parentheses too, and the shell that wants a brace group
// after the keyword takes a bare simple command after the parentheses. See
// [Dialect.FunctionKeywordBodyMustBeBraceGroup].
func (p *Parser) funcKeywordBodyIsTakenHere(body Command) bool {
	if p.dialect.FunctionKeywordBodyMustBeBraceGroup {
		_, isGroup := body.(*Group)
		return isGroup
	}
	if p.dialect.FuncBodyMustBeCompound {
		_, isSimple := body.(*SimpleCmd)
		return !isSimple
	}
	return true
}

func (p *Parser) parseSubshell() Command {
	c := &Subshell{Start: p.tok.Pos}
	defer p.opens("(")()
	p.next()
	c.List = p.parseBody()
	if !p.at(TokRightParen) {
		if p.at(TokEOF) {
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated(")")
			}
			return c
		}
		// Named the same way a brace group names it: the token that stopped
		// the list, and the closer that was wanted alongside it only where
		// the subshell had something in it. `( echo a; fi )` is
		// `"fi" unexpected (expecting ")")` in the one shell that prints an
		// expectation and `( fi )` is `"fi" unexpected` there, which is
		// measured — an empty subshell has nothing to be in the middle of.
		expected := ""
		if len(c.List) > 0 {
			expected = ")"
		}
		p.failUnexpected(expected)
		return c
	}
	c.Stop = p.tok.End
	p.next()
	return c
}

func (p *Parser) parseGroup(funcBody bool) Command {
	c := &Group{Start: p.tok.Pos}
	word := "{"
	if funcBody {
		word = "function"
	}
	defer p.opens(word)()
	p.next()
	c.List = p.parseBody()
	if !p.atWord("}") {
		if p.at(TokEOF) {
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated("}")
			}
			return c
		}
		// A reserved word the group cannot use — `{ echo a; do :; done; }`.
		// Every shell in the panel names the word it stopped on, so this goes
		// through the usual failure rather than describing the brace group:
		// a message that said only "expected }" named neither the token nor
		// the four different ways the shells say it.
		//
		// Whether the expectation is named alongside it depends on whether
		// the group had anything in it. dash writes `(expecting "}")` after
		// `{ echo a; esac; }` and not after `{ esac; }`, which is measured —
		// the empty group has nothing to be in the middle of.
		expected := ""
		if len(c.List) > 0 {
			expected = "}"
		}
		p.failUnexpected(expected)
		return c
	}
	c.Stop = p.tok.End
	p.next()
	return c
}

// parseGroupOrTry reads a brace group, and the `always` block after it where
// the dialect has one.
//
// The keyword is read *here* rather than from the command dispatch or from the
// stop-word set, and that placement is the production: `always` is an ordinary
// word everywhere else, so a shell with this construct still runs `always`,
// defines a function called it, and prints it as an argument. Measured — see
// [Dialect.TryAlways] for the shapes that are refused and why.
//
// A function body is not offered the keyword. `f() { :; } always { … }` is a
// parse error in the shell that has the construct, and funcBody is how the
// dispatch already says which brace group is a body; the loop bodies are
// refused by construction, since [Parser.braceLoopBody] reads its group
// directly rather than through the dispatch.
func (p *Parser) parseGroupOrTry(funcBody bool) Command {
	c := p.parseGroup(funcBody)
	if funcBody || !p.dialect.TryAlways || p.err != nil || !p.atWord("always") {
		return c
	}
	g, ok := c.(*Group)
	if !ok {
		return c
	}
	t := &TryClause{Try: g.List, Start: g.Start, Stop: g.Stop}
	p.next()
	if !p.atWord("{") {
		// The second half has to be a brace group. `{ echo t; } always echo
		// a` is a parse error on the `echo` in the shell that has the
		// construct rather than on the keyword, which is what naming the
		// token we actually stopped on gives.
		p.failUnexpected("")
		return t
	}
	always, ok := p.parseGroup(false).(*Group)
	if !ok {
		return t
	}
	t.Always, t.Stop = always.List, always.Stop
	return t
}

func (p *Parser) parseArithCmd() Command {
	c := &ArithCmdClause{Expr: p.tok.Text, Start: p.tok.Pos, Stop: p.tok.End}
	// The expression is not read as part of reading the file: every shell in
	// the panel that has this construct complains about it when the command
	// runs, so the raw text on Expr is what a diagnostic will quote and
	// Parsed is nil until then if it cannot be read at all (#865).
	c.Parsed = p.parseArithLater(p.tok.Text, p.tok.Pos)
	p.next()
	return c
}

// requireSep consumes the terminator a compound command needs before its
// keyword. The keyword does not delimit the condition; this does.
func (p *Parser) requireSep(before string) {
	switch p.tok.Kind {
	case TokSemi, TokNewline:
		p.next()
		p.skipNewlines()
	default:
		if p.at(TokEOF) {
			// `if` on its own: the input ran out before the construct could
			// be closed, which is a different failure from a word in the
			// wrong place and is reported as one.
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated(before)
			}
			return
		}
		if !p.atWord(before) {
			p.failUnexpected(before)
		}
	}
}

// peekIsArithCmd reports whether `((` follows, which is what tells a
// C-style `for` from one over a list. The lexer has already decided where the
// matching `))` is, so this only has to look.
func (p *Parser) peekIsArithCmd() bool {
	return p.lex.peekIsArithCommand()
}

// parseForArith reads `for ((init; cond; post))`.
//
// The three expressions arrive as one token — the lexer keeps `(( … ))` whole
// because what is inside is arithmetic and not a command list — so they are
// split here on the semicolons the arithmetic grammar has no use for.
//
// Specified in the C-style `for` section of docs/spec/grammar/commands.md,
// which also records the one shape this does not accept: the panel takes a
// brace group as the body where this requires `do … done`.
func (p *Parser) parseForArith(start Pos) Command {
	c := &ForArithClause{Start: start}
	p.next() // for
	text := p.tok.Text
	c.Header = "for ((" + text + "))"
	at := p.tok.Pos
	p.next()

	init, cond, post := splitForArith(text)
	c.InitText, c.CondText, c.PostText = init, cond, post
	// The deferred trees are built from the parts *as written* rather than
	// from the trimmed fields, so that an offset one of them records — the
	// `YStart` a division's blame is sliced from — indexes the same string
	// the interpreter later quotes back. Parsing the trimmed text and
	// quoting the untrimmed one moves every offset by the leading blanks,
	// which blamed `for (( i=1/0 ;; ))` on `/0 ` instead of `0 `.
	rawInit, rawCond, rawPost := c.PartsAsWritten()
	// The three parts defer too, and measured rather than assumed to: bash,
	// ksh93 and zsh all take `for ((echo hi;;))` under `-n` and all reach
	// past one in a branch that never runs. forArithPart already reads a
	// part from its text where there is no tree (#865).
	if init != "" {
		c.Init = p.parseArithLater(rawInit, at)
	}
	if cond != "" {
		c.Cond = p.parseArithLater(rawCond, at)
	}
	if post != "" {
		c.Post = p.parseArithLater(rawPost, at)
	}

	// This header ends itself, so the terminator before the body is optional
	// — and it is consumed here rather than by `requireSep` so that a brace
	// body can be looked for either side of it. `requireSep` still runs, and
	// still tells a loop that ran out of input from one that met the wrong
	// token.
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	// This header ends itself too, so where a body may be short it may also
	// be one command, or nothing.
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}

	p.requireSep("do")
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// braceBodyFollows reports whether a brace group stands where `do` belongs.
func (p *Parser) braceBodyFollows() bool {
	return p.dialect.ForBraceBody && p.atWord("{")
}

// braceLoopBody reads a brace group standing where `do … done` stands.
//
// The group is the ordinary one and keeps every rule it already has: the body
// needs a terminator before `}` wherever a brace group does, an empty body is
// refused wherever an empty group is, and a redirection after the closing
// brace belongs to the loop exactly as one after `done` does. Its list *is*
// the loop's body, so `break` and `continue` reach the loop rather than a
// group in the way.
func (p *Parser) braceLoopBody() (body []*Stmt, stop Pos) {
	g, ok := p.parseGroup(false).(*Group)
	if !ok {
		return nil, p.tok.End
	}
	return g.List, g.Stop
}

// shortFormBody reads a body written where `do … done` stands, for a header
// that has already ended: one command, or none at all.
//
// One rather than a list, and that is the construct rather than a
// simplification — a second command needs a separator before it, and a
// separator here belongs to whatever encloses the loop. `for i (a b) echo $i;
// echo done` prints a, b, done and not a, done, b, done.
//
// None at all is the same rule with nothing after the header, and it is
// reached far more often than it looks: it is what `while cond; { … }` means,
// because the `;` puts the brace group in the condition list and leaves the
// body with nothing.
func (p *Parser) shortFormBody() (body []*Stmt, stop Pos) {
	if p.braceBodyFollows() {
		return p.braceLoopBody()
	}
	if p.at(TokEOF) {
		// Nothing at all because the input ended, which is two different
		// facts wearing one shape. To a program that is all there is, the
		// loop is finished and its body is empty: `for i in 1 2` and a
		// newline is a whole command to `-c`, and it runs nothing. At a
		// prompt the identical text is a promise — the shell asks for
		// another line and takes it as the body — so the difference cannot
		// be in the parse. What the parser can say is that the input ran out
		// while it was still inside the construct, which is what Incomplete
		// means: a reader that can fetch another line does, and one that
		// cannot keeps the empty body it already has (#1298).
		p.ranOut()
		return nil, p.tok.Pos
	}
	if p.atStopWord() || p.at(TokRightParen) {
		// A stop word or a `)` is somebody else's, and it is *here*, so the
		// input did not run out: the body is empty and the construct is
		// finished. `while cond; { … }` is this, with the group taken as the
		// condition and `}` left standing where a body could have been.
		return nil, p.tok.Pos
	}
	st := p.parseStmt()
	if st == nil {
		return nil, p.tok.Pos
	}
	// The separator the body just took is the loop's as well: there is
	// nothing between the two commands but the one `;`, so a list may carry
	// on after the loop where it could not after a `done` or a `}`.
	p.bodyTookTerm = st.Semi
	return []*Stmt{st}, st.End()
}

// PartsAsWritten is the three parts with the blanks the script wrote around
// them, which the fields do not keep.
//
// [ForArithClause].InitText, CondText and PostText are trimmed because they
// are also what the printer lays a header out from, and untrimmed parts print
// back with their blanks doubled — see syntax/printroundtrip_test.go, whose
// promise is that printing a program gives the same program. A diagnostic
// wants the other answer: bash quotes a failing part back as it was written,
// so `for (( $x ;;))` with x=`echo hi` is `((: echo hi : …` there. So the
// spelling is re-split from [ForArithClause].Header, which already holds the
// header verbatim and which SameProgram already skips for being a spelling.
//
// Empty parts and a Header that is not this clause's both come back as the
// trimmed fields, so a tree built by hand answers as it always did.
func (c *ForArithClause) PartsAsWritten() (init, cond, post string) {
	text, ok := strings.CutPrefix(c.Header, "for ((")
	if text, ok2 := strings.CutSuffix(text, "))"); ok && ok2 {
		parts := strings.SplitN(text, ";", 3)
		if len(parts) == 3 {
			return parts[0], parts[1], parts[2]
		}
	}
	return c.InitText, c.CondText, c.PostText
}

// splitForArith cuts the header into its three parts.
//
// An omitted part is empty, and an omitted *condition* means true — which is
// what makes `for ((;;))` an endless loop rather than one that never runs.
func splitForArith(text string) (string, string, string) {
	parts := strings.SplitN(text, ";", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	// Trimmed, and that costs one blank in one diagnostic: bash quotes a
	// failing part back as it was written, so `for (( $x ;;))` with
	// x=`echo hi` is `((: echo hi : …` there and `((: echo hi: …` here. The
	// blanks are not kept because the tree is also what the printer reads,
	// and a header printed from untrimmed parts comes back with the blanks
	// doubled — see syntax/printroundtrip_test.go, whose promise is that
	// printing a program gives the same program.
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
}

// loopWord names the construct a loop opened with, which is what a
// diagnostic has to say back.
func loopWord(until bool) string {
	if until {
		return "until"
	}
	return "while"
}

func (p *Parser) parseIf() Command {
	c := &IfClause{Start: p.tok.Pos}
	defer p.opens("if")()
	p.next()
	c.Cond = p.parseCondition()
	// A condition that ended itself may be followed straight by the body,
	// exactly as a loop's header may — the rule ShortForm stands for, which
	// says nothing about looping. `if [[ -n x ]] { … }` and
	// `if (( 1 )) echo A` are that rule reaching `if`; `if true { … }` is
	// still refused, because `true` is a simple command and `{` is another
	// of its words.
	if body, stop, short := p.shortIf(c.Cond); short {
		c.Then, c.Stop = body, stop
		p.shortElse(c)
		return c
	}
	p.requireSep("then")
	// Recorded after it is consumed: until then the innermost thing awaiting
	// a partner is the `if` itself, which is what one shell names for
	// `if true` and not for `if true; then echo x`.
	p.opensClause("then")
	p.expectWord("then")
	c.Then = p.parseBody()

	for p.atWord("elif") && p.err == nil {
		e := &Elif{Start: p.tok.Pos}
		p.opensClause("elif")
		p.next()
		e.Cond = p.parseCondition()
		if body, stop, short := p.shortIf(e.Cond); short {
			// The two spellings compose: a long `if … ; then …` may carry an
			// `elif` whose condition ended itself and whose body is written
			// with braces. Measured on zsh 5.9.2 — `if (( 0 )); then echo A;
			// elif (( 1 )) { echo B }; echo tail` prints `B` then `tail`
			// there, and `elif [[ -n y ]] { … }` does the same while
			// `elif : { … }` is refused, which is shortIf's own test showing
			// through. From here the chain is a short one and there is no
			// `fi` left to read, so the rest of it is shortElse's (#1880).
			e.Then, c.Stop = body, stop
			c.Elifs = append(c.Elifs, e)
			p.shortElse(c)
			return c
		}
		p.requireSep("then")
		p.expectWord("then")
		e.Then = p.parseBody()
		c.Elifs = append(c.Elifs, e)
	}
	if p.atWord("else") {
		p.opensClause("else")
		p.next()
		c.HasElse = true
		c.Else = p.parseBody()
	}
	c.Stop = p.tok.End
	p.expectWord("fi")
	return c
}

// shortIf reads the body of an `if` whose condition ended itself, where the
// dialect allows one without `then … fi`.
//
// It reports false wherever the long form still applies, so a condition that
// did not end itself, a dialect without the flag, and a `then` standing where
// it always could all fall through untouched. There is no separator to
// consume first, and that is the measurement rather than a simplification:
// `if [[ -z x ]] echo A; else echo B; fi` is an error in the shell that has
// this, because the `;` ended the whole command and left `else` with nothing
// to attach to.
func (p *Parser) shortIf(cond []*Stmt) (body []*Stmt, stop Pos, short bool) {
	if !p.dialect.ShortForm || !condEndedItself(cond) {
		return nil, Pos{}, false
	}
	// `then` needs no test of its own: it is a stop word, so the long form
	// falls through here with everything else the grammar could still want.
	// Naming it as well was a line no test could distinguish.
	if p.at(TokEOF) || p.atStopWord() {
		return nil, Pos{}, false
	}
	body, stop = p.shortFormBody()
	return body, stop, true
}

// shortElse reads the `elif` and `else` arms of a short `if`, which take the
// same body by the same rule and end where it ends: there is no `fi`.
func (p *Parser) shortElse(c *IfClause) {
	for p.atWord("elif") && p.err == nil {
		e := &Elif{Start: p.tok.Pos}
		p.opensClause("elif")
		p.next()
		e.Cond = p.parseCondition()
		if !condEndedItself(e.Cond) {
			p.failUnexpected("")
			return
		}
		e.Then, c.Stop = p.shortFormBody()
		c.Elifs = append(c.Elifs, e)
	}
	if p.atWord("else") && p.err == nil {
		p.opensClause("else")
		p.next()
		c.HasElse = true
		c.Else, c.Stop = p.shortFormBody()
	}
}

// condEndedItself reports whether the condition just parsed closed on its own
// — `(( … ))` and `[[ … ]]` do, a word does not.
//
// The list is what says so rather than the token that follows it: the parser
// stopped where it stopped because nothing could continue the condition, and
// the only lists that can be followed straight by a body are the ones whose
// last command was a construct with its own end. A simple command swallows
// what comes after it as another word, which is why `if true { … }` is a
// syntax error in the shell that takes `if [[ -n x ]] { … }`.
func condEndedItself(cond []*Stmt) bool {
	if len(cond) == 0 {
		return false
	}
	// Nothing here tests for a separator, and it looked as though something
	// should: `if [[ -n x ]]; { echo A }` is an error where the same line
	// without the `;` runs. It cannot reach here. A separator keeps the
	// condition *list* going, so whatever follows becomes another statement
	// of it and is the one this looks at — a brace group, which does not end
	// a header — and a separator with nothing after it leaves the parser at
	// end of input or at a stop word, which shortIf refuses before asking.
	// A guard for it would be a line no test could distinguish, which is how
	// it was found: removing it changed nothing anywhere.
	return exprEndsItself(cond[len(cond)-1].Expr)
}

// exprEndsItself walks to the command the condition finished on: an and-or
// list finishes on its right side and a pipeline on its last command.
//
// A pipeline was recorded as never ending itself, from `if [[ -n x ]] | cat
// { … }` being a syntax error — but that is `cat` failing to end it, not the
// pipe. `if true | [[ -n x ]] { … }` runs, which is the row that tells the
// two apart, and it was found by mutation rather than by a script.
func exprEndsItself(e Expr) bool {
	switch x := e.(type) {
	case *BinaryExpr:
		return exprEndsItself(x.Y)
	case *Pipeline:
		return commandEndsItself(x.Cmds[len(x.Cmds)-1])
	}
	return false
}

// commandEndsItself is the whole of the rule: `(( … ))` and `[[ … ]]` close,
// and a word does not.
//
// The set is what was measured rather than what is tidy. A group, a subshell
// and a `case … esac` close too, and a *loop* does not — `if for i in a; do
// true; done { … }` is a syntax error in the shell that takes every other
// row, which is a fact about that shell rather than a rule anyone could
// derive. A pipeline of more than one command is refused by exprEndsItself
// above, for the same measured reason.
//
// A try-always block closes as well, measured 2026-09-07: `if { true; } always
// { :; } { echo A; }; echo after` prints A and after in zsh 5.9.2, where
// leaving the construct out of the set would call the body's `{` a syntax
// error. The trailing statement is not decoration — a command string whose
// last byte is the `}` of a short body is a parse error there whatever
// precedes it, so a probe without one cannot tell the readings apart.
func commandEndsItself(c Command) bool {
	switch c.(type) {
	case *TestClause, *ArithCmdClause, *Group, *Subshell, *CaseClause, *TryClause:
		return true
	}
	return false
}

func (p *Parser) parseLoop() Command {
	c := &LoopClause{Until: p.atWord("until"), Start: p.tok.Pos}
	defer p.opens(loopWord(c.Until))()
	p.next()
	p.inCondition = true
	c.Cond = p.requireBody(p.parseList())
	p.inCondition = false
	// Where the body may be short, the condition list is the whole header and
	// it has just ended: what stands here is either `do`, or the body, or
	// nothing. The list is what decides — a `;` kept it going, so anything
	// after one was tested rather than run.
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.requireSep("do")
	// A `do` inside a while or until is a keyword awaiting its own partner,
	// and inside a `for` it is not — measured, in the one shell whose wording
	// can tell: `while true; do echo x` is "`do' unmatched" there and
	// `for i in a; do echo x` is "`for' unmatched". An irregularity in that
	// shell rather than in this one, recorded rather than smoothed over.
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseFor() Command {
	start := p.tok.Pos
	defer p.opens("for")()
	if p.dialect.CStyleFor && p.peekIsArithCmd() {
		return p.parseForArith(start)
	}
	c := &ForClause{Start: start}
	p.next()
	name, refused, ok := p.forName("for")
	if !ok {
		return c
	}
	if refused != "" {
		c.RefusedName = refused
	} else {
		c.Names = []string{name}
	}
	nameEnd := p.tok.End
	p.next()
	nameEnd = p.moreForNames(c, nameEnd, "for")
	p.skipNewlines()

	end := nameEnd
	// An absent word list is not an empty one: without `in` the loop iterates
	// the positional parameters, and with `in` and nothing after it, nothing.
	// The parenthesized spelling is the short loop's and says the same thing
	// as `in`, with the closing paren ending the header where `in` needs a
	// separator to.
	parens := p.shortItemsFollow()
	switch {
	case parens:
		c.HasItems = true
		c.Items, end = p.shortItems()
	case p.atWord("in"):
		c.HasItems = true
		end = p.itemList(&c.Items, end)
	}
	c.Header = p.slice(c.Start, end)
	if body, stop, short := p.shortBodyAfterHeader(parens || !c.HasItems); short {
		c.Body, c.Stop = body, stop
		return c
	}
	p.requireSep("do")
	// A brace group where `do … done` stands. The separator `requireSep` has
	// just consumed is what makes the form reachable at all: with nothing
	// between, `{` is another *item* of the list, and the loop then meets `}`
	// where `do` belongs — which is why `for i in a b { … }` is refused by
	// every shell that accepts `for i in a b; { … }`.
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// itemList reads the word list after `in` — for a `for` loop, for `select`
// and for `foreach` alike — and returns where the last word ends.
//
// **No word in the list is a reserved word.** The list ends at a `;` or a
// newline and at nothing else, which is unanimous across the panel. Measured
// 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch HOME and `-n` over a
// script file:
//
//	for x in a b do :; done       refused by all five — `do` is a *word*
//	                              there, so `done` stands where `do` belongs
//	for x in do; do :; done        taken by all five
//	for x in done; do :; done      taken by all five
//	select x in a b do :; done     refused by all five
//	foreach x in a b end           taken by the shell that has the loop
//
// This read `do`, `done` and the rest of the reserved words as stop words
// here, which got both directions wrong at once: it accepted the first line,
// where every shell refuses it, and refused the next two, which every shell
// takes. A list holding the word `done` is not exotic — `for f in $(ls)`
// reaches it the moment a file is called that — and the accepted-malformed
// half is the worse one, because the body then runs with `do` bound as a
// value and nothing is said.
//
// One reader for all three loops, because the three headers are the same
// grammar and each had its own copy of it (#1161).
func (p *Parser) itemList(items *[]*Word, end Pos) Pos {
	// Set before the `p.next()` that reads the first word, because that is
	// the token the flag has to reach. Each word stands where an argument
	// does, so a `(` that begins one belongs to it in the dialect that reads
	// glob qualifiers — the answer an array literal's element already gets
	// (#1149). Restored rather than cleared: a loop header is read at
	// command position, and the token after the list is not an argument
	// either way.
	saved := p.lex.inArgument
	p.lex.inArgument = true
	p.next()
	for p.tok.Kind == TokWord && p.err == nil {
		*items = append(*items, p.word())
	}
	p.lex.inArgument = saved
	if n := len(*items); n > 0 {
		end = (*items)[n-1].End()
	}
	return end
}

// forName reads the word standing where a loop's variable belongs and reports
// whether it may be one, recording the refusal where it may not.
//
// **A name may not come out of an expansion, and that is unanimous.** `n=x;
// for $n in a b` is refused by bash 5.3.15, the same binary as `sh`, bash
// 3.2.57, dash, ksh93u+ and zsh 5.9.2 alike — measured 2026-09-06, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, from a script file and through `-c`.
// So is `${n}`, `"$n"` and `$(echo n)`. The loop would otherwise bind a
// variable literally called `n`, the script's own `$n` would read the list's
// words, and the name it meant to reach through the expansion would stay
// empty — at status 0 and with nothing said (#1076).
//
// The token's literal cannot see it: a `$n` word reports `n`, so [isName] is
// satisfied by a word that names nothing yet. What tells them apart is the
// *spans* — a substitution is a span of its own kind — and asking that way
// rather than by comparing the source text is what keeps the two halves below
// separate, because quoting changes the source text too.
//
// **Whether the word may be quoted at all is an axis**, [Dialect.ForNameMayBeQuoted]:
// ksh93 removes the quoting and takes the name, where bash, dash and zsh want
// the name written plainly. Measured on `for "i"`, `for 'i'`, `for i""`,
// `for "i"x` and `for \i` — ksh93 runs all five and the other three refuse all
// five, so the escape travels with the quotes rather than being its own
// question.
//
// The name is reported **as written**, quotes, escapes, expansion and all:
// three of the four dialects quote the source text back and none of them
// quotes the literal. `for $n` names `$n` and not `n`.
func (p *Parser) forName(word string) (name, refused string, ok bool) {
	if p.forNameIsUsable() {
		return p.tok.Literal(), "", true
	}
	if p.dialect.ForNameCheckedWhenTheLoopRuns && p.tok.Kind == TokWord {
		// The parse succeeds and the word is carried to the interpreter,
		// which is the whole of #1110: `bash -n` accepts this, so refusing
		// it here reported a working script as broken.
		//
		// A *word* only. `for ; in a b` is an ordinary unexpected-token
		// failure in both shells that get here — status 2 and 3, with bash
		// echoing the line — because what they want in that position is a
		// word, and the name is checked afterwards. So the two questions
		// stay apart: whether a word may stand there is the grammar's, and
		// whether the word is a name is the loop's.
		return "", p.forNameAsWritten(), true
	}
	if p.tok.Kind != TokWord && !p.dialect.ForNonWordIsANameError {
		// Nothing that could be a name stands here at all, and the grammar
		// gets to complain about the token before the loop gets to complain
		// about the name. `for`, a newline after `for`, `for ;` — three of
		// the four dialects call each of those what they call the same token
		// anywhere else, so the failure is an ordinary one and the expected
		// word is the one the header is heading for.
		//
		// It reached the name check with the end of input in hand instead,
		// which named `end of input` as though a script had written it and
		// carried ForNameStatus rather than the dialect's parse-error status
		// — 1 where bash answers 2 and ksh93 answers 3 (#1319).
		//
		// The end of the input is a token like any other here, and goes
		// through failUnexpected for the same reason every other construct's
		// does: it is that call which knows an unterminated construct from an
		// unwanted token, and the two are worded differently by every shell
		// in the panel.
		//
		// Except where the input's end *is* a newline, which is one dialect's
		// answer rather than a third classification: the token it names is the
		// newline it appended, at the position the text ran out. Built here
		// rather than lexed, because nothing else in the grammar could see it
		// — every other position takes a newline and would consume this one.
		if p.tok.Kind == TokEOF && p.dialect.ForNameEndOfInputIsANewline {
			p.failUnexpectedAt(Token{Kind: TokNewline, Pos: p.tok.Pos}, "do", false)
			return "", "", false
		}
		p.failUnexpected("do")
		return "", "", false
	}
	if p.err == nil {
		p.err = &Error{
			Pos: p.tok.Pos, Kind: ErrForName,
			Token: p.forNameAsWritten(), Class: p.tokenClass(false),
			Msg: "expected a name after `" + word + "`",
		}
	}
	return "", "", false
}

// forNameIsUsable is the predicate.
//
// A token that is not a word needs no test of its own: only a word has spans,
// so its literal is empty and [isName] refuses it. A guard for it here read as
// if it decided something and did not — the mutant that removed it survived,
// which is the whole of the evidence that it was dead.
//
// What a non-word in that position *should* say is a separate answer and not a
// second predicate. `for ; in a b` is an ordinary unexpected-token failure in
// bash and ksh93 — status 2 and 3, with bash echoing the line — because those
// two want only a word there and check the name when the loop runs; dash gives
// it the same one sentence it gives every bad loop variable, and zsh's two
// wordings coincide. That is the stage question of #1110 showing through, and
// modeling it as its own axis would be modeling the symptom.
func (p *Parser) forNameIsUsable() bool {
	for _, sp := range p.tok.Spans {
		if sp.Kind != Literal {
			// A name out of a substitution: refused everywhere.
			return false
		}
	}
	if !p.dialect.ForNameMayBeQuoted && p.forNameAsWritten() != p.tok.Literal() {
		// Written with quotes or an escape in it, in a dialect that wants
		// the name plain. Comparing the source text with the literal is what
		// says so for both spellings at once.
		return false
	}
	return isName(p.tok.Literal())
}

// forNameAsWritten is the word's source text, which is what a diagnostic
// quotes and what the plainness check above compares against.
func (p *Parser) forNameAsWritten() string {
	if p.tok.Kind != TokWord {
		return p.tokenLiteral()
	}
	return p.slice(p.tok.Pos, p.tok.End)
}

// moreForNames reads the names after the first, where the dialect lets a loop
// have more than one. It returns where the last of them ends.
//
// **Greedy, and that is the whole of the ambiguity.** Every word after the
// first is another name until the header ends — at `in`, at `(`, at a
// separator, at `do`, or at `{` — so nothing else may stand there. Measured
// 2026-09-06 in zsh 5.9.2, the only shell with the form:
// `set -- p q; for a print -r -- "[$a]"` is a parse error near `-r`, because
// `print` was taken as a second name and `-r` is not a name; and
// `set -- p q; for a echo; print "[$a][$echo]"` prints `[p][q]`, which is the
// same reading seen from the side where it succeeds — `echo` was read as a
// name and bound to `q`.
//
// So the flag narrows what the dialect accepts as well as widening it: a short
// body may no longer follow the names *directly*. It still follows a header
// that ended itself, which is every spelling anyone writes —
// `for a b ( 1 2 ) print "$a$b"` and `for a b; print "$a$b"` both run.
//
// A name is a plain unquoted name and is not expanded: `for a "b" ( … )` and
// `for a $n ( … )` are parse errors in zsh, so anything that is not one is
// refused where it stands rather than quietly becoming a body.
//
// The source is compared with the token's literal to say so, because the
// literal alone cannot: a `$n` word reports `n`, so `isName` is satisfied by a
// word that names nothing yet. The *first* name has the same hole and it is
// not closed here — `for $n in a b` is refused by all five shells in the panel
// and accepted by every dialect of ours, which is a core bug of its own with
// five wordings to get right (#1076).
//
// `select` never calls this. Measured, `select a b (x y) { … }` is a parse
// error in the shell that accepts every other spelling here, so the loop whose
// header is otherwise a for-loop's parts company over exactly this.
func (p *Parser) moreForNames(c *ForClause, end Pos, word string) Pos {
	if !p.dialect.ForMultipleNames {
		return end
	}
	for p.tok.Kind == TokWord && !p.atStopWord() && !p.atWord("in") && !p.atWord("{") {
		name, refused, ok := p.forName(word)
		if !ok {
			return end
		}
		if refused != "" {
			// The **first** refused word is the one kept, because that is
			// the one a complaint will quote and a later bad word would
			// otherwise displace it silently.
			//
			// No shell in the panel is both of these — multiple names are
			// zsh's and zsh refuses the word while parsing — so the answer
			// is this repository's own rather than a measurement, and it is
			// written down in a test against a dialect assembled for it. A
			// mutant that let the later word win survived every row until
			// that test existed.
			if c.RefusedName == "" {
				c.RefusedName = refused
			}
			end = p.tok.End
			p.next()
			continue
		}
		c.Names = append(c.Names, name)
		end = p.tok.End
		p.next()
	}
	return end
}

// shortItemsFollow reports whether a `for` or `select` header's word list is
// written in parentheses.
func (p *Parser) shortItemsFollow() bool {
	return p.dialect.ShortForm && p.at(TokLeftParen)
}

// shortItems reads that list. The words are the ordinary ones — expanded,
// split and globbed like the words after `in` — and an empty list is legal
// and iterates nothing.
func (p *Parser) shortItems() (items []*Word, end Pos) {
	// The same two answers the `in` list gets, and for the same reasons: no
	// word in the list is a reserved word — `for x (do)`, `for x (done)` and
	// `for x (a do)` are all taken by the shell that has the form — and each
	// word stands where an argument does, so `for x ((#i)a)` reads the flag.
	// Measured 2026-09-06 on zsh 5.9.2; see itemList, which cannot be shared
	// here because this list is closed by a paren rather than by a separator.
	saved := p.lex.inArgument
	p.lex.inArgument = true
	p.next() // (
	p.skipNewlines()
	for p.tok.Kind == TokWord && p.err == nil {
		items = append(items, p.word())
		p.skipNewlines()
	}
	p.lex.inArgument = saved
	end = p.tok.End
	if !p.at(TokRightParen) {
		p.failUnexpected(")")
		return items, end
	}
	p.next()
	return items, end
}

// shortBodyAfterHeader reads the body of a `for` or `select` whose header
// ended itself, where the dialect allows one without `do … done`.
//
// The separator is optional there rather than required, which is the whole
// difference from the long form: `for i (a b) echo $i` and `for i (a b); echo
// $i` are the same loop. It reports false where the long form still applies,
// so a header that did not end itself, a dialect without the flag, and a `do`
// standing where it always could all fall through untouched.
func (p *Parser) shortBodyAfterHeader(headerEnded bool) (body []*Stmt, stop Pos, short bool) {
	if !p.dialect.ShortForm || !headerEnded {
		return nil, Pos{}, false
	}
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.atWord("do") {
		return nil, Pos{}, false
	}
	body, stop = p.shortFormBody()
	return body, stop, true
}

// parseRepeat reads `repeat N` and its body.
//
// The header is one word and ends itself, so every body spelling the dialect
// has is reachable: `do … done`, a brace group, one command, and — with a
// separator between, which a `while` reads as more condition and this reads
// as nothing at all — the same three again.
func (p *Parser) parseRepeat() Command {
	c := &RepeatClause{Start: p.tok.Pos}
	defer p.opens("repeat")()
	p.next()
	c.Count = p.word()
	if c.Count == nil {
		p.failUnexpectedOperand("a count")
		return c
	}
	c.Header = p.slice(c.Start, c.Count.End())
	// A separator here is optional and belongs to the header rather than to
	// a condition list, because there is no condition: `repeat 2; echo R`
	// prints twice, where `while cond; echo R` would have tested the echo.
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// parseForeach reads `foreach name (a b) … end`, which is a `for` under two
// other words.
//
// The tree is a ForClause, because the construct is one: the same loop
// variable, the same list, the same body, and `break` and `continue` reaching
// the same place. Only the words differ, and a printer that writes it back as
// `for … do … done` writes something every dialect can read.
func (p *Parser) parseForeach() Command {
	c := &ForClause{Start: p.tok.Pos}
	defer p.opens("foreach")()
	p.next()
	name, refused, ok := p.forName("foreach")
	if !ok {
		return c
	}
	if refused != "" {
		c.RefusedName = refused
	} else {
		c.Names = []string{name}
	}
	end := p.tok.End
	p.next()
	end = p.moreForNames(c, end, "foreach")
	p.skipNewlines()
	switch {
	case p.at(TokLeftParen):
		c.HasItems = true
		c.Items, end = p.shortItems()
	case p.atWord("in"):
		c.HasItems = true
		end = p.itemList(&c.Items, end)
	}
	c.Header = p.slice(c.Start, end)
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("end")
	return c
}

// parseSelect reads the menu loop, whose header is a for-loop's.
func (p *Parser) parseSelect() Command {
	c := &SelectClause{Start: p.tok.Pos}
	defer p.opens("select")()
	p.next()
	name, refused, ok := p.forName("select")
	if !ok {
		return c
	}
	if refused != "" {
		c.RefusedName = refused
	} else {
		c.Name = name
	}
	nameEnd := p.tok.End
	p.next()
	p.skipNewlines()
	end := nameEnd
	// The menu loop's header is a for-loop's, parenthesized list included.
	parens := p.shortItemsFollow()
	switch {
	case parens:
		c.HasItems = true
		c.Items, end = p.shortItems()
	case p.atWord("in"):
		c.HasItems = true
		end = p.itemList(&c.Items, end)
	}
	c.Header = p.slice(c.Start, end)
	if body, stop, short := p.shortBodyAfterHeader(parens || !c.HasItems); short {
		c.Body, c.Stop = body, stop
		return c
	}
	p.requireSep("do")
	// The menu loop takes the brace body its header's loop takes.
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseCase() Command {
	c := &CaseClause{Start: p.tok.Pos}
	defer p.opens("case")()
	// The subject stands at what is otherwise command position and is not an
	// assignment: measured, `case a==(echo hi) in *)` matches with no
	// command run and no file made, in the dialect where a bare
	// `a==(echo hi)` assigns a path. Told to the
	// lexer before the `p.next()` that reads it, for the reason inCaseArm is
	// below. See Lexer.noAssignment.
	p.lex.noAssignment = true
	p.next()
	p.lex.noAssignment = false
	if c.Word = p.word(); c.Word == nil {
		p.fail("expected a word after `case`")
		return c
	}
	p.skipNewlines()
	inEnd := p.tok.End
	// An arm begins where no command may, so `((` there is the arm's own
	// paren in front of a group rather than an arithmetic command, and a
	// leading `(` may belong to the pattern. Told to the lexer before the
	// token is read — and `expectWord` is what reads it, the first arm's
	// first token arriving with the `in` — and taken back before the arm's
	// *body* is read, a body being ordinary commands in which `((1))` really
	// is an expression. See Lexer.inCaseArm.
	p.lex.inCaseArm = true
	defer func() { p.lex.inCaseArm = false }()
	if p.dialect.CaseBraceBody && p.atWord("{") {
		// The brace spelling of the same header. The two halves are
		// independent — `case x { … esac` and `case x in … }` both run in
		// the shell that has this — so the closer is not remembered from
		// here. See Dialect.CaseBraceBody.
		p.next()
	} else {
		p.expectWord("in")
	}
	c.Header = p.slice(c.Start, inEnd)
	// Where the dialect says so, an `esac` standing here — after the `in`,
	// before any newline — is the first arm's first pattern and not the
	// terminator. A newline takes the reading away again, so this is asked
	// before the newlines are skipped and cleared if any were.
	// See Dialect.CaseTerminatorIsAPatternAfterIn.
	esacIsAPattern := p.dialect.CaseTerminatorIsAPatternAfterIn && !p.at(TokNewline)
	p.skipNewlines()

	for p.err == nil && !p.at(TokEOF) && (esacIsAPattern || !p.atCaseEnd()) {
		esacIsAPattern = false
		p.lex.inCaseArm = false
		it := &CaseItem{Start: p.tok.Pos}
		// A pattern may carry a leading open paren. Where the paren opened
		// the *pattern* instead the lexer has already folded it into the
		// word, so this sees no paren at all and the pattern arrives whole.
		//
		// Each pattern stands where an argument does either way, so a `(`
		// beginning one belongs to it — `case x in ((a|b))` is the arm's
		// paren and then a group, and `case x in (a|b)|(c|d))` is two
		// groups and no arm paren. Set unconditionally rather than only on
		// the paren branch: the *second* alternative of a list is read by
		// `casePatterns` whichever way the first one arrived, and while
		// this was inside the branch a list whose first pattern was a group
		// left the rest of it in command position, where `(` is an operator
		// and `(a|b)|(c|d))` was `parse error near `(''`.
		saved := p.lex.inArgument
		savedList := p.lex.inCaseParenList
		p.lex.inArgument = true
		parenthesized := false
		if p.at(TokLeftParen) {
			if p.emptyParensStartAt(p.tok) {
				// `()` is one token to this dialect, so the `(` never opens
				// an arm: the pair is a word the grammar cannot take here
				// and the refusal names both characters. `( )` — the same
				// two with a blank between them — is an arm with an empty
				// pattern list and parses, which is what says the refusal
				// is the token's and not the emptiness's (#1111).
				p.lex.inArgument, p.lex.inCaseParenList = saved, savedList
				p.failUnexpected("")
				return c
			}
			// The paren is what puts one dialect's newline inside the
			// pattern rather than ending it. Only here: an arm written
			// *without* the paren is a parse error there too, so this is the
			// parenthesis's rule and not the position's. Set before the
			// `p.next()` that reads the first pattern, which is the token it
			// has to reach.
			p.lex.inCaseParenList = true
			parenthesized = true
			p.next()
		}
		if !p.casePatterns(it, parenthesized) {
			p.lex.inArgument, p.lex.inCaseParenList = saved, savedList
			return c
		}
		p.lex.inArgument = saved
		if !p.at(TokRightParen) {
			p.lex.inCaseParenList = savedList
			p.failUnexpectedOperand(")")
			return c
		}
		// Cleared before the read that follows, which is the arm's *body* —
		// ordinary commands, where a newline is a statement separator again.
		p.lex.inCaseParenList = savedList
		p.next()
		it.Body = p.parseList()

		switch p.tok.Kind {
		case TokDSemi, TokSemiAmp, TokDSemiAmp, TokSemiPipe:
			it.Term, it.TermPos = p.tok.Kind, p.tok.Pos
			// The terminator's own `p.next()` reads the *next arm's* first
			// token, so the flag goes back on in front of it.
			p.lex.inCaseArm = true
			p.next()
		default:
			// The last arm may omit its terminator before `esac`.
			it.TermPos = p.tok.Pos
			if !p.atCaseEnd() {
				if p.at(TokEOF) {
					// The panel expects `;;` here rather than `esac`: an arm
					// that has not been closed is what ran out, not the case.
					p.ranOut()
					if p.err == nil {
						p.err = p.unterminated(";;")
					}
					return c
				}
				// No expectation named: either `;;` or `esac` would be
				// valid here, so naming one of them would be inventing a
				// grammar the parser does not have — and the one dialect
				// that prints expectations does not print one here either.
				p.failUnexpected("")
				return c
			}
		}
		c.Items = append(c.Items, it)
		p.lex.inCaseArm = true
		p.skipNewlines()
	}
	p.lex.inCaseArm = false
	c.Stop = p.tok.End
	if p.dialect.CaseBraceBody && p.atWord("}") {
		p.next()
	} else {
		p.expectWord("esac")
	}
	return c
}

// atCaseEnd reports whether the current token closes a `case`.
//
// Two words can, and only in the dialect that writes the brace spelling of
// the header. They are asked together rather than paired with the opener
// because the shell that has them does not pair them either: `case x { …
// esac` and `case x in … }` both run there. See Dialect.CaseBraceBody.
func (p *Parser) atCaseEnd() bool {
	return p.atWord("esac") || (p.dialect.CaseBraceBody && p.atWord("}"))
}

// casePatterns reads one arm's pattern list, `a | b | c`, and reports whether
// it got one. The caller has already consumed any open paren and stops at the
// close paren, which this does not read.
//
// Where the dialect allows it a pattern may be written as nothing, and the
// emptiness is read off the *separator* rather than off the position: a `|`
// with no word before it, after it, or on either side stands for a pattern
// that matches only the empty string. `(|a|b)` is the idiom for "one of these
// or none" and is what a real zsh library's own startup path is written with.
//
// The list may also be written as nothing at all, where the arm's parentheses
// are there to hold it: `( )` matches only the empty string in that dialect,
// measured 2026-09-12. `()` is a parse error there and this is why — the pair
// with no blank between is one token to that shell's lexer, so it never
// reaches the production at all (#1111). Without the parentheses there is
// nowhere for an empty list to be written and `case a in ) …` is refused.
func (p *Parser) casePatterns(it *CaseItem, parenthesized bool) bool {
	empty := p.dialect.CasePatternMayBeEmpty
	if empty && parenthesized && p.at(TokRightParen) {
		it.Patterns = append(it.Patterns, p.emptyPattern())
		return true
	}
	for {
		switch {
		case empty && (p.at(TokPipe) || p.at(TokOrOr)):
			it.Patterns = append(it.Patterns, p.emptyPattern())
		case p.dialect.CasePatternAcceptsOperator && !p.at(TokWord) && !p.at(TokEOF):
			// One operator may stand where a pattern belongs, and it
			// contributes no pattern at all — not even an empty one, which
			// is what separates this from the flag above. The two are never
			// set together, and the empty alternative is asked first because
			// its `|` is an operator too.
			p.next()
		default:
			w := p.word()
			if w == nil {
				p.failUnexpected("")
				return false
			}
			it.Patterns = append(it.Patterns, w)
		}
		switch {
		case p.at(TokPipe):
			p.next()
		case empty && p.at(TokOrOr):
			// `||` is two separators with a pattern of nothing between,
			// and it arrives as one token because the lexer reads the
			// operator before anything has said this is a pattern list.
			it.Patterns = append(it.Patterns, p.emptyPattern())
			p.next()
		default:
			return true
		}
		if empty && p.at(TokRightParen) {
			it.Patterns = append(it.Patterns, p.emptyPattern())
			return true
		}
	}
}

// emptyPattern is the pattern written as nothing between two separators. It
// has no spans, so it expands to the empty string and matches only that.
func (p *Parser) emptyPattern() *Word {
	return &Word{Start: p.tok.Pos, Stop: p.tok.Pos}
}

// ParseParamExpFor parses the inside of a `${ }` that was captured outside the
// normal word path — a here-document body, which is read as raw text and
// expanded only when the command runs.
//
// The body is double-quoted context, which is what decides how the operand of
// a `${x:-word}` in it reads: `cat <<EOF` with `${u:-'$v'}` in the body prints
// `'VAL'` in every shell in the panel — the quotes two characters of the
// output and the `$v` between them still substituted, exactly as inside a
// pair of double quotes.
func (p *Parser) ParseParamExpFor(src string, at Pos) *ParamExpr {
	return p.parseParamExp(src, at, DoubleQuoted, false)
}

// ParseReference parses text that names a parameter and perhaps one of its
// elements — `x`, `x[@]`, `s[2]`, `a[(r)q]` — into the node every reading of
// a subscript already answers.
//
// It is the shape a *value* takes where a construct reads one as a reference
// at run time rather than at the parse: a `(P)` group's resolved text, and
// `unset`'s operand. Those arrive as strings with the subscript unlexed, so
// the group has nowhere to hang its operand and a range has no two ends —
// which is why the answer was a name plus one arithmetic index and nothing
// else.
//
// Unquoted, unlike ParseParamExpFor: a here-document body is inside quotes
// and a resolved value is not.
func (p *Parser) ParseReference(src string, at Pos) *ParamExpr {
	return p.parseParamExp(src, at, Unquoted, false)
}

// ParseArithFor is ParseParamExpFor for `$(( ))`.
func (p *Parser) ParseArithFor(src string, at Pos) ArithExpr {
	return p.parseArith(src, at)
}
