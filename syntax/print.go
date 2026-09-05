// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// Printing a tree back as source.
//
// Two features want this and neither can be built without it: `type f` shows
// a function's body, and `export -f` carries one through the environment. A
// shell that has read a function has to be able to say it again.
//
// The promise here is the tree and not the text. What comes back parses to
// the same tree as what went in — it is not the original spelling, because
// the tree does not hold one: `$x` and `${x}` are one node, and so are
// `if a; then b; fi` and the same thing over four lines. Anything wanting a
// particular *layout* asks for it; anything wanting the original text should
// keep the text, as a background statement and a redirection target already
// do.
//
// Most of the work is already done by the parser, which keeps the inside of
// every expansion as it was written: a `${…}` span holds `u:-a b`, a `$(…)`
// span holds its script, and an arithmetic command holds its expression. So
// this is mostly re-emission with the punctuation put back, and the only real
// decisions are about quoting and about where a statement ends.

// Print renders a parsed file as source that parses to the same tree.
func Print(f *File) string {
	if f == nil {
		return ""
	}
	var p printer
	p.lines(f.Stmts)
	return p.b.String()
}

// PrintCommand renders one command, which is what a function's body is.
func PrintCommand(c Command) string { return PrintWith(c, Layout{}) }

// PrintWord renders one word as source that parses to the same word, quotes
// and all.
//
// A diagnostic wants this: a grammar that names the word an unreadable
// expansion sits in has to be able to say the word, and by the time anything
// has failed the word has been taken apart into spans.
func PrintWord(w *Word) string {
	if w == nil {
		return ""
	}
	var p printer
	p.word(w)
	return p.b.String()
}

// PrintWordQuotingRun renders the run of spans around index i that share its
// quoting, *without* the quote characters that surrounded the run.
//
// The run rather than the word, because that is the unit one grammar's
// diagnostics name: `echo 'lit'"${x@QQ}"` blames `${x@QQ}` alone while `echo
// "pre${x@QQ}post"` blames all of `pre${x@QQ}post`. Both are one word, and
// what separates them is that the first changes quoting in the middle.
//
// Literal text goes in as it stands. There are no quotes around the result to
// protect anything from, so escaping it would add characters that were never
// written — `"\"${x@QQ}\""` names `"${x@QQ}"`, measured.
func PrintWordQuotingRun(w *Word, i int) string {
	if w == nil || i < 0 || i >= len(w.Spans) {
		return ""
	}
	q := w.Spans[i].Quoting
	lo, hi := i, i+1
	for lo > 0 && w.Spans[lo-1].Quoting == q {
		lo--
	}
	for hi < len(w.Spans) && w.Spans[hi].Quoting == q {
		hi++
	}
	var p printer
	p.raw = true
	for j := lo; j < hi; j++ {
		if w.Spans[j].Kind == Literal {
			p.str(w.Spans[j].Value)
			continue
		}
		p.withNext(w.Spans, j, func() { p.span(w.Spans[j]) })
	}
	return p.b.String()
}

// Layout is how a printed block is arranged.
//
// Every field is a junction where the shells differ, and the type exists so
// that this package decides none of them. The zero value keeps the source's
// own line structure and adds nothing, which is what a round trip wants; a
// caller that needs a particular shape fills the fields in.
//
// That is the whole point of it being data. An arrangement measured from one
// shell and written into this package would be that shell's taste living
// where nothing is supposed to know a shell — and the differences are not
// small: one puts `then` on the line of its `if` and another on a line of its
// own, one terminates statements with `;` and another with nothing at all.
type Layout struct {
	// Indent is one level of indentation, and Nested repeats it once per
	// enclosing block. Without Nested every block is indented the same,
	// however deep.
	Indent string
	Nested bool
	// Lines puts each statement of a block on a line of its own, whatever
	// the source did. Nothing else here applies without it.
	Lines bool

	// Separator goes after every statement of a block but the last.
	Separator string
	// KeywordTerminator goes after the *last* statement of a body closed by
	// a keyword — `fi`, `done`, `else`. A body closed by a bracket takes
	// nothing, in every shell measured.
	KeywordTerminator string

	// ThenOnItsOwnLine, DoAfterWordsOnItsOwnLine and DoAfterCommandOnItsOwnLine
	// say whether the keyword that opens a body starts a line of its own.
	//
	// Three fields and not one, because a shell may answer them differently:
	// one keeps `then` with its `if` and moves the `do` of a loop over words
	// while keeping the `do` of a loop over a command.
	ThenOnItsOwnLine           bool
	DoAfterWordsOnItsOwnLine   bool
	DoAfterCommandOnItsOwnLine bool

	// BraceOpenSuffix follows the `{` that opens a block — a space, or
	// nothing.
	BraceOpenSuffix string
	// OutermostBraceOpensALine puts the first statement of the outermost
	// block on a line of its own. A block inside one always does.
	OutermostBraceOpensALine bool

	// CaseHeaderSuffix follows the `in` of a `case`.
	CaseHeaderSuffix string
	// CaseArmsOnOneLine keeps a `case` arm's pattern, body and terminator
	// together instead of giving each a line.
	CaseArmsOnOneLine bool
	// CasePatternsParenthesised writes an arm's pattern with the opening
	// parenthesis that the grammar allows and most shells leave out.
	CasePatternsParenthesised bool
}

// PrintWith renders one command with a chosen arrangement.
func PrintWith(c Command, l Layout) string {
	if c == nil {
		return ""
	}
	p := printer{layout: l}
	p.command(c)
	return p.b.String()
}

type printer struct {
	b strings.Builder
	// after is the text that will follow the span being written, which is
	// what decides whether a parameter can drop its braces.
	after string
	// raw suppresses escaping of an unquoted literal, for the places where
	// the punctuation belongs to a pattern rather than to the shell.
	raw bool
	// layout is how a block is arranged, when the caller asked for one, and
	// depth how many blocks deep the writing has reached.
	layout Layout
	depth  int
	// heredocs are the bodies owed by the statement being written, which go
	// after it rather than where the operator is.
	heredocs []*Redirect
}

func (p *printer) str(s string) { p.b.WriteString(s) }

// stmts writes a list, separated the way a shell separates them on one line.
//
// `;` between and none after, which is what a group needs — `{ a; b; }` has
// the last one terminated by the brace's own rule and this adds it there.
// lines and stmts both write a list; they differ in nothing and are kept
// apart only because the two callers read better for it.
func (p *printer) lines(list []*Stmt) { p.stmts(list) }

// stmts writes a list, separating each from the one before it the way the
// source did.
//
// Which is not a matter of taste. A shell says which line something happened
// on, and `$LINENO` says where it is, so a printer that decides where the
// line breaks go decides what the script says about itself: two statements
// joined onto one line report line 1 twice, and one line split into two
// reports 1 and 2. Both were wrong here before the behavioral round trip
// said so, in opposite directions.
//
// The tree knows, because a position is a line.
func (p *printer) stmts(list []*Stmt) {
	for i, st := range list {
		if i > 0 {
			p.separate(list[i-1], st)
		}
		p.stmt(st)
	}
}

// separate writes what goes between two statements.
func (p *printer) separate(prev, next *Stmt) {
	if p.layout.Lines {
		// A shape the caller asked for, so the source's own lines do not
		// come into it. A backgrounded statement is already terminated
		// whatever the arrangement says.
		if !prev.Background {
			p.str(p.layout.Separator)
		}
		p.str("\n" + p.pad())
		return
	}
	// A here-document body has already ended the line.
	ended := strings.HasSuffix(p.b.String(), "\n")
	if prev.End().Line != next.Pos().Line {
		if !ended {
			p.str("\n")
		}
		return
	}
	if ended {
		// Written on one line and one of them owed a body, which has to go
		// on lines of its own. There is no way back onto the first line, so
		// the rest of it follows the body — which is what a shell does with
		// `cat <<E; echo after` too.
		return
	}
	// A backgrounded statement is already terminated: `a & b` is two
	// statements and `a &; b` is a syntax error. The `&` is the separator.
	if prev.Background {
		p.str(" ")
		return
	}
	p.str("; ")
}

func (p *printer) stmt(st *Stmt) {
	if st == nil {
		return
	}
	p.expr(st.Expr)
	if st.Background {
		p.str(" &")
	}
	p.flushHeredocs()
}

func (p *printer) expr(e Expr) {
	switch x := e.(type) {
	case *BinaryExpr:
		p.expr(x.X)
		p.str(" " + x.Op.String() + " ")
		p.expr(x.Y)
	case *Pipeline:
		if x.Negated {
			p.str("! ")
		}
		for i, c := range x.Cmds {
			if i > 0 {
				p.str(" | ")
			}
			p.command(c)
		}
	case *TimeClause:
		if x.Negated {
			p.str("! ")
		}
		p.str("time")
		if x.Posix {
			p.str(" -p")
		}
		if x.Pipeline != nil {
			p.str(" ")
			p.expr(x.Pipeline)
		}
	}
}

func (p *printer) command(c Command) {
	switch x := c.(type) {
	case *SimpleCmd:
		p.simple(x)
	case *Subshell:
		// Spaced, which is not decoration: a subshell whose first command is
		// itself a subshell needs the separation, `((` being arithmetic.
		p.str("( ")
		p.stmts(x.List)
		p.str(" )")
		p.redirs(x.Redirs)
	case *Group:
		// The space after `{` and the `;` before `}` are both required: they
		// are what make it a reserved word rather than the start of a name.
		if p.layout.Lines {
			p.str("{" + p.layout.BraceOpenSuffix)
			// The outermost brace — a function's own — is the one that
			// differs; a brace inside one opens a line in every arrangement
			// measured.
			p.bodyAt(x.List, false, p.layout.OutermostBraceOpensALine || p.depth > 0)
			p.str("\n" + p.pad() + "}")
			p.redirs(x.Redirs)
			return
		}
		p.str("{ ")
		p.stmts(x.List)
		p.terminate()
		p.str("}")
		p.redirs(x.Redirs)
	case *IfClause:
		p.ifClause(x)
	case *LoopClause:
		word := "while"
		if x.Until {
			word = "until"
		}
		p.str(word + " ")
		p.stmts(x.Cond)
		if len(x.Body) > 0 {
			p.opener("do", p.layout.DoAfterCommandOnItsOwnLine)
			p.body(x.Body, true)
			p.keyword("done")
		}
		p.redirs(x.Redirs)
	case *ForClause:
		p.str("for " + x.Name)
		if len(x.Body) == 0 {
			p.parenItems(x.HasItems, x.Items)
			p.redirs(x.Redirs)
			return
		}
		p.items(x.HasItems, x.Items)
		p.doKeyword()
		p.body(x.Body, true)
		p.keyword("done")
		p.redirs(x.Redirs)
	case *AnonFunc:
		// Printed as it was written: there is no long spelling, because a
		// function with no name cannot be defined in one place and called in
		// another.
		if x.Keyword {
			p.str("function ")
		} else {
			p.str("() ")
		}
		p.command(x.Body)
		for _, a := range x.Args {
			p.str(" ")
			p.word(a)
		}
		p.redirs(x.Redirs)
	case *RepeatClause:
		// Printed with `do … done`, which parses to the same tree under any
		// dialect that has the construct at all — the same choice a short
		// loop body already makes.
		p.str("repeat ")
		p.word(x.Count)
		p.doKeyword()
		p.body(x.Body, true)
		p.keyword("done")
		p.redirs(x.Redirs)
	case *SelectClause:
		p.str("select " + x.Name)
		if len(x.Body) == 0 {
			p.parenItems(x.HasItems, x.Items)
			p.redirs(x.Redirs)
			return
		}
		p.items(x.HasItems, x.Items)
		p.doKeyword()
		p.body(x.Body, true)
		p.keyword("done")
		p.redirs(x.Redirs)
	case *CaseClause:
		p.caseClause(x)
	case *ForArithClause:
		p.str("for ((" + x.InitText + "; " + x.CondText + "; " + x.PostText + "))")
		if len(x.Body) > 0 {
			p.str("; do")
			p.body(x.Body, true)
			p.keyword("done")
		}
		p.redirs(x.Redirs)
	case *TestClause:
		p.str("[[ ")
		p.cond(x.Expr)
		p.str(" ]]")
		p.redirs(x.Redirs)
	case *ArithCmdClause:
		p.str("((" + x.Expr + "))")
		p.redirs(x.Redirs)
	case *FuncDecl:
		// Without the `function` keyword even where it was written with one:
		// `function f { …; }` and `f() { …; }` are the same declaration to
		// every shell that has both, and the parenthesised form is the one
		// they all read.
		p.str(x.Name + "() ")
		p.command(x.Body)
	case *CoprocClause:
		p.str("coproc ")
		if x.Name != "" {
			p.str(x.Name + " ")
		}
		p.command(x.Cmd)
	}
}

// cond writes the inside of a `[[ … ]]`.
//
// The operands are words rather than strings because what they mean depends
// on how they were written — unquoted, the right operand of `==` is a pattern
// and of `=~` a regular expression — so they go through the word printer like
// any other and keep their quoting with them.
func (p *printer) cond(e CondExpr) {
	switch x := e.(type) {
	case *CondUnary:
		p.str(x.Op + " ")
		p.rawWord(x.X)
	case *CondBinary:
		p.word(x.X)
		p.str(" " + x.Op + " ")
		// The right operand unquoted is a pattern for `==` and a regular
		// expression for `=~`, so its punctuation is the pattern's: escaping
		// `(`, `|` or `$` leaves the tree identical and turns `^(a|x)bc$`
		// into a search for four literal characters. Whatever the parser
		// accepted unquoted here goes back unquoted.
		p.rawWord(x.Y)
	case *CondLogic:
		p.cond(x.X)
		p.str(" " + x.Op + " ")
		p.cond(x.Y)
	case *CondNot:
		p.str("! ")
		p.cond(x.X)
	case *CondGroup:
		// The parentheses are kept rather than dropped where they happen to
		// be redundant: they are in the tree, and a printer that decides
		// they are unnecessary is deciding about precedence.
		p.str("( ")
		p.cond(x.X)
		p.str(" )")
	}
}

// doKeyword writes the `do` that opens a `for` or `select` body.
//
// On a line of its own, which a `while` body's is not — the header of one is
// a word list and of the other a command, and the arrangement follows that
// rather than being uniform.
func (p *printer) doKeyword() { p.opener("do", p.layout.DoAfterWordsOnItsOwnLine) }

// opener writes the keyword that introduces a body — `then`, `do` — either on
// the header's line or on one of its own, which the arrangement decides.
func (p *printer) opener(word string, ownLine bool) {
	if !p.layout.Lines {
		p.str("; " + word)
		return
	}
	if ownLine {
		p.str(p.layout.Separator + "\n" + p.pad() + word)
		return
	}
	p.str(p.layout.Separator + " " + word)
}

// items writes a `for` or `select` header's word list.
//
// `in` with nothing after it is not the same as no `in` at all — the first
// loops over nothing and the second over the positional parameters — so the
// flag decides rather than the length.
func (p *printer) items(has bool, items []*Word) {
	if !has {
		return
	}
	p.str(" in")
	for _, w := range items {
		p.str(" ")
		p.word(w)
	}
}

// parenItems writes the same list in the short loop's parentheses.
//
// Only reached for a loop whose body is empty, and that is the whole reason
// the spelling exists here. `do … done` cannot hold no commands — every shell
// in the panel refuses `do done` — so a tree with an empty body has no long
// form to be printed as, and `for i in a b` with nothing after it is a syntax
// error in the shell that produced it. The parentheses end the header, which
// is what lets the body be absent.
func (p *printer) parenItems(has bool, items []*Word) {
	if !has {
		return
	}
	p.str(" (")
	for i, w := range items {
		if i > 0 {
			p.str(" ")
		}
		p.word(w)
	}
	p.str(")")
}

func (p *printer) ifClause(x *IfClause) {
	p.str("if ")
	p.stmts(x.Cond)
	p.opener("then", p.layout.ThenOnItsOwnLine)
	p.body(x.Then, true)
	for _, e := range x.Elifs {
		p.keyword("elif ")
		p.stmts(e.Cond)
		p.opener("then", p.layout.ThenOnItsOwnLine)
		p.body(e.Then, true)
	}
	if x.HasElse {
		p.keyword("else")
		p.body(x.Else, true)
	}
	p.keyword("fi")
	p.redirs(x.Redirs)
}

// body writes the statements between two keywords.
//
// On one line where the caller asked for nothing, and one to a line where a
// layout was chosen — the arrangement reaches inside a construct rather than
// stopping at the outermost block, because a caller laying a function out
// this way lays all of it out.
//
// closedByKeyword says what follows: a word such as `fi` or `done`, or a
// bracket. It decides the last statement's terminator, which is the one rule
// here that is not about where the line breaks go — a keyword takes a `;`
// before it and `}` and `;;` do not.
func (p *printer) body(list []*Stmt, closedByKeyword bool) {
	p.bodyAt(list, closedByKeyword, true)
}

// bodyAt is body, with whether the first statement starts on a line of its
// own. It does everywhere but at a brace in the flatter of the two
// arrangements, where the body opens on the brace's line.
func (p *printer) bodyAt(list []*Stmt, closedByKeyword, ownLine bool) {
	if !p.layout.Lines {
		p.str(" ")
		p.stmts(list)
		return
	}
	p.depth++
	if ownLine {
		p.str("\n")
	}
	p.str(p.pad())
	p.stmts(list)
	if closedByKeyword {
		p.str(p.layout.KeywordTerminator)
	}
	p.depth--
}

// pad is the indent for the depth being written.
func (p *printer) pad() string {
	if p.depth == 0 {
		// Nothing encloses this, so nothing indents it: the brace that
		// closes a function's body sits where the function does.
		return ""
	}
	if !p.layout.Nested {
		return p.layout.Indent
	}
	return strings.Repeat(p.layout.Indent, p.depth)
}

// keyword writes the word that closes or continues a construct.
func (p *printer) keyword(word string) {
	if p.layout.Lines {
		p.str("\n" + p.pad() + word)
		return
	}
	p.terminate()
	p.str(word)
}

// terminate ends the statement before a closing word.
//
// A `;` unless the line has already ended, which a here-document body does:
// its lines go after the statement, so what follows is at the start of a line
// and `; fi` there is a `;` with nothing before it. Found by printing every
// shell script on this machine — four of them close a block right after a
// here-document, and the corpus has none that do.
func (p *printer) terminate() {
	if strings.HasSuffix(p.b.String(), "\n") {
		return
	}
	// A backgrounded statement is already terminated — the rule separate
	// applies between statements holds at the closing word too: `&` before
	// `}` or `done` takes only a space, and `&;` parses nowhere. The suffix
	// is unambiguous, because ` &` is written by the statement printer alone:
	// a literal ampersand in a word arrives escaped or quoted.
	if strings.HasSuffix(p.b.String(), " &") {
		p.str(" ")
		return
	}
	p.str("; ")
}

func (p *printer) caseClause(x *CaseClause) {
	p.str("case ")
	p.word(x.Word)
	if p.layout.Lines {
		p.str(" in" + p.layout.CaseHeaderSuffix)
		p.caseArms(x)
		return
	}
	p.str(" in ")
	for _, it := range x.Items {
		for i, pat := range it.Patterns {
			if i > 0 {
				p.str("|")
			}
			p.word(pat)
		}
		p.str(") ")
		p.stmts(it.Body)
		term := it.Term.String()
		if it.Term == 0 {
			// The last arm may leave its terminator out, and putting one in
			// is what lets `esac` follow without changing the tree.
			term = ";;"
		}
		p.str(" " + term + " ")
	}
	p.str("esac")
	p.redirs(x.Redirs)
}

// caseArms writes a `case`'s arms one to a line.
//
// The pattern, the body one further in, and the terminator back at the
// pattern's depth — which is a block closed by `;;` rather than by a keyword,
// so its last statement takes no `;`.
func (p *printer) caseArms(x *CaseClause) {
	p.depth++
	for _, it := range x.Items {
		p.str("\n" + p.pad())
		if p.layout.CasePatternsParenthesised {
			p.str("(")
		}
		for i, pat := range it.Patterns {
			if i > 0 {
				p.str("|")
			}
			p.word(pat)
		}
		p.str(")")
		term := it.Term.String()
		if it.Term == 0 {
			// The last arm may leave its terminator out, and putting one in
			// is what lets what follows follow.
			term = ";;"
		}
		if p.layout.CaseArmsOnOneLine {
			p.str(" ")
			p.stmts(it.Body)
			p.str(" " + term)
			continue
		}
		p.bodyAt(it.Body, false, true)
		p.str("\n" + p.pad() + term)
	}
	p.depth--
	p.str("\n" + p.pad() + "esac")
	p.redirs(x.Redirs)
}

func (p *printer) simple(c *SimpleCmd) {
	first := true
	sep := func() {
		if !first {
			p.str(" ")
		}
		first = false
	}
	// Prefixes first, then the command, then the assignments written as its
	// operands. Printing them all in front turned `typeset a=(x y)` into
	// `a=(x y) typeset`, which is a different command: the array became the
	// environment of a `typeset` with nothing to declare.
	for _, a := range c.Assigns {
		if a.Operand {
			continue
		}
		sep()
		p.assign(a)
	}
	for _, w := range c.Args {
		sep()
		p.word(w)
	}
	for _, a := range c.Assigns {
		if !a.Operand {
			continue
		}
		sep()
		p.assign(a)
	}
	p.redirs(c.Redirs)
}

func (p *printer) assign(a *Assign) {
	p.str(a.Name)
	if a.Index != nil {
		p.str("[")
		p.word(a.Index)
		p.str("]")
	}
	if a.Append {
		p.str("+")
	}
	p.str("=")
	switch {
	case a.IsArray:
		p.str("(")
		for i, e := range a.Elems {
			if i > 0 {
				p.str(" ")
			}
			p.word(e)
		}
		p.str(")")
	case a.Value != nil:
		p.word(a.Value)
	}
}

func (p *printer) redirs(rs []*Redirect) {
	for _, rd := range rs {
		p.str(" ")
		if rd.Op == TokLessAmp || rd.Op == TokGreatAmp {
			p.dup(rd)
			continue
		}
		if rd.N != nil {
			p.word(rd.N)
		}
		p.str(rd.Op.String())
		// Always a space, because the two run together otherwise: `< <(cmd)`
		// written without one is `<<`, a here-document, which is not a
		// redirection with a target at all. The round trip found that; a
		// rule about which characters can combine would have been a guess
		// about the lexer, and this needs none.
		p.str(" ")
		p.word(rd.Word)
		if rd.Op.IsHeredoc() {
			// The body is not on the line the operator is on — it is the
			// lines after the command — so it is written after everything
			// else this statement has to say. A printer that stopped at the
			// delimiter produced something that *parsed*, and read from the
			// terminal instead of from the body: the failure a round trip
			// through the parser cannot see, and the reason there is a test
			// that runs both.
			p.heredocs = append(p.heredocs, rd)
		}
	}
}

// dup writes a `<&` or `>&` redirection, which is spelled with no space.
//
// Measured through `type`, which is the only place any shell in the panel
// shows a function body back, and so the only oracle a printer has at all.
// Two things bash does there that the general shape above does not.
//
// It writes the operator tight against its target, whatever the target is:
// `>&$fd` and `>&/dev/null` come back exactly as tight as `2>&1`. The space
// the other operators take is there because `< <(cmd)` written without one
// is `<<`, a different construct — and no dup target can begin an operator,
// so the hazard the space guards against does not arise here.
//
// And it writes out the descriptor being redirected: `>&2` comes back as
// `1>&2` and `<&3` as `0<&3`. Only where the target is one it can read as a
// descriptor now — an unquoted number, or the `-` that closes one. `>&"1"`
// and `>&$fd` are settled when the command runs rather than when it is read,
// and both come back written as they were.
func (p *printer) dup(rd *Redirect) {
	op := rd.Op
	// A close is written the same way whichever operator asked for it, which
	// is the one case where the operator itself changes: `<&-` comes back as
	// `0>&-`. Measured rather than reasoned — it is a normalization, and
	// both spellings close the same descriptor.
	closing := explicitDupTarget(rd.Word) && rd.Word.Literal() == "-"
	if closing {
		op = TokGreatAmp
	}
	switch {
	case rd.N != nil:
		p.word(rd.N)
	case !explicitDupTarget(rd.Word):
	case rd.Op == TokLessAmp:
		// The descriptor is the one the operator as *written* names, even
		// where the operator itself is about to be normalized: `<&-` comes
		// back as `0>&-` and not as `1>&-`.
		p.str("0")
	default:
		p.str("1")
	}
	p.str(op.String())
	p.word(rd.Word)
}

// explicitDupTarget reports whether a dup names a descriptor the reader can
// resolve, which is what decides whether the source descriptor is written
// out. A quoted number does not count: quoting is what tells the two apart.
func explicitDupTarget(w *Word) bool {
	if w == nil || w.IsQuoted() {
		return false
	}
	lit := w.Literal()
	if lit == "-" {
		return true
	}
	if lit == "" {
		return false
	}
	for i := 0; i < len(lit); i++ {
		if lit[i] < '0' || lit[i] > '9' {
			return false
		}
	}
	return true
}

// flushHeredocs writes the bodies queued by the statement just printed.
func (p *printer) flushHeredocs() {
	pending := p.heredocs
	p.heredocs = nil
	for _, rd := range pending {
		p.str("\n")
		if rd.Heredoc != nil {
			p.str(rd.Heredoc.Literal())
		}
		// The delimiter alone on its line is what ends it, and it is written
		// bare however it was quoted: the quoting on the operator's word
		// says whether the *body* expands, and the closing line is never
		// quoted in any shell.
		p.str(rd.Word.Literal() + "\n")
	}
}

// rawWord writes a word whose unquoted punctuation belongs to it.
//
// Inside `[[ … ]]` the lexer reads a word by different rules — parentheses
// and `|` are the pattern's there — so a printer that applies the ordinary
// ones is quoting characters the parser never treated as operators.
func (p *printer) rawWord(w *Word) {
	saved := p.raw
	p.raw = true
	p.word(w)
	p.raw = saved
}

// word writes a word, span by span, putting the quotes back.
//
// Adjacent spans quoted the same way share one pair, which is not decoration:
// `"a $x b"` is three spans and re-emitting each in its own quotes would be
// three words after splitting rather than one.
func (p *printer) word(w *Word) {
	if w == nil {
		return
	}
	if len(w.Spans) == 0 {
		// A word with nothing in it was written as an empty quoted string,
		// and has to be written as one again or it disappears.
		p.str(`''`)
		return
	}
	for i := 0; i < len(w.Spans); {
		q := w.Spans[i].Quoting
		j := i
		for j < len(w.Spans) && w.Spans[j].Quoting == q && q != Unquoted && q != BackslashQuoted {
			j++
		}
		if j == i {
			p.withNext(w.Spans, i, func() { p.span(w.Spans[i]) })
			i++
			continue
		}
		p.quoted(q, w.Spans[i:j])
		i = j
	}
}

// withNext runs write with `after` set to what follows span i, so that a
// parameter can tell whether dropping its braces would swallow it.
func (p *printer) withNext(spans []Span, i int, write func()) {
	saved := p.after
	p.after = ""
	if i+1 < len(spans) {
		var peek printer
		peek.span(spans[i+1])
		p.after = peek.b.String()
	}
	write()
	p.after = saved
}

// quoted writes a run of spans inside one pair of quotes.
func (p *printer) quoted(q Quoting, spans []Span) {
	switch q {
	case SingleQuoted:
		// Nothing expands inside these, so a run of them is always one
		// literal span and the value goes in as it is.
		p.str("'")
		for _, s := range spans {
			p.str(s.Value)
		}
		p.str("'")
	case DollarSingleQuoted:
		p.str("$'")
		for _, s := range spans {
			p.str(s.Value)
		}
		p.str("'")
	default:
		p.str(`"`)
		for i := range spans {
			p.withNext(spans, i, func() { p.span(spans[i]) })
		}
		p.str(`"`)
	}
}

// span writes one span, without the quotes a run of them shares.
func (p *printer) span(s Span) {
	switch s.Kind {
	case CommandSubst:
		if s.CurrentShell {
			// The third spelling, written back as it was read: the space
			// after the brace is what makes it a command rather than a
			// parameter, and it is already the first byte of the body.
			p.str("${")
			p.str(s.Value)
			p.str("}")
			return
		}
		if s.Backquoted {
			// Written back the way it was read, because the two spellings
			// end differently: this one ends at its closing backquote, and
			// the other where its contents end. A substitution holding a
			// here-document whose delimiter never matches is terminated by
			// the first and not by the second — which is a real script on
			// this machine, and the last one that would not reprint.
			p.str("`" + escapeBackquoted(s.Value) + "`")
			return
		}
		// A space where the command starts with its own parenthesis: `$((`
		// is arithmetic, so `$( (echo x) )` written without one is a
		// different construct entirely. The same trap as a redirection whose
		// target begins with `<`, and found the same way — by printing
		// scripts nobody wrote for this.
		if strings.HasPrefix(s.Value, "(") {
			p.str("$( " + s.Value + ")")
			return
		}
		p.str("$(" + s.Value + ")")
	case ArithSubst:
		p.str("$((" + s.Value + "))")
	case ParamExp:
		// `$x` where that is what it means, and `${x}` where the braces are
		// doing something. The braced form is always *correct* — `${x}y` and
		// `$xy` are different words — and always using it is still wrong in
		// a way the tree cannot see: a diagnostic that quotes the target as
		// it was written reads `${e}: ambiguous redirect` where every shell
		// says `$e`.
		if bare, ok := p.bareParam(s); ok {
			p.str("$" + bare)
			return
		}
		p.str("${" + s.Value + "}")
	case ProcSubstIn:
		p.str("<(" + s.Value + ")")
	case ProcSubstOut:
		p.str(">(" + s.Value + ")")
	default:
		p.literal(s)
	}
}

// bareParam reports whether a parameter can be written without its braces.
//
// Two conditions, and the second is the one that bites: the name has to be
// one the shell would read unbraced, and nothing may follow that would run
// into it. `${x}y` unbraced is the parameter `xy`.
func (p *printer) bareParam(s Span) (string, bool) {
	v := s.Value
	if v == "" || !plainParamName(v) {
		return "", false
	}
	if next := p.after; next != "" && continuesName(next[0]) {
		return "", false
	}
	return v, true
}

// plainParamName reports whether a name needs no braces of its own — a name,
// a single digit, or one of the specials.
func plainParamName(v string) bool {
	if len(v) == 1 && strings.IndexByte("@*#?$!-0123456789", v[0]) >= 0 {
		return true
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func continuesName(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// literal writes text, quoting it where leaving it bare would change what it
// means.
func (p *printer) literal(s Span) {
	switch s.Quoting {
	case BackslashQuoted:
		p.str("\\" + s.Value)
	case SingleQuoted:
		p.str("'" + s.Value + "'")
	case DollarSingleQuoted:
		p.str("$'" + s.Value + "'")
	case DoubleQuoted:
		// Inside double quotes, only the four characters that still mean
		// something need protecting.
		p.str(escapeIn(s.Value, "\"\\$`"))
	default:
		if p.raw {
			p.str(s.Value)
			return
		}
		p.str(escapeBare(s.Value))
	}
}

// escapeBackquoted puts back the one layer of backslashes the older
// substitution form requires, which the lexer took off.
//
// The three characters POSIX gives a meaning to there, and no others: a
// backslash before anything else is literal inside backquotes, so escaping it
// would add a character rather than protect one.
func escapeBackquoted(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '$', '`', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeIn backslashes each of chars wherever it appears.
func escapeIn(s, chars string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(chars, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// escapeBare protects an unquoted literal.
//
// Not the pattern characters, and not braces: an unquoted `*` in the source
// was a pattern and `{1..3}` was a brace expansion, and both have to stay
// what they were. Escaping them leaves the tree identical and the meaning
// gone, which the behavioral round trip caught and the syntactic one could
// not. What is escaped is everything that would end the word or start
// something else —
// which cannot appear in an unquoted literal span from this parser, and is
// escaped anyway because a tree does not have to have come from a parser.
func escapeBare(s string) string {
	if s == "" {
		return "''"
	}
	return escapeIn(s, " \t\n\"'\\$`|&;<>()")
}
