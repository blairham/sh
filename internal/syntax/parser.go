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

	err        error
	incomplete bool

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

// Incomplete reports whether the input ended part-way through a construct that
// could still be finished. A prompt should ask for another line.
func (p *Parser) Incomplete() bool { return p.incomplete || p.lex.Incomplete() }

func (p *Parser) next() {
	p.tok = p.lex.Next()
	if p.err == nil && p.lex.Err() != nil {
		p.err = p.lex.Err()
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

func (p *Parser) atStopWord() bool {
	return p.tok.Kind == TokWord && !p.tok.IsQuoted() && stopWords[p.tok.Literal()]
}

func (p *Parser) fail(format string, args ...any) {
	if p.err != nil {
		return
	}
	if p.at(TokEOF) {
		// Running out of input is unfinished rather than wrong.
		p.incomplete = true
	}
	p.err = fmt.Errorf("%s: %s", p.tok.Pos, fmt.Sprintf(format, args...))
}

// expectWord consumes a reserved word or records what was missing.
func (p *Parser) expectWord(s string) Pos {
	pos := p.tok.Pos
	if !p.atWord(s) {
		p.fail("expected %q", s)
		return pos
	}
	p.next()
	return pos
}

// Parse reads the whole input.
func (p *Parser) Parse() *File {
	f := &File{}
	p.skipNewlines()
	for !p.at(TokEOF) && p.err == nil {
		st := p.parseStmt()
		if st == nil {
			break
		}
		f.Stmts = append(f.Stmts, st)
		p.skipNewlines()
	}
	f.Last = p.tok.Pos
	return f
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
		p.next()
	case TokSemi:
		st.Semi = p.tok.Pos
		p.next()
	case TokNewline:
		st.Semi = p.tok.Pos
	}
	return st
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
	for p.at(TokAndAnd) || p.at(TokOrOr) {
		op, pos := p.tok.Kind, p.tok.Pos
		p.next()
		p.skipNewlines()
		right := p.parsePipeline()
		if right == nil {
			p.fail("expected a command after %s", op)
			return left
		}
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
	for {
		cmd := p.parseCommand()
		if cmd == nil {
			if len(pl.Cmds) == 0 {
				return nil
			}
			p.fail("expected a command after |")
			return pl
		}
		pl.Cmds = append(pl.Cmds, cmd)
		if !p.at(TokPipe) {
			return pl
		}
		p.next()
		p.skipNewlines()
	}
}

// parseCommand dispatches on what begins the command.
func (p *Parser) parseCommand() Command {
	switch {
	case p.at(TokEOF), p.at(TokNewline), p.atStopWord():
		return nil
	case p.at(TokLeftParen):
		return p.withRedirs(p.parseSubshell())
	case p.at(TokArithCmd):
		return p.withRedirs(p.parseArithCmd())
	case p.atWord("{"):
		return p.withRedirs(p.parseGroup())
	case p.atWord("if"):
		return p.withRedirs(p.parseIf())
	case p.atWord("while"), p.atWord("until"):
		return p.withRedirs(p.parseLoop())
	case p.atWord("for"):
		return p.withRedirs(p.parseFor())
	case p.atWord("case"):
		return p.withRedirs(p.parseCase())
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
		p.fail("expected a target after %s", r.Op)
		return nil
	}
	// Built without advancing, because advancing is what reaches the newline
	// where the body is read — and the queue has to be set before that
	// happens. Registering after p.word() looks equivalent and silently
	// collects nothing.
	r.Word = p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
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
func isAssign(t Token) (string, bool) {
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
			if name, ok := isAssign(p.tok); ok && !seenArg {
				c.Assigns = append(c.Assigns, p.parseAssign(name))
				continue
			}
			seenArg = true
			c.Args = append(c.Args, p.word())
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
	a := &Assign{Name: name, Start: p.tok.Pos}
	head := p.tok.Spans[0].Value
	rest := head[strings.IndexByte(head, '=')+1:]

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
	return a
}

// looksLikeFuncDef reports whether the current word begins `name()`.
func (p *Parser) looksLikeFuncDef() bool {
	if p.tok.IsQuoted() || len(p.tok.Spans) != 1 || p.tok.Spans[0].Kind != Literal {
		return false
	}
	return p.lex.peekIsFuncParens()
}

func (p *Parser) parseFuncPosix() Command {
	fn := &FuncDecl{Name: p.tok.Literal(), Start: p.tok.Pos}
	p.next()
	p.next() // (
	if !p.at(TokRightParen) {
		p.fail("expected ) in a function definition")
		return fn
	}
	p.next()
	p.skipNewlines()
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
	// The hybrid `function f() {}` is accepted where the dialect has the
	// keyword; ksh93 rejects it, which is why it is not core.
	if p.at(TokLeftParen) {
		p.next()
		if p.at(TokRightParen) {
			p.next()
		}
	}
	p.skipNewlines()
	if fn.Body = p.parseCommand(); fn.Body == nil {
		p.fail("expected a body for function %q", fn.Name)
	}
	return fn
}

func (p *Parser) parseSubshell() Command {
	c := &Subshell{Start: p.tok.Pos}
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

func (p *Parser) parseGroup() Command {
	c := &Group{Start: p.tok.Pos}
	p.next()
	c.List = p.parseList()
	if !p.atWord("}") {
		// `{ echo a }` is a syntax error everywhere but zsh: the brace is a
		// reserved word, so it needs a terminator before it.
		p.fail("expected } — a brace group needs a terminator before it")
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
		if !p.atWord(before) {
			p.fail("expected ; or newline before %q", before)
		}
	}
}

func (p *Parser) parseIf() Command {
	c := &IfClause{Start: p.tok.Pos}
	p.next()
	c.Cond = p.parseList()
	p.requireSep("then")
	p.expectWord("then")
	c.Then = p.parseList()

	for p.atWord("elif") && p.err == nil {
		e := &Elif{Start: p.tok.Pos}
		p.next()
		e.Cond = p.parseList()
		p.requireSep("then")
		p.expectWord("then")
		e.Then = p.parseList()
		c.Elifs = append(c.Elifs, e)
	}
	if p.atWord("else") {
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
	p.next()
	c.Cond = p.parseList()
	p.requireSep("do")
	p.expectWord("do")
	c.Body = p.parseList()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseFor() Command {
	c := &ForClause{Start: p.tok.Pos}
	p.next()
	if p.tok.Kind != TokWord || !isName(p.tok.Literal()) {
		p.fail("expected a name after `for`")
		return c
	}
	c.Name = p.tok.Literal()
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
	p.requireSep("do")
	p.expectWord("do")
	c.Body = p.parseList()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseCase() Command {
	c := &CaseClause{Start: p.tok.Pos}
	p.next()
	if c.Word = p.word(); c.Word == nil {
		p.fail("expected a word after `case`")
		return c
	}
	p.skipNewlines()
	p.expectWord("in")
	p.skipNewlines()

	for p.err == nil && !p.atWord("esac") && !p.at(TokEOF) {
		it := &CaseItem{Start: p.tok.Pos}
		// A pattern may carry a leading open paren.
		if p.at(TokLeftParen) {
			p.next()
		}
		for {
			w := p.word()
			if w == nil {
				p.fail("expected a case pattern")
				return c
			}
			it.Patterns = append(it.Patterns, w)
			if !p.at(TokPipe) {
				break
			}
			p.next()
		}
		if !p.at(TokRightParen) {
			p.fail("expected ) after a case pattern")
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
				p.fail("expected ;; after a case body")
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
