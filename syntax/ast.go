// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// The syntax tree, shaped by docs/spec/grammar/commands.md. Three of its
// decisions are consequences of measurements rather than of taste, and each is
// noted where it appears:
//
//   - Every compound command carries its own redirections.
//   - A simple command's redirections are lifted out of the word list, because
//     they may appear anywhere among the arguments.
//   - `for` distinguishes an absent word list from an empty one.

// Node is anything in the tree.
type Node interface {
	Pos() Pos
	End() Pos
}

// File is a whole parsed input.
type File struct {
	Stmts []*Stmt
	Last  Pos
}

func (f *File) Pos() Pos {
	if len(f.Stmts) == 0 {
		return f.Last
	}
	return f.Stmts[0].Pos()
}
func (f *File) End() Pos { return f.Last }

// Stmt is one entry in a list: an and-or expression and how it was terminated.
type Stmt struct {
	Expr Expr
	// Background is set when the statement ended with `&` rather than `;` or
	// a newline. It belongs to the statement rather than to the command,
	// because `a && b &` backgrounds the whole and-or.
	Background bool
	Semi       Pos
}

func (s *Stmt) Pos() Pos { return s.Expr.Pos() }
func (s *Stmt) End() Pos {
	if s.Semi.IsValid() {
		return s.Semi
	}
	return s.Expr.End()
}

// Expr is an and-or expression: either a single pipeline or a binary tree of
// them.
type Expr interface {
	Node
	exprNode()
}

// BinaryExpr is `x && y` or `x || y`.
//
// The two operators share one precedence level and associate left, so
// `a || b && c` parses as `(a || b) && c`. Giving `&&` a tighter binding is
// C's rule and produces no output for `true || echo A && echo B`, which every
// shell prints B for.
type BinaryExpr struct {
	X     Expr
	Op    Kind // TokAndAnd or TokOrOr
	OpPos Pos
	Y     Expr
}

func (b *BinaryExpr) Pos() Pos  { return b.X.Pos() }
func (b *BinaryExpr) End() Pos  { return b.Y.End() }
func (b *BinaryExpr) exprNode() {}

// Pipeline is one or more commands joined by `|`.
type Pipeline struct {
	// Negated records a leading `!`, which applies to the whole pipeline
	// rather than to its first command: `! true | false` exits 0.
	Negated bool
	Bang    Pos
	Cmds    []Command
}

func (p *Pipeline) Pos() Pos {
	if p.Negated {
		return p.Bang
	}
	return p.Cmds[0].Pos()
}
func (p *Pipeline) End() Pos  { return p.Cmds[len(p.Cmds)-1].End() }
func (p *Pipeline) exprNode() {}

// Command is a simple command, a compound command, or a function definition.
type Command interface {
	Node
	commandNode()
}

// Word is one word: a sequence of spans, because quoting is recorded per span
// and not per word.
type Word struct {
	Spans []Span
	Start Pos
	Stop  Pos
}

func (w *Word) Pos() Pos { return w.Start }
func (w *Word) End() Pos { return w.Stop }

// IsQuoted reports whether any span of the word was quoted. A word can be
// partly quoted, so this is not "the word was written in quotes" — and for a
// condition's right operand it is what separates a pattern from a literal.
func (w *Word) IsQuoted() bool {
	if w == nil {
		return false
	}
	for _, s := range w.Spans {
		if s.Quoting != Unquoted {
			return true
		}
	}
	return false
}

// Literal joins the spans, which is the word with its quote characters
// removed and nothing else done. Meaningful only where no expansion applies.
func (w *Word) Literal() string {
	if w == nil {
		return ""
	}
	var b []byte
	for _, s := range w.Spans {
		b = append(b, s.Value...)
	}
	return string(b)
}

// Assign is `name=value` in a command prefix or on its own.
type Assign struct {
	Name  string
	Value *Word // nil for a bare `name=`
	// Elems is `name=( … )`, and IsArray distinguishes an empty array from a
	// bare `name=` — `a=()` and `a=` are different states, exactly as an
	// absent `for` list differs from an empty one.
	Elems   []*Word
	IsArray bool
	// Index is the subscript of `name[i]=value`, nil otherwise.
	Index *Word
	// Append is `name+=value`, which adds to what is there rather than
	// replacing it — and adds to the *end* of an array rather than to its
	// first element.
	Append bool
	Start  Pos
	Stop   Pos
}

func (a *Assign) Pos() Pos { return a.Start }
func (a *Assign) End() Pos { return a.Stop }

// Redirect is one redirection.
type Redirect struct {
	// N is the file descriptor written immediately before the operator, or
	// nil. The adjacency is the rule: `echo 1>b` has one, `echo 1 >b` does
	// not and the 1 is an argument.
	N     *Word
	Op    Kind
	OpPos Pos
	// Word is the target: a filename, a descriptor for `>&`, or a here-string
	// body. For a here-document it is the delimiter.
	Word *Word
	// Heredoc is the body, which is read from the lines after the command
	// rather than from the token stream. Nil until here-documents are
	// implemented; the field exists so the shape is settled.
	Heredoc *Word
}

func (r *Redirect) Pos() Pos {
	if r.N != nil {
		return r.N.Pos()
	}
	return r.OpPos
}

func (r *Redirect) End() Pos {
	if r.Word != nil {
		return r.Word.End()
	}
	return r.OpPos
}

// SimpleCmd is assignments, arguments and redirections.
//
// The three are interleaved in the source and separated here, because a
// redirection may appear before the command name or between its arguments —
// `>b echo hi` and `echo one >b two` both work — so they cannot be modeled as
// a suffix.
type SimpleCmd struct {
	Assigns []*Assign
	Args    []*Word
	Redirs  []*Redirect
	Start   Pos
	Stop    Pos
}

func (c *SimpleCmd) Pos() Pos     { return c.Start }
func (c *SimpleCmd) End() Pos     { return c.Stop }
func (c *SimpleCmd) commandNode() {}

// redirs is embedded in every compound command, because a redirection on one
// applies to everything inside it: `{ …; } >f` and `for … done >f` both work.
type redirs struct {
	Redirs []*Redirect
}

// Subshell is `( list )`, which runs in a child shell so its assignments do
// not escape.
type Subshell struct {
	List  []*Stmt
	Start Pos
	Stop  Pos
	redirs
}

func (c *Subshell) Pos() Pos     { return c.Start }
func (c *Subshell) End() Pos     { return c.Stop }
func (c *Subshell) commandNode() {}

// Group is `{ list; }`, which runs in the current shell.
type Group struct {
	List  []*Stmt
	Start Pos
	Stop  Pos
	redirs
}

func (c *Group) Pos() Pos     { return c.Start }
func (c *Group) End() Pos     { return c.Stop }
func (c *Group) commandNode() {}

// IfClause is `if … then … [elif …] [else …] fi`.
//
// Cond is a list rather than a single command, and its *last* command decides:
// `if false; true; then` takes the branch.
type IfClause struct {
	Cond  []*Stmt
	Then  []*Stmt
	Elifs []*Elif
	Else  []*Stmt
	// HasElse distinguishes `else` with an empty body from no `else` at all.
	HasElse bool
	Start   Pos
	Stop    Pos
	redirs
}

func (c *IfClause) Pos() Pos     { return c.Start }
func (c *IfClause) End() Pos     { return c.Stop }
func (c *IfClause) commandNode() {}

// Elif is one `elif … then …` in a chain.
type Elif struct {
	Cond  []*Stmt
	Then  []*Stmt
	Start Pos
}

func (e *Elif) Pos() Pos { return e.Start }
func (e *Elif) End() Pos {
	if len(e.Then) > 0 {
		return e.Then[len(e.Then)-1].End()
	}
	return e.Start
}

// LoopClause is `while … do … done` or `until … do … done`.
type LoopClause struct {
	// Until inverts the sense of the condition.
	Until bool
	Cond  []*Stmt
	Body  []*Stmt
	Start Pos
	Stop  Pos
	redirs
}

func (c *LoopClause) Pos() Pos     { return c.Start }
func (c *LoopClause) End() Pos     { return c.Stop }
func (c *LoopClause) commandNode() {}

// ForClause is `for name [in words] do … done`.
type ForClause struct {
	Name string
	// Items is the word list. HasItems is what distinguishes an absent list
	// from an empty one, which is a real difference and not a nicety: with
	// `in` omitted the loop iterates the positional parameters, and with `in`
	// present and nothing after it, nothing. A nil slice cannot say which.
	Items    []*Word
	HasItems bool
	Body     []*Stmt
	// Header is `for i in 1 2` as written, kept for the same reason
	// ArithCmdClause keeps its expression: it is what a diagnostic quotes.
	// bash prints it under `set -x` unexpanded — `for i in $x`, quotes and
	// all — so nothing rebuilt from the tree would match it.
	Header string
	Start  Pos
	Stop   Pos
	redirs
}

func (c *ForClause) Pos() Pos     { return c.Start }
func (c *ForClause) End() Pos     { return c.Stop }
func (c *ForClause) commandNode() {}

// SelectClause is `select name [in words] do … done`.
//
// The same shape as ForClause and not the same loop: the words are a menu
// rather than a sequence, the body runs once per *reply* rather than once per
// word, and the loop ends when the input does rather than when the list does.
// Sharing a node would have made every use of one ask which it was.
type SelectClause struct {
	Name string
	// Items and HasItems carry the same distinction as ForClause's: with `in`
	// omitted the menu is built from the positional parameters.
	Items    []*Word
	HasItems bool
	Body     []*Stmt
	// Header is `select x in a b` as written. See ForClause.Header.
	Header string
	Start  Pos
	Stop   Pos
	redirs
}

func (c *SelectClause) Pos() Pos     { return c.Start }
func (c *SelectClause) End() Pos     { return c.Stop }
func (c *SelectClause) commandNode() {}

// CaseClause is `case word in … esac`.
type CaseClause struct {
	Word  *Word
	Items []*CaseItem
	// Header is `case $v in` as written. See ForClause.Header.
	Header string
	Start  Pos
	Stop   Pos
	redirs
}

func (c *CaseClause) Pos() Pos     { return c.Start }
func (c *CaseClause) End() Pos     { return c.Stop }
func (c *CaseClause) commandNode() {}

// CaseItem is one `pattern | pattern) list ;;` arm.
type CaseItem struct {
	Patterns []*Word
	Body     []*Stmt
	// Term is TokDSemi, TokSemiAmp or TokDSemiAmp. They are separate operators rather
	// than one "case extension": `;&` is core and `;;&` is bash only.
	Term    Kind
	TermPos Pos
	Start   Pos
}

func (i *CaseItem) Pos() Pos { return i.Start }
func (i *CaseItem) End() Pos { return i.TermPos }

// ForArithClause is `for ((init; cond; post)) do … done`.
//
// A separate node from ForClause rather than a variant of it, because it
// iterates on a condition rather than over a list: the two share a keyword
// and nothing else.
type ForArithClause struct {
	Init, Cond, Post ArithExpr
	Body             []*Stmt
	Header           string
	Start, Stop      Pos
}

func (c *ForArithClause) Pos() Pos     { return c.Start }
func (c *ForArithClause) End() Pos     { return c.Stop }
func (c *ForArithClause) commandNode() {}

// ArithCmdClause is `(( expr ))` used as a command.
//
// It exits 0 when the expression is non-zero, which is the reverse of the
// usual convention and is unanimous across the panel.
type ArithCmdClause struct {
	// Expr is the source text, kept because it is what a diagnostic quotes.
	Expr string
	// Parsed is the expression tree.
	Parsed ArithExpr
	Start  Pos
	Stop   Pos
	redirs
}

func (c *ArithCmdClause) Pos() Pos     { return c.Start }
func (c *ArithCmdClause) End() Pos     { return c.Stop }
func (c *ArithCmdClause) commandNode() {}

// FuncDecl is `name() compound` or `function name compound`.
//
// The body is a compound command rather than specifically a brace group, so it
// can be any of them and can carry its own redirections.
type FuncDecl struct {
	Name string
	// Keyword records that the `function` word was used, which is not
	// universal: dash rejects it, and ksh93 rejects the hybrid form with
	// parentheses as well.
	Keyword bool
	Body    Command
	Start   Pos
}

func (c *FuncDecl) Pos() Pos     { return c.Start }
func (c *FuncDecl) End() Pos     { return c.Body.End() }
func (c *FuncDecl) commandNode() {}
