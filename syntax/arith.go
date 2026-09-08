// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"unicode/utf8"
)

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

// ArithCharCode is the character-code operator: `#name` is the code of the
// first character of that parameter's value, and `##c` the code of the
// character written out.
//
// Not a unary operator, because its operand is not an expression: `#b` reads
// the *parameter* b rather than b's arithmetic value, and `##a` reads a
// character where an expression would read a name. It is a primary, and the
// two spellings differ in how the operand is read rather than in what the
// result means.
type ArithCharCode struct {
	// Op is `#` or `##`, kept because it decides how Char is read: `$((##\n))`
	// is a newline and `$((#\n))` the letter n.
	Op string
	// Name is the parameter whose first character is taken, empty when the
	// operand is a character or when there is no operand at all. A `#` with
	// nothing it can read is zero rather than a refusal, measured.
	Name string
	// Char is the character the operand spells out, as written — `a`, `\n`,
	// `\x41`. The escapes are decoded where Op says they are, which is done
	// against the dialect's table rather than here.
	Char string
	// Subscripted records a `[…]` written after the name. The shell reads it
	// and then finds nothing under the whole of it, so `$((#a[1]))` is zero
	// however `${a[1]}` reads — measured, and a fact about that shell rather
	// than a rule anything derives.
	Subscripted bool
	Start       Pos
	Stop        Pos
}

func (n *ArithCharCode) Pos() Pos   { return n.Start }
func (n *ArithCharCode) End() Pos   { return n.Stop }
func (n *ArithCharCode) arithNode() {}

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
// ArithIndex is `name[expr]` inside an arithmetic expression.
//
// The subscript is an expression of its own — `a[i+1]` — and neither the name
// nor the subscript is written with a `$`, which is what makes this a node
// rather than an expansion: `${a[1]}` is substituted before the expression is
// read, and `a[1]` is read as part of it.
type ArithIndex struct {
	Name  string
	Index ArithExpr
	// Sub is the subscript exactly as it was written, brackets excluded.
	//
	// Kept beside the parsed expression because a subscript is only an
	// expression when the name is an indexed array. On an associative one it
	// is a key — `m[k]` is the element stored under the two characters, not
	// the one at whatever `k` evaluates to — and which of the two a name is
	// cannot be known until it runs. Measured unanimous in the three shells
	// with the attribute, whitespace included: `m[ k ]` is a different key
	// from `m[k]`, in an expression exactly as in `${m[ k ]}`.
	Sub   string
	Start Pos
	Stop  Pos
}

func (n *ArithIndex) Pos() Pos   { return n.Start }
func (n *ArithIndex) End() Pos   { return n.Stop }
func (n *ArithIndex) arithNode() {}

// ArithCall is `name(args)` inside an arithmetic expression: a *math
// function*, whose value is produced by running a shell function.
//
// One shell in the panel has the construct at all — zsh, through
// `functions -M` — so the grammar is a dialect's, [Dialect.ArithFunctionCall],
// and where it is off the `(` after a name is a leftover operator, which is
// the complaint bash, dash and ksh93 make about `mf(5)`.
//
// The name is not resolved here and does not have to exist: an unregistered
// name is a runtime failure with its own sentence, not a parse error, which is
// measured — `$(( nosuchmf(1) ))` is `unknown function: nosuchmf` in zsh 5.9.2
// and never a syntax complaint.
type ArithCall struct {
	Name string
	// Args are the arguments, each an expression of its own. `mf()` with
	// none is a call and not a name: zsh registers a math function with a
	// minimum of zero and takes `$(( mf() ))`.
	Args []ArithExpr
	// Text is the call exactly as it was written, from the first byte of
	// the name through the closing parenthesis.
	//
	// Kept because the one shell with the construct quotes it back verbatim
	// when the count of arguments is wrong: measured 2026-09-08,
	// `$(( mf( 5 , 6 ) ))` against a one-argument registration is
	// `wrong number of arguments: mf( 5 , 6 )`, spaces and all, so the
	// sentence is the source text rather than a rendering of the tree.
	Text  string
	Start Pos
	Stop  Pos
}

func (n *ArithCall) Pos() Pos   { return n.Start }
func (n *ArithCall) End() Pos   { return n.Stop }
func (n *ArithCall) arithNode() {}

type ArithAssign struct {
	Name string
	// Index is the subscript when the target is an array element, as in
	// `(( a[0] = 9 ))`, and nil when the target is a plain name.
	Index ArithExpr
	// Sub is the subscript as written, for the reason ArithIndex.Sub is.
	Sub   string
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

// hasExpansion reports whether an arithmetic expression contains something
// that has to be expanded before the expression can be read.
//
// `$(( $x$y ))` with x=`1+` and y=`2` is 3 in every shell in the panel: the
// text is substituted first and the *result* is the expression. So an
// expression with a `$` or a backtick in it has no tree until it runs, and a
// tree built now would be built from something that is not the program.
func hasExpansion(src string) bool {
	return strings.ContainsAny(src, "$`")
}

// parseArithLater is parseArith for the places where the text is read as part
// of reading the file, and where a failure is not the file's to report.
//
// No shell in the panel refuses a program for an expression it cannot read.
// `bash -n` takes `((echo hi))`, and so do ksh93 and zsh; the complaint comes
// when the command runs, so a branch that never runs never raises it and
// `false && ((echo hi)); echo reached` reaches the echo in all six. Ours
// refused the whole program instead — a parse-time answer to a run-time
// question, and the one on the conformance list for that reason.
//
// The tree a *successful* read builds is still kept, because it is the same
// tree the run would build and building it once is free. A failure is
// discarded along with the error it recorded, which leaves nil beside the raw
// text — the state an expression with an expansion in it has always been in,
// and the state [Runner.arithTree] already reads: expand, then parse, then
// evaluate. So there is nothing new on the far side of this.
//
// Rolling the error back rather than not looking is what keeps it to one
// question. The read has to happen either way for the tree, and the error is
// the only part of it that was ever wrong.
func (p *Parser) parseArithLater(src string, at Pos) ArithExpr {
	saved := p.err
	e := p.parseArith(src, at)
	if p.err != saved {
		p.err = saved
		return nil
	}
	return e
}

// parseArith parses the text inside `$(( … ))` or `(( … ))`.
//
// An expression containing an expansion is left alone: nil, with the raw text
// still beside it, for the interpreter to expand and read when it runs. That
// is the only thing that can — `$#` is not an operand until something knows
// what the parameters are.
//
// A failure here is recorded on the parser, which is right for the one caller
// that is reading an expression *as* the program — the interpreter, through
// [Parser.ParseArithFor], at the moment the command runs. Everything reading
// an expression while reading a file goes through parseArithLater instead.
func (p *Parser) parseArith(src string, at Pos) ArithExpr {
	if hasExpansion(src) {
		return nil
	}
	a := &arithParser{src: src, at: at, p: p, dial: p.dialect}
	e := a.expr()
	a.space()
	if e != nil && a.off < len(a.src) {
		kind := a.leftoverKind()
		// The two operand and operator failures name everything from here to
		// the end of the expression; the byte's own verdict names the byte.
		token := a.src[a.off:]
		if kind == ErrArithIllegalByte {
			token = a.src[a.off : a.off+1]
		}
		a.failArith(kind, token)
	}
	return e
}

// leftoverKind tells the three ways an expression can have something left
// over apart: an operand where an operator belonged, a byte the dialect's
// reader refuses outright, or text that could be neither.
//
// Only one shell in the panel words all three differently, but the
// distinction is the parser's to make — it is about what the text could have
// been, which is a grammar question and not a wording one.
func (a *arithParser) leftoverKind() ErrorKind {
	c := a.src[a.off]
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z',
		c == '_', c == '(', c == '$':
		return ErrArithOperator
	}
	// Where an operator belonged is one of the two positions the byte's own
	// verdict stands at, the expression having read a complete value and been
	// able to stop.
	if a.refusedOutright() {
		return ErrArithIllegalByte
	}
	return ErrArithBadOperator
}

// refusedOutright reports whether the byte at the cursor is one this dialect's
// arithmetic reader refuses as part of no token.
func (a *arithParser) refusedOutright() bool {
	return a.off < len(a.src) &&
		strings.IndexByte(a.dial.ArithBytesRefusedOutright, a.src[a.off]) >= 0
}

// nothingReadYet reports whether the expression has consumed nothing but
// whitespace, which is the *other* position a refused byte's own verdict
// stands at.
//
// Asked of the text rather than kept as a flag, and that is deliberate: the
// question is "could this expression have stopped here", and the answer is a
// property of what has been consumed, not of which of the seven frames that
// consume an operator happened to recurse last. A flag would have to be set
// at every one of them and would be wrong the first time one was added —
// `$((  @  ))` is `illegal character` in that shell, so the cursor being at
// offset zero is not the test either.
func (a *arithParser) nothingReadYet() bool {
	return strings.TrimLeft(a.src[:a.off], " \t\n") == ""
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
		// `a[0] = 9` assigns to an element, so the subscript belongs to the
		// target rather than being a value of its own.
		index, sub := a.subscript()
		a.space()
		for _, op := range assignOps {
			// `==` is equality, not assignment, so it must not be taken here.
			if op == "=" && a.has("==") {
				break
			}
			if a.take(op) {
				v := a.assign()
				if v == nil {
					a.failArith(ErrArithOperandEnd, op)
					return nil
				}
				return &ArithAssign{Name: name, Index: index, Sub: sub, Op: op, Value: v, Start: start}
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
		return a.power()
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
			a.failArith(ErrArithOperandEnd, op)
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
	//
	// `**` is here only when the dialect has it: where it does not, the
	// first `*` is a multiplication and the second fails as an operand,
	// which is how the shell without the operator reports it.
	return cand == "*" && a.dial.ArithExponent && a.has("**")
}

// power is `**`, when the dialect has it. Measured across the three shells
// that parse it: tighter than `*` (`2*3**2` is 18) and looser than unary
// (`-2**2` is 4 — the sign is part of the base), and right-associative
// (`2**3**2` is 512), which the recursion on the right encodes.
func (a *arithParser) power() ArithExpr {
	x := a.unary()
	if x == nil {
		return nil
	}
	a.space()
	if !a.dial.ArithExponent || !a.take("**") {
		return x
	}
	y := a.power()
	if y == nil {
		a.failArith(ErrArithOperandEnd, "**")
		return x
	}
	return &ArithBinary{Op: "**", X: x, Y: y}
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
			a.failArith(ErrArithOperandEnd, op)
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
	if a.dial.ArithCharacterCode && a.src[a.off] == '#' {
		return a.charCode(start)
	}
	if c := a.src[a.off]; c >= '0' && c <= '9' {
		return a.number(start)
	}
	// A literal may begin with its point where the dialect has floats: `.5`
	// is a number there and is an operator followed by one everywhere else,
	// which is why the two shells without floats blame the `.5` rather than
	// the `1.5` it was part of.
	if a.dial.ArithFloat && a.src[a.off] == '.' && a.off+1 < len(a.src) &&
		a.src[a.off+1] >= '0' && a.src[a.off+1] <= '9' {
		return a.number(start)
	}
	begin := a.off
	if name, ok := a.name(); ok {
		// A `(` *touching* the name is a call, and one with a space before
		// it is not: measured, `$(( mf ( 5 ) ))` against a live registration
		// is `bad math expression: operator expected at `( 5 ) '` where
		// `$(( mf(5) ))` runs the function. So the test is the very next
		// byte, before any space is skipped.
		if a.dial.ArithFunctionCall && a.off < len(a.src) && a.src[a.off] == '(' {
			return a.call(name, start, begin)
		}
		if index, sub := a.subscript(); index != nil {
			return &ArithIndex{Name: name, Index: index, Sub: sub, Start: start, Stop: start}
		}
		return &ArithVar{Name: name, Start: start, Stop: start}
	}
	// A `$` here is a parameter expansion the lexer left in place; both
	// spellings work, so it is read as the same variable reference.
	if a.take("$") {
		if name, ok := a.name(); ok {
			return &ArithVar{Name: name, Start: start, Stop: start}
		}
	}
	// Nothing here could begin a value, which is a missing *operand* and not
	// a leftover operator: `$((.5))` in a dialect without floats is blamed
	// for wanting a number, where `$((1 @))` is blamed for the `@`. Both
	// shells without floats word the two apart.
	//
	// There *is* text here, which is the half that separates this from a
	// caller's ErrArithOperandEnd. Running out is not reported at all —
	// primary returns nil at the top with no error, so the frame that wanted
	// the operand names the operator it was left holding. The two are the
	// same failure to two of the panel and different failures to the other
	// two, and this is the only place the parser can tell them apart.
	//
	// Unless the byte itself is one this dialect refuses and nothing has been
	// read yet, which is the start of the expression: there is no operator to
	// have been left wanting an operand, so the byte answers for itself. The
	// token is the byte alone rather than the rest of the text, because that
	// is what the sentence names.
	if a.refusedOutright() && a.nothingReadYet() {
		a.failArith(ErrArithIllegalByte, a.src[a.off:a.off+1])
		return nil
	}
	a.failArith(ErrArithOperand, a.src[a.off:])
	return nil
}

// call reads `name(a, b)`, the math-function call form, with the name already
// read and begin the offset it started at.
//
// Arguments are parsed at the assignment level rather than through expr,
// because the comma here separates arguments and is not the sequence
// operator: `mf(1,2)` is two arguments in the dialect that also reads
// `$(( 1,2 ))` as a sequence, so the two readings of `,` are told apart by
// which of them is inside the parentheses.
func (a *arithParser) call(name string, start Pos, begin int) ArithExpr {
	a.off++ // the `(`
	n := &ArithCall{Name: name, Start: start, Stop: start}
	a.space()
	if !a.has(")") {
		for {
			arg := a.assign()
			if arg == nil {
				return nil
			}
			n.Args = append(n.Args, arg)
			a.space()
			if a.take(",") {
				a.space()
				// A trailing comma before the `)` is allowed and adds no
				// argument: measured, `mf(5,)` passes one argument and
				// `mf(5,6,)` two. A comma with nothing *before* it is a
				// different matter — `mf(,)` and `mf(5,,6)` are both an
				// operand expected — which is what the loop's shape already
				// says, because an argument is read before every comma.
				if a.has(")") {
					break
				}
				continue
			}
			break
		}
	}
	a.space()
	if !a.take(")") {
		a.p.fail("expected ) in arithmetic")
		return nil
	}
	n.Text = a.src[begin:a.off]
	return n
}

// number reads a literal without converting it, including the `base#digits`
// form where the dialect has it.
func (a *arithParser) number(start Pos) ArithExpr {
	begin := a.off
	for a.off < len(a.src) && isNumByte(a.src[a.off]) {
		a.off++
	}
	if a.dial.ArithFloat && !isBasedLiteral(a.src[begin:a.off]) {
		a.floatTail(begin)
	}
	if a.off < len(a.src) && a.src[a.off] == '#' && a.dial.ArithExplicitBase {
		a.off++
		// The digit set is the base-64 alphabet, not the hex one the scan
		// above uses: `36#z` and `64#_` are numbers where the dialect has
		// explicit bases, and which bases a dialect accepts is decided at
		// conversion, where the number is actually read.
		for a.off < len(a.src) && isBaseDigit(a.src[a.off]) {
			a.off++
		}
	}
	return &ArithNum{Text: a.src[begin:a.off], Start: start, Stop: start}
}

// floatTail extends a literal over the point and exponent a float may carry.
//
// Only where the dialect has floats: `1.5` is one number there and two tokens
// with an operator between them in the shells that do not, which is the
// difference their diagnostics show.
//
// A sign is taken only straight after the exponent letter, so `1e-3` is one
// literal and `1-3` stays a subtraction.
//
// The exponent needs care because the digit scan before this one accepts `e`
// as a hex digit, so `1e-3` arrives with the `e` already consumed and only the
// `-3` left to find.
func (a *arithParser) floatTail(begin int) {
	if a.off < len(a.src) && a.src[a.off] == '.' {
		a.off++
		for a.off < len(a.src) && a.src[a.off] >= '0' && a.src[a.off] <= '9' {
			a.off++
		}
	}
	next := a.off
	if a.off < len(a.src) && (a.src[a.off] == 'e' || a.src[a.off] == 'E') {
		next++
	} else if a.off > begin && (a.src[a.off-1] == 'e' || a.src[a.off-1] == 'E') {
		// Already swallowed by the digit scan, which reads `e` as hex.
	} else {
		return
	}
	if next < len(a.src) && (a.src[next] == '+' || a.src[next] == '-') {
		next++
	}
	if next < len(a.src) && a.src[next] >= '0' && a.src[next] <= '9' {
		a.off = next
		for a.off < len(a.src) && a.src[a.off] >= '0' && a.src[a.off] <= '9' {
			a.off++
		}
	}
}

// isBasedLiteral reports whether a literal states its own base, which takes it
// out of the float rules: `0x1e` is a hex integer whose digits include an `e`,
// not a number with an exponent.
func isBasedLiteral(text string) bool {
	return strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") ||
		strings.Contains(text, "#")
}

// isBaseDigit is the base-64 alphabet a `base#digits` literal may draw on:
// 0-9, both letter cases, `@` and `_`.
func isBaseDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || c == '@' || c == '_'
}

func isNumByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F') || c == 'x' || c == 'X'
}

// subscript reads `[ … ]` after a name, returning nil when there is none.
//
// The inside is an expression, so `a[i+1]` and `a[n]` both work and the
// subscript is evaluated rather than taken as text.
// subscript reads `[…]` after a name, returning both the expression it parses
// as and the text it was written with — see ArithIndex.Sub for why both.
func (a *arithParser) subscript() (ArithExpr, string) {
	// The same flag that admits `${a[1]}`: one question about whether the
	// dialect has subscripts at all, asked in the two places that need it.
	// Where it is off, `a[0]` is a name followed by text that cannot be an
	// operator, and is reported as that.
	if !a.dial.ArraySubscript {
		return nil, ""
	}
	if a.off >= len(a.src) || a.src[a.off] != '[' {
		return nil, ""
	}
	open := a.off
	depth := 0
	for a.off < len(a.src) {
		switch a.src[a.off] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				inner := a.src[open+1 : a.off]
				a.off++
				sub := &arithParser{src: inner, at: a.at, p: a.p, dial: a.dial}
				e := sub.expr()
				sub.space()
				if e == nil || sub.off < len(sub.src) {
					a.p.failKind(ErrArithOperand, "bad array subscript: %s", inner)
					return nil, ""
				}
				return e, inner
			}
		}
		a.off++
	}
	// No closing bracket: not a subscript at all, so the name stands alone
	// and whatever follows is the caller's problem to report.
	a.off = open
	return nil, ""
}

// charCode reads `#name`, `#\c` and `##c`, where the dialect has them.
//
// The operand is not an expression and is read here rather than by recursing:
// `#b` names the parameter b, and the reading stops at the name. Measured on
// zsh 5.9.2, which is the whole panel for this — bash 5.3, bash 3.2, bash as
// `sh`, ksh93 and dash all call `$((#b))` an arithmetic syntax error, and the
// grammar without the flag reaches the same refusal by the same route.
func (a *arithParser) charCode(start Pos) ArithExpr {
	n := &ArithCharCode{Op: "#", Start: start, Stop: start}
	a.off++
	if a.off < len(a.src) && a.src[a.off] == '#' {
		n.Op = "##"
		a.off++
	}
	begin := a.off
	switch {
	case n.Op == "##" || (a.off < len(a.src) && a.src[a.off] == '\\'):
		// One character, and exactly one: `$((##ab))` is the code of `a` with
		// a `b` left over, which the caller then refuses as an operator it
		// cannot read. An escape counts as the character it stands for, so
		// `$((##\x41x))` is the same shape.
		size := charCodeOperandLen(a.src[a.off:], n.Op == "##")
		if size == 0 {
			a.p.failKind(ErrArithCharacterMissing, "character missing after %s", n.Op)
			return nil
		}
		a.off += size
		n.Char = a.src[begin:a.off]
	default:
		// A parameter, which here includes the positional ones: `$((#1))` is
		// the first character of `$1` and `$((#0))` of the shell's own name,
		// so the scan takes digits as readily as letters.
		for a.off < len(a.src) && isCharCodeNameByte(a.src[a.off]) {
			a.off++
		}
		n.Name = a.src[begin:a.off]
		if a.off < len(a.src) && a.src[a.off] == '[' {
			if end := closingBracket(a.src[a.off:]); end > 0 {
				a.off += end + 1
				n.Subscripted = true
			}
		}
	}
	n.Stop = start
	return n
}

// isCharCodeNameByte reports whether a byte can stand in the parameter name
// the character-code operator reads.
func isCharCodeNameByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// charCodeOperandLen is how many bytes the one character after the operator
// occupies, 0 when there is nothing there.
//
// Shapes and not meanings: what an escape *stands for* is the dialect's table,
// decoded where the value is taken, and all this has to know is where the
// operand ends. `escapes` is false for the single-`#` spelling, where a
// backslash takes the next character as itself rather than beginning an escape
// — measured, `$((#\n))` is the letter n where `$((##\n))` is a newline.
func charCodeOperandLen(s string, escapes bool) int {
	if s == "" {
		return 0
	}
	if s[0] != '\\' {
		_, size := utf8.DecodeRuneInString(s)
		return size
	}
	if len(s) == 1 {
		return 0
	}
	if !escapes {
		_, size := utf8.DecodeRuneInString(s[1:])
		return 1 + size
	}
	switch c := s[1]; {
	case c == 'x':
		return 2 + baseDigits(s[2:], 16, 2)
	case c == 'u':
		return 2 + baseDigits(s[2:], 16, 4)
	case c == 'U':
		return 2 + baseDigits(s[2:], 16, 8)
	case c >= '0' && c <= '7':
		return 1 + baseDigits(s[1:], 8, 3)
	}
	_, size := utf8.DecodeRuneInString(s[1:])
	return 1 + size
}

// baseDigits counts how many of the first max bytes are digits in the base.
func baseDigits(s string, base, max int) int {
	n := 0
	for n < len(s) && n < max {
		c := s[n]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		default:
			return n
		}
		if v >= base {
			return n
		}
		n++
	}
	return n
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
