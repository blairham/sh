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
func PrintCommand(c Command) string {
	if c == nil {
		return ""
	}
	var p printer
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
	}
}

func (p *printer) command(c Command) {
	switch x := c.(type) {
	case *SimpleCmd:
		p.simple(x)
	case *Subshell:
		p.str("(")
		p.stmts(x.List)
		p.str(")")
		p.redirs(x.Redirs)
	case *Group:
		// The space after `{` and the `;` before `}` are both required: they
		// are what make it a reserved word rather than the start of a name.
		p.str("{ ")
		p.stmts(x.List)
		p.str("; }")
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
		p.str("; do ")
		p.stmts(x.Body)
		p.str("; done")
		p.redirs(x.Redirs)
	case *ForClause:
		p.str("for " + x.Name)
		p.items(x.HasItems, x.Items)
		p.str("; do ")
		p.stmts(x.Body)
		p.str("; done")
		p.redirs(x.Redirs)
	case *SelectClause:
		p.str("select " + x.Name)
		p.items(x.HasItems, x.Items)
		p.str("; do ")
		p.stmts(x.Body)
		p.str("; done")
		p.redirs(x.Redirs)
	case *CaseClause:
		p.caseClause(x)
	case *ForArithClause:
		p.str("for ((" + x.InitText + "; " + x.CondText + "; " + x.PostText + ")); do ")
		p.stmts(x.Body)
		p.str("; done")
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

func (p *printer) ifClause(x *IfClause) {
	p.str("if ")
	p.stmts(x.Cond)
	p.str("; then ")
	p.stmts(x.Then)
	for _, e := range x.Elifs {
		p.str("; elif ")
		p.stmts(e.Cond)
		p.str("; then ")
		p.stmts(e.Then)
	}
	if x.HasElse {
		p.str("; else ")
		p.stmts(x.Else)
	}
	p.str("; fi")
	p.redirs(x.Redirs)
}

func (p *printer) caseClause(x *CaseClause) {
	p.str("case ")
	p.word(x.Word)
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

func (p *printer) simple(c *SimpleCmd) {
	first := true
	sep := func() {
		if !first {
			p.str(" ")
		}
		first = false
	}
	for _, a := range c.Assigns {
		sep()
		p.assign(a)
	}
	for _, w := range c.Args {
		sep()
		p.word(w)
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
