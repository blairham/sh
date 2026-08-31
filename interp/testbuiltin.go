// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

// `test` and `[`, which until now were not builtins at all.
//
// They appeared to work because macOS ships /bin/test and /bin/[, so every
// script that used one was running a separate program the shell had no say
// over: no dialect wording, no gate on the comparison, and nothing at all on a
// machine that does not ship them.
//
// This is *not* `[[ ]]` with different brackets, and the difference is the
// trap the construct is famous for. `[[ ]]` is grammar: the parser knows where
// the operands are, so nothing inside is split or globbed and an unquoted `==`
// right operand is a *pattern*. `test` is a command whose operands are already
// words by the time it sees them, so `=` is a plain string comparison and an
// unquoted empty variable does not become an empty argument — it disappears,
// and the argument count is what decides the meaning:
//
//	u=; test -n $u    → one argument, "-n", which is a non-empty string: true
//	u=; test -n "$u"  → two arguments, a real -n test: false
//
// Both are unanimous across the panel, and the second is what people mean.

func init() {
	builtins["test"] = biTest
	builtins["["] = biBracket
}

func biTest(r *Runner, _ context.Context, args []string) int {
	return r.runTest("test", args)
}

// biBracket is `test` that insists on its closing bracket.
//
// The `]` is an ordinary argument — `[` is a command, not syntax — so nothing
// but this checks for it. `test x ]` is an error in every shell for the same
// reason: `]` is then just a second operand, and two operands with no operator
// between them is not an expression.
func biBracket(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 || args[len(args)-1] != "]" {
		r.diagf("%s\n", Wording(r.diag().TestMissingBracket, "[: missing ]"))
		return 2
	}
	return r.runTest("[", args[:len(args)-1])
}

func (r *Runner) runTest(name string, args []string) int {
	ok, err := r.testExpr(args)
	if err != nil {
		var te *testError
		if errors.As(err, &te) {
			r.diagf("%s\n", Wording(te.format(r.diag()), te.fallback(), te.operand))
		} else {
			r.diagf("%s: %v\n", name, err)
		}
		// 2 for "this is not an expression", which every shell in the panel
		// uses and which is deliberately distinct from 1, "the expression is
		// false". A script that branches on `$?` can tell them apart.
		return 2
	}
	if ok {
		return 0
	}
	return 1
}

// testError is a malformed expression rather than a false one.
type testError struct {
	kind    testErrorKind
	operand string
}

type testErrorKind int

const (
	// errUnaryExpected is a word where a unary operator belonged. Split from
	// the binary case because one dialect words them differently — "unary
	// operator expected" against "binary operator expected" — where the other
	// three use one message for both.
	errUnaryExpected testErrorKind = iota
	// errBinaryExpected is a word where a binary operator belonged.
	errBinaryExpected
	// errOperandExpected is an operator with nothing after it.
	errOperandExpected
	// errTooManyArguments is a well-formed expression with words left over.
	errTooManyArguments
	// errIntegerExpected is a non-numeric operand to a numeric comparison.
	errIntegerExpected
)

func (e *testError) Error() string { return e.fallback() }

func (e *testError) fallback() string {
	switch e.kind {
	case errOperandExpected:
		return "test: argument expected"
	case errTooManyArguments:
		return "test: too many arguments"
	case errIntegerExpected:
		return "test: %[1]s: integer expected"
	case errBinaryExpected:
		return "test: %[1]s: binary operator expected"
	}
	return "test: %[1]s: unary operator expected"
}

func (e *testError) format(d Diagnostics) string {
	switch e.kind {
	case errOperandExpected:
		return d.TestOperandExpected
	case errTooManyArguments:
		return d.TestTooManyArguments
	case errIntegerExpected:
		return d.TestIntegerExpected
	case errBinaryExpected:
		return d.TestBinaryExpected
	}
	return d.TestUnaryExpected
}

// testExpr evaluates an expression the way POSIX specifies it: by argument
// count first, and only then by grammar.
//
// That order is not a shortcut, it is the specification, and it is why `test`
// behaves in ways a grammar alone would not predict. With one argument the
// word is a string and nothing else — `test -f` is *true*, because `-f` is a
// non-empty string rather than an operator with a missing operand. With three,
// `test ( x )` is the parenthesised form and `test a = a` is a comparison, and
// which one it is depends on the shape of those three words rather than on any
// precedence.
//
// Only past four arguments does a grammar take over, with `-a` binding tighter
// than `-o`. All of this is unanimous across the panel.
func (r *Runner) testExpr(args []string) (bool, error) {
	switch len(args) {
	case 0:
		// No expression is false rather than an error.
		return false, nil
	case 1:
		return args[0] != "", nil
	case 2:
		if args[0] == "!" {
			return args[1] == "", nil
		}
		return r.unaryTest(args[0], args[1])
	case 3:
		if ok, err, handled := r.binaryTest(args[0], args[1], args[2]); handled {
			return ok, err
		}
		if args[0] == "!" {
			v, err := r.testExpr(args[1:])
			return !v, err
		}
		if args[0] == "(" && args[2] == ")" {
			return args[1] != "", nil
		}
		return false, &testError{kind: errBinaryExpected, operand: args[1]}
	case 4:
		if args[0] == "!" {
			v, err := r.testExpr(args[1:])
			return !v, err
		}
		if args[0] == "(" && args[3] == ")" {
			return r.testExpr(args[1:3])
		}
	}
	p := &testParser{r: r, args: args}
	v, err := p.orExpr()
	if err != nil {
		return false, err
	}
	if p.pos != len(p.args) {
		return false, &testError{kind: errTooManyArguments}
	}
	return v, nil
}

// testParser is the grammar that takes over past four arguments: `-a` binds
// tighter than `-o`, `!` binds tighter still, and `( )` groups.
type testParser struct {
	r    *Runner
	args []string
	pos  int
}

func (p *testParser) peek() string {
	if p.pos < len(p.args) {
		return p.args[p.pos]
	}
	return ""
}

func (p *testParser) more() bool { return p.pos < len(p.args) }

func (p *testParser) orExpr() (bool, error) {
	left, err := p.andExpr()
	if err != nil {
		return false, err
	}
	for p.more() && p.peek() == "-o" {
		p.pos++
		right, err := p.andExpr()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *testParser) andExpr() (bool, error) {
	left, err := p.notExpr()
	if err != nil {
		return false, err
	}
	for p.more() && p.peek() == "-a" {
		p.pos++
		right, err := p.notExpr()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *testParser) notExpr() (bool, error) {
	if p.more() && p.peek() == "!" {
		p.pos++
		v, err := p.notExpr()
		return !v, err
	}
	return p.primary()
}

func (p *testParser) primary() (bool, error) {
	if !p.more() {
		return false, &testError{kind: errOperandExpected}
	}
	if p.peek() == "(" {
		p.pos++
		v, err := p.orExpr()
		if err != nil {
			return false, err
		}
		if !p.more() || p.peek() != ")" {
			return false, &testError{kind: errOperandExpected}
		}
		p.pos++
		return v, nil
	}

	// A binary operator is decided by the *second* word, so it is looked for
	// before a unary one: `test -n = -n` compares two strings rather than
	// testing whether "=" is non-empty.
	if p.pos+2 < len(p.args) {
		if ok, err, handled := p.r.binaryTest(p.args[p.pos], p.args[p.pos+1], p.args[p.pos+2]); handled {
			p.pos += 3
			return ok, err
		}
	}
	if isTestUnary(p.args[p.pos]) {
		if p.pos+1 >= len(p.args) {
			return false, &testError{kind: errOperandExpected}
		}
		v, err := p.r.unaryTest(p.args[p.pos], p.args[p.pos+1])
		p.pos += 2
		return v, err
	}
	v := p.args[p.pos] != ""
	p.pos++
	return v, nil
}

// isTestUnary reports whether a word is one of the unary operators, which is
// what decides between "an operator and its operand" and "a bare string".
func isTestUnary(s string) bool {
	switch s {
	case "-n", "-z", "-e", "-f", "-d", "-s", "-r", "-w", "-x", "-L", "-h",
		"-b", "-c", "-p", "-S", "-g", "-u", "-k", "-t":
		return true
	}
	return false
}

// unaryTest is `-op operand`.
func (r *Runner) unaryTest(op, operand string) (bool, error) {
	switch op {
	case "-n":
		return operand != "", nil
	case "-z":
		return operand == "", nil
	case "-t":
		// A terminal test on a descriptor this shell may not even own. Never
		// true here: the streams are io.Writers, which is the honest answer
		// for a library rather than a guess about the process's descriptors.
		return false, nil
	}
	if !isTestUnary(op) {
		return false, &testError{kind: errUnaryExpected, operand: op}
	}
	return r.fileTest(op, operand), nil
}

// fileTest answers the operators that ask the filesystem something.
//
// Shared in spirit with `[[ ]]`, which asks the same questions of an operand
// it obtained differently. Kept separate because the two disagree about
// everything *else* — `=` above all — and a shared entry point would invite
// sharing the parts that must not be.
func (r *Runner) fileTest(op, operand string) bool {
	path := r.atDir(operand)
	info, err := os.Stat(path)
	switch op {
	case "-e":
		return err == nil
	case "-f":
		return err == nil && info.Mode().IsRegular()
	case "-d":
		return err == nil && info.IsDir()
	case "-s":
		return err == nil && info.Size() > 0
	case "-r":
		return err == nil && info.Mode().Perm()&0o400 != 0
	case "-w":
		return err == nil && info.Mode().Perm()&0o200 != 0
	case "-x":
		return err == nil && info.Mode().Perm()&0o100 != 0
	case "-b":
		return err == nil && info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0
	case "-c":
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	case "-p":
		return err == nil && info.Mode()&os.ModeNamedPipe != 0
	case "-S":
		return err == nil && info.Mode()&os.ModeSocket != 0
	case "-g":
		return err == nil && info.Mode()&os.ModeSetgid != 0
	case "-u":
		return err == nil && info.Mode()&os.ModeSetuid != 0
	case "-k":
		return err == nil && info.Mode()&os.ModeSticky != 0
	case "-L", "-h":
		li, lerr := os.Lstat(path)
		return lerr == nil && li.Mode()&os.ModeSymlink != 0
	}
	return false
}

// binaryTest is `a OP b`. The third return says whether the middle word was an
// operator at all, which is what lets the caller fall back to another reading
// rather than guessing.
func (r *Runner) binaryTest(left, op, right string) (bool, error, bool) {
	switch op {
	case "=":
		// A *string* comparison, and never a pattern. `[[ abc == a* ]]` is
		// true and `test abc = a*` is false, which is the sharpest difference
		// between the two constructs and is unanimous across the panel.
		return left == right, nil, true
	case "!=":
		return left != right, nil, true
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		l, lerr := strconv.Atoi(strings.TrimSpace(left))
		rv, rerr := strconv.Atoi(strings.TrimSpace(right))
		if lerr != nil {
			return false, &testError{kind: errIntegerExpected, operand: left}, true
		}
		if rerr != nil {
			return false, &testError{kind: errIntegerExpected, operand: right}, true
		}
		switch op {
		case "-eq":
			return l == rv, nil, true
		case "-ne":
			return l != rv, nil, true
		case "-lt":
			return l < rv, nil, true
		case "-le":
			return l <= rv, nil, true
		case "-gt":
			return l > rv, nil, true
		}
		return l >= rv, nil, true
	}
	return false, nil, false
}
