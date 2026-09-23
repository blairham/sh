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

// CondUnknown is a condition the grammar accepted and the interpreter refuses
// by name — `unknown condition: -n` — which is two shapes and one sentence:
// an operator this dialect has, standing with the wrong number of operands,
// and a `-word` it has no condition for at all.
//
// One dialect resolves a condition's name when it runs and the rest refuse
// while reading — see [Dialect.ConditionIsResolvedWhenItRuns] for both
// measurements. The operator and every word that stood with it are kept,
// because what a shell says about this names the operator and a printer has
// to write the line back as it was.
type CondUnknown struct {
	// Op is the operator: a known one with the wrong arity, or a word that is
	// no condition here.
	Op string
	// Words is what stood after it, which is none of them for `[[ -n ]]`, one
	// for `[[ -Q x ]]` and two or more for `[[ -n x y ]]`. A known operator
	// with exactly one is an ordinary CondUnary and never reaches here.
	Words []*Word
	Start Pos
	// Stop is the end of the operator itself, so a node with no words at all
	// still has an end.
	Stop Pos
}

func (c *CondUnknown) Pos() Pos { return c.Start }
func (c *CondUnknown) End() Pos {
	if len(c.Words) == 0 {
		return c.Stop
	}
	return c.Words[len(c.Words)-1].End()
}
func (c *CondUnknown) condNode() {}

// CondCompletion is one of the four completion-context tests — `-prefix`,
// `-suffix`, `-after` and `-between` — standing with the operands its arity
// allows.
//
// A node of its own rather than a CondUnary because the operator is in front
// of its operands and there may be two of them, which neither of the other
// two shapes can hold: `[[ -prefix 1 '*=' ]]` is the pair's optional leading
// count, and `-between` takes a start pattern and an end pattern. See
// [Dialect.CompletionConditions] for the arities and where they were
// measured.
type CondCompletion struct {
	// Op is the operator, written with its leading `-`.
	Op string
	// Words is what stood after it: one or two, never none — an operator
	// with nothing after it is an ordinary word and never reaches here.
	Words []*Word
	Start Pos
	// Stop is the end of the operator itself, kept so a node whose operand
	// list is somehow empty still has an end.
	Stop Pos
}

func (c *CondCompletion) Pos() Pos { return c.Start }
func (c *CondCompletion) End() Pos {
	if len(c.Words) == 0 {
		return c.Stop
	}
	return c.Words[len(c.Words)-1].End()
}
func (c *CondCompletion) condNode() {}

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
// Most are core. Four are not, and each says why in dialect.go: `-v` behind
// [Dialect.ParameterIsSetTest], `-R` behind [Dialect.NameReferenceTest], and
// `-prefix` and `-suffix` behind [Dialect.CompletionConditions].
func (p *Parser) condUnaryOp(s string) bool {
	switch s {
	case "-v":
		return p.dialect.ParameterIsSetTest
	case "-R":
		return p.dialect.NameReferenceTest
	}
	return condUnaryOps[s]
}

// condUnknownOp reports whether a word standing where an operator does is a
// *named* condition this dialect has not got, in the dialect that looks a
// condition's name up when it runs rather than while it reads.
//
// Every `-word` is a name there: measured on zsh 5.9.2, 2026-09-22,
// `[[ -Q x ]]`, `[[ -R x ]]`, `[[ -1 x ]]`, `[[ -- x ]]`, `[[ -eq x ]]` and
// `[[ -bogus x ]]` are each `unknown condition: <word>` at status 2 with the
// commands before them already run — where this parser refused all six while
// reading. `add-zle-hook-widget` is not why this matters and `-R` is not
// either: the rule is that an operator this dialect does not have is looked
// up and missed rather than never read (#4261).
//
// A lone `-` is not a name — `[[ - x ]]` is `parse error: condition expected:
// -` in the same shell — so two characters is the shortest one.
func (p *Parser) condUnknownOp(s string) bool {
	if !p.dialect.ConditionIsResolvedWhenItRuns || p.condUnaryOp(s) {
		return false
	}
	if _, _, completion := p.condCompletionOp(s); completion {
		return false
	}
	return strings.HasPrefix(s, "-") && len(s) >= 2
}

// condCompletionOp is the operand count one of the four completion-context
// tests takes, and false for anything else or where the dialect has not got
// them. See [Dialect.CompletionConditions] for the measurement.
//
// Two numbers rather than one because `-prefix` and `-suffix` take an
// optional count in front of their pattern — `[[ -prefix 1 '*=' ]]` — and
// `-between` takes two patterns and only two.
func (p *Parser) condCompletionOp(s string) (low, high int, ok bool) {
	if !p.dialect.CompletionConditions {
		return 0, 0, false
	}
	switch s {
	case "-prefix", "-suffix":
		return 1, 2, true
	case "-after":
		return 1, 1, true
	case "-between":
		return 2, 2, true
	}
	return 0, 0, false
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
	// Existence under its older spelling, and the same head count again:
	// `[[ -a /etc ]]` is 0 and `[[ -a /nosuch ]]` is 1 on bash 5.3, bash
	// 3.2, ksh93 and zsh 5.9.2, measured 2026-09-22, and none of the four
	// reads it as a connective inside `[[ ]]` — `[[ -n x -a -n y ]]` is a
	// syntax error in bash and ksh93 and the arity refusal in zsh. It is
	// only the `[` builtin where the word is `and`, which is why this sits
	// here and not in isTestUnary. Missing, it was the one row of #4261's
	// measurement where this shell said `unknown condition: -a` about an
	// operator every column has.
	"-a": true,
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
	// Where the `[[` stood, for the five sites below it that raise a failure
	// the construct has to be named in and that cannot see this frame. See
	// failCondTerm and blameCondition.
	outerStart, outerGroups := p.condStart, p.condGroups
	p.condStart, p.condGroups = c.Start, 0
	defer func() { p.condStart, p.condGroups = outerStart, outerGroups }()
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
		p.failCondTerm()
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
		p.recordCondGroup()
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
func (p *Parser) condWord() *Word { return p.condWordAt(false) }

// condTermWord is condWord at the one position where a `]]` may be an
// ordinary word rather than the closer — see
// [Dialect.ConditionCloserIsAWordWhereATermBegins], which is where the
// measurement is. Every other call is an operand's and takes the closer as
// the closer in every column.
func (p *Parser) condTermWord() *Word { return p.condWordAt(true) }

func (p *Parser) condWordAt(term bool) *Word {
	if p.atWord("]]") &&
		(!term || !p.dialect.ConditionCloserIsAWordWhereATermBegins) {
		return nil
	}
	w := p.word()
	// One dialect has no word here for a process substitution and says so
	// while reading, before the condition is ever reached. See
	// Parser.refuseProcSubstOutOfPlace; the axis that answers the *other*
	// three columns is interp.Semantics.ProcessSubstitutionInCondition, and
	// this is deliberately not it (#930).
	if p.refuseProcSubstOutOfPlace(w) {
		return nil
	}
	return w
}

// completionConditionAhead reports whether the word current is one of the four
// completion-context tests.
func (p *Parser) completionConditionAhead() bool {
	_, _, ok := p.condCompletionOp(p.tok.Literal())
	return ok
}

// condCompletion reads one of the four completion-context tests and the
// operands standing with it. The operator is current on entry, and it is
// already known to have at least one operand behind it.
//
// Measured on zsh 5.9.2, 2026-09-19, with `-n` for the parse and a run for
// the verdict — the whole of the arity rule in seven rows:
//
//	[[ -prefix a ]]        [[ -prefix a b ]]      the condition
//	[[ -prefix a b c ]]    unknown condition: -prefix, 2
//	[[ -after a ]]                                the condition
//	[[ -after a b ]]       unknown condition: -after, 2
//	[[ -between a b ]]                            the condition
//	[[ -between a ]]       [[ -between a b c ]]   unknown condition: -between, 2
//
// So each operator has a lowest and a highest count, a count inside the range
// is the condition, and a count outside it is the run-time refusal every
// other known operator with a bad arity gets — see
// [Dialect.ConditionIsResolvedWhenItRuns], whose node this reuses because
// the sentence and the status are the same one.
//
// **An operand that is itself an operator ends the reading**, which is the
// rule the one-operand family already has — see condSurplusOperands — and is
// measured here too: `[[ -prefix -n x ]]` is a parse error naming the `x` in
// that shell, the `-n` having been taken as the whole of `-prefix`'s operands
// and the `x` being simply unexpected. Nothing is collected past one of
// those, which is what leaves the `x` to be refused as a token rather than
// counted as a surplus operand.
func (p *Parser) condCompletion() CondExpr {
	op, start, stop := p.tok.Literal(), p.tok.Pos, p.tok.End
	low, high, _ := p.condCompletionOp(op)
	p.next()
	var words []*Word
	operatorShaped := false
	for len(words) < high && p.err == nil {
		// The lexer is told about a group for the same reason the one-operand
		// family tells it — the operand is a pattern and `//(a|b)/` is one
		// word — and taken back afterwards. **Only while the first operand is
		// read**, because what the flag governs is the token lexed *behind*
		// the word in hand, and that is the condition's third word exactly
		// once: `[[ -prefix 1 ( a ) ]]` has its group there and
		// `[[ -prefix 1 2 ( a ) ]]` has a fourth word, which that shell
		// refuses. See [Dialect.ConditionOperandMayOpenWithAGroup].
		p.lex.inCondOperandGroup = len(words) == 0 &&
			p.dialect.ConditionOperandMayOpenWithAGroup
		w := p.condWord()
		p.lex.inCondOperandGroup = false
		if w == nil {
			break
		}
		words = append(words, w)
		if lit, ok := unquotedLiteralWord(w); ok && p.condUnaryOp(lit) {
			operatorShaped = true
			break
		}
	}
	if p.err != nil {
		return nil
	}
	if !p.dialect.ConditionIsResolvedWhenItRuns {
		// A dialect that refuses a bad arity while *reading* refuses these
		// the same way: too few operands is the missing-operand failure and
		// a surplus word is simply left where it stands, so the `]]` test
		// names it. See [Dialect.ConditionIsResolvedWhenItRuns], which
		// is the only flag that lets a condition parse and then be refused.
		if len(words) < low {
			p.failCondOperand(op, "unary")
			return nil
		}
		return &CondCompletion{Op: op, Words: words, Start: start, Stop: stop}
	}
	if !operatorShaped {
		// Anything still standing beyond the highest count this operator
		// takes is the arity refusal from the other side, and every word is
		// kept so a printer writes the line back as it was read.
		for p.err == nil && p.tok.Kind == TokWord && !p.atWord("]]") {
			w := p.condWord()
			if w == nil {
				break
			}
			words = append(words, w)
		}
	}
	if len(words) < low || len(words) > high {
		return &CondUnknown{Op: op, Words: words, Start: start, Stop: stop}
	}
	return &CondCompletion{Op: op, Words: words, Start: start, Stop: stop}
}

// condOperatorHasItsOperand reports whether a test really has an operand
// after it, for the four operators that fall back to being ordinary words
// when it does not.
//
// Every other unary operator in the table *demands* its operand and says so:
// `[[ -n ]]` is “ unknown condition: -n “ in the shell this is about, and
// `unexpected argument `]]'` in bash. The four completion-context tests do
// not — measured 2026-09-12 on zsh 5.9.2 for the first two and 2026-09-19 for
// the other two, `[[ -prefix ]]`, `[[ -suffix ]]`, `[[ -after ]]`,
// `[[ -between ]]`, `[[ -prefix && -n x ]]`, `[[ -prefix || -n x ]]`,
// `[[ -after && -n x ]]`, `[[ -between && -n x ]]`, `[[ ( -prefix ) ]]` and
// `[[ ( -between ) ]]` all answer 0 with nothing said, which is the bare-word
// reading: a word on its own is a test for non-emptiness, and `-prefix` is
// not empty.
//
// bash 5.3, that binary as `sh` and ksh93 answer 0 for the same line because
// they read the word as an ordinary one, having no such operator — so
// **adding it without this would have made one dialect refuse a line three
// other columns run**. The two that answer otherwise are not counter-examples:
// dash has no `[[ ]]` at all, and bash 3.2 alone reads `-prefix` as *a*
// conditional unary operator and calls the `]]` an unexpected argument to it.
//
// A second family asks the same question, and its answer is measured the same
// way: a *named* condition this dialect has not got is an operator only where
// something stands behind it. Measured on zsh 5.9.2, 2026-09-22, `[[ -bogus
// ]]`, `[[ -zz ]]`, `[[ -eq ]]` and `[[ -bogus && -n x ]]` are all 0 with
// nothing said — the bare-word reading again — where `[[ -bogus x ]]` is
// `unknown condition: -bogus` at 2.
//
// **Two characters is the exception**, and it is the row that says the length
// is doing the work: `[[ -Q ]]`, `[[ -1 ]]` and `[[ -- ]]` are each `unknown
// condition` with no operand at all, where the longer words on the same line
// are words. So a `-X` is an operator wherever it stands and a longer name
// has to be followed by something.
func (p *Parser) condOperatorHasItsOperand(op string) bool {
	_, _, completion := p.condCompletionOp(op)
	// The two families that fall back to being an ordinary word. Every other
	// operator demands its operand, so there is nothing to look for.
	mayBeAWord := completion || (p.condUnknownOp(op) && len(op) > 2)
	return !mayBeAWord || p.condOperandAhead()
}

// condOperandAhead reports whether a word stands behind the operator current,
// rather than the closer, a connective, a group's end or nothing at all.
//
// The look is at the source rather than at a token, the way peekIsAnonBody's
// is: reading the next token would consume it, and there is nothing to put it
// back into.
func (p *Parser) condOperandAhead() bool {
	rest := p.condSourceAhead()
	for _, end := range []string{"]]", "&&", "||", ")"} {
		if strings.HasPrefix(rest, end) {
			return false
		}
	}
	return rest != ""
}

// condBinaryOperatorAhead reports whether the word behind the operator
// current is a two-operand operator, which makes the word in hand that
// operator's *left operand* rather than an operator of its own.
//
// The row this is for: `[[ -Q == bar ]]` is a string comparison answering 1
// in zsh 5.9.2 where `[[ -Q x ]]` is `unknown condition: -Q`, and
// `[[ -1 -lt 2 ]]` is arithmetic answering 0 — measured 2026-09-22. A rule
// that made every `-word` an operator would refuse both, and a negative
// number on the left of a comparison is not a rare thing to write.
//
// The same source look as condOperandAhead, and with the same limit: an
// operator that arrives through an expansion is not seen here, because there
// is nothing to look at yet.
func (p *Parser) condBinaryOperatorAhead() bool {
	rest := p.condSourceAhead()
	if strings.HasPrefix(rest, "<") || strings.HasPrefix(rest, ">") {
		return true
	}
	end := 0
	for end < len(rest) && !isBlank(rest[end]) && rest[end] != '\n' {
		end++
	}
	return condBinaryWordOps[rest[:end]]
}

// condSourceAhead is the source standing after the token current, with the
// blanks between them passed over.
func (p *Parser) condSourceAhead() string {
	i := p.lex.off
	for i < len(p.lex.src) && isBlank(p.lex.src[i]) {
		i++
	}
	return p.lex.src[i:]
}

// condUnaryOperatorAhead reports whether the word current stands as the
// operator of a one-operand condition: one this dialect has, or — where it
// looks a condition's name up when it runs — one it has not.
//
// The second half is what makes `[[ -Q x ]]` a condition that parses and is
// refused by name rather than a parse failure, and the two exceptions to it
// are the two lines below: a name with nothing behind it may be an ordinary
// word, and a name with a two-operand operator behind it is that operator's
// left-hand side.
func (p *Parser) condUnaryOperatorAhead() bool {
	if p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	lit := p.tok.Literal()
	switch {
	case p.condUnaryOp(lit):
		return p.condOperatorHasItsOperand(lit)
	case p.condUnknownOp(lit):
		return p.condOperatorHasItsOperand(lit) && !p.condBinaryOperatorAhead()
	}
	return false
}

// failCondTerm is a condition the grammar wanted and did not find: after the
// `[[` itself, after a `!`, after a connective, or just inside a group.
//
// Five sites and one sentence, and until #2909 that sentence was the parser's
// own prose — `expected a condition after [[` — which no shell in the panel
// writes. Measured 2026-09-15 over `-c`, each of the five: bash writes a
// `syntax error near` quoting the `]]` and echoes the line, zsh a `parse
// error near` quoting the same, and both name the *token the reading stopped
// on*. So this is the
// ordinary unexpected-token path and not a message of the construct's own,
// which is the same answer #2013 reached for the arithmetic command and
// #2872's `]]` test reached one line further down.
//
// What is recorded beside it is where the token stood, because one dialect
// words a token refused *here* differently from one refused after a condition
// has been read — and for the closer itself writes no extra line at all. See
// Diagnostics.CondCommandPreamble.
//
// Two of the three columns are only *partly* answered by this, and the `-c`
// route cannot tell: zsh and ksh93 do not refuse the closer as a token at all
// — they read it as an ordinary word and blame whatever stands after it, so
// the same file with a trailing newline is a parse error at the newline in
// both of them, on line 2, against bash's unmoved two lines. Under
// `-c` there is nothing after it, which is why zsh's line and this one agree
// byte for byte and the agreement proves less than it looks. That is a change
// to what the parser *consumes* rather than to how a refusal is worded, so it
// is measured and filed rather than guessed at here (#2964).
func (p *Parser) failCondTerm() {
	if p.err != nil {
		// A site further in has already recorded this failure, and it is the
		// one that knows how deep it was. Every one of the five is reached
		// on the way back out — a group whose condition is missing returns
		// nil to the connective that called it, which returns nil to the
		// `[[` — so without this the innermost group's depth was overwritten
		// by the outermost frame's, and every nesting reported one.
		return
	}
	after := p.dialect.ConditionTermMissingBlamesTheTokenAfterTheCloser && p.atWord("]]")
	if after {
		// The closer is consumed and the complaint falls on whatever stands
		// behind it, at that token's own line — see
		// [Dialect.ConditionTermMissingBlamesTheTokenAfterTheCloser]. Where
		// nothing does, the `]]` is still the last token read and is what
		// gets named, which is the same sentence this shell would have
		// written anyway.
		p.lex.inCondition = false
		p.next()
		if p.at(TokNewline) {
			// Blank lines are passed over, so the complaint lands on the
			// next real token rather than on the first newline — measured,
			// three blank lines before an `echo` put it on the `echo`'s
			// line. The first newline is kept and put back where the input
			// holds nothing else, which is the shape a file ending in one
			// has and is what that route's row is about.
			newline := p.tok
			p.skipNewlines()
			if p.at(TokEOF) {
				p.tok = newline
			}
		}
	}
	p.failUnexpected("]]")
	var se *Error
	if errors.As(p.err, &se) {
		se.CondTermMissing, se.CondGroupsOpen = true, p.condGroups
		if after {
			// And the words of the group are **not** collected, because in
			// that reading there is no group: the refusal is about the one
			// token the parse stopped on, and the words behind it were never
			// read as a condition's. An empty list rather than a nil one is
			// what says so, recordCondGroup leaving a list that is already
			// set alone.
			se.CondWords = []string{}
		}
	}
	p.blameCondition(p.condStart)
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
			p.failCondTerm()
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
			p.failCondTerm()
			return x
		}
		x = &CondLogic{Op: "&&", X: x, Y: y}
	}
	return x
}

func (p *Parser) condPrimary() CondExpr {
	// Every primary is the start of a group as far as the one dialect that
	// counts a condition's words is concerned: `[[`, a `(`, a `!` and either
	// connective all arrive here, and the list below is rebuilt from
	// whichever of them this is. See Parser.condWords.
	p.condWords = nil
	switch {
	case p.err != nil, p.at(TokEOF):
		return nil

	case p.atWord("!"):
		start := p.tok.Pos
		p.next()
		p.skipNewlines()
		x := p.condPrimary()
		if x == nil {
			p.failCondTerm()
			return nil
		}
		return &CondNot{X: x, Start: start}

	case p.at(TokLeftParen):
		// Grouping, not a subshell: nothing here runs.
		start := p.tok.Pos
		p.next()
		p.skipNewlines()
		p.condGroups++
		x := p.condOr()
		if x == nil {
			// Still counted as open, because the failure is *inside* it:
			// one dialect writes a line per group the refusal was under.
			p.failCondTerm()
			p.condGroups--
			return nil
		}
		p.condGroups--
		// Nor here, and for the same reason.
		stop := p.tok.End
		if !p.at(TokRightParen) {
			p.fail("expected ) in a condition")
			p.recordCondGroup()
			return nil
		}
		p.next()
		return &CondGroup{X: x, Start: start, Stop: stop}

	case p.tok.Kind == TokWord && !p.tok.IsQuoted() &&
		p.completionConditionAhead() &&
		p.condOperatorHasItsOperand(p.tok.Literal()):
		return p.condCompletion()

	case p.condUnaryOperatorAhead():
		op, start, stop := p.tok.Literal(), p.tok.Pos, p.tok.End
		// A name this dialect has no condition for is refused when it runs,
		// whatever its arity — so the one-operand reading below, which is an
		// ordinary condition for a known operator, is the refusal for this
		// one. See Parser.condUnknownOp.
		unknown := p.condUnknownOp(op)
		p.next()
		// The word after the operand is the condition's *third*, which is
		// the one position a `(` belongs to the word in. `p.condWord()`
		// hands back the operand already read and lexes the token behind
		// it, so the lexer is told here and taken back on the line after.
		// See [Dialect.ConditionOperandMayOpenWithAGroup].
		p.lex.inCondOperandGroup = p.dialect.ConditionOperandMayOpenWithAGroup
		x := p.condWord()
		p.lex.inCondOperandGroup = false
		if x == nil {
			if p.dialect.ConditionIsResolvedWhenItRuns && p.err == nil {
				// No operand at all, which this dialect accepts and refuses
				// when it runs. p.err is checked because condWord answers
				// nil for a refusal of its own as well — a process
				// substitution out of place — and that one is a real parse
				// failure rather than an arity.
				return &CondUnknown{Op: op, Start: start, Stop: stop}
			}
			p.failCondOperand(op, "unary")
			return nil
		}
		if surplus := p.condSurplusOperands(x); surplus != nil {
			return &CondUnknown{Op: op, Words: surplus, Start: start, Stop: stop}
		}
		if unknown {
			return &CondUnknown{Op: op, Words: []*Word{x}, Start: start, Stop: stop}
		}
		return &CondUnary{Op: op, X: x, Start: start}
	}

	left := p.condTermWord()
	if left == nil {
		return nil
	}
	// The plain form, and the only one whose words are recorded: see
	// Error.CondWords for why the operator forms are not.
	p.condWords = append(p.condWords, PrintWord(left))
	op := p.condOperator()
	if op == "" {
		if p.condNewlineAfterATermsFirstWord() {
			return nil
		}
		// A bare word is a test for non-emptiness.
		return &CondUnary{Op: "-n", X: left, Start: left.Pos()}
	}
	p.condWords = append(p.condWords, op)
	right := p.condWord()
	if right == nil {
		p.failCondOperand(op, "binary")
		return nil
	}
	p.condWords = append(p.condWords, PrintWord(right))
	return &CondBinary{Op: op, X: left, Y: right}
}

// condNewlineAfterATermsFirstWord refuses the newline standing behind a
// condition term whose first word has been read and whose shape is not yet
// decided — a binary operator may still follow it — for the dialects that
// will not take one there.
//
// The position the parser is *at* rather than the first newline: blank lines
// are passed over and the complaint lands where the next real token is, with
// `newline` still the word quoted. Measured on ksh93u+, three blank lines
// between `[[ y` and an `echo` putting the complaint on the `echo`'s line.
// See [Dialect.ConditionNewlineMayFollowATermsFirstWord] for the nine rows
// and for the three that say the question is asked at one position only.
//
// A synthetic token rather than a message of its own, so that every dialect
// words this the way it words any other token the grammar did not want — and
// so that the run-out case is a newline too, which is what the column that
// names it does: `[[ y` and a final newline is `newline' unexpected` there
// where the same text without one is “ `[[' unmatched “.
func (p *Parser) condNewlineAfterATermsFirstWord() bool {
	if p.dialect.ConditionNewlineMayFollowATermsFirstWord || !p.at(TokNewline) {
		return false
	}
	nl := p.tok
	for p.at(TokNewline) {
		// The *last* of the blank lines rather than the token behind them:
		// the complaint is located there, and a newline's own position is
		// what numbers the line after it.
		nl = p.tok
		p.next()
	}
	p.failUnexpectedAt(nl, "]]", false)
	var se *Error
	if errors.As(p.err, &se) {
		se.CondTermUndecided = true
	}
	p.recordCondGroup()
	p.blameCondition(p.condStart)
	return true
}

// recordCondGroup writes the words of the condition group onto the failure
// just recorded: the ones already read, and the ones still standing between
// here and the group's closer.
//
// The rest are read *after* the failure rather than looked at before it,
// because the refusal has to name the token the parser stopped on and reading
// ahead would move it. The error is put aside for the length of the scan so
// that the reads themselves do not fail against it, and put back afterwards;
// nothing the parser does from here on is kept, since the caller returns
// straight into a failed parse.
//
// Fewer than two words is not this refusal's shape and is left alone: a group
// that never got past its first word is refused by naming the token, the way
// every other dialect names it.
func (p *Parser) recordCondGroup() {
	var se *Error
	if !errors.As(p.err, &se) || se.CondWords != nil {
		return
	}
	words := append([]string(nil), p.condWords...)
	saved := p.err
	p.err = nil
	for p.err == nil && p.tok.Kind == TokWord && !p.atWord("]]") {
		w := p.word()
		if w == nil {
			break
		}
		words = append(words, PrintWord(w))
	}
	p.err = saved
	if len(words) < 2 {
		return
	}
	if namedConditionWord(words[0]) || namedConditionWord(words[1]) {
		// A `-word` long enough to be a *named* condition is refused by
		// name in that shell — `[[ p -zz q ]]` and `[[ p -prefix q ]]` are
		// both `unknown condition: …` there, and `[[ -zz x ]]` prints the
		// line before it first, so that refusal happens when the condition
		// runs rather than while it is read. It is a different shape from
		// the one this list is for and is left to the ordinary token
		// refusal (#2846, and the run-time half of #965).
		return
	}
	se.CondWords = words
}

// namedConditionWord reports whether a word is long enough to be looked up as
// a *named* condition rather than read as a single-letter test.
//
// The two-operand word operators are the exception, because they are the only
// long `-word`s that legitimately stand between two operands: `[[ p -eq q r ]]`
// is the group refusal this excludes everything else from, and `[[ p -zz q ]]`
// is not.
func namedConditionWord(s string) bool {
	return strings.HasPrefix(s, "-") && len(s) > 2 && !condBinaryWordOps[s]
}

// condSurplusOperands reads the words standing after a one-operand test that
// already has its operand, for the dialect that accepts them — or nil where
// there are none, where the dialect refuses them, or where the operand is
// itself an operator.
//
// The last of those is the row that makes this a rule rather than "everything
// after the operand is surplus". Measured on zsh 5.9.2: `[[ -n -z x ]]` is
// “ parse error near `x' “ and `[[ -n -n ]]` is 0, so an operator-shaped
// operand is read as beginning something of its own and the word after it is
// simply unexpected — where `[[ -n x y ]]`, whose operand is an ordinary
// word, is the run-time refusal this collects for.
//
// The words are returned with the operand in front of them, because what the
// refusal names is the operator and what a printer writes back is the line.
func (p *Parser) condSurplusOperands(operand *Word) []*Word {
	if !p.dialect.ConditionIsResolvedWhenItRuns || p.err != nil {
		return nil
	}
	if lit, ok := unquotedLiteralWord(operand); ok && p.condUnaryOp(lit) {
		return nil
	}
	var extra []*Word
	for p.err == nil && p.tok.Kind == TokWord && !p.atWord("]]") {
		w := p.condWord()
		if w == nil {
			break
		}
		extra = append(extra, w)
	}
	if len(extra) == 0 {
		return nil
	}
	return append([]*Word{operand}, extra...)
}

// unquotedLiteralWord is a word's text where the word is one unquoted literal
// span, which is what an operator has to be written as.
func unquotedLiteralWord(w *Word) (string, bool) {
	if w == nil || len(w.Spans) != 1 {
		return "", false
	}
	s := w.Spans[0]
	if s.Kind != Literal || s.Quoting != Unquoted {
		return "", false
	}
	return s.Value, true
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
	p.recordCondGroup()
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
	// Whatever the operator, the token it is followed by is the condition's
	// third word, where one dialect reads a leading `(` as the word's. Told
	// to the lexer before each `p.next()` below, which is what fetches that
	// token, and taken back after.
	group := p.dialect.ConditionOperandMayOpenWithAGroup
	switch p.tok.Kind {
	case TokLess:
		p.lex.inCondOperandGroup = group
		p.next()
		p.lex.inCondOperandGroup = false
		return "<"
	case TokGreat:
		p.lex.inCondOperandGroup = group
		p.next()
		p.lex.inCondOperandGroup = false
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
		// And every other operator's operand takes a leading group too,
		// which is the third word's position rather than the operator's.
		p.lex.inCondOperandGroup = group
		p.next()
		p.lex.inRegex = false
		p.lex.inPattern = false
		p.lex.inCondOperandGroup = false
		return op
	}
	return ""
}
