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
	expr := strings.TrimSpace(text)
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
		return r.arithValueOf(x.Name, 0)

	case *syntax.ArithIndex:
		return r.arithElement(x)

	case *syntax.ArithCharCode:
		return intNum(r.charCode(x)), nil

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
		// Brackets holding only space. There is no expression in them and
		// nothing was wrong with what was there either, so the parser has no
		// complaint to hand over — the expression simply ran out, which is
		// the failure the panel names: measured on zsh 5.9.2, `$(( a[ ] ))`
		// against a declared array is `operand expected at end of string`,
		// the same sentence `$(( 1+ ))` earns. Not the empty pair `a[]`,
		// which is a different answer again and has an axis of its own.
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
	return r.arithNumOfStored(v, 0)
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
		return r.arithValueOf(p.name, 0)
	}
	return r.arithElement(&syntax.ArithIndex{Name: p.name, Index: p.index, Sub: p.sub, Empty: p.empty})
}

// writePlace stores a value back through a target, written the way the
// dialect writes a number — so `i+=1.5` leaves 1.5 behind and not 1.
func (r *Runner) writePlace(p arithPlace, v arithNum) error {
	text := r.formatNum(v)
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
		if x.Op == "##" {
			// The escapes are the ones `$'…'` decodes, against this dialect's
			// table rather than a second copy of it. The single-`#` spelling
			// has no escapes: a backslash there takes the next character as
			// itself.
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
		if err := r.writePlace(place, next); err != nil {
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
	if err := r.writePlace(place, v); err != nil {
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
func (r *Runner) arithValueOf(name string, depth int) (arithNum, error) {
	if depth > 32 {
		return intNum(0), arithError{msg: "expression nested too deeply: " + name}
	}
	value, ok := r.getVar(name)
	if !ok {
		return intNum(0), nil
	}
	return r.arithNumOfStored(value, depth)
}

// arithNumOfStored reads a value a variable was holding as a number.
//
// Split out of arithValueOf so that an *element* is read the same way a plain
// name is. It was not: an element went straight to the literal parser, so
// `a=(y); y=5; echo $(( a[0] ))` refused where all three shells with arrays
// answer 5, and an element holding a word that is no name at all refused with
// a different sentence from the identical scalar. One operand rule, asked
// once — the storage it came out of is not what decides how it reads.
func (r *Runner) arithNumOfStored(value string, depth int) (arithNum, error) {
	if strings.TrimSpace(value) == "" {
		return intNum(0), nil
	}
	if n, err := r.parseArithNum(strings.TrimSpace(value)); err == nil {
		return n, nil
	}
	if isNameLike(value) && r.ask(r.sem().ArithNameValueRecurses, "re-evaluating a name-shaped value") {
		return r.arithValueOf(strings.TrimSpace(value), depth+1)
	}
	// Not a number and not a name: an error rather than a silent zero.
	text := strings.TrimSpace(value)
	return intNum(0), arithError{msg: r.wordInvalidNumber(text), token: text, complete: true}
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
	badDigit := false
	switch {
	case strings.Contains(s, "#"):
		base, digits, _ := strings.Cut(s, "#")
		b, berr := strconv.Atoi(base)
		if berr != nil || b < 2 || b > 64 {
			return 0, arithError{msg: "invalid base: " + base}
		}
		if b > 36 && !r.ask(r.sem().ArithBaseAbove36, "a base above 36") {
			if r.unspecified {
				return 0, arithError{msg: r.unanswered("a base above 36")}
			}
			// One dialect stops at 36 and says so, naming the base.
			return 0, arithError{msg: Wording(r.diag().ArithInvalidBase,
				"invalid base: %[1]s", base)}
		}
		var bad bool
		n, bad = parseBaseDigits(digits, b)
		if bad {
			err = strconv.ErrSyntax
			badDigit = true
		}
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		n, err = strconv.ParseInt(s[2:], 16, 64)
	case len(s) > 1 && s[0] == '0' && !strings.ContainsAny(s, "xX#") && r.octalLeadingZero():
		n, err = strconv.ParseInt(s[1:], 8, 64)
		badDigit = err != nil
		if err != nil && !r.ask(r.sem().ArithInvalidOctalDigitIsError, "an invalid octal digit being an error") {
			// ksh93 is octal *and* tolerant: `08` is 8 there, not a
			// failure. Asked only once the octal read has actually failed,
			// so a dialect that never sees a bad digit is never questioned.
			n, err = strconv.ParseInt(s, 10, 64)
		}
	default:
		n, err = strconv.ParseInt(s, 10, 64)
	}
	if err != nil {
		if badDigit {
			return 0, arithError{
				msg:   Wording(r.diag().DigitTooGreatForBase, "invalid number"),
				token: s,
			}
		}
		msg := r.wordInvalidNumber(s)
		if w := r.diag().DigitTooGreatForBase; w != "" {
			// One dialect calls every unreadable literal the same thing —
			// `1e3` and `2#12` fail with the octal digit's own sentence.
			msg = w
		}
		return 0, arithError{msg: msg, token: s, complete: msg != r.diag().DigitTooGreatForBase}
	}
	if neg {
		n = -n
	}
	return int(n), nil
}

// parseBaseDigits reads digits in a base up to 64: 0-9, then letters — one
// case as good as the other through 36, and apart above it, where a-z is
// 10..35, A-Z 36..61, `@` 62 and `_` 63. The second result reports a digit
// the base does not have, which each dialect words its own way.
func parseBaseDigits(digits string, base int) (int64, bool) {
	if digits == "" {
		return 0, true
	}
	var n int64
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'z':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			if base <= 36 {
				v = int(c-'A') + 10
			} else {
				v = int(c-'A') + 36
			}
		case c == '@':
			v = 62
		case c == '_':
			v = 63
		default:
			return 0, true
		}
		if v >= base {
			return 0, true
		}
		n = n*int64(base) + int64(v)
	}
	return n, false
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
		tree, perr := r.arithTree(c.Parsed, c.Expr)
		if perr != nil {
			r.diagf("%s\n", r.diag().ParseFailure(perr))
			r.status = r.arithCmdFailed(r.diag().StatusForParseError(perr))
			return nil
		}
		v, err := r.evalArith(tree)
		if r.unspecified {
			r.status = 2
			return nil
		}
		if err != nil {
			r.diagf("%v\n", err)
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
	if r.ask(r.sem().ArithCommandErrorStatusIsTwo, "the status a failed `(( ))` leaves") {
		return 2
	}
	return otherwise
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
	if tree != nil {
		return tree, nil
	}
	p := syntax.NewParser("", r.dialect())
	out := p.ParseArithFor(r.expandArithText(text), syntax.Pos{})
	if err := p.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
