// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// The arithmetic expression tree, from docs/spec/grammar/arithmetic.md.
//
// Operators are held as their source spelling rather than as an enum. There
// are around thirty of them, they are already unambiguous strings, and every
// consumer either switches on them or prints them; a parallel set of constants
// would be a second thing to keep in step for no gain.

// ArithExpr is a node in an arithmetic expression.
type ArithExpr interface {
	Node
	arithNode()
}

// ArithNum is a numeric literal, kept as written.
//
// The text is not converted here, because what it means is a dialect
// question: a leading zero is octal everywhere but zsh, where `0100` is one
// hundred rather than sixty-four. Converting at parse time would bake one
// answer into the tree.
type ArithNum struct {
	Text  string
	Start Pos
	Stop  Pos
}

func (n *ArithNum) Pos() Pos   { return n.Start }
func (n *ArithNum) End() Pos   { return n.Stop }
func (n *ArithNum) arithNode() {}

// ArithVar is a bare name, which inside arithmetic is a variable reference.
// An unset one is zero rather than an error.
type ArithVar struct {
	Name  string
	Start Pos
	Stop  Pos
}

func (n *ArithVar) Pos() Pos   { return n.Start }
func (n *ArithVar) End() Pos   { return n.Stop }
func (n *ArithVar) arithNode() {}

// ArithUnary is a prefix or postfix operator.
type ArithUnary struct {
	Op string // + - ~ ! ++ --
	// Postfix distinguishes `x++` from `++x`, which differ in what they
	// evaluate to even though they have the same effect.
	Postfix bool
	X       ArithExpr
	Start   Pos
}

func (n *ArithUnary) Pos() Pos   { return n.Start }
func (n *ArithUnary) End() Pos   { return n.X.End() }
func (n *ArithUnary) arithNode() {}

// ArithBinary is any infix operator, including the sequence operator.
type ArithBinary struct {
	Op   string
	X, Y ArithExpr
}

func (n *ArithBinary) Pos() Pos   { return n.X.Pos() }
func (n *ArithBinary) End() Pos   { return n.Y.End() }
func (n *ArithBinary) arithNode() {}

// ArithCond is `c ? a : b`.
type ArithCond struct {
	Cond, Then, Else ArithExpr
}

func (n *ArithCond) Pos() Pos   { return n.Cond.Pos() }
func (n *ArithCond) End() Pos   { return n.Else.End() }
func (n *ArithCond) arithNode() {}

// ArithAssign is `x = e` and its compound forms.
//
// It is a separate node rather than a binary operator because its effect
// outlives the expression — `$((x=5))` leaves x set — which combined with
// short-circuiting makes evaluation order part of the specification.
type ArithAssign struct {
	Name  string
	Op    string // = += -= *= /= %= <<= >>= &= ^= |=
	Value ArithExpr
	Start Pos
}

func (n *ArithAssign) Pos() Pos   { return n.Start }
func (n *ArithAssign) End() Pos   { return n.Value.End() }
func (n *ArithAssign) arithNode() {}

// arithLevels are the binary operators by precedence, loosest first. The order
// follows ISO C, which POSIX defers to.
var arithLevels = [][]string{
	{"||"},
	{"&&"},
	{"|"},
	{"^"},
	{"&"},
	{"==", "!="},
	{"<=", ">=", "<", ">"},
	{"<<", ">>"},
	{"+", "-"},
	{"*", "/", "%"},
}

var assignOps = []string{"<<=", ">>=", "*=", "/=", "%=", "+=", "-=", "&=", "^=", "|=", "="}

// arithParser is a precedence-climbing parser over one expression's text.
type arithParser struct {
	src  string
	off  int
	at   Pos
	p    *Parser
	dial Dialect
}

// parseArith parses the text inside `$(( … ))` or `(( … ))`.
func (p *Parser) parseArith(src string, at Pos) ArithExpr {
	a := &arithParser{src: src, at: at, p: p, dial: p.dialect}
	e := a.expr()
	a.space()
	if e != nil && a.off < len(a.src) {
		a.failArith(ErrArithOperator, a.src[a.off:])
	}
	return e
}

// failArith records an arithmetic failure with the whole expression and the
// part of it that failed.
//
// It is not p.fail: a shell does not call this a syntax error. All four report
// it the way they report a division by zero — the expression quoted, and a
// reason — so it travels as its own kind and is worded by the same vector.
func (a *arithParser) failArith(kind ErrorKind, token string) {
	if a.p.err != nil {
		return
	}
	a.p.err = &Error{
		Pos: a.at, Kind: kind, Expr: a.src, Token: token,
		Msg: "arithmetic expression: " + a.src,
	}
}

func (a *arithParser) space() {
	for a.off < len(a.src) && (a.src[a.off] == ' ' || a.src[a.off] == '\t' || a.src[a.off] == '\n') {
		a.off++
	}
}

func (a *arithParser) has(s string) bool {
	return strings.HasPrefix(a.src[a.off:], s)
}

func (a *arithParser) take(s string) bool {
	if a.has(s) {
		a.off += len(s)
		return true
	}
	return false
}

// expr is the loosest level: the sequence operator, when the dialect has it.
func (a *arithParser) expr() ArithExpr {
	x := a.assign()
	if x == nil {
		return nil
	}
	for {
		a.space()
		if !a.has(",") || !a.dial.ArithComma {
			return x
		}
		a.off++
		y := a.assign()
		if y == nil {
			a.p.fail("expected an expression after , in arithmetic")
			return x
		}
		x = &ArithBinary{Op: ",", X: x, Y: y}
	}
}

// assign handles assignment, which is right-associative and only valid with a
// name on the left.
func (a *arithParser) assign() ArithExpr {
	save := a.off
	a.space()
	start := a.at
	if name, ok := a.name(); ok {
		a.space()
		for _, op := range assignOps {
			// `==` is equality, not assignment, so it must not be taken here.
			if op == "=" && a.has("==") {
				break
			}
			if a.take(op) {
				v := a.assign()
				if v == nil {
					a.failArith(ErrArithOperand, op)
					return nil
				}
				return &ArithAssign{Name: name, Op: op, Value: v, Start: start}
			}
		}
	}
	a.off = save
	return a.ternary()
}

func (a *arithParser) ternary() ArithExpr {
	cond := a.binary(0)
	if cond == nil {
		return nil
	}
	a.space()
	if !a.take("?") {
		return cond
	}
	then := a.assign()
	a.space()
	if !a.take(":") {
		a.p.fail("expected : in an arithmetic conditional")
		return cond
	}
	els := a.assign()
	if then == nil || els == nil {
		a.p.fail("incomplete arithmetic conditional")
		return cond
	}
	return &ArithCond{Cond: cond, Then: then, Else: els}
}

func (a *arithParser) binary(level int) ArithExpr {
	if level >= len(arithLevels) {
		return a.unary()
	}
	x := a.binary(level + 1)
	if x == nil {
		return nil
	}
	for {
		a.space()
		op := ""
		for _, cand := range arithLevels[level] {
			if !a.has(cand) {
				continue
			}
			// Longest match: `<` must not be taken out of `<<` or `<=`, and
			// `&` not out of `&&`.
			if longer := a.longerOperator(cand); longer {
				continue
			}
			op = cand
			break
		}
		if op == "" {
			return x
		}
		a.off += len(op)
		y := a.binary(level + 1)
		if y == nil {
			a.failArith(ErrArithOperand, op)
			return x
		}
		x = &ArithBinary{Op: op, X: x, Y: y}
	}
}

// longerOperator reports whether a longer operator starts here, so a shorter
// one at a looser level is not taken by mistake.
func (a *arithParser) longerOperator(cand string) bool {
	for _, longer := range []string{"<<=", ">>=", "&&", "||", "<<", ">>", "<=", ">=", "==", "!="} {
		if len(longer) > len(cand) && a.has(longer) && strings.HasPrefix(longer, cand) {
			return true
		}
	}
	// `x =` after an operator candidate like `<` is fine; only compound
	// assignment matters, and those are caught above.
	return false
}

func (a *arithParser) unary() ArithExpr {
	a.space()
	start := a.at
	switch {
	case a.has("++"), a.has("--"):
		op := a.src[a.off : a.off+2]
		if !a.dial.ArithIncDec {
			a.p.fail("%s is not available in this dialect", op)
			return nil
		}
		a.off += 2
		x := a.unary()
		if x == nil {
			return nil
		}
		return &ArithUnary{Op: op, X: x, Start: start}
	case a.has("+"), a.has("-"), a.has("~"), a.has("!"):
		op := a.src[a.off : a.off+1]
		a.off++
		x := a.unary()
		if x == nil {
			a.failArith(ErrArithOperand, op)
			return nil
		}
		return &ArithUnary{Op: op, X: x, Start: start}
	}
	return a.postfix()
}

func (a *arithParser) postfix() ArithExpr {
	x := a.primary()
	if x == nil {
		return nil
	}
	a.space()
	if (a.has("++") || a.has("--")) && a.dial.ArithIncDec {
		op := a.src[a.off : a.off+2]
		a.off += 2
		return &ArithUnary{Op: op, Postfix: true, X: x, Start: x.Pos()}
	}
	return x
}

func (a *arithParser) primary() ArithExpr {
	a.space()
	start := a.at
	if a.off >= len(a.src) {
		return nil
	}
	if a.take("(") {
		x := a.expr()
		a.space()
		if !a.take(")") {
			a.p.fail("expected ) in arithmetic")
		}
		return x
	}
	if c := a.src[a.off]; c >= '0' && c <= '9' {
		return a.number(start)
	}
	if name, ok := a.name(); ok {
		return &ArithVar{Name: name, Start: start, Stop: start}
	}
	// A `$` here is a parameter expansion the lexer left in place; both
	// spellings work, so it is read as the same variable reference.
	if a.take("$") {
		if name, ok := a.name(); ok {
			return &ArithVar{Name: name, Start: start, Stop: start}
		}
	}
	a.failArith(ErrArithOperator, a.src[a.off:])
	return nil
}

// number reads a literal without converting it, including the `base#digits`
// form where the dialect has it.
func (a *arithParser) number(start Pos) ArithExpr {
	begin := a.off
	for a.off < len(a.src) && isNumByte(a.src[a.off]) {
		a.off++
	}
	if a.off < len(a.src) && a.src[a.off] == '#' && a.dial.ArithExplicitBase {
		a.off++
		for a.off < len(a.src) && isNumByte(a.src[a.off]) {
			a.off++
		}
	}
	return &ArithNum{Text: a.src[begin:a.off], Start: start, Stop: start}
}

func isNumByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F') || c == 'x' || c == 'X'
}

func (a *arithParser) name() (string, bool) {
	if a.off >= len(a.src) || !isNameStart(a.src[a.off]) {
		return "", false
	}
	begin := a.off
	for a.off < len(a.src) && isNameByte(a.src[a.off], a.off-begin) {
		a.off++
	}
	return a.src[begin:a.off], true
}
