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
var condUnaryOps = map[string]bool{
	"-n": true, "-z": true,
	"-e": true, "-f": true, "-d": true, "-s": true,
	"-r": true, "-w": true, "-x": true,
	"-b": true, "-c": true, "-p": true, "-L": true, "-h": true,
	"-g": true, "-u": true, "-k": true, "-t": true,
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
	// something. Unanimous across the three shells that have `[[ ]]`, at
	// every structural point: after `[[`, after `&&`, `||` and `!`, on both
	// sides of a group's parentheses, and before `]]`.
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
	for x != nil && p.at(TokOrOr) && p.err == nil {
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
	for x != nil && p.at(TokAndAnd) && p.err == nil {
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
			p.fail("expected an operand after %s", op)
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
		p.fail("expected an operand after %s", op)
		return nil
	}
	return &CondBinary{Op: op, X: left, Y: right}
}

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
		p.next()
		p.lex.inRegex = false
		return op
	}
	return ""
}
