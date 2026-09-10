// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package printer lays out a parsed script one way.
//
// The promise is docs/design.md's property 1: nothing is lost. Every token is
// emitted verbatim by its source extent — a word is never respelled — and the
// printer owns only what lies between tokens: indentation, keyword placement,
// operator spacing, statement separation, and where each recovered comment
// stands. Constructs the layout pass does not yet reshape are emitted as
// their whole source extent, which is safe rather than pretty.
package printer

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"

	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/walk"
)

// Format lays out f, which must be src's tree, with cs recovered from the
// same pair, arranged the way st says. The result ends in exactly one newline
// for non-empty input.
//
// st is the dialect's answer, from `dialect/<shell>.Style()`. A zero Style is
// not a usable one — its Indent is empty and its MaxBlankLines is zero — so a
// caller with no dialect in hand wants [syntax.CoreStyle].
func Format(src string, f *syntax.File, cs []comments.Comment, st syntax.Style) string {
	p := &printer{src: src, comments: cs, style: st}
	p.stmtList(f.Stmts, f.Last.Offset)
	p.ownLineComments(len(src) + 1)
	out := p.b.String()
	if st.AlignTrailingComments {
		out = alignTrailingComments(out)
	} else {
		out = strings.ReplaceAll(out, string(alignMark), " ")
	}
	if out == "" || p.rawTail {
		// The tail is a here-document body, verbatim; an unterminated one
		// runs to end of file and its blank lines are content.
		return out
	}
	return strings.TrimRight(out, "\n") + "\n"
}

type printer struct {
	src      string
	style    syntax.Style
	comments []comments.Comment
	ci       int
	b        strings.Builder
	indent   int
	lastLine int
	heredocs []*syntax.Redirect
	rawTail  bool
}

// alignMark stands where a trailing comment's separating space belongs. The
// printer streams; how far the comment moves right depends on lines it has
// not written yet, so the mark defers the answer to one aligning pass at the
// end. NUL, because a script that parses does not contain one.
const alignMark = '\x00'

// alignTrailingComments lines the trailing comments up: a run of consecutive
// lines that each carry one, at the same indent, gets its `#` in one column,
// one space past the run's longest code. A run of one keeps a single space.
func alignTrailingComments(s string) string {
	if !strings.ContainsRune(s, alignMark) {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines); {
		if !strings.ContainsRune(lines[i], alignMark) {
			i++
			continue
		}
		indent := leadingWhite(lines[i])
		j := i
		for j < len(lines) && strings.ContainsRune(lines[j], alignMark) &&
			leadingWhite(lines[j]) == indent {
			j++
		}
		width := 0
		for k := i; k < j; k++ {
			code := lines[k][:strings.IndexByte(lines[k], alignMark)]
			if w := utf8.RuneCountInString(code); w > width {
				width = w
			}
		}
		for k := i; k < j; k++ {
			at := strings.IndexByte(lines[k], alignMark)
			code, comment := lines[k][:at], lines[k][at+1:]
			pad := width - utf8.RuneCountInString(code) + 1
			lines[k] = code + strings.Repeat(" ", pad) + comment
		}
		i = j
	}
	return strings.Join(lines, "\n")
}

func leadingWhite(s string) string {
	for i := range len(s) {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i]
		}
	}
	return s
}

func (p *printer) node(n syntax.Node) {
	p.b.WriteString(p.src[n.Pos().Offset:n.End().Offset])
}

// pad writes the current indentation, one [syntax.Style.Indent] per level.
//
// The unit is the dialect's — two spaces everywhere measured, per
// docs/spec/style.md — and a <<- here-document body is unaffected by it
// whatever it says, because bodies are emitted verbatim. Which is also what
// keeps the tabs that make <<- strip anything at all.
func (p *printer) pad() {
	for range p.indent {
		p.b.WriteString(p.style.Indent)
	}
}

// newline ends an output line and immediately flushes any here-document
// bodies waiting on it, because that is where a shell reads them from.
func (p *printer) newline() {
	p.b.WriteByte('\n')
	p.rawTail = len(p.heredocs) > 0
	for _, r := range p.heredocs {
		body := p.src[r.Heredoc.Start.Offset:r.Heredoc.Stop.Offset]
		p.b.WriteString(body)
		// The newline that separates a body from its delimiter is the
		// printer's to restore — unless there is no delimiter, because the
		// body ran to the end of the file. Adding one there appends a byte
		// to the body itself, which is content: `"body"` became `"body\n"`
		// and the tree said so.
		if !strings.HasSuffix(body, "\n") && r.Heredoc.Stop.Offset < len(p.src) {
			p.b.WriteByte('\n')
		}
		if l := r.Heredoc.Stop.Line; l > p.lastLine {
			p.lastLine = l - 1 // Stop sits one past the delimiter's newline
		}
	}
	p.heredocs = p.heredocs[:0]
}

// blankGap keeps the blank lines the source had before an item starting on
// the given source line, capped at [syntax.Style.MaxBlankLines].
//
// Capped and not preserved: gofmt's rule, adopted. The `p.lastLine > 0` guard
// is what stops a blank line falling directly under an opener, where the gap
// measures the header's height rather than the author's paragraphing.
func (p *printer) blankGap(line int) {
	if p.lastLine == 0 {
		return
	}
	gap := line - p.lastLine - 1
	if gap > p.style.MaxBlankLines {
		gap = p.style.MaxBlankLines
	}
	for range gap {
		p.b.WriteByte('\n')
	}
}

// ownLineComments emits every not-yet-consumed comment that starts before
// limit, each on its own line at the current indent.
func (p *printer) ownLineComments(limit int) {
	for p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < limit {
		c := p.comments[p.ci]
		p.blankGap(c.Pos.Line)
		p.pad()
		p.b.WriteString(c.Text)
		p.lastLine = c.Pos.Line
		p.newline()
		p.ci++
	}
}

// skipComments drops queued comments before limit: they sit inside an extent
// that was just emitted verbatim, so they are already on the page.
func (p *printer) skipComments(limit int) {
	for p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < limit {
		p.ci++
	}
}

// stmtList is the workhorse: statements and comments interleaved by source
// position, statements the author put on one line kept on one line, blank
// runs capped at one.
func (p *printer) stmtList(list []*syntax.Stmt, closeOffset int) {
	for i := 0; i < len(list); {
		st := list[i]
		p.ownLineComments(st.Pos().Offset)
		p.blankGap(st.Pos().Line)
		// A run: statements chained on one line. The test is against the
		// previous statement's *end* — `a ||<newline> b; print` puts print
		// on b's line, and the `;` between them is the author's.
		j := i + 1
		for j < len(list) && list[j].Pos().Line == list[j-1].End().Line {
			j++
		}
		p.pad()
		for k := i; k < j; k++ {
			if k > i {
				if list[k-1].Background {
					p.b.WriteByte(' ')
				} else {
					p.b.WriteString("; ")
				}
			}
			p.stmt(list[k])
		}
		last := list[j-1]
		endLine := last.End().Line
		if p.ci < len(p.comments) && p.comments[p.ci].Pos.Line == endLine {
			p.b.WriteByte(alignMark)
			p.b.WriteString(p.comments[p.ci].Text)
			p.ci++
		}
		p.lastLine = endLine
		p.newline()
		i = j
	}
	p.ownLineComments(closeOffset)
}

// stmt prints one statement without indentation or terminator, which belongs
// to whoever placed it on a line.
func (p *printer) stmt(st *syntax.Stmt) {
	p.expr(st.Expr)
	switch {
	case st.Coprocess:
		p.b.WriteString(" |&")
	case st.Disown:
		// The two spellings are distinct tokens; the terminator's own two
		// bytes are the only record of which one was written.
		if st.Semi.Line > 0 && st.Semi.Offset+2 <= len(p.src) {
			p.b.WriteByte(' ')
			p.b.WriteString(p.src[st.Semi.Offset : st.Semi.Offset+2])
		} else {
			p.b.WriteString(" &!")
		}
	case st.Background:
		p.b.WriteString(" &")
	}
}

func (p *printer) expr(e syntax.Expr) {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		p.expr(x.X)
		p.b.WriteByte(' ')
		p.b.WriteString(x.Op.String())
		if x.Y.Pos().Line > x.OpPos.Line {
			p.newline()
			p.indent++
			p.pad()
			p.indent--
		} else {
			p.b.WriteByte(' ')
		}
		p.expr(x.Y)
	case *syntax.Pipeline:
		if x.Negated {
			p.b.WriteString("! ")
		}
		for i, c := range x.Cmds {
			if i > 0 {
				prev := x.Cmds[i-1]
				if mergesBoth(prev) {
					p.b.WriteString(" |&")
				} else {
					p.b.WriteString(" |")
				}
				if c.Pos().Line > prev.End().Line {
					p.newline()
					p.indent++
					p.pad()
					p.indent--
				} else {
					p.b.WriteByte(' ')
				}
			}
			p.command(c)
		}
	case *syntax.TimeClause:
		if x.Negated {
			p.b.WriteString("! ")
		}
		p.b.WriteString("time")
		if x.Posix {
			p.b.WriteString(" -p")
		}
		if x.Pipeline != nil {
			p.b.WriteByte(' ')
			p.expr(x.Pipeline)
		}
	}
}

// mergesBoth reports whether c ends in the redirection a `|&` stands for, so
// the pipeline can write the operator back instead of the redirection.
func mergesBoth(c syntax.Command) bool {
	if f, ok := c.(*syntax.FuncDecl); ok {
		return f.Body != nil && mergesBoth(f.Body)
	}
	rs := redirsOf(c)
	return len(rs) > 0 && rs[len(rs)-1].PipeBoth
}

// redirsOf reaches each command type's redirection list. The substrate keeps
// the accessor unexported, so the type switch lives here.
func redirsOf(c syntax.Command) []*syntax.Redirect {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		return x.Redirs
	case *syntax.Subshell:
		return x.Redirs
	case *syntax.Group:
		return x.Redirs
	case *syntax.TryClause:
		return x.Redirs
	case *syntax.IfClause:
		return x.Redirs
	case *syntax.LoopClause:
		return x.Redirs
	case *syntax.ForClause:
		return x.Redirs
	case *syntax.SelectClause:
		return x.Redirs
	case *syntax.AnonFunc:
		return x.Redirs
	case *syntax.RepeatClause:
		return x.Redirs
	case *syntax.CaseClause:
		return x.Redirs
	case *syntax.ForArithClause:
		return x.Redirs
	case *syntax.ArithCmdClause:
		return x.Redirs
	case *syntax.TestClause:
		return x.Redirs
	}
	return nil
}

func (p *printer) command(c syntax.Command) {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		p.simple(x)
	case *syntax.Group:
		p.group(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.Subshell:
		p.subshell(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.IfClause:
		p.ifClause(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.LoopClause:
		p.loop(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.ForClause:
		p.forClause(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.SelectClause:
		p.selectClause(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.CaseClause:
		p.caseClause(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.FuncDecl:
		p.funcDecl(x)
	case *syntax.ForArithClause:
		p.forArith(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.TestClause:
		// Verbatim, and nothing to skip: comment recovery already treats
		// the whole extent as opaque, so no queued comment can be inside.
		p.node(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.ArithCmdClause:
		p.node(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.AnonFunc:
		p.anonFunc(x)
	case *syntax.TryClause:
		p.tryClause(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.RepeatClause:
		p.repeatClause(x)
		p.suffixRedirs(x.Redirs)
	case *syntax.CoprocClause:
		p.b.WriteString("coproc ")
		if x.Name != "" {
			p.b.WriteString(x.Name)
			p.b.WriteByte(' ')
		}
		p.command(x.Cmd)
	default:
		// Nothing reaches here today; a construct the layout pass does not
		// know is still emitted whole rather than wrong.
		p.verbatim(c)
	}
}

// verbatim emits a command's whole extent as written, consumes the comments
// inside it, and queues any here-document whose body follows the extent.
func (p *printer) verbatim(c syntax.Command) {
	p.node(c)
	end := c.End().Offset
	p.skipComments(end)
	walk.Nodes(c, func(n syntax.Node) bool {
		if r, ok := n.(*syntax.Redirect); ok && r.Heredoc != nil &&
			r.Heredoc.Start.Offset >= end {
			p.heredocs = append(p.heredocs, r)
		}
		return true
	})
	p.suffixRedirs(afterOnly(redirsOf(c), end))
}

// tryClause is `{ … } always { … }`, whose braces are the construct's own
// punctuation rather than nested groups.
func (p *printer) tryClause(x *syntax.TryClause) {
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("{ ")
		p.inlineStmts(x.Try)
		p.headerKeyword(x.Try, "} always { ")
		p.inlineStmts(x.Always)
		p.headerKeyword(x.Always, "}")
		return
	}
	tryClose := x.Stop.Offset
	if len(x.Always) > 0 {
		tryClose = x.Always[0].Pos().Offset
	}
	p.b.WriteString("{")
	p.newline()
	p.block(x.Try, tryClose)
	p.b.WriteString("} always {")
	p.newline()
	p.block(x.Always, x.Stop.Offset)
	p.b.WriteString("}")
}

// repeatClause always writes the `do … done` spelling; the dialect's shorter
// forms parse to the same tree.
func (p *printer) repeatClause(x *syntax.RepeatClause) {
	if b := p.shortForm(x.Start.Offset, x.Count.End().Offset, bodyStart(x.Body, x.Stop.Offset)); b >= 0 {
		p.braceClause(x.Start.Offset, b, x.Body, x.Stop.Offset, oneLine(x.Start, x.Stop))
		return
	}
	p.b.WriteString("repeat ")
	p.node(x.Count)
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("; do ")
		p.inlineStmts(x.Body)
		p.headerKeyword(x.Body, "done")
		return
	}
	p.doKeyword()
	p.newline()
	p.block(x.Body, x.Stop.Offset)
	p.b.WriteString("done")
}

// anonFunc lays out the header, the body as an ordinary command, and then
// what the node's extent cannot hold — an anonymous function's extent ends
// at its body, so the call's arguments and any redirections written after it
// come from the fields, in source order.
func (p *printer) anonFunc(x *syntax.AnonFunc) {
	end := x.End().Offset
	p.b.WriteString(p.headerText(x.Pos().Offset, x.Body.Pos().Offset))
	p.b.WriteByte(' ')
	p.command(x.Body)
	type piece struct {
		from int
		w    *syntax.Word
		r    *syntax.Redirect
	}
	var pieces []piece
	for _, w := range x.Args {
		pieces = append(pieces, piece{from: w.Start.Offset, w: w})
	}
	for _, r := range afterOnly(x.Redirs, end) {
		if !r.PipeBoth {
			pieces = append(pieces, piece{from: r.Pos().Offset, r: r})
		}
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].from < pieces[j].from })
	for _, pc := range pieces {
		p.b.WriteByte(' ')
		if pc.r != nil {
			p.redirect(pc.r)
		} else {
			p.node(pc.w)
		}
	}
}

// headerText is a declaration's header — the source between a node's start
// and its body — squeezed to one spaced line. A comment can sit in that span;
// it stays queued for whoever flushes next, so it must not be squashed into
// the header here.
func (p *printer) headerText(from, to int) string {
	header := p.src[from:to]
	for i := p.ci; i < len(p.comments); i++ {
		c := p.comments[i]
		if c.Pos.Offset >= to {
			break
		}
		if c.Pos.Offset >= from {
			header = strings.Replace(header, c.Text, "", 1)
		}
	}
	return squeezedHeader(header)
}

// squeezedHeader is a header's text on one line: every run of whitespace that
// *separates* words becomes one space, and every other byte is kept.
//
// The separating half is what strings.Fields used to do here, and the other
// half is why it cannot do it any more. A name may hold whitespace — see
// [syntax.Dialect.FunctionNameIsAnyWord] — and Fields cannot see that it is
// held rather than separating, so it flattened whatever it found:
//
//	a\<tab>b() { … }    came back `a\ b() { … }`, an escaped *space*
//	'a  b'() { … }      came back `'a b'()`, one space where two were
//
// Both of those are a formatted file that defines a different function from
// the one it was made from, silently, at status 0 — and the file that reaches
// it is any zsh startup, where a plugin quotes a widget's name with
// `${(q)…}` and the widget's name has blanks in it.
//
// Quoting is tracked byte by byte because that is the only thing separating
// the two readings; there is no node to take an extent from, a name written
// as literal text having none.
//
// A line continuation is *removed* rather than turned into a space. It is not
// part of any word and it separates nothing: `function man \` newline
// `<tab>dman { … }` is two names either way, and the blank before the
// backslash is what parts them. Turning it into a space instead — which is
// what stood here — made the backslash a word of its own that Fields then
// joined to the next with a space, so the header came back as `function man
// \ dman` and the names that reparsed from it were ` dman` and ` debman`.
func squeezedHeader(s string) string {
	var b []byte
	var quote byte
	sep := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Outside single quotes a backslash-newline is a continuation, which
		// the shell removes before words are formed.
		if quote != '\'' && c == '\\' {
			if n := continuationAt(s, i); n > 0 {
				i += n - 1
				continue
			}
		}
		if quote == 0 && isHeaderSpace(c) {
			// Leading whitespace goes; anything else marks a separator that
			// is written only if a word follows it, which is what leaves no
			// trailing space either.
			sep = len(b) > 0
			continue
		}
		if sep {
			b = append(b, ' ')
			sep = false
		}
		switch {
		case quote == '\'':
			// Nothing is special inside single quotes, not even a backslash.
			b = append(b, c)
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			// The escape and the byte it escapes are one unit, and neither of
			// them separates anything.
			b = append(b, c)
			if i+1 < len(s) {
				i++
				b = append(b, s[i])
			}
		case quote == '"':
			b = append(b, c)
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
			b = append(b, c)
		default:
			b = append(b, c)
		}
	}
	return string(b)
}

// continuationAt reports the length of the line continuation beginning at the
// backslash at i, or 0 if what follows is not a line break.
func continuationAt(s string, i int) int {
	switch {
	case i+1 < len(s) && s[i+1] == '\n':
		return 2
	case i+2 < len(s) && s[i+1] == '\r' && s[i+2] == '\n':
		return 3
	}
	return 0
}

// isHeaderSpace is the whitespace strings.Fields treated as separating, which
// is what this keeps answering for the bytes that really do separate.
func isHeaderSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// afterOnly keeps redirections at or past end: the ones a verbatim slice did
// not already include.
func afterOnly(rs []*syntax.Redirect, end int) []*syntax.Redirect {
	var out []*syntax.Redirect
	for _, r := range rs {
		if r.Pos().Offset >= end {
			out = append(out, r)
		}
	}
	return out
}

// simple prints assignments, arguments and redirections back in the order
// they were written, single-spaced, each verbatim.
func (p *printer) simple(x *syntax.SimpleCmd) {
	type piece struct {
		from int
		r    *syntax.Redirect
		n    syntax.Node
	}
	var pieces []piece
	for _, a := range x.Assigns {
		pieces = append(pieces, piece{from: a.Start.Offset, n: a})
	}
	for _, w := range x.Args {
		pieces = append(pieces, piece{from: w.Start.Offset, n: w})
	}
	for _, r := range x.Redirs {
		if r.PipeBoth {
			continue // spelled `|&` by the pipeline, not by us
		}
		pieces = append(pieces, piece{from: r.Pos().Offset, r: r})
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].from < pieces[j].from })
	prevEnd := 0
	for i, pc := range pieces {
		node := pc.n
		if pc.r != nil {
			node = pc.r
		}
		if i > 0 {
			if node.Pos().Line > prevEnd {
				// The author split the list with backslash-newlines; keep
				// each element on its own line rather than rebuilding one
				// long one. A raw newline, not p.newline(): a continuation
				// is not a newline token, so no here-document starts here.
				p.b.WriteString(" \\\n")
				p.indent++
				p.pad()
				p.indent--
			} else {
				p.b.WriteByte(' ')
			}
		}
		if pc.r != nil {
			p.redirect(pc.r)
		} else {
			p.node(pc.n)
		}
		prevEnd = node.End().Line
	}
}

func (p *printer) redirect(r *syntax.Redirect) {
	if r.N != nil {
		p.node(r.N)
	}
	p.b.WriteString(r.Op.String())
	if r.Word != nil {
		// `< <(cmd)` must not fuse into `<<(cmd)`: when the target's first
		// byte would extend the operator's token, the space is structure.
		if s := p.src[r.Word.Start.Offset]; s == '<' || s == '>' || s == '(' {
			p.b.WriteByte(' ')
		}
		p.node(r.Word)
	}
	if r.Heredoc != nil {
		p.heredocs = append(p.heredocs, r)
	}
}

func (p *printer) suffixRedirs(rs []*syntax.Redirect) {
	for _, r := range rs {
		if r.PipeBoth {
			continue
		}
		p.b.WriteByte(' ')
		p.redirect(r)
	}
}

// inlineStmts prints a list on the current line, `; `-separated, the way a
// clause header or a one-line group wants it.
func (p *printer) inlineStmts(list []*syntax.Stmt) {
	for i, st := range list {
		if i > 0 {
			if list[i-1].Background {
				p.b.WriteByte(' ')
			} else {
				p.b.WriteString("; ")
			}
		}
		p.stmt(st)
	}
}

// seqStmts prints a header's statement list keeping the author's separators:
// same-line statements stay `; `-joined and a statement on a new line keeps
// its newline, one continuation level in. The two spellings are recorded
// distinctly in the tree, so a formatter must hand back the one it was given.
// Comments between the statements ride along at the continuation indent.
func (p *printer) seqStmts(list []*syntax.Stmt) {
	for i, st := range list {
		if i > 0 {
			prev := list[i-1]
			switch {
			case st.Pos().Line > prev.End().Line:
				if p.ci < len(p.comments) &&
					p.comments[p.ci].Pos.Line == prev.End().Line &&
					p.comments[p.ci].Pos.Offset < st.Pos().Offset {
					p.b.WriteByte(alignMark)
					p.b.WriteString(p.comments[p.ci].Text)
					p.ci++
				}
				p.newline()
				p.indent++
				for p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < st.Pos().Offset {
					p.pad()
					p.b.WriteString(p.comments[p.ci].Text)
					p.newline()
					p.ci++
				}
				p.pad()
				p.indent--
			case prev.Background:
				p.b.WriteByte(' ')
			default:
				p.b.WriteString("; ")
			}
		}
		p.stmt(st)
	}
}

// headerKeyword closes a header list with its keyword: `; then` ordinarily,
// ` then` after `&`, which is already a terminator and takes no `;`.
func (p *printer) headerKeyword(list []*syntax.Stmt, kw string) {
	if n := len(list); n > 0 && list[n-1].Background {
		p.b.WriteString(" " + kw)
		return
	}
	p.b.WriteString("; " + kw)
}

// bodyKeyword closes a header with the keyword that opens its body, on the
// header's line or on a line of its own, as the style says.
//
// Every dialect measured answers "on the header's line" — by 13:1 or better,
// and the Google guide states it outright — so the other branch is unreached
// today. It is here because the question is the dialect's to answer and a
// constant is a question nobody can reopen. See docs/spec/style.md.
func (p *printer) bodyKeyword(list []*syntax.Stmt, kw string, onHeaderLine bool) {
	if onHeaderLine {
		p.headerKeyword(list, kw)
		return
	}
	p.newline()
	p.pad()
	p.b.WriteString(kw)
}

// doKeyword writes the `do` of a loop whose header is not a statement list —
// `for`, `select` and the arithmetic `for`, whose headers are words.
func (p *printer) doKeyword() {
	if p.style.DoOnHeaderLine {
		p.b.WriteString("; do")
		return
	}
	p.newline()
	p.pad()
	p.b.WriteString("do")
}

// block prints a statement list one level in, ending at closeOffset. The
// line tracker is reset so the first item never draws a blank line under the
// opener it belongs to — the source line gap there measures the header's
// height, not the author's paragraphing.
func (p *printer) block(list []*syntax.Stmt, closeOffset int) {
	p.indent++
	p.lastLine = 0
	p.stmtList(list, closeOffset)
	p.indent--
	p.pad()
}

func oneLine(from, to syntax.Pos) bool { return from.Line == to.Line }

// hasNewlineInAPattern reports whether any `case` pattern's own extent spans
// a line break, which the layout pass cannot reproduce.
// armNeedsParen reports whether a `case` arm's opening `(` is load-bearing.
//
// The paren is ordinarily optional and this printer drops it. It stops being
// optional where a pattern holds a *bare* blank: that is a grammar one
// dialect has only inside the parentheses — see
// [syntax.Dialect.CasePatternListSpansBlanks] — so `(a b)` written back as
// `a b)` is a parse error, and `((x) y)` written back as `(x) y)` is worse,
// being a program that parses to a different one. `VCS_INFO_get_data_git` is
// written with both shapes.
//
// Read off the spans rather than off the source text, so that a blank written
// *quoted* — `("a b")`, which every shell reads without any paren at all —
// keeps the layout it had.
func armNeedsParen(it *syntax.CaseItem) bool {
	for _, w := range it.Patterns {
		for _, sp := range w.Spans {
			if sp.Kind != syntax.Literal || sp.Quoting != syntax.Unquoted {
				continue
			}
			if strings.ContainsAny(sp.Value, " \t") {
				return true
			}
		}
	}
	return false
}

func hasNewlineInAPattern(src string, x *syntax.CaseClause) bool {
	for _, it := range x.Items {
		for _, pat := range it.Patterns {
			from, to := pat.Pos().Offset, pat.End().Offset
			if to <= len(src) && from < to && strings.Contains(src[from:to], "\n") {
				return true
			}
		}
	}
	return false
}

// armEnd is where a `case` arm stops. [syntax.CaseItem.End] is its
// terminator's position, which a final arm need not have, so a missing one
// falls back to the body it would have followed.
func armEnd(it *syntax.CaseItem) syntax.Pos {
	if strings.HasPrefix(it.Term.String(), ";") {
		return it.TermPos
	}
	if n := len(it.Body); n > 0 {
		return it.Body[n-1].End()
	}
	return it.Start
}

func (p *printer) group(x *syntax.Group) {
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("{ ")
		p.inlineStmts(x.List)
		if n := len(x.List); n == 0 || !x.List[n-1].Background {
			p.b.WriteByte(';')
		}
		p.b.WriteString(" }")
		return
	}
	p.b.WriteString("{")
	p.newline()
	p.block(x.List, x.Stop.Offset)
	p.b.WriteString("}")
}

func (p *printer) subshell(x *syntax.Subshell) {
	if oneLine(x.Start, x.Stop) {
		p.b.WriteByte('(')
		// `( ((x)) )` must not close up: `(((` opens arithmetic, and a
		// trailing `))` closes it. The guard is the first inner byte and
		// the last one written.
		if len(x.List) > 0 && p.src[x.List[0].Pos().Offset] == '(' {
			p.b.WriteByte(' ')
		}
		p.inlineStmts(x.List)
		if strings.HasSuffix(p.b.String(), ")") {
			p.b.WriteByte(' ')
		}
		p.b.WriteByte(')')
		return
	}
	p.b.WriteString("(")
	p.newline()
	p.block(x.List, x.Stop.Offset)
	p.b.WriteString(")")
}

func (p *printer) ifClause(x *syntax.IfClause) {
	if p.ifShortForm(x) {
		return
	}
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("if ")
		p.inlineStmts(x.Cond)
		p.headerKeyword(x.Cond, "then ")
		p.inlineStmts(x.Then)
		for _, e := range x.Elifs {
			p.b.WriteString("; elif ")
			p.inlineStmts(e.Cond)
			p.headerKeyword(e.Cond, "then ")
			p.inlineStmts(e.Then)
		}
		if x.HasElse {
			p.b.WriteString("; else ")
			p.inlineStmts(x.Else)
		}
		p.b.WriteString("; fi")
		return
	}
	p.b.WriteString("if ")
	p.seqStmts(x.Cond)
	p.bodyKeyword(x.Cond, "then", p.style.ThenOnHeaderLine)
	p.newline()
	then := x.Then
	for _, e := range x.Elifs {
		p.block(then, e.Start.Offset)
		p.b.WriteString("elif ")
		p.seqStmts(e.Cond)
		p.bodyKeyword(e.Cond, "then", p.style.ThenOnHeaderLine)
		p.newline()
		then = e.Then
	}
	if x.HasElse {
		limit := x.Stop.Offset
		if len(x.Else) > 0 {
			limit = x.Else[0].Pos().Offset
		}
		p.block(then, limit)
		p.b.WriteString("else")
		p.newline()
		p.block(x.Else, x.Stop.Offset)
	} else {
		p.block(then, x.Stop.Offset)
	}
	p.b.WriteString("fi")
}

func (p *printer) loop(x *syntax.LoopClause) {
	kw := "while"
	if x.Until {
		kw = "until"
	}
	if b := p.shortForm(x.Start.Offset, condEnd(x.Cond, x.Start.Offset), bodyStart(x.Body, x.Stop.Offset)); b >= 0 {
		p.braceClause(x.Start.Offset, b, x.Body, x.Stop.Offset, oneLine(x.Start, x.Stop))
		return
	}
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString(kw + " ")
		p.inlineStmts(x.Cond)
		p.headerKeyword(x.Cond, "do ")
		p.inlineStmts(x.Body)
		p.headerKeyword(x.Body, "done")
		return
	}
	p.b.WriteString(kw + " ")
	p.seqStmts(x.Cond)
	p.bodyKeyword(x.Cond, "do", p.style.DoOnHeaderLine)
	p.newline()
	p.block(x.Body, x.Stop.Offset)
	p.b.WriteString("done")
}

func (p *printer) forHeader(x *syntax.ForClause) {
	p.b.WriteString("for ")
	if x.RefusedName != "" {
		p.b.WriteString(x.RefusedName)
	} else {
		p.b.WriteString(strings.Join(x.Names, " "))
	}
	if x.HasItems {
		p.b.WriteString(" in")
		for _, w := range x.Items {
			p.b.WriteByte(' ')
			p.node(w)
		}
	}
}

func (p *printer) forClause(x *syntax.ForClause) {
	if b := p.shortForm(x.Start.Offset, itemsEnd(x.Items, x.Start.Offset), bodyStart(x.Body, x.Stop.Offset)); b >= 0 {
		p.braceClause(x.Start.Offset, b, x.Body, x.Stop.Offset, oneLine(x.Start, x.Stop))
		return
	}
	p.forHeader(x)
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("; do ")
		p.inlineStmts(x.Body)
		p.headerKeyword(x.Body, "done")
		return
	}
	p.doKeyword()
	p.newline()
	p.block(x.Body, x.Stop.Offset)
	p.b.WriteString("done")
}

func (p *printer) selectClause(x *syntax.SelectClause) {
	if b := p.shortForm(x.Start.Offset, itemsEnd(x.Items, x.Start.Offset), bodyStart(x.Body, x.Stop.Offset)); b >= 0 {
		p.braceClause(x.Start.Offset, b, x.Body, x.Stop.Offset, oneLine(x.Start, x.Stop))
		return
	}
	p.b.WriteString("select ")
	if x.RefusedName != "" {
		p.b.WriteString(x.RefusedName)
	} else {
		p.b.WriteString(x.Name)
	}
	if x.HasItems {
		p.b.WriteString(" in")
		for _, w := range x.Items {
			p.b.WriteByte(' ')
			p.node(w)
		}
	}
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("; do ")
		p.inlineStmts(x.Body)
		p.headerKeyword(x.Body, "done")
		return
	}
	p.doKeyword()
	p.newline()
	p.block(x.Body, x.Stop.Offset)
	p.b.WriteString("done")
}

func (p *printer) forArith(x *syntax.ForArithClause) {
	header := p.src[x.Start.Offset : x.Start.Offset+len(x.Header)]
	p.b.WriteString(header)
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("; do ")
		p.inlineStmts(x.Body)
		p.headerKeyword(x.Body, "done")
		return
	}
	p.doKeyword()
	p.newline()
	p.block(x.Body, x.Stop.Offset)
	p.b.WriteString("done")
}

func (p *printer) caseClause(x *syntax.CaseClause) {
	if hasNewlineInAPattern(p.src, x) {
		// A newline *inside* a pattern list — `case a in (a\n|b) …` — is
		// joined into the alternative it follows, so the pattern's own text
		// holds the newline. Re-flowing the list would move that newline and
		// the output would not parse. The whole clause goes out as written,
		// which is safe rather than pretty, and is what verbatim is for.
		p.verbatim(x)
		return
	}
	if oneLine(x.Start, x.Stop) {
		p.b.WriteString("case ")
		p.node(x.Word)
		p.b.WriteString(" in")
		for _, it := range x.Items {
			p.b.WriteByte(' ')
			if armNeedsParen(it) {
				p.b.WriteByte('(')
			}
			for i, pat := range it.Patterns {
				if i > 0 {
					p.b.WriteString(" | ")
				}
				p.node(pat)
			}
			p.b.WriteByte(')')
			if len(it.Body) > 0 {
				p.b.WriteByte(' ')
				p.inlineStmts(it.Body)
			}
			if s := it.Term.String(); it.TermPos.Line > 0 && strings.HasPrefix(s, ";") {
				p.b.WriteString(" " + s)
			} else {
				// The last arm may end at esac with no terminator; a `;`
				// still has to close its body before the keyword.
				p.b.WriteByte(';')
			}
		}
		p.b.WriteString(" esac")
		return
	}
	p.b.WriteString("case ")
	p.node(x.Word)
	p.b.WriteString(" in")
	p.newline()
	p.indent++
	p.lastLine = 0
	for _, it := range x.Items {
		p.ownLineComments(it.Start.Offset)
		p.blankGap(it.Start.Line)
		p.pad()
		if armNeedsParen(it) {
			p.b.WriteByte('(')
		}
		for i, pat := range it.Patterns {
			if i > 0 {
				p.b.WriteString(" | ")
			}
			p.node(pat)
		}
		p.b.WriteByte(')')
		// A final arm need not carry a terminator: POSIX lets the last one
		// run to `esac`, and bash, zsh and dash all accept it. Writing one
		// in would be inventing source the author did not write — and the
		// arm's own End() is its terminator's position, so without this the
		// arm also measures as spanning no lines and gets exploded into a
		// block. Both were live until the corpus sweep compared programs
		// with syntax.SameProgram instead of two canonical prints.
		// The presence test is Term and not TermPos: an arm that ends at
		// `esac` still carries a TermPos, pointing at the keyword, so
		// TermPos answers "where would it be" rather than "is there one".
		term := it.Term.String()
		hasTerm := strings.HasPrefix(term, ";")
		end := armEnd(it)
		if oneLine(it.Start, end) {
			if len(it.Body) > 0 {
				p.b.WriteByte(' ')
				p.inlineStmts(it.Body)
			}
			if hasTerm {
				p.b.WriteString(" " + term)
			}
			p.lastLine = end.Line
			p.newline()
			continue
		}
		p.newline()
		p.indent++
		p.lastLine = 0
		p.stmtList(it.Body, end.Offset)
		if hasTerm {
			p.pad()
			p.b.WriteString(term)
		}
		p.indent--
		if l := end.Line; l > 0 {
			p.lastLine = l
		}
		if hasTerm {
			// Without a terminator the body's own last line already ended,
			// and a second newline here would open a blank line before esac.
			p.newline()
		}
	}
	p.ownLineComments(x.Stop.Offset)
	p.indent--
	p.pad()
	p.b.WriteString("esac")
}

// funcDecl normalizes the header's spacing and keeps its dialect's spelling:
// `function f { }` stays keyword-only where that is what was written, because
// one dialect rejects the hybrid form.
func (p *printer) funcDecl(x *syntax.FuncDecl) {
	if x.Body == nil {
		p.node(x)
		return
	}
	p.b.WriteString(p.headerText(x.Pos().Offset, x.Body.Pos().Offset))
	p.b.WriteByte(' ')
	p.command(x.Body)
}
