// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// arithError is a failure inside an expression, such as dividing by zero.
// It is a runtime error rather than a syntax one: the expression parsed.
//
// token is the part of the expression the failure is attributed to, which
// bash prints and the other three do not. Empty means "the whole
// expression", which is both a sensible default and what bash itself says
// when the failing literal is the entire thing: `$((08))` names `08`.
type arithError struct {
	msg   string
	token string
	// blameTail says the blamed text runs from where the failing operand was
	// written to the *end* of the expression, rather than being the operand
	// alone. Measured 2026-09-12 on bash 5.3.15, which parts the two by where
	// the failure was raised:
	//
	//	$(( 1/0 + 2 ))   1/0 + 2 : division by 0 (error token is "0 + 2 ")
	//	$(( 8#9 + 1 ))   8#9: value too great for base (error token is "8#9")
	//
	// An *evaluation* failure names the tail; a literal the number reader
	// refused names the literal, and the expression stops there. So it is a
	// property of the failure and not of the dialect — the other five columns
	// quote no token at all, and the one that does quotes both shapes.
	blameTail bool
	// from is where that tail begins, as a byte offset into the expression's
	// own text, and -1 where only the token's text is known. The tree cannot
	// always answer it: `1/(0)` is blamed on `(0) ` and the parenthesised
	// divisor has no text of its own, so the offset comes from the parser —
	// see [syntax.ArithBinary].YStart.
	from int
	// complete says the message is the whole diagnostic and must not be
	// wrapped. Measured: dash wraps a division by zero — `arithmetic
	// expression: division by zero: "1/0"` — but reports a non-numeric
	// operand bare, as `Illegal number: abc`. One wrapper for every
	// arithmetic failure would put a shape on the second that dash does not
	// use, so the failure says which it is.
	complete bool
}

func (e arithError) Error() string { return e.msg }

// arithFailure is the sentence a dialect writes about an expression that
// would not evaluate, given the text it was written as and what went wrong.
//
// One function rather than one per caller, because an expression is written
// in more places than `$(( ))` and every one of them is reported the same way
// by every shell measured. A subscript is the case that proved it: `${a[b c]}`
// is the identical complaint to `$((b c))` in all four, and reached its own
// code path, which said nothing at all.
func (r *Runner) arithFailure(text string, err error) string {
	// The expression as written is what dash and ksh93 quote back, and the
	// caller still has it: the parser keeps the raw text beside the tree it
	// built from it.
	ae, _ := err.(arithError)
	token := ae.token
	expr := r.diag().arithBlamedText(text)
	if ae.blameTail {
		if at := ae.tailStart(text, token); at >= 0 {
			token = text[at:]
		}
	}
	if token == "" {
		// bash blames the whole expression when the failing part is the whole
		// expression, which is also the honest answer when the tree cannot
		// name a smaller piece.
		token = expr
	}
	if ae.complete {
		return err.Error()
	}
	if r.diag().ArithErrorNamesThePrefix && token != "" {
		// One dialect's leading position is what it had consumed when the
		// token failed: `08+1` is blamed as `08` and `1+08` as `1+08`.
		if i := strings.Index(expr, token); i >= 0 {
			expr = expr[:i+len(token)]
		}
	}
	return Wording(r.diag().ArithError, "%[2]s", expr, err.Error(), token)
}

// tailStart is where the blamed tail begins in text, or -1 when nothing in
// the failure locates it.
//
// The recorded offset is preferred over a search because a search cannot tell
// two identical operands apart: `$(( 0/0 ))` is blamed on the *divisor*, and
// the first `0` in the text is the dividend.
func (e arithError) tailStart(text, token string) int {
	if e.from >= 0 && e.from <= len(text) {
		return e.from
	}
	if token == "" {
		return -1
	}
	return strings.Index(text, token)
}

// arithToken names the part of an expression a failure should be blamed on.
//
// Only a literal and a bare name can be named this way. Anything else — a
// parenthesised sub-expression, say — has no text of its own in the tree,
// because the parser records what an operand *is* rather than the characters
// it was written with. Those return empty and are reported against the whole
// expression, which is the honest answer rather than a reconstructed one.
func arithToken(e syntax.ArithExpr) string {
	switch x := e.(type) {
	case *syntax.ArithNum:
		return x.Text
	case *syntax.ArithVar:
		return x.Name
	case *syntax.ArithUnary:
		// A signed operand is blamed on its leaf: `2**-1` names the `1`,
		// which is measured — the shell that names error tokens reads the
		// sign as part of the expression and stops on the literal.
		if !x.Postfix {
			return arithToken(x.X)
		}
	}
	return ""
}

// evalArith evaluates an expression tree.
//
// Evaluation order is part of the specification rather than an implementation
// detail, because assignment is an operator whose effect outlives the
// expression: `x=0; $((0 && (x=9)))` must leave x alone. So the logical
// operators short-circuit here, and nothing evaluates both sides eagerly.
// arithNum is a value in an arithmetic expression.
//
// Two shells in the panel do floating point and two do not, and an expression
// in the two that do is integer until a float enters it: `3/2` is 1 there as
// well, and `3.0/2` is 1.5. So a value carries which it is rather than being
// promoted everywhere, and the promotion happens per operation.
type arithNum struct {
	i     int
	f     float64
	float bool
}

func intNum(i int) arithNum       { return arithNum{i: i} }
func floatNum(f float64) arithNum { return arithNum{f: f, float: true} }

// asFloat is the value as a float, whichever it is.
func (n arithNum) asFloat() float64 {
	if n.float {
		return n.f
	}
	return float64(n.i)
}

// asInt truncates, which is what an integer context does with a float: an
// array subscript, the truth of `(( ))`, or a shell that has no floats at all.
func (n arithNum) asInt() int {
	if n.float {
		return int(n.f)
	}
	return n.i
}

func (n arithNum) isZero() bool {
	if n.float {
		return n.f == 0
	}
	return n.i == 0
}

// evalArith is the integer answer, for the callers that can only use one: an
// array subscript, the truth test of `(( ))`, the integer attribute.
func (r *Runner) evalArith(e syntax.ArithExpr) (int, error) {
	v, err := r.evalNum(e)
	return v.asInt(), err
}

// evalNum is the value of an expression, and the point where the shell's
// record of the *last* arithmetic value is kept up to date.
//
// The record is written here, at every node, rather than at the outermost one
// only, and that is what makes the reading right: a math function's value is
// the last arithmetic evaluated anywhere during its call, and the arguments of
// the call are part of that — which is why an implementation that evaluates
// nothing at all hands back its last argument. See mathfunc.go for the
// measurement.
func (r *Runner) evalNum(e syntax.ArithExpr) (arithNum, error) {
	v, err := r.evalNumNode(e)
	if err == nil {
		r.lastArith = v
	}
	return v, err
}

func (r *Runner) evalNumNode(e syntax.ArithExpr) (arithNum, error) {
	switch x := e.(type) {
	case nil:
		return intNum(0), nil

	case *syntax.ArithNum:
		return r.parseArithNum(x.Text)

	case *syntax.ArithVar:
		return r.arithValueOf(x.Name)

	case *syntax.ArithIndex:
		return r.arithElement(x)

	case *syntax.ArithCharCode:
		return intNum(r.charCode(x)), nil

	case *syntax.ArithOutput:
		// The value is the operand's, unchanged: the specifier decides how the
		// answer is written and never what it is. See arithoutput.go.
		return r.evalArithOutput(x)

	case *syntax.ArithCall:
		// The seam: an expression that runs a shell function. See
		// mathfunc.go, which is where everything about it lives.
		return r.evalMathFunc(x)

	case *syntax.ArithUnary:
		return r.evalUnary(x)

	case *syntax.ArithCond:
		c, err := r.evalNum(x.Cond)
		if err != nil {
			return intNum(0), err
		}
		if !c.isZero() {
			return r.evalNum(x.Then)
		}
		return r.evalNum(x.Else)

	case *syntax.ArithAssign:
		return r.evalAssign(x)

	case *syntax.ArithBinary:
		return r.evalBinary(x)
	}
	return intNum(0), arithError{msg: fmt.Sprintf("unsupported expression %T", e)}
}

// arithElement reads `a[i]` written inside an expression.
//
// The subscript is an expression, so it is evaluated first; the base it counts
// from is the dialect's, the same one `${a[1]}` uses. An element that is not
// there is zero rather than an error, which is what every shell in the panel
// does with a subscript past the end and with a name that was never an array.
func (r *Runner) arithElement(x *syntax.ArithIndex) (arithNum, error) {
	// The name first, where the dialect looks at the name first: one shell
	// answers zero for a name that is not there without reading the brackets
	// at all, so `$(( nodecl[1/0] ))` divides nothing and `$(( nodecl[i++] ))`
	// steps nothing. Ahead of the associative test because a name that is not
	// there is not an associative array either, and ahead of the empty-
	// subscript answer because that is the row this decides: an empty
	// subscript on a name nothing declared is the plain unset operand
	// `$(( nosuchvar ))` is (#1745).
	if r.sem().ArithSubscriptSkippedWhenNameUnset == Yes && !r.arithNameIsSet(x.Name) {
		return intNum(0), nil
	}
	if x.Empty {
		if handled, v, err := r.emptyArithSubscript(x.Name); handled {
			return v, err
		}
		// Not handled: the dialect reads the brackets as the empty
		// *expression*, so the ordinary read below answers it — a subscript
		// of zero on an indexed name and the empty key on an associative one,
		// which is exactly what a nil Index and an empty Sub already mean to
		// the two paths that follow.
	}
	// A `*` or `@` is the whole array rather than a subscript at all where
	// the dialect reads the slice here, and it is asked *before* the
	// association below: the key `*` is what the other answer makes of it,
	// and the two would collapse into one if the table were consulted first.
	if v, whole := r.arithWholeArraySlice(x); whole {
		return r.arithElemValue(v)
	}
	// An associative name's subscript is a key and not an expression, which
	// is the same reading `${m[k]}` takes and for the same reason: with
	// `m[k]=7`, `m[0]=99` and `k=0`, all three shells with the attribute
	// answer `$(( m[k] ))` with 7. Evaluating it instead read the wrong
	// element and said nothing, which is the silent half of a wrong answer.
	if a, ok := r.assocFor(x.Name); ok {
		return r.arithElemValue(a[x.Sub])
	}
	idx, err := r.arithSubscriptIndex(x)
	if err != nil {
		return intNum(0), err
	}
	// No check that the name is an array: an unset one yields nothing, and
	// nothing is out of range for every subscript, so the bounds test inside
	// elemAt already answers it. A guard here would be a line no test could
	// tell from its absence.
	//
	// A negative subscript counts back from the end here too — `$((a[-1]))`
	// is the last element in all three shells with arrays — which elemAt
	// answers the same way for `${a[-1]}`, so the two spellings cannot drift.
	elems, _ := r.arrayElems(x.Name)
	v, ok := r.elemAt(x.Name, elems, idx.asInt())
	if !ok {
		return intNum(0), nil
	}
	return r.arithElemValue(v)
}

// arithWholeArraySlice is `$(( a[*] ))` and `$(( a[@] ))` where the dialect
// reads the brackets as the slice `${a[*]}` takes rather than as a subscript.
//
// The joined text is then read the way every other element's value is read —
// as an expression, not as a numeral — which is measured and is what makes
// the two spellings of the same array agree: `a=(1+1); $(( a[*] * 3 ))` is 6
// in the shell that answers yes, exactly as `$(( a[1] * 3 ))` is.
//
// The join is the first character of IFS, `@` and `*` alike: measured
// 2026-09-11 on zsh 5.9.2, `a=(3 4); IFS=:; $(( a[*] ))` and `$(( a[@] ))`
// both complain about the `:` they were handed, and `IFS=` makes the same
// array 34. So this is not the unquoted `@` question, where the two spellings
// part — there is no field splitting inside an expression for them to part
// over.
//
// The re-read is the element route's, so it inherits that route's own gap:
// a value holding an expression rather than a numeral is refused here where
// every column reads it, which is #1977 and reaches `$(( v * 3 ))` on a plain
// name as squarely as it reaches this.
//
// A slice of more than one element is therefore usually a *failure* rather
// than a number, and that is the answer rather than a defect in it:
// `a=(3 4 5); $(( a[*] ))` is `operator expected at ` + "`4 5'" + ` there.
// An empty array joins to nothing and is zero, and a scalar is its own value.
func (r *Runner) arithWholeArraySlice(x *syntax.ArithIndex) (string, bool) {
	if x.Index != nil || x.Empty || !wholeArraySubscript(strings.TrimSpace(x.Sub)) {
		return "", false
	}
	if !r.ask(r.sem().ArithWholeArraySubscriptIsTheSlice,
		"`$(( a[*] ))`, a whole-array subscript inside an expression") {
		return "", false
	}
	return strings.Join(r.wholeArrayElems(x.Name), ifsFirst(r.ifs())), true
}

// wholeArrayElems is every element a name holds, whichever of the three
// shapes holds them: an association's values, an array's elements, or a
// scalar as the one value it is.
func (r *Runner) wholeArrayElems(name string) []string {
	if a, ok := r.assocFor(name); ok {
		return a.values()
	}
	if elems, ok := r.arrayElems(name); ok {
		return elems
	}
	if v, ok := r.getVar(name); ok {
		return []string{v}
	}
	return nil
}

// arithSubscriptIndex is the number a subscript counts from, on a name that is
// not an association.
//
// Where the parser built a tree that tree is used. Where it did not, the
// brackets held a text it could not read as an expression — which is not a
// parse failure, because the identical text on an associative name is a key
// and the parser cannot see which kind of name it followed. So the reading is
// finished here, at the one point where that is known: read it as an
// expression, and refuse it as one if it will not.
//
// The refusal is worded from the subscript's own text, which makes it the same
// complaint `$(( .accept-line ))` earns. That is what the parser wrote from
// the same text before this moved, and it is what the panel writes: a shell
// says about `a[b c]` exactly what it says about `b c`.
//
// The text is read as it stands, without a second round of expansion. It is
// already the result of one — an arithmetic expansion substitutes into the
// whole expression before reading any of it — so a `$` still in it is a
// literal `$` and not the start of anything.
func (r *Runner) arithSubscriptIndex(x *syntax.ArithIndex) (arithNum, error) {
	if x.Index != nil || x.Empty {
		return r.evalNum(x.Index)
	}
	p := syntax.NewParser("", r.dialect())
	tree := p.ParseArithFor(x.Sub, syntax.Pos{})
	err := p.Err()
	if err == nil && tree == nil {
		// Brackets holding only space, which is the shape `a[$w]` takes once
		// a `$w` holding spaces has gone in. There is no expression in them
		// and nothing was wrong with what was there either, so the parser has
		// no complaint to hand over, and the panel divides over what that
		// means — see Semantics.BlankArithSubscriptIsTheEmptyExpression. Not
		// the empty pair `a[]`, which is a different answer again and has an
		// axis of its own.
		switch r.sem().BlankArithSubscriptIsTheEmptyExpression {
		case Yes:
			// The brackets hold the blank expression, which is zero, so the
			// element this names is element zero — which is what a nil tree
			// already means to evalNum.
			return r.evalNum(nil)
		case Unspecified:
			// Refused by name rather than guessed at: one answer is a value
			// and the other is no value at all, and neither can stand in for
			// the other.
			return intNum(0), arithError{
				msg:      r.unanswered("a subscript holding only whitespace"),
				complete: true,
			}
		}
		// No: the expression simply ran out, which is the failure the panel
		// names — measured on zsh 5.9.2, `$(( a[ ] ))` against a declared
		// array is `operand expected at end of string`, the same sentence
		// `$(( 1+ ))` earns. Worded through the same path every other
		// subscript failure takes rather than a second copy of it.
		err = &syntax.Error{Kind: syntax.ErrArithOperandEnd, Expr: x.Sub, Token: x.Sub}
	}
	if err != nil {
		return intNum(0), arithError{msg: r.subscriptFailure(x.Sub, err), complete: true}
	}
	return r.evalNum(tree)
}

// arithElemValue reads an element as a number, whichever kind of array it
// came out of, by the rule a plain name reads by: empty is zero — what every
// shell in the panel gives a subscript past the end and a name that was never
// an array — and a value that is no literal is re-read as a name where the
// dialect does that.
func (r *Runner) arithElemValue(v string) (arithNum, error) {
	return r.arithNumOfStored(v)
}

// arithNameIsSet reports whether the name a subscript follows exists at all,
// which is the question ArithSubscriptSkippedWhenNameUnset asks and not a
// question about the name's value: a scalar holding the empty string is there,
// and so is an array declared with nothing in it.
//
// Three stores rather than one, because a name reaches this from any of them
// and the plain reader cannot see the other two: an associative array is
// declared before it holds a key, and an indexed array declared empty has no
// bare-name value to hand back.
func (r *Runner) arithNameIsSet(name string) bool {
	if r.assocDeclared(name) {
		return true
	}
	if _, ok := r.Arrays[name]; ok && !r.removed[name] {
		return true
	}
	_, ok := r.getVar(name)
	return ok
}

// emptyArithSubscript is `a[]` where an expression reads or writes it — the
// shape `a[$w]` takes once an empty `$w` has been substituted, since an
// arithmetic expansion puts its parameters in before it parses.
//
// handled is false for the one answer that is not a fault at all: the brackets
// are the empty *expression*, and the caller carries on with the ordinary
// element it names. The rest are answered here, and an unanswered preset is
// refused by name rather than given one of them — the value, the stream and
// whether the expression survives all differ. See EmptyArithSubscriptPolicy.
func (r *Runner) emptyArithSubscript(name string) (handled bool, v arithNum, err error) {
	switch r.sem().EmptyArithSubscript {
	case EmptyArithSubscriptIsTheEmptyExpression:
		return false, intNum(0), nil
	case EmptyArithSubscriptIsReported:
		// Reported and then answered: the expression keeps going and the
		// operand is zero, which is why this writes here rather than
		// returning an error for a caller to word.
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			Wording(r.diag().ArithEmptySubscript, "%[1]s[]: bad array subscript", name)))
		return true, intNum(0), nil
	case EmptyArithSubscriptIsInvalid:
		// complete, because the sentence is the subscript machinery's whole
		// complaint: measured, `zsh:1: invalid subscript` with no `bad math
		// expression` in front of it, where an expression that will not parse
		// in the same place carries that prefix.
		return true, intNum(0), arithError{
			msg:      Wording(r.diag().ArithEmptySubscript, "invalid subscript", name),
			complete: true,
		}
	}
	return true, intNum(0), arithError{
		msg:      r.unanswered("a subscript written with nothing in it"),
		complete: true,
	}
}

// arithPlace is what an expression reads from and writes back to: a name, and
// the subscript it carries when it names an element.
//
// One type rather than two paths, because `++` and `=` disagreed about what a
// target could be — the assignment reached an element and the increment
// refused anything but a bare name, so `(( m[k]++ ))` was an error in every
// dialect while `(( m[k] += 1 ))` was not. A name that can be assigned to can
// be incremented; the operator is not what decides it.
type arithPlace struct {
	name string
	// index is nil when the target is a plain name.
	index syntax.ArithExpr
	// sub is the subscript as written, which is the key on an associative
	// name and the text a refusal quotes on an indexed one.
	sub string
	// empty says the brackets held nothing — `(( a[]++ ))` — which index
	// alone cannot say, since a plain name has no index either. Carried so
	// the read the operator makes reaches the same answer `$(( a[] ))` does.
	empty bool
	// subscripted says brackets were written at all, which neither of the two
	// above can say on its own: `(( m[.k]++ ))` has no index and is not
	// empty, and so does a plain name. Without it a key that is not an
	// expression read and wrote the *bare name* — `(( m[.k] = 3 ))` would set
	// m rather than the element, which is a wrong answer with no diagnostic.
	subscripted bool
}

// arithAssignmentDeclaresAnInteger reports whether writing this name from
// inside arithmetic gives it the integer attribute.
//
// Only a name the assignment *creates*, which is the half of the rule the
// issue's table could not show and a probe on a fresh shell cannot see:
// measured 2026-09-12 on zsh 5.9.2, `(( x = 5 ))` leaves `typeset -i x=5`
// where `x=3; (( x = 5 ))` and even `typeset x; (( x = 5 ))` leave an
// ordinary scalar. So it is a declaration and not an attribute the operator
// applies.
//
// The axis is **read** rather than asked, which is the reason
// Semantics.FailedExpansionAbandonsTheLine gives one file over: where the
// answer is not yes, leaving an ordinary scalar is what three of the four
// shells do and what this path already did, so an unanswered axis has a
// correct answer to fall back on rather than a missing one to complain about.
// Asking would refuse `for (( i=0; i<3; i++ ))` in a run with no dialect,
// which is a construct the core has and a question the script never posed.
func (r *Runner) arithAssignmentDeclaresAnInteger(name string) bool {
	if r.sem().ArithmeticAssignmentDeclaresAnInteger != Yes {
		return false
	}
	_, set := r.getVar(name)
	return !set
}

// declareIntegerFromArithmetic gives a name the arithmetic just created the
// integer attribute, and the output base a radix literal in the expression
// wrote.
//
// The base comes from the *expression* rather than from the text stored,
// which is the only place it survives: the answer is a decimal number by the
// time it is written down. Measured, `(( y = 0x1f ))` is `typeset -i16 y=31`
// and `(( y = 1 + 0x1f ))` is `typeset -i16 y=32`, so it is any radix literal
// the expression holds and not only one standing alone — while `y=0x1f; ((
// z = y ))` is a plain `typeset -i z=31`, the prefix having arrived through a
// value rather than been written here.
func (r *Runner) declareIntegerFromArithmetic(name string, from syntax.ArithExpr) {
	if r.integer == nil {
		r.integer = map[string]bool{}
	}
	r.integer[name] = true
	// The expression's own output specifier first, which is a base the script
	// wrote as squarely as a literal is: `(( i = [#16] 255 ))` on an unset
	// name is `typeset -i16 i=255`. It is not a base a name that *already*
	// had the attribute learns — `typeset -i i; (( i = [#16] 255 ))` stays
	// plain — but that route never reaches here, this being a declaration.
	base := 0
	if f := r.arithOutput; f != nil && f.Based {
		base = f.Base
	}
	if base == 0 {
		base = radixBaseWritten(from)
	}
	if base == 0 || !r.validIntegerBase(base) {
		return
	}
	if base == 10 && r.integerBaseTenIsNone() {
		return
	}
	if r.integerBase == nil {
		r.integerBase = map[string]int{}
	}
	r.integerBase[name] = base
}

// radixBaseWritten is the base named by the first radix literal in an
// expression, or 0 where it holds none.
func radixBaseWritten(e syntax.ArithExpr) int {
	switch x := e.(type) {
	case nil:
		return 0
	case *syntax.ArithNum:
		return integerBaseOfLiteral(x.Text)
	case *syntax.ArithUnary:
		return radixBaseWritten(x.X)
	case *syntax.ArithCond:
		if b := radixBaseWritten(x.Then); b != 0 {
			return b
		}
		return radixBaseWritten(x.Else)
	case *syntax.ArithBinary:
		if b := radixBaseWritten(x.X); b != 0 {
			return b
		}
		return radixBaseWritten(x.Y)
	case *syntax.ArithAssign:
		return radixBaseWritten(x.Value)
	}
	return 0
}

// arithPlaceOf is the target an operator can write through, and false for an
// expression that names no storage — `(( 1++ ))`, `(( (a)++ ))`.
func arithPlaceOf(e syntax.ArithExpr) (arithPlace, bool) {
	switch x := e.(type) {
	case *syntax.ArithVar:
		return arithPlace{name: x.Name}, true
	case *syntax.ArithIndex:
		return arithPlace{
			name: x.Name, index: x.Index, sub: x.Sub, empty: x.Empty,
			subscripted: true,
		}, true
	}
	return arithPlace{}, false
}

// readPlace is the value a target currently holds.
func (r *Runner) readPlace(p arithPlace) (arithNum, error) {
	if !p.subscripted {
		return r.arithValueOf(p.name)
	}
	return r.arithElement(&syntax.ArithIndex{Name: p.name, Index: p.index, Sub: p.sub, Empty: p.empty})
}

// writePlace stores a value back through a target, written the way the
// dialect writes a number — so `i+=1.5` leaves 1.5 behind and not 1.
func (r *Runner) writePlace(p arithPlace, v arithNum, from syntax.ArithExpr) error {
	// The expression's output format reaches the value an assignment stores,
	// not only the answer an expansion produces: measured, `x=5; (( x = [#16]
	// 255 ))` leaves x holding the six characters `16#FF`.
	text := r.formatArith(v)
	if p.empty {
		// A write through brackets with nothing in them stores nothing unless
		// the dialect reads them as the empty expression, where `(( a[]++ ))`
		// steps the element the empty subscript names — measured, element
		// zero in ksh93. The guard is load-bearing rather than defensive: the
		// branch below writes through the *bare name*, so without it a
		// refused `(( a[]++ ))` would silently overwrite `a` the moment the
		// read stopped refusing.
		if handled, _, err := r.emptyArithSubscript(p.name); handled {
			if err == nil {
				err = arithError{
					msg: Wording(r.diag().ArithEmptySubscript,
						"%[1]s[]: bad array subscript", p.name),
					complete: true,
				}
			}
			return err
		}
	}
	if !p.subscripted || p.empty {
		// The empty pair joins the bare name here rather than below, which is
		// where it has always been written: a dialect that read `a[]` as the
		// empty expression and did not answer it above writes through the
		// name. Only the two of them — a target with a subscript in it is an
		// element, and stops being one the moment this condition widens.
		if r.arithAssignmentDeclaresAnInteger(p.name) {
			// A name the arithmetic itself created carries the base on the
			// *name* rather than in the characters it holds, which is the
			// other half of the note above: `x=5; (( x = [#16] 255 ))` leaves
			// the six characters `16#FF` in an ordinary scalar, and the same
			// expression on an unset name is `typeset -i16 x=255` reading
			// back as `16#FF`. So the plain number is stored and the
			// declaration renders it.
			r.setVar(p.name, r.formatNum(v))
			r.declareIntegerFromArithmetic(p.name, from)
			r.rerenderInTheNewBase(p.name)
			return nil
		}
		r.setVar(p.name, text)
		return nil
	}
	if r.assocDeclared(p.name) {
		r.setAssocElem(p.name, p.sub, text)
		return nil
	}
	// Through the same reader the element is read by, so a text that is no
	// expression is refused here as it is there — refused rather than
	// written, which is what bash 5.3 and zsh 5.9.2 both do: `(( a[.k] = 9 ))`
	// on an indexed array complains and leaves every element as it was. One
	// call rather than a guard and an evaluation, so the number a subscript
	// counts from cannot be worked out one way for the read and another for
	// the write.
	idx, err := r.arithSubscriptIndex(&syntax.ArithIndex{
		Name: p.name, Index: p.index, Sub: p.sub, Empty: p.empty,
	})
	if err != nil {
		return err
	}
	sub := p.sub
	if sub == "" {
		sub = r.formatNum(idx)
	}
	r.setArrayElem(p.name, idx.asInt(), sub, text)
	return nil
}

// charCode answers the character-code operator, which is a *character* and
// never a number: `$((#b))` on `zebra` is 122 and not 5, and the length is
// spelled `$(( $#b ))`.
//
// Nothing here reports. Measured on zsh 5.9.2, every operand that names
// nothing is zero: a parameter never set, one holding the empty string, a `#`
// with nothing after it at all, and a name written with a subscript — which
// the shell reads and then finds nothing under, so `$((#a[1]))` is 0 however
// `${a[1]}` reads. The last of those is a fact about that shell rather than
// something derived, and the quiet answer is the one to keep: a refusal here
// would be louder than the shell a script was written for.
func (r *Runner) charCode(x *syntax.ArithCharCode) int {
	if x.Char != "" {
		text := x.Char
		if x.Op != "#" {
			// The escapes are the ones `$'…'` decodes, against this dialect's
			// table rather than a second copy of it. That covers `##c` and
			// the `'c'` constant alike — measured, `$(( '\101' ))` is 65 in
			// the shell that has the constant, so its escapes are the same
			// ones. The single-`#` spelling is the exception and has none: a
			// backslash there takes the next character as itself.
			text = r.expandDollarSingle(text)
		} else {
			text = strings.TrimPrefix(text, `\`)
		}
		return firstCharCode(text)
	}
	if x.Name == "" || x.Subscripted {
		return 0
	}
	v, ok := r.getVar(x.Name)
	if !ok {
		v, _ = r.specialParam(&syntax.ParamExpr{Name: x.Name})
	}
	return firstCharCode(v)
}

// firstCharCode is the code of the first character of a string, and 0 for a
// string with no first character.
//
// A character and not a byte where the text holds one: `é` is 233 rather than
// the 195 its first byte is. But a byte that is no character at all is its own
// value — measured, a lone 0x80 is 128 — rather than the replacement rune,
// which is a number no shell produces and which the decoder would otherwise
// hand back for every high byte a `\x` escape can write.
func firstCharCode(s string) int {
	if s == "" {
		return 0
	}
	c, size := utf8.DecodeRuneInString(s)
	if c == utf8.RuneError && size <= 1 {
		return int(s[0])
	}
	return int(c)
}

func (r *Runner) evalUnary(x *syntax.ArithUnary) (arithNum, error) {
	// ++ and -- read and write a variable, so they need its name rather than
	// its value.
	if x.Op == "++" || x.Op == "--" {
		place, ok := arithPlaceOf(x.X)
		if !ok {
			return intNum(0), arithError{msg: x.Op + " needs a variable"}
		}
		old, err := r.readPlace(place)
		if err != nil {
			return intNum(0), err
		}
		step := 1.0
		if x.Op == "--" {
			step = -1
		}
		next := r.addNum(old, step)
		if err := r.writePlace(place, next, nil); err != nil {
			return intNum(0), err
		}
		if x.Postfix {
			// The difference between the two spellings is what they evaluate
			// to, not what they do.
			return old, nil
		}
		return next, nil
	}

	v, err := r.evalNum(x.X)
	if err != nil {
		return intNum(0), err
	}
	switch x.Op {
	case "-":
		if v.float {
			return floatNum(-v.f), nil
		}
		return intNum(-v.i), nil
	case "+":
		return v, nil
	case "~":
		i, err := r.integerOperand(v, "~")
		if err != nil {
			return intNum(0), err
		}
		return intNum(^i), nil
	case "!":
		return intNum(boolInt(v.isZero())), nil
	}
	return intNum(0), arithError{msg: "unknown unary " + x.Op}
}

// addNum steps a value by one, keeping it whichever kind it was.
func (r *Runner) addNum(n arithNum, step float64) arithNum {
	if n.float {
		return floatNum(n.f + step)
	}
	return intNum(n.i + int(step))
}

func (r *Runner) evalAssign(x *syntax.ArithAssign) (arithNum, error) {
	place := arithPlace{
		name: x.Name, index: x.Index, sub: x.Sub,
		// An assignment target with an empty subscript is refused while
		// parsing, so brackets here are exactly a subscript that held
		// something — an expression the parser read, or a text it could not.
		subscripted: x.Index != nil || x.Sub != "",
	}
	v, err := r.evalNum(x.Value)
	if err != nil {
		return intNum(0), err
	}
	if x.Op != "=" {
		old, err := r.readPlace(place)
		if err != nil {
			return intNum(0), err
		}
		v, err = r.apply(strings.TrimSuffix(x.Op, "="), old, v)
		if err != nil {
			return intNum(0), err
		}
	}
	// The side effect that outlives the expression.
	if err := r.writePlace(place, v, x.Value); err != nil {
		return intNum(0), err
	}
	return v, nil
}

func (r *Runner) evalBinary(x *syntax.ArithBinary) (arithNum, error) {
	// The short-circuiting operators must not evaluate their right side when
	// the answer is already known, because that side can assign.
	switch x.Op {
	case "&&":
		l, err := r.evalNum(x.X)
		if err != nil || l.isZero() {
			return intNum(0), err
		}
		v, err := r.evalNum(x.Y)
		return intNum(boolInt(!v.isZero())), err
	case "||":
		l, err := r.evalNum(x.X)
		if err != nil {
			return intNum(0), err
		}
		if !l.isZero() {
			return intNum(1), nil
		}
		v, err := r.evalNum(x.Y)
		return intNum(boolInt(!v.isZero())), err
	}

	l, err := r.evalNum(x.X)
	if err != nil {
		return intNum(0), err
	}
	rv, err := r.evalNum(x.Y)
	if err != nil {
		return intNum(0), err
	}
	if x.Op == "," {
		// The sequence operator evaluates both and yields the right.
		return rv, nil
	}
	v, err := r.apply(x.Op, l, rv)
	if ae, ok := err.(arithError); ok && ae.token == "" {
		// apply sees values, not the tree, so the operand that caused the
		// failure is named here where the tree is still in hand. `5/y` with
		// y unset is blamed on `y` rather than on the zero it became.
		ae.token = arithToken(x.Y)
		// And the blame runs from there to the end of the expression, which
		// is the shape an evaluation failure takes in the one column that
		// quotes a token at all. Where that text begins splits by operator,
		// measured 2026-09-12 on bash 5.3.15:
		//
		//	$(( 1/((0)) ))   division by 0 (error token is "((0)) ")
		//	$(( 2**-1 ))     exponent less than 0 (error token is "1 ")
		//
		// A division names the *divisor as written*, so the parser's offset
		// is what answers it; the exponent names the last operand read, which
		// is the leaf the tree already hands back and which the sign is not
		// part of. -1 asks for the second.
		ae.blameTail, ae.from = true, -1
		if x.Op == "/" || x.Op == "%" {
			ae.from = x.YStart
		}
		err = ae
	}
	return v, err
}

// apply is a method because a division by zero is worded by the dialect,
// and the receiver is named `sh` because `r` is already the right operand.
//
// An operation is floating point when either operand is, and integer
// otherwise — so `3/2` is 1 even in a shell that has floats, and `3.0/2` is
// 1.5. A comparison is the exception in the other direction: it answers 0 or 1
// whatever it compared.
func (sh *Runner) apply(op string, l, r arithNum) (arithNum, error) {
	if v, ok, err := sh.compare(op, l, r); ok {
		return v, err
	}
	if l.float || r.float {
		return sh.applyFloat(op, l, r)
	}
	switch op {
	case "+":
		return sh.saturating(l.i+r.i, overflowedAdd(l.i, r.i)), nil
	case "-":
		return sh.saturating(l.i-r.i, overflowedAdd(l.i, -r.i) && r.i != minInt),
			nil
	case "*":
		return sh.saturating(l.i*r.i, overflowedMul(l.i, r.i)), nil
	case "/", "%":
		if r.i == 0 {
			return intNum(0), arithError{msg: Wording(sh.diag().DivisionByZero, "division by zero")}
		}
		if op == "/" {
			return intNum(l.i / r.i), nil
		}
		return intNum(l.i % r.i), nil
	case "<<":
		return intNum(l.i << uint(r.i)), nil
	case ">>":
		return intNum(l.i >> uint(r.i)), nil
	case "&":
		return intNum(l.i & r.i), nil
	case "^":
		return intNum(l.i ^ r.i), nil
	case "|":
		return intNum(l.i | r.i), nil
	case "**":
		return sh.intPow(l.i, r.i)
	}
	return intNum(0), arithError{msg: "unknown operator " + op}
}

const (
	maxInt = int(^uint(0) >> 1)
	minInt = -maxInt - 1
)

// saturating answers an integer operation, clamping at the edge where the
// dialect does: ksh93 holds 9223372036854775807 + 1 at the maximum where the
// other shells wrap around, and the axis is asked only when an overflow
// actually happened.
func (sh *Runner) saturating(wrapped int, overflowed bool) arithNum {
	if !overflowed ||
		!sh.ask(sh.sem().ArithOverflowSaturates, "integer overflow clamping at the edge") {
		return intNum(wrapped)
	}
	if wrapped < 0 {
		return intNum(maxInt)
	}
	return intNum(minInt)
}

func overflowedAdd(a, b int) bool {
	sum := a + b
	return (a > 0 && b > 0 && sum < 0) || (a < 0 && b < 0 && sum >= 0)
}

func overflowedMul(a, b int) bool {
	if a == 0 || b == 0 {
		return false
	}
	p := a * b
	return p/b != a
}

// intPow is `**` on integers.
//
// A negative exponent cannot yield an integer, and the shells that parse the
// operator split on what to do about it: one refuses, two answer with a
// float — `2**-1` is 0.5 there — so the axis is asked, and only when the
// exponent really is negative, because `2**3` means the same thing in all of
// them. Overflow wraps, which is what both integer-arithmetic shells do and
// what squaring modulo the word size preserves; the spec records overflow as
// unportable by construction, so nothing finer is promised.
func (sh *Runner) intPow(base, exp int) (arithNum, error) {
	if exp < 0 {
		if sh.ask(sh.sem().ArithNegativeExponentIsError, "a negative exponent") {
			return intNum(0), arithError{msg: Wording(sh.diag().ArithNegativeExponent, "exponent less than 0")}
		}
		return floatNum(math.Pow(float64(base), float64(exp))), nil
	}
	v := 1
	for b := base; exp > 0; exp >>= 1 {
		if exp&1 == 1 {
			v *= b
		}
		b *= b
	}
	return intNum(v), nil
}

// compare answers the operators that yield a truth rather than a number. They
// are separated because their answer is an integer whatever they compared,
// which is measured: `1.5 < 2` is 1 and not 1. in the shell that prints a
// point after a whole float.
func (sh *Runner) compare(op string, l, r arithNum) (arithNum, bool, error) {
	if !l.float && !r.float {
		switch op {
		case "<":
			return intNum(boolInt(l.i < r.i)), true, nil
		case "<=":
			return intNum(boolInt(l.i <= r.i)), true, nil
		case ">":
			return intNum(boolInt(l.i > r.i)), true, nil
		case ">=":
			return intNum(boolInt(l.i >= r.i)), true, nil
		case "==":
			return intNum(boolInt(l.i == r.i)), true, nil
		case "!=":
			return intNum(boolInt(l.i != r.i)), true, nil
		}
		return intNum(0), false, nil
	}
	a, b := l.asFloat(), r.asFloat()
	switch op {
	case "<":
		return intNum(boolInt(a < b)), true, nil
	case "<=":
		return intNum(boolInt(a <= b)), true, nil
	case ">":
		return intNum(boolInt(a > b)), true, nil
	case ">=":
		return intNum(boolInt(a >= b)), true, nil
	case "==":
		return intNum(boolInt(a == b)), true, nil
	case "!=":
		return intNum(boolInt(a != b)), true, nil
	}
	return intNum(0), false, nil
}

// applyFloat is apply where at least one operand is a float.
//
// Division by zero is not an error here: it is an infinity, which is what both
// shells with floats produce. The integer path keeps the error, because there
// is no integer to give back.
func (sh *Runner) applyFloat(op string, l, r arithNum) (arithNum, error) {
	a, b := l.asFloat(), r.asFloat()
	switch op {
	case "+":
		return floatNum(a + b), nil
	case "-":
		return floatNum(a - b), nil
	case "*":
		return floatNum(a * b), nil
	case "/":
		return floatNum(a / b), nil
	case "**":
		// Exponentiation is a float operation in both shells that have
		// floats — `9**0.5` is 3 — so no integer-only refusal arises, and a
		// negative exponent is unremarkable here: the answer was already
		// going to be a float.
		return floatNum(math.Pow(a, b)), nil
	case "%":
		// A remainder is a float operation in one of the two shells with
		// floats — `7 % 2.5` is 2 there — and refused outright in the other,
		// which is the same axis the bitwise operators answer.
		//
		// Both operands are offered, because either may be the float: `7%2.5`
		// has a whole number on the left, and asking only about that one let
		// the refusal through.
		if _, err := sh.integerOperand(l, op); err != nil {
			return intNum(0), err
		}
		if _, err := sh.integerOperand(r, op); err != nil {
			return intNum(0), err
		}
		return floatNum(math.Mod(a, b)), nil
	}
	// Everything else is defined on integers only, and what a float does to
	// it is the axis: one shell refuses and the other truncates.
	li, err := sh.integerOperand(l, op)
	if err != nil {
		return intNum(0), err
	}
	ri, err := sh.integerOperand(r, op)
	if err != nil {
		return intNum(0), err
	}
	return sh.apply(op, intNum(li), intNum(ri))
}

// integerOperand is a value where only an integer will do.
//
// `1.5 & 1` and `7 % 2.5` are the cases. ksh93 refuses them and zsh truncates,
// so the axis is asked — and only when the value really is a float, because a
// shell whose numbers are all integers never reaches the question.
func (sh *Runner) integerOperand(n arithNum, op string) (int, error) {
	if !n.float {
		return n.i, nil
	}
	if sh.ask(sh.sem().ArithIntegerOperatorRefusesFloat, "an integer-only operator refusing a float") {
		return 0, arithError{msg: Wording(sh.diag().ArithInvalidFloatOperation, "invalid floating point operation"), token: op}
	}
	return int(n.f), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// arithValueOf reads a variable as a number.
//
// An unset one is zero rather than an error. A set one whose value is not a
// number is *re-evaluated as an expression* where the dialect says so:
// `x=abc; $((x+1))` finds abc, which is unset, so 0, so 1. The shells split —
// docs/spec/grammar/arithmetic.md records the divergence — so this is not a
// choice made here but the ArithNameValueRecurses ask below.
//
// The depth bound is not decoration: `x=x` would otherwise recur forever.
func (r *Runner) arithValueOf(name string) (arithNum, error) {
	if r.arithValueDepth > 32 {
		// Which name the bound is reported against is itself a divergence:
		// bash and ksh93 name the one it stopped on and zsh the one the
		// expression was written with, which are the same name only when
		// the value points at itself. See
		// Diagnostics.ArithRecursionBlamesTheWrittenName.
		blamed := name
		if r.diag().ArithRecursionBlamesTheWrittenName && r.arithValueTopName != "" {
			blamed = r.arithValueTopName
		}
		return intNum(0), arithError{
			msg:   Wording(r.diag().ArithRecursionLimit, "expression nested too deeply: %[1]s", blamed),
			token: blamed,
		}
	}
	if r.arithValueDepth == 0 {
		// The name the expression itself holds, kept for the sentence above.
		// Read here rather than passed down because every frame below this
		// one is a *value* being read again, and none of them knows how it
		// was reached.
		r.arithValueTopName = name
	}
	value, ok := r.getVar(name)
	if !ok {
		if r.arithValueDepth > 0 && r.ask(r.sem().ArithRecursedNameMustBeSet, "an unset name reached through a value") {
			// One dialect reads a name arrived at through another name's
			// value as a *parameter reference* rather than as text that
			// might be a number, so an unset one is the same refusal its
			// `set -u` writes — with nounset off, and fatal. See
			// Semantics.ArithRecursedNameMustBeSet.
			//
			// Only below the top: a name written in the expression itself
			// is zero when it is unset in every shell in the panel.
			text := Wording(r.diag().UnboundVariable, "%s: parameter not set", name)
			return intNum(0), arithError{msg: text, token: name, complete: true}
		}
		return intNum(0), nil
	}
	return r.arithNumOfStored(value)
}

// arithNumOfStored reads a value a variable was holding as a number.
//
// Split out of arithValueOf so that an *element* is read the same way a plain
// name is. It was not: an element went straight to the literal parser, so
// `a=(y); y=5; echo $(( a[0] ))` refused where all three shells with arrays
// answer 5, and an element holding a word that is no name at all refused with
// a different sentence from the identical scalar. One operand rule, asked
// once — the storage it came out of is not what decides how it reads.
func (r *Runner) arithNumOfStored(value string) (arithNum, error) {
	if strings.TrimSpace(value) == "" {
		return intNum(0), nil
	}
	value = r.decimalLeadingNumeral(value)
	if n, err := r.parseArithNum(strings.TrimSpace(value)); err == nil {
		return n, nil
	}
	return r.arithValueAsExpression(value)
}

// arithValueAsExpression reads a stored value that is no numeral by parsing it
// again as an expression, where the dialect does that.
//
// `v=1+1; $(( v * 3 ))` is 6 in bash, ksh93 and zsh, and dash alone refuses
// it — the same split ArithNameValueRecurses already records, which is why
// this asks that axis rather than a second one beside it. A name-shaped value
// is the case the axis was written for and is not a case at all here: `x=y`
// parses as the expression `y`, so the ordinary walk looks `y` up and the
// recursion, the unset-name question and the depth bound are all the ones a
// name written in the expression itself gets. Reading a name and reading an
// expression were two helpers with one rule between them, and the one that
// only knew names refused `1+1`.
//
// The value is parsed as it stands, without expansion: a `$` in it is an
// ordinary character, which is why `q=5; v='$q'` is a syntax error and not 5
// in all three shells that re-read at all.
func (r *Runner) arithValueAsExpression(value string) (arithNum, error) {
	text := strings.TrimSpace(value)
	p := syntax.NewParser("", r.dialect())
	tree := p.ParseArithFor(value, syntax.Pos{})
	err := p.Err()
	if err == nil && tree == nil {
		// No tree and no complaint means the text holds an expansion, which
		// the parser leaves for its caller to substitute first. A stored
		// value has already been through that once and does not go through
		// it again, so what is left is a `$` standing in an expression as an
		// ordinary character — an operand failure, which is what the shells
		// that re-read at all report for it.
		err = &syntax.Error{Kind: syntax.ErrArithOperand, Expr: text, Token: text}
	}
	if err != nil {
		if r.sem().ArithNameValueRecurses == Yes {
			// The value became an expression and that expression would not
			// parse, which is the failure a written one earns, worded the
			// same way and blaming the value rather than the name it came
			// out of: `v="3 4"; $(( v ))` names `3 4`.
			return intNum(0), arithError{msg: r.subscriptFailure(text, err), complete: true}
		}
		// Nowhere for it to be an expression, so it is simply not a number.
		// No ask: every dialect refuses this text and only the sentence
		// differs, so a question here would be one asked where the panel
		// agrees.
		return intNum(0), arithError{msg: r.wordInvalidNumber(text), token: text, complete: true}
	}
	if !r.ask(r.sem().ArithNameValueRecurses, "re-reading a stored value as an expression") {
		return intNum(0), arithError{msg: r.wordInvalidNumber(text), token: text, complete: true}
	}
	r.arithValueDepth++
	defer func() { r.arithValueDepth-- }()
	n, err := r.evalNum(tree)
	if err != nil {
		// The complaint names the *value*, because that is the expression
		// that failed: `v=1/0; $(( v ))` is `1/0: division by 0` and not
		// `v: division by 0`, measured on bash 5.3.15 and ksh93u+ alike.
		// Already-complete failures are handed back as they stand, which is
		// what arithFailure does with them.
		return intNum(0), arithError{msg: r.arithFailure(text, err), complete: true}
	}
	return n, nil
}

// decimalLeadingNumeral answers the leading numeral of a stored value in
// decimal where the dialect reads one that way — see
// Semantics.ArithStoredValueReadsALeadingZeroAsDecimal — by handing back the
// value with that numeral rewritten. Everything else is returned unchanged.
//
// A rewrite rather than a number, because the value may be a whole expression
// and only its first numeral is read this way: measured 2026-09-11 on ksh93u+,
// `k=010+1` is 11 where the identical literal `$((010+1))` is 9.
//
// Asked only where the two readings can differ: the value has to *begin* with
// a zero in front of another digit, with nothing before it.
//
//	k=010     10   the digits, in decimal
//	k=0010    10   however many zeros
//	k=09       9   an invalid octal digit is just a digit
//	k=010+1   11   the leading numeral only; `k=1+010` is 9
//	k=010#5    5   the numeral is the base, and in decimal
//	k=-010    -8   a sign is not part of it, so nothing is rewritten
//	k=" 010"   8   nor is anything in front of it
//	k=0x10    16   a prefix both readings agree about
func (r *Runner) decimalLeadingNumeral(value string) string {
	if r.sem().ArithLeadingZeroIsOctal != Yes {
		// Nothing made the zero octal, so the two readings already agree and
		// there is no choice to put to the dialect.
		return value
	}
	n := leadingZeroPaddedRun(value)
	if n == 0 {
		return value
	}
	if !r.ask(r.sem().ArithStoredValueReadsALeadingZeroAsDecimal,
		"a zero-padded number read out of a variable") {
		return value
	}
	digits := strings.TrimLeft(value[:n], "0")
	if digits == "" {
		digits = "0"
	}
	return digits + value[n:]
}

// leadingZeroPaddedRun is how long the value's leading run of decimal digits
// is, when that run starts with a zero standing in front of another digit —
// the one shape the two readings answer differently. Zero means no such run.
func leadingZeroPaddedRun(value string) int {
	if len(value) < 2 || value[0] != '0' {
		return 0
	}
	n := 0
	for n < len(value) && value[n] >= '0' && value[n] <= '9' {
		n++
	}
	if n < 2 {
		return 0
	}
	return n
}

// parseArithNum reads a literal, which may be a float where the dialect has
// them.
//
// The axis is asked only when the text is float-shaped. An expression of whole
// numbers means the same thing in every shell in the panel, so `3/2` needs no
// dialect and `3.0/2` does.
func (r *Runner) parseArithNum(s string) (arithNum, error) {
	s = strings.TrimSpace(s)
	if !floatShaped(s) {
		n, err := r.parseNum(s)
		return intNum(n), err
	}
	if !r.dialect().ArithFloat {
		// The dialect has no floats, so this is not a number at all. It is
		// the *grammar* that answers — the parser would not have produced a
		// float literal here either — which is why this reads the dialect
		// rather than an axis: one question, asked in the two places that
		// need it, rather than two fields that could disagree.
		msg := r.wordInvalidNumber(s)
		if w := r.diag().DigitTooGreatForBase; w != "" {
			// The dialect that calls every unreadable literal the same
			// thing says it here too: `1e3` fails with the octal digit's
			// own sentence.
			msg = w
		}
		return intNum(0), arithError{msg: msg, token: s}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return intNum(0), arithError{msg: r.wordInvalidNumber(s), token: s}
	}
	return floatNum(f), nil
}

// floatShaped reports whether a literal can only be a float.
//
// A point or an exponent, and not a hex literal — `0x1e5` is an integer whose
// digits happen to include an `e`, and `16#1f` is one whose base separator is
// not a point.
func floatShaped(s string) bool {
	if s == "" || strings.ContainsAny(s, "#") || strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return false
	}
	if strings.Contains(s, ".") {
		return true
	}
	i := strings.IndexAny(s, "eE")
	// An exponent needs digits before it, or it is a name: `e5` is a
	// variable and `1e5` is a number.
	return i > 0 && strings.IndexFunc(s[:i], func(c rune) bool { return c < '0' || c > '9' }) < 0
}

// formatNum writes a value the way the dialect writes one.
//
// Integers are the same everywhere. Floats are not: the two shells that have
// them disagree on how many digits to show and on whether a whole one keeps
// its point, so both are the dialect's to answer.
func (r *Runner) formatNum(n arithNum) string {
	if !n.float {
		return itoa(n.i)
	}
	// An infinity and a NaN are named rather than formatted, and each shell
	// names them its own way.
	switch {
	case math.IsInf(n.f, 1):
		return Wording(r.diag().ArithInfinity, "+Inf")
	case math.IsInf(n.f, -1):
		return "-" + Wording(r.diag().ArithInfinity, "Inf")
	case math.IsNaN(n.f):
		return Wording(r.diag().ArithNotANumber, "NaN")
	}
	digits := r.diag().ArithFloatDigits
	if digits == 0 {
		digits = 17
	}
	out := strconv.FormatFloat(n.f, 'g', digits, 64)
	if r.diag().ArithFloatKeepsPoint && !strings.ContainsAny(out, ".eEnif") {
		// A whole float still reads as one: 4 becomes `4.`. Skipped when the
		// text already carries a point, an exponent, or is an infinity or a
		// NaN, none of which could be mistaken for an integer.
		out += "."
	}
	return out
}

func isNameLike(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || isLetter(c) || (i > 0 && isDigit(c)) {
			continue
		}
		return false
	}
	return true
}

// parseNum reads a literal, which the parser deliberately kept as written.
//
// Whether a leading zero means octal is a dialect question — zsh reads
// `0100` as one hundred where everything else reads sixty-four — so the
// answer is given here, where the dialect is, rather than baked into the tree.
func (r *Runner) parseNum(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, arithError{msg: "empty number"}
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}

	var n int64
	var err error
	// Why the literal is not a number, which is worded separately from the
	// fact that it isn't: bash calls a bad octal digit "value too great for
	// base" and reserves its generic wording for operands that are not
	// literals at all.
	// badDigit says the literal parsed as far as its base and then met a
	// digit that base does not allow. bash words that separately from an
	// operand that is not a literal at all, and wraps it where it reports
	// the other bare.
	// digits is what the conversion actually read, and base what it read them
	// in. Both are kept so that a failure can be blamed the way bash blames
	// one: it parts a digit the base cannot reach from a byte that is no
	// digit at all, and only the pair says which happened.
	digits, base := s, 10
	switch {
	case r.spellsANamedBase(s):
		text, rest, _ := strings.Cut(s, "#")
		b, _ := strconv.Atoi(text)
		if b < 2 || b > 64 {
			// Below two is no base at all and above 64 is past the alphabet.
			// Worded the same way as a base the dialect stops short of,
			// because it is the same refusal: bash writes `invalid
			// arithmetic base` for `1#0` and ksh93 its one sentence.
			return 0, arithError{msg: Wording(r.diag().ArithInvalidBase,
				"invalid base: %[1]s", strconv.Itoa(b)), token: s}
		}
		if b > 36 && !r.ask(r.sem().ArithBaseAbove36, "a base above 36") {
			if r.unspecified {
				return 0, arithError{msg: r.unanswered("a base above 36")}
			}
			// One dialect stops at 36 and says so, naming the base — as a
			// number rather than as written, which is what it prints for a
			// padded one: `064#10` is `invalid base (must be 2 to 36
			// inclusive): 64` there.
			return 0, arithError{msg: Wording(r.diag().ArithInvalidBase,
				"invalid base: %[1]s", strconv.Itoa(b)), token: s}
		}
		digits, base = rest, b
		var bad bool
		if n, bad = parseBaseDigits(digits, base); bad {
			err = strconv.ErrSyntax
		}
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		digits, base = s[2:], 16
		n, err = r.parseRadixDigits(digits, base)
	case r.dialect().ArithBinaryLiteral &&
		(strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B")):
		digits, base = s[2:], 2
		n, err = r.parseRadixDigits(digits, base)
	case len(s) > 1 && s[0] == '0' && !strings.ContainsAny(s, "xX") && r.octalLeadingZero():
		digits, base = s[1:], 8
		n, err = strconv.ParseInt(digits, 8, 64)
		if err != nil && !r.ask(r.sem().ArithInvalidOctalDigitIsError, "an invalid octal digit being an error") {
			// ksh93 is octal *and* tolerant: `08` is 8 there, not a
			// failure. Asked only once the octal read has actually failed,
			// so a dialect that never sees a bad digit is never questioned.
			digits, base = s, 10
			n, err = strconv.ParseInt(digits, 10, 64)
		}
	default:
		n, err = strconv.ParseInt(digits, base, 64)
	}
	if err != nil {
		if w := r.diag().DigitTooGreatForBase; w != "" {
			// Three dialects word every unreadable literal through the same
			// wrapper, and one of them parts two diagnoses inside it: a
			// digit the base cannot reach — `08`, `2#12`, and `1@2` whose
			// `@` is digit 62 — from a byte that is no digit anywhere, which
			// is the `#` left behind when a leading zero made the text an
			// octal constant. See Diagnostics.ArithByteIsNoDigit.
			if _, isDigit, found := firstByteTheBaseCannotUse(digits, base); found && !isDigit {
				if n := r.diag().ArithByteIsNoDigit; n != "" {
					w = n
				}
			}
			return 0, arithError{msg: w, token: s}
		}
		return 0, arithError{msg: r.wordInvalidNumber(s), token: s, complete: true}
	}
	if neg {
		n = -n
	}
	return int(n), nil
}

// parseBaseDigits reads digits in a base up to 64. The second result reports
// a digit the base cannot use, which each dialect words its own way.
func parseBaseDigits(digits string, base int) (int64, bool) {
	if digits == "" {
		return 0, true
	}
	var n int64
	for i := 0; i < len(digits); i++ {
		v, known := baseDigitValue(digits[i], base)
		if !known || v >= base {
			return 0, true
		}
		n = n*int64(base) + int64(v)
	}
	return n, false
}

// baseDigitValue is the base-64 alphabet, in one place: 0-9, then letters —
// one case as good as the other through 36, and apart above it, where a-z is
// 10..35, A-Z 36..61, `@` 62 and `_` 63.
//
// The second result says the byte is a digit *somewhere* in that alphabet,
// even where this base cannot reach it. That is the distinction a complaint
// turns on and the reason the two questions share one function: a digit the
// base does not have and a byte that is no digit at all are two diagnoses,
// and an alphabet written down twice is the way they stop agreeing.
func baseDigitValue(c byte, base int) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'Z':
		if base <= 36 {
			return int(c-'A') + 10, true
		}
		return int(c-'A') + 36, true
	case c == '@':
		return 62, true
	case c == '_':
		return 63, true
	}
	return 0, false
}

// firstByteTheBaseCannotUse finds what stopped a conversion: the byte, whether
// the alphabet knows it as a digit at all, and whether there was one.
func firstByteTheBaseCannotUse(digits string, base int) (byte, bool, bool) {
	for i := 0; i < len(digits); i++ {
		v, known := baseDigitValue(digits[i], base)
		if !known || v >= base {
			return digits[i], known, true
		}
	}
	return 0, false, false
}

// spellsANamedBase reports whether the text is the `base#digits` form *as this
// dialect spells it*, rather than a numeral that happens to hold a `#`.
//
// Where it is not, the text falls through to the ordinary numeral reading and
// the `#` is simply a byte no base can use — which is not a fallback but
// bash's whole rule: a leading zero opens an octal constant there, so
// `010#5` is the octal `010` with `#5` behind it and fails as a number.
func (r *Runner) spellsANamedBase(s string) bool {
	text, _, ok := strings.Cut(s, "#")
	if !ok || text == "" || !allDecimalDigits(text) {
		// Only decimal digits name a base in any shell in the panel:
		// `0x10#5` is a hex literal with a `#` after it, not base sixteen of
		// something.
		return false
	}
	if text[0] == '0' && !r.ask(r.sem().ArithBaseMayHaveALeadingZero,
		"a base written with a leading zero") {
		return false
	}
	if len(text) > 2 && r.ask(r.sem().ArithBaseIsAtMostTwoDigits,
		"a base longer than two characters") {
		return false
	}
	return true
}

func allDecimalDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseRadixDigits reads the digits after a radix prefix, where an empty run
// is a question rather than a failure: `$(( 0x ))` is zero in bash and zsh and
// refused in ksh93 and dash — see Semantics.ArithEmptyRadixDigitsAreZero.
func (r *Runner) parseRadixDigits(digits string, base int) (int64, error) {
	if digits == "" {
		if r.ask(r.sem().ArithEmptyRadixDigitsAreZero, "a radix prefix with no digits after it") {
			return 0, nil
		}
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(digits, base, 64)
}

// octalLeadingZero is the dialect answer, and the quietest divergence
// measured: `0100` is sixty-four everywhere but zsh, where it is one hundred.
func (r *Runner) octalLeadingZero() bool {
	return r.ask(r.sem().ArithLeadingZeroIsOctal, "a leading zero meaning octal")
}

// arithCmd runs `(( expr ))` as a command.
//
// It exits 0 when the expression is non-zero, which is the reverse of the
// usual convention and is unanimous across the panel.
func (r *Runner) arithCmd(ctx context.Context, c *syntax.ArithCmdClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		r.unspecified = false
		tree, text, perr := r.arithTreeOver(c.Parsed, c.Expr)
		// Traced from the expanded text and after the expansion, which is
		// where the shells put it: `(( $(echo 1) ))` traces the substitution
		// first and then `((  1  ))`. Before the parse, so an expression that
		// will not parse is still reported as having been reached — a trace
		// that skips what failed is the gap #2126 is about.
		r.traceArithCommand(text, r.diag().TraceArithCommand)
		if perr != nil {
			r.diagf("%s\n", r.diag().arithConstructFailure("((", r.diag().ParseFailure(perr)))
			r.status = r.arithCmdFailed(r.diag().StatusForParseError(perr))
			return nil
		}
		v, err := r.evalArith(tree)
		if r.unspecified {
			r.status = 2
			return nil
		}
		if err != nil {
			// The expression is named here as it is everywhere else an
			// expression fails. It was not, and a loop doing arithmetic
			// reported `division by 0` with nothing to say which iteration
			// or which expression had done it — where the expansion route
			// for the identical failure quoted it back (#1985).
			r.diagf("%s\n", r.diag().arithConstructFailure("((", r.arithFailure(text, err)))
			r.status = r.arithCmdFailed(1)
			return nil
		}
		r.status = boolInt(v == 0)
		return nil
	})
}

// arithCmdFailed is what `(( ))` leaves behind once it has said what went
// wrong, given the status the failure would otherwise carry.
//
// One question for both ways an expression can fail, because the shell that
// answers it differently answers the same for both: `(( 1+ ))`, `(( 1 2 ))`
// and `(( 8#9 ))` never reach the evaluator and `(( 1/0 ))` and a call to a
// math function nothing defines do, and all five are 2 in zsh 5.9.2 and 1 in
// bash 5.3. A status split across the two branches would have been two axes
// with one answer each and no measurement separating them.
//
// The dialect's answer stands in front of the general one on the parse branch
// rather than beside it: `(( ))` is a construct whose failure has a status of
// its own, and taking the syntax status there gave zsh 1 where it leaves 2
// (#1625).
func (r *Runner) arithCmdFailed(otherwise int) int {
	if r.ask(r.sem().ArithCommandErrorIsFatal, "a `(( ))` that could not be evaluated abandoning the input") {
		// The same shape ConditionArithmeticErrorIsFatal has, and through the
		// same door: the status is decided below either way, and what the
		// dialect adds is that there is no next line to read it.
		r.abandonOverArithmetic()
	}
	if r.ask(r.sem().ArithCommandErrorStatusIsTwo, "the status a failed `(( ))` leaves") {
		return 2
	}
	return otherwise
}

// abandonOverArithmetic gives up the input over an expression the dialect
// calls fatal, for the two constructs that ask that question.
//
// An *error* rather than a request to stop, which is the half that decides how
// far the give-up reaches. A boundary reading a file of its own catches an
// error and carries on past it, and both constructs stop at that boundary:
// measured 2026-09-11, ksh93's `. ./s.sh; echo after` prints `after` over a
// file holding `(( 1+ ))` and over one holding `[[ 1+ -eq 0 ]]`, and so does
// zsh for the second. Marked only as a stop, the give-up cost the whole
// script in both — which is the same error one level down as the one
// interp/source.go is named for.
//
// Not fatalQuiet, which is the other door: that one decides the status as
// well, and both of these constructs have a status of their own that the
// caller has already worked out.
func (r *Runner) abandonOverArithmetic() {
	r.ctl, r.abandon = controlExit, abandonError
}

// wordInvalidNumber words "this is not a number" the way the dialect does.
func (r *Runner) wordInvalidNumber(text string) string {
	return Wording(r.diag().InvalidNumber, "invalid number: %s", text)
}

// arithTree is the expression to evaluate, given what the parser managed and
// the text it came from.
//
// A tree the parser built is used as it stands. A nil one means the text had
// an expansion in it, so it is not an expression until that has happened —
// substituted first, read second, which is the order every shell in the panel
// uses and the only order that makes `$(( $x$y ))` with x=`1+` and y=`2`
// come to 3.
func (r *Runner) arithTree(tree syntax.ArithExpr, text string) (syntax.ArithExpr, error) {
	out, _, err := r.arithTreeOver(tree, text)
	return out, err
}

// arithTreeOver is arithTree with the text the parser was actually handed, for
// the one caller that has to look at where in it the failure was.
//
// The expansion happens once, here, and the text is handed back rather than
// recomputed: expanding it a second time to find an offset would run a command
// substitution on the right-hand side twice, which is the mistake #1915 was.
func (r *Runner) arithTreeOver(tree syntax.ArithExpr, text string) (syntax.ArithExpr, string, error) {
	if tree != nil {
		return tree, text, nil
	}
	expanded := r.expandArithText(text)
	p := syntax.NewParser("", r.dialect())
	out := p.ParseArithFor(expanded, syntax.Pos{})
	if err := p.Err(); err != nil {
		return nil, expanded, err
	}
	return out, expanded, nil
}

// expandArithText substitutes into an arithmetic expression before it is read.
//
// The inside of `$(( ))` is expanded the way a double-quoted string is —
// parameters, command substitutions and nested arithmetic — and only then is
// the result an expression. `$(( $x$y ))` with x=`1+` and y=`2` is 3 in every
// shell in the panel, which is a fact about *when* the substitution happens
// and cannot be reproduced by a tree built from the text as written.
//
// It is the same scan a here-document body gets, and for the same reason: in
// both, a quote is an ordinary character and only the expansions matter.
func (r *Runner) expandArithText(text string) string {
	if !strings.ContainsAny(text, "$`") {
		return text
	}
	return r.expandRawText(text)
}
