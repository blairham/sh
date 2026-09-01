// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

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

func (r *Runner) evalNum(e syntax.ArithExpr) (arithNum, error) {
	switch x := e.(type) {
	case nil:
		return intNum(0), nil

	case *syntax.ArithNum:
		return r.parseArithNum(x.Text)

	case *syntax.ArithVar:
		return r.arithValueOf(x.Name, 0)

	case *syntax.ArithIndex:
		return r.arithElement(x)

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
	idx, err := r.evalNum(x.Index)
	if err != nil {
		return intNum(0), err
	}
	// No check that the name is an array: an unset one yields nothing, and
	// nothing is out of range for every subscript, so the bounds test below
	// already answers it. A guard here would be a line no test could tell
	// from its absence.
	elems, _ := r.arrayElems(x.Name)
	i := idx.asInt() - r.arrayBase()
	if i < 0 || i >= len(elems) {
		return intNum(0), nil
	}
	if strings.TrimSpace(elems[i]) == "" {
		return intNum(0), nil
	}
	return r.parseArithNum(strings.TrimSpace(elems[i]))
}

func (r *Runner) evalUnary(x *syntax.ArithUnary) (arithNum, error) {
	// ++ and -- read and write a variable, so they need its name rather than
	// its value.
	if x.Op == "++" || x.Op == "--" {
		v, ok := x.X.(*syntax.ArithVar)
		if !ok {
			return intNum(0), arithError{msg: x.Op + " needs a variable"}
		}
		old, err := r.arithValueOf(v.Name, 0)
		if err != nil {
			return intNum(0), err
		}
		step := 1.0
		if x.Op == "--" {
			step = -1
		}
		next := r.addNum(old, step)
		r.setVar(v.Name, r.formatNum(next))
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
	v, err := r.evalNum(x.Value)
	if err != nil {
		return intNum(0), err
	}
	if x.Op != "=" {
		old, err := r.arithAssignTarget(x)
		if err != nil {
			return intNum(0), err
		}
		v, err = r.apply(strings.TrimSuffix(x.Op, "="), old, v)
		if err != nil {
			return intNum(0), err
		}
	}
	// The side effect that outlives the expression, written the way the
	// dialect writes a number — so `i+=1.5` leaves 1.5 behind and not 1.
	if x.Index != nil {
		idx, ierr := r.evalNum(x.Index)
		if ierr != nil {
			return intNum(0), ierr
		}
		r.setArrayElem(x.Name, idx.asInt(), r.formatNum(v))
		return v, nil
	}
	r.setVar(x.Name, r.formatNum(v))
	return v, nil
}

// arithAssignTarget is the current value of what an assignment writes to,
// which is an element when the target carries a subscript.
func (r *Runner) arithAssignTarget(x *syntax.ArithAssign) (arithNum, error) {
	if x.Index == nil {
		return r.arithValueOf(x.Name, 0)
	}
	return r.arithElement(&syntax.ArithIndex{Name: x.Name, Index: x.Index})
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
		return intNum(l.i + r.i), nil
	case "-":
		return intNum(l.i - r.i), nil
	case "*":
		return intNum(l.i * r.i), nil
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
	}
	return intNum(0), arithError{msg: "unknown operator " + op}
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
// number is *re-evaluated as an expression*, which is what bash and zsh do:
// `x=abc; $((x+1))` finds abc, which is unset, so 0, so 1. dash and ksh93
// error instead — docs/spec/grammar/arithmetic.md records that as a
// three-way divergence, and this takes the two that agree.
//
// The depth bound is not decoration: `x=x` would otherwise recur forever.
func (r *Runner) arithValueOf(name string, depth int) (arithNum, error) {
	if depth > 32 {
		return intNum(0), arithError{msg: "expression nested too deeply: " + name}
	}
	value, ok := r.getVar(name)
	if !ok || strings.TrimSpace(value) == "" {
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
		return intNum(0), arithError{msg: r.wordInvalidNumber(s), token: s}
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
		n, err = strconv.ParseInt(digits, b, 64)
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
		return 0, arithError{msg: r.wordInvalidNumber(s), token: s, complete: true}
	}
	if neg {
		n = -n
	}
	return int(n), nil
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
			r.status = r.diag().StatusForParseError(perr)
			return nil
		}
		v, err := r.evalArith(tree)
		if r.unspecified {
			r.status = 2
			return nil
		}
		if err != nil {
			r.diagf("%v\n", err)
			r.status = 1
			return nil
		}
		r.status = boolInt(v == 0)
		return nil
	})
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
	// No guard for empty text: the parser reads it as no expression at all,
	// with no error, and the evaluator answers zero for a nil tree — which is
	// what `$(( ))` is. A check here would be a line no test could tell from
	// its absence.
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
