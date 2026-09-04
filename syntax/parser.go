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

	// pending are tokens an alias expansion put in front of the lexer, and
	// aliasNextWord says the last expansion ended in a space, so the word
	// after it is eligible in turn.
	pending       []Token
	aliasNextWord bool
	// aliasSpliced counts the tokens of the current expansion still in hand,
	// so the trailing-space rule can tell a word that *came from* the value
	// from the word that follows it.
	aliasSpliced int

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
}

// maxParamDepth is how far `${x:-${y:-…}}` may nest before the parser stops.
// Deep enough that no real script reaches it, shallow enough that no input
// can exhaust the stack.
const maxParamDepth = 64

// NewParser returns a parser over src.
func NewParser(src string, d Dialect) *Parser {
	p := &Parser{lex: NewLexer(src, d), dialect: d}
	p.next()
	return p
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

// slice returns the source between two positions, which is how a node keeps
// the text it was written as. A diagnostic quotes what the author typed, and
// no reconstruction from the tree can be relied on to match it.
func (p *Parser) slice(from, to Pos) string {
	if from.Offset < 0 || to.Offset > len(p.lex.src) || from.Offset > to.Offset {
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
		return
	}
	p.tok = p.lex.Next()
	if p.err == nil && p.lex.Err() != nil {
		p.err = p.lex.Err()
	}
	if p.lex.Incomplete() {
		// The lexer ran out inside a quote or an expansion. The parser may
		// never fail over it — a word that never finished is still a word —
		// so the snapshot has to be taken here, while the constructs around
		// it are still on the stack. Taken once, like every other.
		p.ranOut()
	}
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

func (p *Parser) atStopWord() bool {
	return p.tok.Kind == TokWord && !p.tok.IsQuoted() && stopWords[p.tok.Literal()]
}

// tokenText names the current token the way a diagnostic should: the word
// itself when there is one, and the operator's spelling otherwise.
func (p *Parser) tokenText() string {
	if p.tok.Kind == TokWord {
		return `"` + p.tok.Literal() + `"`
	}
	return `"` + p.tok.Kind.String() + `"`
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
	p.open = append(p.open, opener{word: word, line: p.tok.Pos.Line, construct: true})
	return func() { p.open = p.open[:depth] }
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
	p.open = append(p.open, opener{word: word, line: p.tok.Pos.Line})
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
		for i := n - 1; i >= 0; i-- {
			if p.open[i].construct {
				e.Construct, e.ConstructLine = p.open[i].word, p.open[i].line
				break
			}
		}
	}
	e.EndLine = p.tok.Pos.Line
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
	if p.err != nil {
		return
	}
	if p.at(TokEOF) {
		p.ranOut()
		p.err = p.unterminated(expected)
		return
	}
	p.err = &Error{
		Pos: p.tok.Pos, Kind: ErrUnexpected,
		Token: p.tokenLiteral(), Class: p.tokenClass(plain), Expected: expected,
		Redirect: p.tok.Kind.IsRedirect(),
		Msg:      p.tokenText() + " unexpected",
	}
}

// tokenLiteral is the token as a diagnostic writes it, without the quotes a
// message may add of its own.
func (p *Parser) tokenLiteral() string {
	if p.tok.Kind == TokWord {
		return p.tok.Literal()
	}
	return p.tok.Kind.String()
}

func (p *Parser) tokenClass(plain bool) TokenClass {
	if p.tok.Kind != TokWord {
		return ClassOperator
	}
	if !plain && !p.tok.IsQuoted() && reservedWords[p.tok.Literal()] {
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
	p.skipNewlines()
	if p.at(TokEOF) || p.err != nil {
		return nil, false
	}
	f := &File{}
	for p.err == nil {
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
	}
	f.Last = p.tok.Pos
	return f, true
}

func (p *Parser) skipNewlines() {
	for p.at(TokNewline) {
		p.next()
	}
}

// parseList reads statements until a stop word, a closing paren, or the end.
func (p *Parser) parseList() []*Stmt {
	var out []*Stmt
	p.skipNewlines()
	for p.err == nil && !p.at(TokEOF) && !p.atStopWord() && !p.at(TokRightParen) {
		st := p.parseStmt()
		if st == nil {
			break
		}
		out = append(out, st)
		p.skipNewlines()
	}
	return out
}

// parseStmt reads one and-or list and its terminator.
func (p *Parser) parseStmt() *Stmt {
	expr := p.parseAndOr()
	if expr == nil {
		return nil
	}
	st := &Stmt{Expr: expr}
	switch p.tok.Kind {
	case TokAmp:
		// `&` belongs to the statement, not the command: `a && b &`
		// backgrounds the whole and-or.
		st.Background = true
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
	if from.Offset < 0 || to.Offset > len(src) || from.Offset >= to.Offset {
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
		p.open = append(p.open, opener{word: op.String(), line: pos.Line})
		p.next()
		p.skipNewlines()
		right := p.parsePipeline()
		if right == nil {
			p.fail("expected a command after %s", op)
			return left
		}
		p.open = p.open[:depth]
		left = &BinaryExpr{X: left, Op: op, OpPos: pos, Y: right}
	}
	return left
}

// parsePipeline reads commands joined by `|`, with an optional leading `!`
// that negates the whole pipeline rather than its first command.
func (p *Parser) parsePipeline() Expr {
	pl := &Pipeline{}
	if p.atWord("!") {
		pl.Negated = true
		pl.Bang = p.tok.Pos
		p.next()
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
			p.fail("expected a command after |")
			return pl
		}
		// The bar has its command, so the pipeline is whole again: a failure
		// after this has nothing to do with it.
		p.open = p.open[:depth]
		pl.Cmds = append(pl.Cmds, cmd)
		if !p.at(TokPipe) {
			return pl
		}
		// Pushed rather than opened as a clause: a clause displaces the
		// clause before it, and a bar is not one of those — it belongs to
		// the pipeline and not to whatever construct the pipeline is in. As
		// a clause it evicted the `then` it was written inside, which then
		// reported an `if` waiting for a bar.
		p.open = append(p.open, opener{word: "|", line: p.tok.Pos.Line})
		p.next()
		p.skipNewlines()
	}
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
	if p.Aliases != nil {
		p.aliasNextWord = false
		p.expandAlias(map[string]bool{})
	}
	switch {
	case p.at(TokEOF), p.at(TokNewline), p.atStopWord():
		return nil
	case p.at(TokLeftParen):
		return p.withRedirs(p.parseSubshell())
	case p.at(TokArithCmd):
		return p.withRedirs(p.parseArithCmd())
	case p.atWord("{"):
		return p.withRedirs(p.parseGroup(body))
	case p.atWord("if"):
		return p.withRedirs(p.parseIf())
	case p.atWord("while"), p.atWord("until"):
		return p.withRedirs(p.parseLoop())
	case p.atWord("for"):
		return p.withRedirs(p.parseFor())
	case p.atWord("select") && p.dialect.Select:
		return p.withRedirs(p.parseSelect())
	case p.atWord("case"):
		return p.withRedirs(p.parseCase())
	case p.atWord("[[") && p.dialect.DoubleBracket:
		return p.withRedirs(p.parseTestClause())
	case p.atWord("function") && p.dialect.FunctionKeyword:
		return p.parseFuncKeyword()
	}
	return p.parseSimple()
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
			out[i].Param = p.parseParamExp(out[i].Value, out[i].Pos)
		case out[i].Kind == ArithSubst && out[i].Arith == nil:
			out[i].Arith = p.parseArith(out[i].Value, out[i].Pos)
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
	p.next()
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

// isAssign reports whether a word is `name=…` written so the name is unquoted.
func (p *Parser) isAssign(t Token) (string, bool) {
	if t.Kind != TokWord || len(t.Spans) == 0 || t.Spans[0].Quoting != Unquoted ||
		t.Spans[0].Kind != Literal {
		return "", false
	}
	head := t.Spans[0].Value
	eq := strings.IndexByte(head, '=')
	if eq <= 0 {
		return "", false
	}
	name := head[:eq]
	// `name+=value` appends. The `+` is part of neither the name nor the
	// value, so it is taken off here and remembered by parseAssign, which
	// reads the same head.
	if strings.HasSuffix(name, "+") {
		if !p.dialect.AppendAssign {
			return "", false
		}
		name = name[:len(name)-1]
	}
	// `name[i]=` is an assignment too; the subscript is unpacked later.
	if i := strings.IndexByte(name, '['); i >= 0 && strings.HasSuffix(name, "]") {
		if !isName(name[:i]) {
			return "", false
		}
		return name, true
	}
	if !isName(name) {
		return "", false
	}
	return name, true
}

// isName reports whether s is a shell name: the production the grammar spells
// `name`, which POSIX defines as an identifier. `for 1 in …` is rejected by
// every shell for this reason.
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

	for p.err == nil {
		switch {
		case p.at(TokIONumber), p.tok.Kind.IsRedirect():
			if r := p.parseRedirect(); r != nil {
				c.Redirs = append(c.Redirs, r)
			}
		case p.at(TokWord):
			// A function definition announces itself only at the paren.
			if !seenArg && len(c.Assigns) == 0 && p.looksLikeFuncDef() {
				return p.parseFuncPosix()
			}
			if name, ok := p.isAssign(p.tok); ok && !seenArg {
				c.Assigns = append(c.Assigns, p.parseAssign(name))
				continue
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
				// The expansion before this one ended in a space, so this
				// word is eligible too — the rule behind `alias sudo='sudo '`.
				// Only once the expansion's own tokens are spent: the space
				// makes the word *after* the value eligible, not the value's
				// own second word. Cleared first so a value that does not end
				// in a space stops the chain here.
				p.aliasNextWord = false
				p.expandAlias(map[string]bool{})
				continue
			}
			seenArg = true
			c.Args = append(c.Args, p.word())
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
			if len(c.Assigns) == 0 && len(c.Args) == 0 && len(c.Redirs) == 0 {
				return nil
			}
			return c
		}
	}
	c.Stop = p.tok.Pos
	return c
}

func (p *Parser) parseAssign(name string) *Assign {
	head := p.tok.Spans[0].Value
	eq := strings.IndexByte(head, '=')
	a := &Assign{Name: name, Start: p.tok.Pos, Append: eq > 0 && head[eq-1] == '+'}
	// `name[i]=value`: the subscript is part of the name half, which the
	// assignment scan already left in place.
	if i := strings.IndexByte(name, '['); i >= 0 && strings.HasSuffix(name, "]") {
		a.Name = name[:i]
		a.Index = &Word{
			Spans: []Span{{Kind: Literal, Value: name[i+1 : len(name)-1], Pos: p.tok.Pos}},
			Start: p.tok.Pos, Stop: p.tok.End,
		}
	}
	rest := head[eq+1:]

	spans := make([]Span, 0, len(p.tok.Spans))
	if rest != "" {
		spans = append(spans, Span{Kind: Literal, Value: rest, Pos: p.tok.Pos})
	}
	spans = append(spans, p.tok.Spans[1:]...)
	a.Stop = p.tok.End
	if len(spans) > 0 {
		a.Value = p.newWord(spans, p.tok.Pos, p.tok.End)
	}
	p.next()

	// `a=(1 2)` is an array, and the parenthesis has to be adjacent. With a
	// space it is not a subshell — measured, against the comment that used
	// to stand here: `a= (echo x)` is a syntax error in dash, bash and zsh,
	// and only ksh93 accepts it. The adjacency check still matters, because
	// it decides *which* error, and the non-adjacent form now falls through
	// to the paren-after-a-word rule in parseSimple.
	if a.Value == nil && p.at(TokLeftParen) && p.tok.Pos.Offset == a.Stop.Offset {
		if !p.dialect.ArrayLiteral {
			p.failUnexpected("")
			return a
		}
		a.IsArray = true
		p.next()
		p.skipNewlines()
		for p.tok.Kind == TokWord && p.err == nil {
			a.Elems = append(a.Elems, p.word())
			p.skipNewlines()
		}
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
	name, ok := p.isAssign(p.tok)
	if !ok || !strings.HasSuffix(p.tok.Text, "=") {
		// The suffix test is what keeps a scalar off this path rather than
		// what makes it come out right — the fallback below would hand
		// `local a=1` back unchanged anyway. It is here so the common case
		// does not take a round trip through parseAssign to arrive where it
		// started.
		return nil, false
	}
	tok := p.tok
	a = p.parseAssign(name)
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
	if p.tok.IsQuoted() || len(p.tok.Spans) != 1 || p.tok.Spans[0].Kind != Literal {
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
	// A function name is a name, so it cannot contain `=`. Without this,
	// `a=()` — an empty array — was read as a definition of a function
	// called `a=`, because a parenthesis pair follows either way.
	if !isName(p.tok.Literal()) {
		return false
	}
	return p.lex.peekIsFuncParens()
}

func (p *Parser) parseFuncPosix() Command {
	fn := &FuncDecl{Name: p.tok.Literal(), Start: p.tok.Pos}
	p.next()
	p.next() // (
	if !p.at(TokRightParen) {
		p.failUnexpectedOperand(")")
		return fn
	}
	p.next()
	p.skipNewlines()
	p.funcBody = true
	if fn.Body = p.parseCommand(); fn.Body == nil {
		p.fail("expected a body for function %q", fn.Name)
	}
	return fn
}

func (p *Parser) parseFuncKeyword() Command {
	fn := &FuncDecl{Keyword: true, Start: p.tok.Pos}
	p.next()
	if p.tok.Kind != TokWord || !isName(p.tok.Literal()) {
		p.fail("expected a name after `function`")
		return fn
	}
	fn.Name = p.tok.Literal()
	p.next()
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
	p.skipNewlines()
	p.funcBody = true
	if fn.Body = p.parseCommand(); fn.Body == nil {
		p.fail("expected a body for function %q", fn.Name)
	}
	return fn
}

func (p *Parser) parseSubshell() Command {
	c := &Subshell{Start: p.tok.Pos}
	defer p.opens("(")()
	p.next()
	c.List = p.parseList()
	if !p.at(TokRightParen) {
		p.fail("expected )")
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
	c.List = p.parseList()
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

func (p *Parser) parseArithCmd() Command {
	c := &ArithCmdClause{Expr: p.tok.Text, Start: p.tok.Pos, Stop: p.tok.End}
	c.Parsed = p.parseArith(p.tok.Text, p.tok.Pos)
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
func (p *Parser) parseForArith(start Pos) Command {
	c := &ForArithClause{Start: start}
	p.next() // for
	text := p.tok.Text
	c.Header = "for ((" + text + "))"
	at := p.tok.Pos
	p.next()

	init, cond, post := splitForArith(text)
	c.InitText, c.CondText, c.PostText = init, cond, post
	if init != "" {
		c.Init = p.parseArith(init, at)
	}
	if cond != "" {
		c.Cond = p.parseArith(cond, at)
	}
	if post != "" {
		c.Post = p.parseArith(post, at)
	}
	p.requireSep("do")
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseList()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
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
	c.Cond = p.parseList()
	p.requireSep("then")
	// Recorded after it is consumed: until then the innermost thing awaiting
	// a partner is the `if` itself, which is what one shell names for
	// `if true` and not for `if true; then echo x`.
	p.opensClause("then")
	p.expectWord("then")
	c.Then = p.parseList()

	for p.atWord("elif") && p.err == nil {
		e := &Elif{Start: p.tok.Pos}
		p.opensClause("elif")
		p.next()
		e.Cond = p.parseList()
		p.requireSep("then")
		p.expectWord("then")
		e.Then = p.parseList()
		c.Elifs = append(c.Elifs, e)
	}
	if p.atWord("else") {
		p.opensClause("else")
		p.next()
		c.HasElse = true
		c.Else = p.parseList()
	}
	c.Stop = p.tok.End
	p.expectWord("fi")
	return c
}

func (p *Parser) parseLoop() Command {
	c := &LoopClause{Until: p.atWord("until"), Start: p.tok.Pos}
	defer p.opens(loopWord(c.Until))()
	p.next()
	c.Cond = p.parseList()
	p.requireSep("do")
	// A `do` inside a while or until is a keyword awaiting its own partner,
	// and inside a `for` it is not — measured, in the one shell whose wording
	// can tell: `while true; do echo x` is "`do' unmatched" there and
	// `for i in a; do echo x` is "`for' unmatched". An irregularity in that
	// shell rather than in this one, recorded rather than smoothed over.
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseList()
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
	if p.tok.Kind != TokWord || !isName(p.tok.Literal()) {
		if p.err == nil {
			p.err = &Error{
				Pos: p.tok.Pos, Kind: ErrForName,
				Token: p.tokenLiteral(), Class: p.tokenClass(false),
				Msg: "expected a name after `for`",
			}
		}
		return c
	}
	c.Name = p.tok.Literal()
	nameEnd := p.tok.End
	p.next()
	p.skipNewlines()

	// An absent word list is not an empty one: without `in` the loop iterates
	// the positional parameters, and with `in` and nothing after it, nothing.
	if p.atWord("in") {
		c.HasItems = true
		p.next()
		for p.tok.Kind == TokWord && !p.atStopWord() {
			c.Items = append(c.Items, p.word())
		}
	}
	end := nameEnd
	if n := len(c.Items); n > 0 {
		end = c.Items[n-1].End()
	}
	c.Header = p.slice(c.Start, end)
	p.requireSep("do")
	p.expectWord("do")
	c.Body = p.parseList()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// parseSelect reads the menu loop, whose header is a for-loop's.
func (p *Parser) parseSelect() Command {
	c := &SelectClause{Start: p.tok.Pos}
	defer p.opens("select")()
	p.next()
	if p.tok.Kind != TokWord || !isName(p.tok.Literal()) {
		if p.err == nil {
			p.err = &Error{
				Pos: p.tok.Pos, Kind: ErrForName,
				Token: p.tokenLiteral(), Class: p.tokenClass(false),
				Msg: "expected a name after `select`",
			}
		}
		return c
	}
	c.Name = p.tok.Literal()
	nameEnd := p.tok.End
	p.next()
	p.skipNewlines()
	if p.atWord("in") {
		c.HasItems = true
		p.next()
		for p.tok.Kind == TokWord && !p.atStopWord() {
			c.Items = append(c.Items, p.word())
		}
	}
	end := nameEnd
	if n := len(c.Items); n > 0 {
		end = c.Items[n-1].End()
	}
	c.Header = p.slice(c.Start, end)
	p.requireSep("do")
	p.expectWord("do")
	c.Body = p.parseList()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseCase() Command {
	c := &CaseClause{Start: p.tok.Pos}
	defer p.opens("case")()
	p.next()
	if c.Word = p.word(); c.Word == nil {
		p.fail("expected a word after `case`")
		return c
	}
	p.skipNewlines()
	inEnd := p.tok.End
	p.expectWord("in")
	c.Header = p.slice(c.Start, inEnd)
	p.skipNewlines()

	for p.err == nil && !p.atWord("esac") && !p.at(TokEOF) {
		it := &CaseItem{Start: p.tok.Pos}
		// A pattern may carry a leading open paren.
		if p.at(TokLeftParen) {
			p.next()
		}
		if p.dialect.CasePatternAcceptsOperator && !p.at(TokWord) && !p.at(TokRightParen) && !p.at(TokEOF) {
			// One operator may stand where the pattern list would start, and
			// the arm it opens has no patterns — so it matches nothing, which
			// is what the dialect that allows this does with it.
			p.next()
		} else {
			for {
				w := p.word()
				if w == nil {
					p.failUnexpected("")
					return c
				}
				it.Patterns = append(it.Patterns, w)
				if !p.at(TokPipe) {
					break
				}
				p.next()
			}
		}
		if !p.at(TokRightParen) {
			p.failUnexpectedOperand(")")
			return c
		}
		p.next()
		it.Body = p.parseList()

		switch p.tok.Kind {
		case TokDSemi, TokSemiAmp, TokDSemiAmp:
			it.Term, it.TermPos = p.tok.Kind, p.tok.Pos
			p.next()
		default:
			// The last arm may omit its terminator before `esac`.
			it.TermPos = p.tok.Pos
			if !p.atWord("esac") {
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
		p.skipNewlines()
	}
	c.Stop = p.tok.End
	p.expectWord("esac")
	return c
}

// ParseParamExpFor parses the inside of a `${ }` that was captured outside the
// normal word path — a here-document body, which is read as raw text and
// expanded only when the command runs.
func (p *Parser) ParseParamExpFor(src string, at Pos) *ParamExpr {
	return p.parseParamExp(src, at)
}

// ParseArithFor is ParseParamExpFor for `$(( ))`.
func (p *Parser) ParseArithFor(src string, at Pos) ArithExpr {
	return p.parseArith(src, at)
}
