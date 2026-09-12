// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"strings"
)

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
// condUnaryOp reports whether a word is a one-operand test in this dialect.
//
// Most are core. Three are not, and each says why in dialect.go: `-v` behind
// [Dialect.ParameterIsSetTest], and `-prefix` and `-suffix` behind
// [Dialect.CompletionConditions].
func (p *Parser) condUnaryOp(s string) bool {
	switch s {
	case "-v":
		return p.dialect.ParameterIsSetTest
	case "-prefix", "-suffix":
		// The completion-context tests, which one shell has in the grammar
		// unconditionally and refuses when they *run*. See
		// Dialect.CompletionConditions.
		return p.dialect.CompletionConditions
	}
	return condUnaryOps[s]
}

var condUnaryOps = map[string]bool{
	"-n": true, "-z": true,
	"-e": true, "-f": true, "-d": true, "-s": true,
	"-r": true, "-w": true, "-x": true,
	"-b": true, "-c": true, "-p": true, "-S": true, "-L": true, "-h": true,
	"-g": true, "-u": true, "-k": true, "-t": true,
	"-o": true,
	// Ownership, and core for the same head count as `-o`: bash 5.3, bash
	// 3.2, bash-as-`sh`, ksh93 and zsh 5.9 all have both, and dash has no
	// `[[ ]]` to put them in. Missing here, `[[ -O f ]]` was not an
	// expression at all — a syntax error that abandoned the whole clause,
	// where the question it asks has an answer every shell agrees on.
	"-O": true, "-G": true,
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
	// No skip before the `]]` test: condAnd has already done it, for every
	// closer at once. The one above is still needed — nothing has read
	// anything yet at that point.
	if c.Expr == nil && p.err == nil {
		p.fail("expected a condition after [[")
	}
	c.Stop = p.tok.End
	p.lex.inCondition = false
	if !p.atWord("]]") {
		// The token, not what we wanted. `expected ]]` was one sentence for
		// every way a condition can be malformed — a second operand with no
		// operator, a `;`, a `|` — and every shell in the panel names the
		// *offending* token instead. Routed through the ordinary
		// unexpected-token path so each dialect words it the way it words
		// any other, rather than through a message of this construct's own.
		//
		// Marked as being inside the condition, because one dialect words a
		// token refused *there* differently from one refused anywhere else
		// and prints a line about the construct in front of it — at the
		// `[[`'s line rather than the token's, which is why the construct's
		// position travels with the error.
		p.failUnexpected("]]")
		p.blameCondition(c.Start)
		return c
	}
	p.next()
	return c
}

// condWord is a word standing as an operand, which a `]]` is not.
//
// Without this the closing `]]` was read as an operand and the failure landed
// on whatever followed the condition: `[[ -n ]]` blamed the `;` after it
// rather than the `]]` in front of it, which is what every shell in the panel
// names.
//
// A *quoted* `]]` is an ordinary word — `[[ -n "]]" ]]` tests a
// two-character string — and nothing here has to say so: atWord already
// asks, because every reserved word in this grammar stops being one when it
// is quoted. A second check beside it was written and a mutant that deleted
// it passed everything, which is what said the question was already answered.
func (p *Parser) condWord() *Word {
	if p.atWord("]]") {
		return nil
	}
	return p.word()
}

// condOperatorHasItsOperand reports whether a one-operand test really has one
// after it, for the two operators that fall back to being ordinary words when
// it does not.
//
// Every other unary operator in the table *demands* its operand and says so:
// `[[ -n ]]` is “ unknown condition: -n “ in the shell this is about, and
// `unexpected argument `]]'` in bash. The completion-context pair does not —
// measured 2026-09-12 on zsh 5.9.2, `[[ -prefix ]]`, `[[ -suffix ]]`,
// `[[ -prefix && -n x ]]`, `[[ -prefix || -n x ]]` and `[[ ( -prefix ) ]]` all
// answer 0 with nothing said, which is the bare-word reading: a word on its
// own is a test for non-emptiness, and `-prefix` is not empty.
//
// bash 5.3, that binary as `sh` and ksh93 answer 0 for the same line because
// they read the word as an ordinary one, having no such operator — so
// **adding it without this would have made one dialect refuse a line three
// other columns run**. The two that answer otherwise are not counter-examples:
// dash has no `[[ ]]` at all, and bash 3.2 alone reads `-prefix` as *a*
// conditional unary operator and calls the `]]` an unexpected argument to it.
//
// The look is at the source rather than at a token, the way peekIsAnonBody's
// is: reading the next token would consume it, and there is nothing to put it
// back into.
func (p *Parser) condOperatorHasItsOperand(op string) bool {
	if op != "-prefix" && op != "-suffix" {
		return true
	}
	i := p.lex.off
	for i < len(p.lex.src) && isBlank(p.lex.src[i]) {
		i++
	}
	rest := p.lex.src[i:]
	for _, end := range []string{"]]", "&&", "||", ")"} {
		if strings.HasPrefix(rest, end) {
			return false
		}
	}
	return rest != ""
}

// blameCondition marks the failure just recorded as one the `[[` was open
// over, and records where the `[[` was.
//
// Set after the failure rather than passed into it: every route to a refused
// token inside a condition goes through the ordinary helpers, and threading a
// position through each of them would be paying for one dialect's extra line
// in five signatures.
func (p *Parser) blameCondition(start Pos) {
	var se *Error
	if !errors.As(p.err, &se) {
		return
	}
	se.Construct, se.ConstructLine = "[[", int(start.Line)
	if se.Kind == ErrUnterminated {
		// A `[[` that never closed is unclosed *by the condition*, and two
		// dialects name it: one as the construct and one as the innermost
		// keyword still open. `[[` is not a word the list parser stacks, so
		// nothing else filled either of them in and one came out as a hole —
		// `` `' unmatched ``.
		//
		// Unconditionally, and that is measured rather than tidy: the `[[`
		// *is* the innermost thing still open, so it displaces whatever the
		// stack was holding. `if true; then [[ -n x` and `while [[ -n x`
		// and `{ [[ -n x` are all `` `[[' unmatched `` in ksh93, not
		// `then`, `while` or `{`. A guard that wrote this only when nothing
		// else had was written first, and a mutant that deleted it passed
		// every test — which is how the three nested shapes came to be
		// measured at all.
		se.Innermost = "[["
	}
}

func (p *Parser) condOr() CondExpr {
	x := p.condAnd()
	// No skip of its own before the test: every route to this operator runs
	// through condAnd, which has already skipped whatever newlines stood
	// between the condition and here. Skipping again would be a second
	// spelling of the same rule, and one no input could tell from the first.
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
	for x != nil && p.err == nil {
		// Speculatively, per parseTestClause: a complete condition may be
		// followed on the next line by `&&`, by `||`, by a group's `)` or by
		// the `]]`, and all four tolerate newlines in front of them — so the
		// newline can be consumed before it is known which of the four it
		// was. This is the only place it happens for any of them, condOr and
		// the group both reaching their operator through here.
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
		// Nor here, and for the same reason.
		stop := p.tok.End
		if !p.at(TokRightParen) {
			p.fail("expected ) in a condition")
			return nil
		}
		p.next()
		return &CondGroup{X: x, Start: start, Stop: stop}

	case p.tok.Kind == TokWord && !p.tok.IsQuoted() && p.condUnaryOp(p.tok.Literal()) &&
		p.condOperatorHasItsOperand(p.tok.Literal()):
		op, start := p.tok.Literal(), p.tok.Pos
		p.next()
		x := p.condWord()
		if x == nil {
			p.failCondOperand(op, "unary")
			return nil
		}
		return &CondUnary{Op: op, X: x, Start: start}
	}

	left := p.condWord()
	if left == nil {
		return nil
	}
	op := p.condOperator()
	if op == "" {
		// A bare word is a test for non-emptiness.
		return &CondUnary{Op: "-n", X: left, Start: left.Pos()}
	}
	right := p.condWord()
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
