// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Bodies the author spelled with braces.
//
// zsh alone writes `if cond { … }`, `for f ( a b c ) { … }`, `while cond
// { … }`, `repeat n { … }` and `select f ( a b ) { … }`, and every one of them
// parses to exactly the tree its keyword spelling parses to. No AST field
// separates them — so the tree cannot say which was written, and a printer
// working from the tree alone rewrites one into the other silently, on some
// five hundred lines of a real zsh tree.
//
// The source can say, though, and the printer is already reading it for every
// token. So the spelling is recovered the same way everything else here is:
// by looking at the bytes. [syntax.Style.BraceShortForm] decides whether that
// is done at all, and only zsh asks for it — no other dialect can parse a
// brace body, so in those the question never arises.
//
// Measured from zsh 5.9.2, and one of these is not what it looks like:
//
//	if c { … } elif c { … } else { … }    parses
//	for f ( a b c ) { … }                 parses
//	for f in a b c { … }                  PARSE ERROR
//
// The brace body and the parenthesized item list are one spelling, not two
// independent choices, which is why a `for` header is written back from its
// source extent rather than rebuilt from Names and Items.

// shortForm returns the offset of the `{` opening a brace-spelled body
// between from and to, or -1 when the body was not spelled that way, the
// style does not preserve it, or the header cannot be written back.
//
// headerFrom is where the construct starts, and it is here for the last of
// those: a comment inside the header makes the brace spelling unwritable,
// because the `{` would land after a `#` and be swallowed. The keyword layout
// is the fallback, and it is correct for every input — just not the author's.
func (p *printer) shortForm(headerFrom, from, to int) int {
	if p.style.BraceShortForm != syntax.PreserveShortForm {
		return -1
	}
	if to > len(p.src) {
		to = len(p.src)
	}
	for i := from; i < to; i++ {
		switch p.src[i] {
		case ' ', '\t', '\n', '\r', ';':
			// Separators, skipped.
		case ')':
			// The close of a `for`/`select` parenthesized item list, which
			// sits outside the last item's own extent. Nothing else puts a
			// `)` between a header's end and its body's first statement:
			// a subshell condition ends *inside* the condition's extent.
		case '{':
			if p.commentIn(headerFrom, i) {
				return -1
			}
			return i
		default:
			// A keyword — `then`, `do` — or anything else at all. Either
			// way the body was not spelled with a brace.
			return -1
		}
	}
	return -1
}

// commentIn reports whether a not-yet-emitted comment lies in the span.
func (p *printer) commentIn(from, to int) bool {
	for i := p.ci; i < len(p.comments); i++ {
		c := p.comments[i]
		if int(c.Pos.Offset) >= to {
			return false
		}
		if int(c.Pos.Offset) >= from {
			return true
		}
	}
	return false
}

// headerSource is a short form's header exactly as written, less only the
// whitespace that ran up to the brace.
//
// Deliberately not [printer.headerText], which squeezes every run of
// whitespace to a single space. That is right for a function declaration's
// `f  ()` and wrong here: a short form's header holds a condition or an
// arithmetic expression whose interior text the *tree* keeps, so squeezing
// `(( a >= 1  ))` to one space changes ArithCmdClause.Expr and the output is
// a different program. One script in the corpus sweep said so.
func (p *printer) headerSource(from, to int) string {
	// The `;` is kept, and that is not cosmetic. There are two brace-body
	// spellings, not one: zsh's takes no separator (`for f ( a b c ) { … }`)
	// and the core's requires one (`for i in a b; { … }`) — the corpus pins
	// `for i in a b { … }` as a syntax error in its own right. Trimming the
	// separator turned the second into the first and made the output
	// unparseable, which is what the corpus gate reported.
	return strings.TrimRight(p.src[from:to], " \t\n\r")
}

// braceClause writes `HEADER { … }`, on one line where the author wrote it on
// one. The header is its own source extent, which is what keeps
// `for f ( a b c )` intact.
func (p *printer) braceClause(headerFrom, brace int, body []*syntax.Stmt, closeOffset int, single bool) {
	p.b.WriteString(p.headerSource(headerFrom, brace))
	if single {
		p.b.WriteString(" {")
		if len(body) > 0 {
			p.b.WriteByte(' ')
			p.inlineStmts(body)
			// The third place the two spellings differ. The core's brace
			// body is a *group*, so its last statement needs a terminator
			// before the `}` — `for i in a b; { printf x; }` — and zsh's
			// clause body does not take one: `repeat 3 { echo hi }`. Read
			// from the source, like everything else here, rather than
			// decided from the dialect.
			if p.separatorBeforeClose(body) {
				p.b.WriteByte(';')
			}
		}
		p.b.WriteString(" }")
		return
	}
	p.b.WriteString(" {")
	p.newline()
	p.block(body, closeOffset)
	p.b.WriteString("}")
}

// separatorBeforeClose reports whether the source wrote a `;` between the
// body's last statement and the `}` that closes it.
func (p *printer) separatorBeforeClose(body []*syntax.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	semi := false
	for i := int(body[len(body)-1].End().Offset); i < len(p.src); i++ {
		switch p.src[i] {
		case ' ', '\t', '\n', '\r':
		case ';':
			semi = true
		case '}':
			return semi
		default:
			return false
		}
	}
	return false
}

// condEnd is where a clause's condition list stops, which is where the search
// for its body's brace begins.
func condEnd(cond []*syntax.Stmt, fallback int) int {
	if n := len(cond); n > 0 {
		return int(cond[n-1].End().Offset)
	}
	return fallback
}

// bodyStart is the upper bound of that search: the first statement of the
// body, or where the body would have been had it any statements.
func bodyStart(body []*syntax.Stmt, fallback int) int {
	if len(body) > 0 {
		return int(body[0].Pos().Offset)
	}
	return fallback
}

// ifShortForm lays out an `if` whose bodies were spelled with braces, chain
// and all. It reports false when any link of the chain was not, in which case
// the keyword layout runs instead — a half-converted chain is not a spelling
// anything wrote.
func (p *printer) ifShortForm(x *syntax.IfClause) bool {
	// Where each body's search ends: the next link of the chain.
	thenLimit := x.Stop.Offset
	switch {
	case len(x.Elifs) > 0:
		thenLimit = x.Elifs[0].Start.Offset
	case x.HasElse && len(x.Else) > 0:
		thenLimit = x.Else[0].Pos().Offset
	}
	brace := p.shortForm(int(x.Start.Offset), condEnd(x.Cond, int(x.Start.Offset)), bodyStart(x.Then, int(thenLimit)))
	if brace < 0 {
		return false
	}
	type link struct {
		e     *syntax.Elif
		brace int
		limit int
	}
	links := make([]link, 0, len(x.Elifs))
	for i, e := range x.Elifs {
		limit := x.Stop.Offset
		switch {
		case i+1 < len(x.Elifs):
			limit = x.Elifs[i+1].Start.Offset
		case x.HasElse && len(x.Else) > 0:
			limit = x.Else[0].Pos().Offset
		}
		b := p.shortForm(int(e.Start.Offset), condEnd(e.Cond, int(e.Start.Offset)), bodyStart(e.Then, int(limit)))
		if b < 0 {
			return false
		}
		links = append(links, link{e: e, brace: b, limit: int(limit)})
	}

	single := oneLine(x.Start, x.Stop)
	if single {
		p.braceClause(int(x.Start.Offset), brace, x.Then, int(thenLimit), true)
		for _, l := range links {
			p.b.WriteByte(' ')
			p.braceClause(int(l.e.Start.Offset), l.brace, l.e.Then, l.limit, true)
		}
		if x.HasElse {
			p.b.WriteString(" else {")
			if len(x.Else) > 0 {
				p.b.WriteByte(' ')
				p.inlineStmts(x.Else)
			}
			p.b.WriteString(" }")
		}
		return true
	}

	p.b.WriteString(p.headerSource(int(x.Start.Offset), brace))
	p.b.WriteString(" {")
	p.newline()
	p.block(x.Then, int(thenLimit))
	for _, l := range links {
		p.b.WriteString("} ")
		p.b.WriteString(p.headerSource(int(l.e.Start.Offset), l.brace))
		p.b.WriteString(" {")
		p.newline()
		p.block(l.e.Then, l.limit)
	}
	if x.HasElse {
		p.b.WriteString("} else {")
		p.newline()
		p.block(x.Else, int(x.Stop.Offset))
	}
	p.b.WriteString("}")
	return true
}

// itemsEnd is where a `for` or `select` item list stops. The closing `)` of a
// parenthesized list sits outside the last item's extent, which is why
// [printer.shortForm] steps over one.
func itemsEnd(items []*syntax.Word, fallback int) int {
	if n := len(items); n > 0 {
		return int(items[n-1].End().Offset)
	}
	return fallback
}
