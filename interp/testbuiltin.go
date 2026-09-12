// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"io/fs"
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
			if te.kind == errArithmeticOperand {
				// An operand the arithmetic could not read, in the dialect
				// that reads one that way. It is loud and it is *not* the
				// not-an-expression 2: `[ 1x1 -eq 0 ]` is 1 and the script
				// runs on, where the same words inside `[[ ]]` abandon the
				// input. The complaint is already worded; all that is added
				// here is the name the builtin was called by.
				r.diagf("%s\n", Wording(te.format(r.diag()), te.fallback(), te.operand, name))
				return 1
			}
			if r.unspecified {
				return 2
			}
			if te.kind == errBinaryExpected {
				// An expression that never parsed is not the builtin's
				// complaint. The one dialect that names a builtin in the
				// location bears this out: `test a b c` is `zsh:1: condition
				// expected: b` with no `test` in it, and so is `[[ a b c ]]`,
				// which is not a builtin at all — while `test -Q x` and
				// `test 1 -eq a`, which failed *evaluating* an expression
				// that did parse, are `zsh:test:1:`. Same wordings, two
				// speakers.
				outer := r.inBuiltin
				r.inBuiltin = ""
				defer func() { r.inBuiltin = outer }()
			}
			// The name is the word that was typed. `[` is `test` under
			// another name, and every shell in the panel blames the name it
			// was called by rather than a fixed one — including zsh, which
			// carries it in the location instead of the message and so needs
			// nothing here.
			r.diagf("%s\n", Wording(te.format(r.diag()), te.fallback(), te.operand, name))
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
	// decided marks an error an axis has already chosen to raise. The `-t`
	// refusal is the case: it exists only where TerminalTestRequiresANumber
	// said yes, so nothing downstream may put the same sentence to a second
	// axis. Nothing reads it today — the gate it was written for went with
	// TestIntegerRefusalIsSilent, whose one dialect now reads the operand as
	// arithmetic instead — and it is kept because the fact it records is
	// about the error and not about the gate that happened to read it.
	decided bool
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
	// errArithmeticOperand is an operand of a numeric comparison that would
	// not read as an arithmetic expression, in a dialect that reads one that
	// way. The operand field carries the whole worded math complaint rather
	// than the text, because the arithmetic has already blamed the part of
	// it that failed and said why.
	errArithmeticOperand
)

func (e *testError) Error() string { return e.fallback() }

func (e *testError) fallback() string {
	switch e.kind {
	case errOperandExpected:
		return "%[2]s: argument expected"
	case errTooManyArguments:
		return "%[2]s: too many arguments"
	case errIntegerExpected:
		return "%[2]s: %[1]s: integer expected"
	case errArithmeticOperand:
		// The math complaint as the arithmetic worded it, behind the name
		// the builtin was called by: `[: 1x1: arithmetic syntax error`.
		return "%[2]s: %[1]s"
	case errBinaryExpected:
		return "%[2]s: %[1]s: binary operator expected"
	}
	return "%[2]s: %[1]s: unary operator expected"
}

func (e *testError) format(d Diagnostics) string {
	switch e.kind {
	case errOperandExpected:
		return d.TestOperandExpected
	case errTooManyArguments:
		return d.TestTooManyArguments
	case errIntegerExpected:
		return d.TestIntegerExpected
	case errArithmeticOperand:
		// No dialect wording: the sentence is the arithmetic's, which each
		// dialect has already worded through its own math diagnostics.
		return ""
	case errBinaryExpected:
		return d.TestBinaryExpected
	}
	return d.TestUnaryExpected
}

// bareTerminalTest is whether a lone `-t` is `-t 1` rather than a non-empty
// string. Two shells read it that way and four do not — see
// Semantics.BareTerminalTestIsDescriptorOne, where the measurement is.
//
// Asked only for the word `-t`, so no other operator's one-argument reading
// can be moved by the axis and no expression without a `-t` in it pays for it.
func (r *Runner) bareTerminalTest() bool {
	return r.ask(r.sem().BareTerminalTestIsDescriptorOne, "`[ -t ]` meaning `[ -t 1 ]`")
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
		if args[0] == "-t" && r.bareTerminalTest() {
			return r.unaryTest("-t", "1")
		}
		return args[0] != "", nil
	case 2:
		if args[0] == "!" {
			if args[1] == "-t" && r.bareTerminalTest() {
				// The same one-argument rule with a `!` in front of it, so
				// the exception has to be the same one or the negation
				// answers a different question than the word it negates.
				on, err := r.unaryTest("-t", "1")
				return !on, err
			}
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
		blamed := args[1]
		if r.diag().TestNamesFirstOperand {
			// dash names the first word instead of the one that should have
			// been an operator.
			blamed = args[0]
		}
		return false, &testError{kind: errBinaryExpected, operand: blamed}
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
	if err := p.unknownOperator(); err != nil {
		return false, err
	}
	if p.r.isTestUnary(p.args[p.pos]) {
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

// unknownOperator is a word spelled like an operator this dialect does not
// have, standing where a primary begins with another word after it.
//
// The two short forms never reach the grammar and so already name it: two
// words go straight to unaryTest and three to binaryTest. Here the word is
// not in the operator table, so without this it becomes a bare string, the
// expression parses, and the leftover words are reported as a count — which
// says nothing about the one token that was actually wrong (#1290).
//
// A word standing *alone* as a primary is a non-empty string and is true, in
// every column: `[ -Q -a -n x ]` succeeds everywhere. So the complaint needs
// a word after it that the grammar cannot take, which is any word that is
// neither a connective nor a closing paren.
//
// Which complaint it is belongs to the dialect, because the panel gives three
// answers — see Diagnostics.TestUnknownLongOperator.
func (p *testParser) unknownOperator() error {
	word := p.args[p.pos]
	if len(word) < 2 || word[0] != '-' || p.r.isTestUnary(word) {
		return nil
	}
	switch next := p.pos + 1; {
	case next >= len(p.args):
		return nil
	case p.args[next] == "-a", p.args[next] == "-o", p.args[next] == ")":
		return nil
	}
	switch p.r.diag().TestUnknownLongOperator {
	case TestUnknownOperatorNamed:
		return &testError{kind: errUnaryExpected, operand: word}
	case TestUnknownOperatorLeavesAnOperand:
		// Two operands with no operator between them, which is the
		// three-word complaint — including which of the two it blames.
		blamed := p.args[p.pos+1]
		if p.r.diag().TestNamesFirstOperand {
			blamed = word
		}
		return &testError{kind: errBinaryExpected, operand: blamed}
	}
	return nil
}

// isTestUnary reports whether a word is one of the unary operators, which is
// what decides between "an operator and its operand" and "a bare string".
//
// All but one are unanimous. `-v` is the exception and is asked of the
// dialect, so a shell without it keeps the refusal by name it already gave —
// `[: -v: unary operator expected` — rather than answering a question it does
// not have. That is the same head count that keeps `-v` out of the core
// grammar: dash has neither the operator nor `[[ ]]`, and bash 3.2 answers
// `[: -v: unary operator expected` too.
func (r *Runner) isTestUnary(s string) bool {
	if s == "-v" {
		return r.dialect().ParameterIsSetTest
	}
	return isTestUnary(s)
}

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
	case "-v":
		// The same question `[[ -v ]]` asks and the same answer: the two
		// constructs agree in every shell that has the operator, so they
		// share the function rather than each having one.
		//
		// Gated here *and* at isTestUnary, and the pair is not redundant:
		// there are two ways in. An expression of exactly two words comes
		// straight here from testExpr without the operator table being
		// consulted at all, and a longer one is read by the parser, which
		// asks isTestUnary first. Taking either gate away leaves `-v`
		// answered on one route and refused on the other, which is why both
		// routes are asserted.
		//
		// Removing isTestUnary's gate alone survives the suite, and that
		// survivor is explained rather than closed: the only answer it
		// moves is the *wording* of a refusal on the multi-term route, and
		// that wording is already wrong there for every operator this shell
		// lacks — `[ -Q x -a -n x ]` says `too many arguments` where the
		// panel names the `-Q`. Asserting it would pin the defect. #1290.
		if !r.dialect().ParameterIsSetTest {
			break
		}
		return r.parameterIsSet(operand)
	case "-t":
		// Whether this shell's descriptor is a terminal, asked of the shell's
		// own table — see descriptorIsTerminal. An operand that is not a
		// number is a question of its own first.
		on, isNumber := r.terminalTest(operand)
		if !isNumber &&
			r.ask(r.sem().TerminalTestRequiresANumber, "`test -t x` refusing a non-number") {
			return false, &testError{kind: errIntegerExpected, operand: operand, decided: true}
		}
		return on, nil
	}
	if !isTestUnary(op) {
		return false, &testError{kind: errUnaryExpected, operand: op}
	}
	return r.fileTest(op, operand), nil
}

// fileTest answers the operators that ask the filesystem something.
//
// Shared with `[[ ]]`, which asks the same questions of an operand it
// obtained differently: the answers are unanimous across the panel and
// identical between the two constructs, so the code is one. Only the file
// questions are shared — the constructs disagree about everything *else*,
// `=` above all, and a shared entry point would invite sharing the parts
// that must not be.
func (r *Runner) fileTest(op, operand string) bool {
	if operand == "" {
		// An empty operand is not a path, so the answer is false for every
		// question below without asking the filesystem — measured across
		// bash 5.3, bash 3.2, dash, ksh93 and zsh 5.9.2, in `[ ]`, `test`
		// and `[[ ]]` alike, and unanimous. Unset, set-and-empty and
		// `$(false)` are one case here rather than three: expansion has
		// already made them the same empty word, and the panel does not
		// distinguish them either.
		//
		// Answered at the one place the operand becomes a path, not in the
		// sixteen arms: atDir used to resolve "" against the runner's
		// directory, so `-e -d -s -r -w -x` all stat'd a directory that
		// exists and all said **true**. `-f` was false only because a
		// directory is not a regular file, which is why the wrong answer
		// survived — the guard most scripts write, `[ -f "$x" ]`, is the
		// one that happened to work, and `[ -d "$dir" ]` is the one that
		// let a script build its whole layout under `/`.
		//
		// It also keeps a non-path out of the policy gate: an empty operand
		// asks the filesystem nothing, so it is not a probe to audit.
		return false
	}
	path := r.atDir(operand)
	// Through the gate, like every stat; a denied one is err != nil here,
	// so every test below reads it as the file not existing.
	info, err := r.stat(path)
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
		li, lerr := r.lstat(path)
		return lerr == nil && li.Mode()&os.ModeSymlink != 0
	}
	return false
}

// compareStat stats one side of a binary file comparison.
//
// An empty operand is a file that is not there, for the reason fileTest
// gives: it is not a path. Said here so both sides get it and the answer
// then falls to the same rules a name that does not exist falls to — the
// MissingFileIsOlder axis below, which measures the same split for an empty
// operand as for a missing one: `[ f -nt "" ]` is true in bash and ksh93,
// false in dash and zsh.
//
// Without it, `[ "" -ef "" ]` was **true**: both sides resolved to the
// runner's directory and os.SameFile agreed they were the same file. Every
// shell in the panel says false.
func (r *Runner) compareStat(operand string) (os.FileInfo, error) {
	if operand == "" {
		return nil, fs.ErrNotExist
	}
	return r.stat(r.atDir(operand))
}

// compareFiles is `a -nt b`, `a -ot b` and `a -ef b` — the binary file
// comparisons, shared with `[[ ]]` exactly as fileTest is.
//
// Newer and older compare modification times when both files exist, which is
// unanimous; so is equal times answering false to both. When one side is
// missing the panel splits: bash and ksh93 count a missing file as older than
// any file that does exist, dash and zsh answer false unless both exist — the
// MissingFileIsOlder axis, asked only there. The mirrored cases ask nothing:
// a file that does not exist is never *newer*, in any shell measured.
//
// Both stats go through the gate, like every stat: a denied path answers as
// a missing one, which the axis and the false branches below already read
// as absence — the documented deny semantics for ActionStat.
func (r *Runner) compareFiles(op, left, right string) (bool, error) {
	li, lerr := r.compareStat(left)
	ri, rerr := r.compareStat(right)
	switch op {
	case "-nt":
		if lerr != nil {
			return false, nil
		}
		if rerr != nil {
			return r.ask(r.sem().MissingFileIsOlder, "`f -nt missing` when f exists"), nil
		}
		return li.ModTime().After(ri.ModTime()), nil
	case "-ot":
		if rerr != nil {
			return false, nil
		}
		if lerr != nil {
			return r.ask(r.sem().MissingFileIsOlder, "`missing -ot f` when f exists"), nil
		}
		return li.ModTime().Before(ri.ModTime()), nil
	}
	// -ef: the same file by identity rather than by name — a hard link, or a
	// symlink followed to it, compares equal; two files with identical
	// content do not, and a missing file is the same as no file.
	return lerr == nil && rerr == nil && os.SameFile(li, ri), nil
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
	case "==":
		// Where the answer is no, `==` is not an operator at all — so the
		// caller has to be told this was not handled and go on to read the
		// three words some other way, which is how dash arrives at
		// "unexpected operator". Refusing to guess is the one case that
		// still counts as handled: the refusal has already been reported and
		// a second complaint about the same words would only obscure it.
		if !r.ask(r.sem().TestAcceptsDoubleEqual, "`test a == b`") {
			return false, nil, r.unspecified
		}
		return left == right, nil, true
	case "!=":
		return left != right, nil, true
	case "-nt", "-ot", "-ef":
		// In `test` as in `[[ ]]`, and in every shell in the panel — dash
		// included, whose lack of `[[ ]]` does not extend to these.
		ok, err := r.compareFiles(op, left, right)
		return ok, err, true
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		l, rv, err := r.testComparisonOperands(left, right)
		if err != nil {
			return false, err, true
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

// testComparisonOperands reads the two operands of a word-spelled comparison.
//
// Two readings, and they are a conflict rather than a subset: one dialect
// reads each operand as an arithmetic *expression*, the way every shell with
// `[[ ]]` reads that construct's operands, and the rest want a numeral and
// name the word that is not one. See
// Semantics.TestBuiltinComparisonOperandsAreArithmetic, where the measurement
// is.
//
// The arithmetic reading is the whole language and not a name lookup, so the
// failures it has are the arithmetic's: a text that will not parse, a name
// whose value will not, a division by zero. Each arrives already worded, and
// the caller puts the builtin's name in front of it.
//
// Asked at the disagreement and nowhere else: two plain numerals are the same
// two numbers under either reading, so `[ 2 -eq 2 ]` puts no question to the
// dialect. A leading zero is not plain — it is eight to the expression and ten
// to the numeral — and neither is anything the numeral reader would refuse.
func (r *Runner) testComparisonOperands(left, right string) (int, int, error) {
	if !plainNumeral(left) || !plainNumeral(right) {
		if r.ask(r.sem().TestBuiltinComparisonOperandsAreArithmetic,
			"`[ n -eq 5 ]` reading its operands as arithmetic") {
			l, failure := r.conditionOperand(left)
			if failure != "" {
				return 0, 0, &testError{kind: errArithmeticOperand, operand: failure}
			}
			rv, failure := r.conditionOperand(right)
			if failure != "" {
				return 0, 0, &testError{kind: errArithmeticOperand, operand: failure}
			}
			return l, rv, nil
		}
	}
	l, lerr := strconv.Atoi(strings.TrimSpace(left))
	rv, rerr := strconv.Atoi(strings.TrimSpace(right))
	if lerr != nil {
		return 0, 0, &testError{kind: errIntegerExpected, operand: left}
	}
	if rerr != nil {
		return 0, 0, &testError{kind: errIntegerExpected, operand: right}
	}
	return l, rv, nil
}

// plainNumeral reports whether the text is a decimal numeral both readings
// answer with the same number: an optional sign, then digits, with no leading
// zero in front of another digit and nothing too wide to hold.
//
// The leading zero is the reason this is not simply "does it parse as a
// number": `[ 010 -eq 10 ]` and `[ 010 -eq 8 ]` are both written by somebody,
// and which holds is exactly the question the arithmetic reading answers
// differently.
func plainNumeral(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	if s == "" || (s[0] == '0' && len(s) > 1) {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	_, err := strconv.Atoi(strings.TrimSpace(text))
	return err == nil
}
