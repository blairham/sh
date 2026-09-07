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

	// Disown is set when the terminator was `&!` or `&|` rather than `&`,
	// where the dialect has them: the job is started in the background and
	// then let go of, so no listing shows it and nothing waits for it by
	// number.
	//
	// Always with Background, never instead of it — the two spellings are
	// `&` plus a disowning, which is why this is a second bool rather than a
	// third state of the first. A dialect without the operators never sets
	// it.
	Disown bool

	// Coprocess is set when the terminator was `|&` in the dialect that
	// spells a coprocess that way: the statement is started in the
	// background with a pipe on each of its named streams, and the shell
	// keeps the near ends.
	//
	// Always with Background, for the same reason Disown is and with the
	// same shape: `|&` is `&` plus two pipes, so this is a second bool
	// rather than a third state of the first. It sits on the *statement*
	// because the operator terminates the whole and-or — measured,
	// `echo A && cat |&` puts `echo A`'s output where a later `read -p`
	// finds it. A dialect without [Dialect.CoprocPipeOperator] never sets
	// it.
	Coprocess bool

	Semi Pos

	// Text is the source this statement was written as, and is recorded only
	// for a background one.
	//
	// Only there because that is the only place a shell has to show a
	// command back to someone long after reading it: a `jobs` listing names
	// what is running, and by then the words have been expanded, the
	// process has been started, and nothing else remembers how it was
	// spelled. Every other node can be re-read from the input it came from,
	// so paying for the text everywhere would be paying for one case.
	Text string
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

// TimeClause is `time [-p] [pipeline]`: the pipeline runs whole and the shell
// writes how long it took to its own standard error — which is why the report
// of `time true 2>&1 | wc -l` is not counted: the redirection belongs to an
// element inside the pipeline and the report lands outside it.
//
// It is an expression rather than a command because that is where it binds:
// the whole pipeline and nothing past it. `time true && echo ok` times `true`
// alone.
type TimeClause struct {
	// Negated records a `!` written before `time`, which negates the timed
	// pipeline's status — `! time true` is 1 and still reports. A `!` after
	// `time` is the pipeline's own and lives on it.
	Negated bool
	Bang    Pos
	Time    Pos
	// Posix is the `-p` flag, which switches the report to the POSIX format.
	// Read only where [Dialect.TimePosixFlag] says so; elsewhere a `-p` is
	// the first word of the pipeline.
	Posix    bool
	PosixPos Pos
	// Pipeline is what is timed: a *Pipeline, another *TimeClause, or nil
	// for a bare `time`, which runs nothing and still reports.
	Pipeline Expr
	// Stop is the end of the last keyword token, for a bare `time` that has
	// no pipeline to end at.
	Stop Pos
}

func (t *TimeClause) Pos() Pos {
	if t.Negated {
		return t.Bang
	}
	return t.Time
}

func (t *TimeClause) End() Pos {
	if t.Pipeline != nil {
		return t.Pipeline.End()
	}
	return t.Stop
}
func (t *TimeClause) exprNode() {}

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
	// Operand marks an assignment written *after* the command word rather
	// than in front of it — `local a=(x y)` rather than `a=1 cmd`. The two
	// are different things wearing one syntax: a prefix assignment is the
	// command's environment, and this one is an argument to a utility that
	// takes assignments.
	//
	// Only the array form is ever parsed this way. `local a=1` stays an
	// ordinary word, which is the path every shell's scalar case has always
	// taken here and which expands by its own rules.
	Operand bool

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
	// Text is the target as it was written, before any expansion.
	//
	// One dialect names it in a diagnostic — `$e: ambiguous redirect`, where
	// `$e` is what was typed and not what it came to — and by the time that
	// is known the word has been expanded and there is nothing left that
	// remembers how it was spelled. Same reason a background statement keeps
	// its text.
	Text string

	// Word is the target: a filename, a descriptor for `>&`, or a here-string
	// body. For a here-document it is the delimiter.
	Word *Word
	// Heredoc is the body, which is read from the lines after the command
	// rather than from the token stream. Nil for every redirection that is
	// not a here-document — including `<<<`, which shares a prefix with `<<`
	// and nothing else: a here-string's input is its Word, on the same line.
	Heredoc *Word

	// PipeBoth records a redirection nobody wrote: the `2>&1` that a `|&`
	// after this command stands for. The operator is the source text and
	// this is what it means, which is why the meaning is here rather than
	// on the pipeline — the redirection has to be the command's *last*, and
	// the command's own list is the only place that can say so.
	//
	// Kept marked so the printer writes `|&` back rather than the
	// redirection, and so anything reading the tree can tell what the script
	// said from what the grammar added.
	PipeBoth bool
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

func (r *redirs) redirList() []*Redirect { return r.Redirs }

func (c *SimpleCmd) redirList() []*Redirect { return c.Redirs }

// mergesStderr reports whether c ends in the redirection a `|&` stands for.
//
// Asked of the command to the *left* of a bar, which is the one that carries
// it: a pipeline's spelling is recoverable from the tree rather than recorded
// twice, so nothing can record the two halves and disagree.
func mergesStderr(c Command) bool {
	if f, ok := c.(*FuncDecl); ok {
		// A definition holds its redirections on its body, which is where
		// the parser puts the written ones too.
		return f.Body != nil && mergesStderr(f.Body)
	}
	h, ok := c.(interface{ redirList() []*Redirect })
	if !ok {
		return false
	}
	rs := h.redirList()
	return len(rs) > 0 && rs[len(rs)-1].PipeBoth
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

// TryClause is `{ … } always { … }`, the try-always block.
//
// The two halves are lists rather than [Group] nodes, the way an [IfClause]'s
// branches are: the braces are mandatory punctuation of this production, so a
// nested group node would carry no fact the construct does not already state —
// and a list is what lets `break` inside the try half reach the loop around
// the whole construct rather than a group standing in the way.
//
// Redirs belongs to the construct and not to either half, which is measured:
// `{ echo t; } always { echo a; } > /dev/null` sends *both* halves there, and
// a redirection written after the try half instead ends it, so that spelling
// is a parse error. See [Dialect.TryAlways] for the grammar.
type TryClause struct {
	// Try is the first half, run first and whatever happens.
	Try []*Stmt
	// Always is the second half, run however the first half ended.
	Always []*Stmt
	Start  Pos
	Stop   Pos
	redirs
}

func (c *TryClause) Pos() Pos     { return c.Start }
func (c *TryClause) End() Pos     { return c.Stop }
func (c *TryClause) commandNode() {}

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
	// Names is the loop's variables, and there is always at least one. More
	// than one is zsh's — see [Dialect.ForMultipleNames] — and the loop then
	// takes that many words from the list on every pass, so the count is the
	// stride and not a decoration.
	//
	// A slice rather than a name and a tail, because a `for` binds a *list*
	// of names and one of them being first is not a fact about the language.
	// Two fields would have been two things to keep in step, and every reader
	// would have had to ask which it wanted.
	Names []string
	// Items is the word list. HasItems is what distinguishes an absent list
	// from an empty one, which is a real difference and not a nicety: with
	// `in` omitted the loop iterates the positional parameters, and with `in`
	// present and nothing after it, nothing. A nil slice cannot say which.
	Items    []*Word
	HasItems bool
	Body     []*Stmt
	// RefusedName is the word standing where a name belonged when it was not
	// one, in the dialect that checks it when the loop runs — see
	// [Dialect.ForNameCheckedWhenTheLoopRuns]. It is the source text, quotes
	// and expansion and all, because that is what the complaint quotes:
	// `for $n` refuses `$n` and not `n`.
	//
	// Set instead of Names rather than beside it: there is no name to bind,
	// so a clause carrying one would be a clause the interpreter could try
	// to run. Empty is the ordinary case and the only one in five of the six
	// dialects.
	RefusedName string
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
	// RefusedName is ForClause.RefusedName for the menu loop, and the same
	// two shells check it at the same stage: measured, `select $n in a b`
	// and `select 1x in a b` parse in bash and ksh93 and are refused while
	// parsing by dash and zsh, exactly as the `for` spellings are.
	RefusedName string
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

// AnonFunc is `() { … }` — a function with no name, defined and run where it
// stands, with the words after its body as its positional parameters.
//
// A function rather than a group, and the difference is observable: `local`
// inside one is local, `$#` counts the words after the body, and a `return`
// leaves it. Only the *name* is missing, which is why its own frame reports
// one the shell invents.
type AnonFunc struct {
	Body Command
	// Args are the words after the body, which become the positional
	// parameters of the call.
	Args  []*Word
	Start Pos
	// Keyword records that the `function` word was used in place of the
	// empty parameter list, which is the other spelling.
	Keyword bool
	redirs
}

func (c *AnonFunc) Pos() Pos     { return c.Start }
func (c *AnonFunc) End() Pos     { return c.Body.End() }
func (c *AnonFunc) commandNode() {}

// RepeatClause is `repeat N; do … done`, and the shorter spellings the same
// dialect gives every loop.
//
// A count rather than a condition, which is what makes it a construct of its
// own rather than a `for` in disguise: the word is evaluated as an arithmetic
// expression once, before the first iteration, and a count that is not a
// number or is not positive runs the body no times at all.
type RepeatClause struct {
	Count *Word
	Body  []*Stmt
	// Header is `repeat N` as written. See ForClause.Header.
	Header string
	Start  Pos
	Stop   Pos
	redirs
}

func (c *RepeatClause) Pos() Pos     { return c.Start }
func (c *RepeatClause) End() Pos     { return c.Stop }
func (c *RepeatClause) commandNode() {}

// CoprocClause is `coproc [NAME] command`: the command runs in the
// background with a pipe on each of its named streams, and the shell keeps
// the near ends in the array the name names.
type CoprocClause struct {
	// Name is what the coprocess was called, or empty for the default. A
	// name can only be written before a compound command: with a simple one
	// the first word is the command itself.
	Name   string
	Cmd    Command
	Coproc Pos
	Stop   Pos
}

func (c *CoprocClause) Pos() Pos     { return c.Coproc }
func (c *CoprocClause) End() Pos     { return c.Stop }
func (c *CoprocClause) commandNode() {}

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
	// Term is TokDSemi, TokSemiAmp, TokDSemiAmp or TokSemiPipe. They are
	// separate operators rather than one "case extension": `;&` is core,
	// `;;&` is bash only, and `;|` is zsh's spelling of `;;&` — kept as its
	// own Kind because the two spellings are mutually exclusive, so a tree
	// that folded them together could not be printed back in either shell.
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
	// InitText, CondText and PostText are the three parts as written. A part
	// containing an expansion has no tree until it runs, so the text is what
	// the interpreter reads it from.
	InitText, CondText, PostText string
	Body                         []*Stmt
	Header                       string
	Start, Stop                  Pos
	redirs
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

func (c *FuncDecl) Pos() Pos { return c.Start }

// End is the body's end, or the declaration's own start where there is no
// body.
//
// A definition the parser refused keeps its node — the name and the
// parentheses were read, and the error is what the caller acts on — so a
// [FuncDecl] with a nil body is a normal product of a failed parse rather
// than a malformed tree. Two extent computations reach one through a failure
// and neither can know it did: a coproc takes its own end from the command
// after the word, and a short-form loop body takes it from the statement it
// read. Both dereferenced the body that was never there (#911).
//
// The extent is empty rather than wrong. Nothing downstream of a failed parse
// measures a declaration that has no body, and an empty extent at the name
// keeps the one invariant a reader may hold: Pos is never past End.
func (c *FuncDecl) End() Pos {
	if c.Body == nil {
		return c.Start
	}
	return c.Body.End()
}

func (c *FuncDecl) commandNode() {}
