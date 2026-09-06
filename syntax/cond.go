// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// The condition tree for `[[ … ]]`, from docs/spec/grammar/conditions.md.
//
// It has its own node types rather than reusing the command ones because
// `&&`, `||` and `( )` inside `[[ ]]` join and group *conditions* rather than
// commands. That is a consequence of `[[ ]]` being parsed rather than
// executed, which is also why the words inside are never split or globbed.

// CondExpr is a node in a condition.
type CondExpr interface {
	Node
	condNode()
}

// CondUnary is a one-operand test such as `-n s` or `-f path`.
type CondUnary struct {
	Op    string
	X     *Word
	Start Pos
}

func (c *CondUnary) Pos() Pos  { return c.Start }
func (c *CondUnary) End() Pos  { return c.X.End() }
func (c *CondUnary) condNode() {}

// CondBinary is a two-operand test.
//
// The right operand is a *word* rather than a string because what it means
// depends on how it was written: unquoted it is a pattern for `==` and a
// regular expression for `=~`, and quoted it is a literal. Only the spans
// still know which.
type CondBinary struct {
	Op   string
	X, Y *Word
}

func (c *CondBinary) Pos() Pos  { return c.X.Pos() }
func (c *CondBinary) End() Pos  { return c.Y.End() }
func (c *CondBinary) condNode() {}

// CondLogic is `a && b` or `a || b` between conditions.
type CondLogic struct {
	Op   string
	X, Y CondExpr
}

func (c *CondLogic) Pos() Pos  { return c.X.Pos() }
func (c *CondLogic) End() Pos  { return c.Y.End() }
func (c *CondLogic) condNode() {}

// CondNot is `! c`.
type CondNot struct {
	X     CondExpr
	Start Pos
}

func (c *CondNot) Pos() Pos  { return c.Start }
func (c *CondNot) End() Pos  { return c.X.End() }
func (c *CondNot) condNode() {}

// CondGroup is `( c )`, which groups a condition rather than starting a
// subshell.
type CondGroup struct {
	X     CondExpr
	Start Pos
	Stop  Pos
}

func (c *CondGroup) Pos() Pos  { return c.Start }
func (c *CondGroup) End() Pos  { return c.Stop }
func (c *CondGroup) condNode() {}

// TestClause is `[[ … ]]`.
type TestClause struct {
	Expr  CondExpr
	Start Pos
	Stop  Pos
	redirs
}

func (c *TestClause) Pos() Pos     { return c.Start }
func (c *TestClause) End() Pos     { return c.Stop }
func (c *TestClause) commandNode() {}

// condUnaryOps are the one-operand tests.
//
// `-o` is here for the same head count that put DoubleBracket in the core:
// every shell in the panel that has `[[ ]]` at all has the option test inside
// it, measured on bash 5.3, bash 3.2, bash-as-`sh`, ksh93 and zsh 5.9, and
// dash has no `[[ ]]` to put it in. It is not a dialect flag, because the
// three that answer disagree about *names* and not about the grammar: the
// operand is an ordinary word everywhere — `[[ -o 'aliases' ]]` reads the
// same name as `[[ -o aliases ]]`, and `v=errexit; [[ -o $v ]]` reads it out
// of the variable — and a missing one is a syntax error in all three.
var condUnaryOps = map[string]bool{
	"-n": true, "-z": true,
	"-e": true, "-f": true, "-d": true, "-s": true,
	"-r": true, "-w": true, "-x": true,
	"-b": true, "-c": true, "-p": true, "-S": true, "-L": true, "-h": true,
	"-g": true, "-u": true, "-k": true, "-t": true,
	"-o": true,
}

// condBinaryWordOps are the two-operand tests spelled as words. These compare
// numbers, unlike `<` and `>`, which compare strings — `[[ 10 > 9 ]]` is
// false and `[[ 10 -gt 9 ]]` is true.
var condBinaryWordOps = map[string]bool{
	"-eq": true, "-ne": true, "-lt": true, "-le": true, "-gt": true, "-ge": true,
	"-nt": true, "-ot": true, "-ef": true,
	"=": true, "==": true, "!=": true, "=~": true,
}

// parseTestClause parses `[[ … ]]`. The opening word is current on entry.
func (p *Parser) parseTestClause() Command {
	c := &TestClause{Start: p.tok.Pos}
	// One dialect reads pattern groups here and nowhere else, and the lexer
	// decides where a word ends — so it has to be told before the first token
	// inside the condition is read.
	p.lex.inCondition = true
	p.next()
	// A newline inside `[[ ]]` continues the condition rather than ending a
	// command, so it is skipped wherever the grammar is still waiting for
	// something. Unanimous across the shells that have `[[ ]]`, at every
	// structural point: after `[[`, after `&&`, `||` and `!`, on both sides
	// of a group's parentheses, before `]]`, and — the one this list was
	// missing — *before* `&&` and `||`, where the condition so far is
	// complete and an operator may still follow.
	//
	// That last one cannot be a point at all, because at the newline it is
	// not yet known whether an operator or the `]]` comes next; condOr and
	// condAnd skip speculatively instead. Skipping there is safe precisely
	// because the two places a complete condition may end — an operator and
	// the `]]` — both tolerate newlines in front of them.
	//
	// Not after a *binary operator* — `[[ 1 ==` then a newline is an error in
	// bash and ksh93, and only zsh takes it. That one is left refused, which
	// is what the two agree on.
	p.skipNewlines()
	c.Expr = p.condOr()
	p.skipNewlines()
	if c.Expr == nil && p.err == nil {
		p.fail("expected a condition after [[")
	}
	c.Stop = p.tok.End
	p.lex.inCondition = false
	if !p.atWord("]]") {
		p.fail("expected ]]")
		return c
	}
	p.next()
	return c
}

func (p *Parser) condOr() CondExpr {
	x := p.condAnd()
	for x != nil && p.err == nil {
		// Speculatively, per parseTestClause: the operator may be on the next
		// line. What is skipped here is given back by the caller, which skips
		// newlines before the `]]` anyway — and a word that is neither
		// operator nor closer is refused either way, only a line later.
		p.skipNewlines()
		if !p.at(TokOrOr) {
			break
		}
		p.next()
		p.skipNewlines()
		y := p.condAnd()
		if y == nil {
			p.fail("expected a condition after ||")
			return x
		}
		x = &CondLogic{Op: "||", X: x, Y: y}
	}
	return x
}

func (p *Parser) condAnd() CondExpr {
	x := p.condPrimary()
	for x != nil && p.err == nil {
		p.skipNewlines()
		if !p.at(TokAndAnd) {
			break
		}
		p.next()
		p.skipNewlines()
		y := p.condPrimary()
		if y == nil {
			p.fail("expected a condition after &&")
			return x
		}
		x = &CondLogic{Op: "&&", X: x, Y: y}
	}
	return x
}

func (p *Parser) condPrimary() CondExpr {
	switch {
	case p.err != nil, p.at(TokEOF):
		return nil

	case p.atWord("!"):
		start := p.tok.Pos
		p.next()
		p.skipNewlines()
		x := p.condPrimary()
		if x == nil {
			p.fail("expected a condition after !")
			return nil
		}
		return &CondNot{X: x, Start: start}

	case p.at(TokLeftParen):
		// Grouping, not a subshell: nothing here runs.
		start := p.tok.Pos
		p.next()
		p.skipNewlines()
		x := p.condOr()
		if x == nil {
			p.fail("expected a condition after (")
			return nil
		}
		p.skipNewlines()
		stop := p.tok.End
		if !p.at(TokRightParen) {
			p.fail("expected ) in a condition")
			return nil
		}
		p.next()
		return &CondGroup{X: x, Start: start, Stop: stop}

	case p.tok.Kind == TokWord && !p.tok.IsQuoted() && condUnaryOps[p.tok.Literal()]:
		op, start := p.tok.Literal(), p.tok.Pos
		p.next()
		x := p.word()
		if x == nil {
			p.failCondOperand(op, "unary")
			return nil
		}
		return &CondUnary{Op: op, X: x, Start: start}
	}

	left := p.word()
	if left == nil {
		return nil
	}
	op := p.condOperator()
	if op == "" {
		// A bare word is a test for non-emptiness.
		return &CondUnary{Op: "-n", X: left, Start: left.Pos()}
	}
	right := p.word()
	if right == nil {
		p.failCondOperand(op, "binary")
		return nil
	}
	return &CondBinary{Op: op, X: left, Y: right}
}

// failCondOperand records a token standing where a conditional operator
// wanted a word.
//
// It names the token rather than the operator, which is what every shell in
// the panel does — one of them says which *kind* of operator was waiting as
// well, which is the `arity` here. Written as its own kind rather than as a
// message because a dialect may not match on our phrasing; see
// Diagnostics.CondOperand.
func (p *Parser) failCondOperand(op, arity string) {
	if p.err != nil {
		return
	}
	if p.at(TokEOF) {
		p.ranOut()
		p.err = p.unterminated("]]")
		return
	}
	p.err = &Error{
		Pos: p.tok.Pos, Kind: ErrCondOperand,
		Token: p.tokenLiteral(), Class: p.tokenClass(false),
		Expected: arity, LastToken: op,
		Msg: p.tokenLiteral() + " unexpected",
	}
}

// condPatternOps are the operators whose right operand is a *pattern* — the
// language of patterns.md, rather than the regular expression `=~` takes or
// the number the `-eq` family reads. It is the same set `case` matches with,
// and the reason a group there belongs to the operand rather than to the
// shell.
var condPatternOps = map[string]bool{"=": true, "==": true, "!=": true}

// condOperator reads a comparison operator, reinterpreting `<` and `>`.
//
// The lexer produced them as redirection operators, deliberately: `[[` is only
// special where a command may begin, and the lexer does not know where that
// is. Here the parser does, so it reinterprets them — which is the contract
// recorded in dialect.go and pinned by a test in lexer_test.go.
func (p *Parser) condOperator() string {
	switch p.tok.Kind {
	case TokLess:
		p.next()
		return "<"
	case TokGreat:
		p.next()
		return ">"
	}
	if p.tok.Kind == TokWord && !p.tok.IsQuoted() && condBinaryWordOps[p.tok.Literal()] {
		op := p.tok.Literal()
		// The operand of `=~` is a regular expression, where `(`, `)` and in
		// some dialects `|` are the regex's and not the shell's. Whether a
		// word ends at one is settled by the lexer, so it has to be told
		// before the operand is read — which is here, before the token after
		// the operator is fetched.
		p.lex.inRegex = op == "=~"
		// And the operand of `==`, `=` and `!=` is a *pattern*, where one
		// dialect reads a bare `(a|b)` group — including one that starts the
		// operand, which is the position the operator table reaches first.
		// Set here for the same reason and in the same place.
		p.lex.inPattern = condPatternOps[op]
		p.next()
		p.lex.inRegex = false
		p.lex.inPattern = false
		return op
	}
	return ""
}
