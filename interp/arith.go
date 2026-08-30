// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// arithError is a failure inside an expression, such as dividing by zero.
// It is a runtime error rather than a syntax one: the expression parsed.
type arithError struct{ msg string }

func (e arithError) Error() string { return e.msg }

// evalArith evaluates an expression tree.
//
// Evaluation order is part of the specification rather than an implementation
// detail, because assignment is an operator whose effect outlives the
// expression: `x=0; $((0 && (x=9)))` must leave x alone. So the logical
// operators short-circuit here, and nothing evaluates both sides eagerly.
func (r *Runner) evalArith(e syntax.ArithExpr) (int, error) {
	switch x := e.(type) {
	case nil:
		return 0, nil

	case *syntax.ArithNum:
		return r.parseNum(x.Text)

	case *syntax.ArithVar:
		return r.arithValueOf(x.Name, 0)

	case *syntax.ArithUnary:
		return r.evalUnary(x)

	case *syntax.ArithCond:
		c, err := r.evalArith(x.Cond)
		if err != nil {
			return 0, err
		}
		if c != 0 {
			return r.evalArith(x.Then)
		}
		return r.evalArith(x.Else)

	case *syntax.ArithAssign:
		return r.evalAssign(x)

	case *syntax.ArithBinary:
		return r.evalBinary(x)
	}
	return 0, arithError{fmt.Sprintf("unsupported expression %T", e)}
}

func (r *Runner) evalUnary(x *syntax.ArithUnary) (int, error) {
	// ++ and -- read and write a variable, so they need its name rather than
	// its value.
	if x.Op == "++" || x.Op == "--" {
		v, ok := x.X.(*syntax.ArithVar)
		if !ok {
			return 0, arithError{x.Op + " needs a variable"}
		}
		old, err := r.arithValueOf(v.Name, 0)
		if err != nil {
			return 0, err
		}
		next := old + 1
		if x.Op == "--" {
			next = old - 1
		}
		r.setVar(v.Name, strconv.Itoa(next))
		if x.Postfix {
			// The difference between the two spellings is what they evaluate
			// to, not what they do.
			return old, nil
		}
		return next, nil
	}

	v, err := r.evalArith(x.X)
	if err != nil {
		return 0, err
	}
	switch x.Op {
	case "-":
		return -v, nil
	case "+":
		return v, nil
	case "~":
		return ^v, nil
	case "!":
		return boolInt(v == 0), nil
	}
	return 0, arithError{"unknown unary " + x.Op}
}

func (r *Runner) evalAssign(x *syntax.ArithAssign) (int, error) {
	v, err := r.evalArith(x.Value)
	if err != nil {
		return 0, err
	}
	if x.Op != "=" {
		old, err := r.arithValueOf(x.Name, 0)
		if err != nil {
			return 0, err
		}
		v, err = apply(strings.TrimSuffix(x.Op, "="), old, v)
		if err != nil {
			return 0, err
		}
	}
	// The side effect that outlives the expression.
	r.setVar(x.Name, strconv.Itoa(v))
	return v, nil
}

func (r *Runner) evalBinary(x *syntax.ArithBinary) (int, error) {
	// The short-circuiting operators must not evaluate their right side when
	// the answer is already known, because that side can assign.
	switch x.Op {
	case "&&":
		l, err := r.evalArith(x.X)
		if err != nil || l == 0 {
			return 0, err
		}
		v, err := r.evalArith(x.Y)
		return boolInt(v != 0), err
	case "||":
		l, err := r.evalArith(x.X)
		if err != nil {
			return 0, err
		}
		if l != 0 {
			return 1, nil
		}
		v, err := r.evalArith(x.Y)
		return boolInt(v != 0), err
	}

	l, err := r.evalArith(x.X)
	if err != nil {
		return 0, err
	}
	rv, err := r.evalArith(x.Y)
	if err != nil {
		return 0, err
	}
	if x.Op == "," {
		// The sequence operator evaluates both and yields the right.
		return rv, nil
	}
	return apply(x.Op, l, rv)
}

func apply(op string, l, r int) (int, error) {
	switch op {
	case "+":
		return l + r, nil
	case "-":
		return l - r, nil
	case "*":
		return l * r, nil
	case "/", "%":
		if r == 0 {
			return 0, arithError{"division by zero"}
		}
		if op == "/" {
			return l / r, nil
		}
		return l % r, nil
	case "<<":
		return l << uint(r), nil
	case ">>":
		return l >> uint(r), nil
	case "<":
		return boolInt(l < r), nil
	case "<=":
		return boolInt(l <= r), nil
	case ">":
		return boolInt(l > r), nil
	case ">=":
		return boolInt(l >= r), nil
	case "==":
		return boolInt(l == r), nil
	case "!=":
		return boolInt(l != r), nil
	case "&":
		return l & r, nil
	case "^":
		return l ^ r, nil
	case "|":
		return l | r, nil
	}
	return 0, arithError{"unknown operator " + op}
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
func (r *Runner) arithValueOf(name string, depth int) (int, error) {
	if depth > 32 {
		return 0, arithError{"expression nested too deeply: " + name}
	}
	value, ok := r.getVar(name)
	if !ok || strings.TrimSpace(value) == "" {
		return 0, nil
	}
	if n, err := r.parseNum(strings.TrimSpace(value)); err == nil {
		return n, nil
	}
	if isNameLike(value) {
		// bash and zsh re-evaluate a name-shaped value as an expression;
		// dash and ksh93 error. Following the two that agree.
		return r.arithValueOf(strings.TrimSpace(value), depth+1)
	}
	// Not a number and not a name: an error rather than a silent zero.
	return 0, arithError{"invalid number: " + strings.TrimSpace(value)}
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
		return 0, arithError{"empty number"}
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}

	var n int64
	var err error
	switch {
	case strings.Contains(s, "#"):
		base, digits, _ := strings.Cut(s, "#")
		b, berr := strconv.Atoi(base)
		if berr != nil || b < 2 || b > 64 {
			return 0, arithError{"invalid base: " + base}
		}
		n, err = strconv.ParseInt(digits, b, 64)
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		n, err = strconv.ParseInt(s[2:], 16, 64)
	case len(s) > 1 && s[0] == '0' && !strings.ContainsAny(s, "xX#") && r.octalLeadingZero():
		n, err = strconv.ParseInt(s[1:], 8, 64)
	default:
		n, err = strconv.ParseInt(s, 10, 64)
	}
	if err != nil {
		return 0, arithError{"invalid number: " + s}
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
		v, err := r.evalArith(c.Parsed)
		if r.unspecified {
			r.status = 2
			return nil
		}
		if err != nil {
			r.errf("sh: %v\n", err)
			r.status = 1
			return nil
		}
		r.status = boolInt(v == 0)
		return nil
	})
}
