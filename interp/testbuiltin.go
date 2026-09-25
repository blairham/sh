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

// errTestRegexDoesNotCompile is a `=~` right operand the engine refused, in
// the form that reports it with its status alone.
var errTestRegexDoesNotCompile = errors.New("regular expression does not compile")

// testForm is which of the two expression languages a `test`-shaped builtin
// reads: `test` and `[` themselves, or the `[[` that is a command in the one
// dialect whose `[[` is not grammar (see syntax.Dialect.DoubleBracketIsACommand).
//
// The two share the argument-count rules and the grammar past four words, and
// differ in exactly three places, each measured on BusyBox v1.37.0 on
// 2026-09-16 against `[` in the same shell:
//
//   - the connectives are `&&` and `||`, and `-a` and `-o` are not connectives
//     at all — `[[ a = a -a b = b ]]` is `-a: unknown operand` where `[ a = a
//     -a b = b ]` is 0;
//   - `=`, `==` and `!=` match their right operand as a *pattern*, whatever
//     quoting it was written with, since the builtin sees only the expanded
//     word — `[[ abc == "a*" ]]` is 0 and `[ abc == "a*" ]` is 1;
//   - `=~` is a regular expression match, which `[` refuses as an operand.
type testForm struct {
	and, or  string
	patterns bool
}

// testCommandForm is `test` and `[`.
var testCommandForm = testForm{and: "-a", or: "-o"}

// doubleBracketCommandForm is `[[` read as a command.
var doubleBracketCommandForm = testForm{and: "&&", or: "||", patterns: true}

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
		r.diagf("%s\n", Wording(r.diag().TestMissingBracket, "[: missing ]", "]"))
		return 2
	}
	return r.runTest("[", args[:len(args)-1])
}

// biDoubleBracket is `[[` in a dialect whose `[[` is a command rather than a
// keyword: `test` in its own form, closed by a `]]` that is an ordinary last
// argument. Reached only where syntax.Dialect.DoubleBracketIsACommand says so
// — see lookupBuiltin — so a shell whose `[[` is grammar has no command of
// that name, which is what `v='[['; $v a ]]` measures there.
//
// Measured on BusyBox v1.37.0, 2026-09-16: `[[ a` is `missing ]]` at 2, `[[ ]]`
// is 1 as `[ ]` is, `[[ a ]] ]]` is `]]: unknown operand` because only the
// last word closes, and `[[ a "]]"` is 0 because the builtin never sees the
// quotes. A regular expression that will not compile is 2 *with nothing
// written*: `[[ abc =~ "(b" ]]` is silent there.
func biDoubleBracket(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 || args[len(args)-1] != "]]" {
		r.diagf("%s\n", Wording(r.diag().TestMissingBracket, "[[: missing ]]", "]]"))
		return 2
	}
	return r.runTestForm("[[", doubleBracketCommandForm, args[:len(args)-1])
}

func (r *Runner) runTest(name string, args []string) int {
	return r.runTestForm(name, testCommandForm, args)
}

func (r *Runner) runTestForm(name string, form testForm, args []string) int {
	ok, err := r.testExpr(form, args)
	if errors.Is(err, errTestRegexDoesNotCompile) {
		return 2
	}
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
				r.diagf("%s\n", Wording(te.format(*r.diag()), te.fallback(), te.operand, name))
				return 1
			}
			if r.unspecified {
				return 2
			}
			if te.kind == errBinaryExpected && r.diag().NamesBuiltinInLocation &&
				!r.diag().BuiltinNamesTheShellAlone[name] {
				// An expression that never parsed is not the builtin's
				// complaint, *in the dialects that write the builtin's name
				// into the location*. Those are what the measurement is
				// about: `test a b c` is `zsh:1: condition expected: b` with
				// no `test` in it, and so is `[[ a b c ]]`, which is not a
				// builtin at all — while `test -Q x` and `test 1 -eq a`,
				// which failed *evaluating* an expression that did parse, are
				// `zsh:test:1:`. Same wordings, two speakers.
				//
				// And not where this builtin reports as the shell itself,
				// which is a third location and the one a dialect that
				// answers BuiltinNamesTheShellAlone for `test` writes for
				// every complaint it makes. Clearing the speaker there loses
				// the only prefix that column has (#3278).
				//
				// Gated on NamesBuiltinInLocation because clearing the
				// speaker does a second thing nothing measured asked for: it
				// also picks Diagnostics.Location over BuiltinLocation, and a
				// dialect that locates a builtin differently from its parser
				// then writes the parser's location for a builtin's sentence.
				// ksh93 is the case — `test a b c` in a script is
				// `<file>[1]: test: b: unknown operator` there, the bracket
				// form every other `test` complaint in that column already
				// takes, and this shell wrote `<file>: line 1: ` for it
				// alone (#3035). bash and dash have one location for both, so
				// neither can see the difference either way.
				outer := r.inBuiltin
				r.inBuiltin = ""
				defer func() { r.inBuiltin = outer }()
			}
			// The name is the word that was typed. `[` is `test` under
			// another name, and every shell in the panel blames the name it
			// was called by rather than a fixed one — including zsh, which
			// carries it in the location instead of the message and so needs
			// nothing here.
			r.diagf("%s\n", Wording(te.format(*r.diag()), te.fallback(), te.operand, name))
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
	// errTrailingOperandExpected is the same with the operator named, for
	// the two columns that parse the two-word form rather than reading the
	// first word as a unary operator. See
	// Diagnostics.TestTrailingBinaryOperandExpected.
	errTrailingOperandExpected
	// errLeftoverOperator is a well-formed expression with a word left over
	// that is spelled like an operator — see
	// Diagnostics.TestLeftoverOperator, where the split is.
	errLeftoverOperator
	// errTooManyArguments is a well-formed expression with words left over.
	errTooManyArguments
	// errIncorrectSyntax is a list the grammar could not finish: a word it
	// cannot place, in a reading that had already committed to consuming the
	// whole list. Reached only where
	// Semantics.TestReadsOneExpressionOffTheOperands says yes — the reader
	// that drops what follows an expression is the one that has to say when
	// it will not, and no other column has the sentence.
	errIncorrectSyntax
	// errIntegerExpected is a non-numeric operand to a numeric comparison.
	errIntegerExpected
	// errClosingParenExpected is a group the reading never closed. One
	// column reaches it where no other does — see
	// Semantics.TestGroupedUnaryAloneLosesTheClosingParen.
	errClosingParenExpected
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
	case errTrailingOperandExpected:
		return "%[2]s: %[1]s: argument expected"
	case errLeftoverOperator:
		return "%[2]s: too many arguments"
	case errTooManyArguments:
		return "%[2]s: too many arguments"
	case errIncorrectSyntax:
		return "%[2]s: incorrect syntax"
	case errIntegerExpected:
		return "%[2]s: %[1]s: integer expected"
	case errArithmeticOperand:
		// The math complaint as the arithmetic worded it, behind the name
		// the builtin was called by: `[: 1x1: arithmetic syntax error`.
		return "%[2]s: %[1]s"
	case errBinaryExpected:
		return "%[2]s: %[1]s: binary operator expected"
	case errClosingParenExpected:
		return "%[2]s: closing paren expected"
	}
	return "%[2]s: %[1]s: unary operator expected"
}

func (e *testError) format(d Diagnostics) string {
	switch e.kind {
	case errOperandExpected:
		return d.TestOperandExpected
	case errTrailingOperandExpected:
		return d.TestTrailingBinaryOperandExpected
	case errLeftoverOperator:
		if d.TestLeftoverOperator != "" {
			return d.TestLeftoverOperator
		}
		return d.TestTooManyArguments
	case errTooManyArguments:
		return d.TestTooManyArguments
	case errIncorrectSyntax:
		return d.TestIncorrectSyntax
	case errIntegerExpected:
		return d.TestIntegerExpected
	case errArithmeticOperand:
		// No dialect wording: the sentence is the arithmetic's, which each
		// dialect has already worded through its own math diagnostics.
		return ""
	case errBinaryExpected:
		return d.TestBinaryExpected
	case errClosingParenExpected:
		if e.operand != "" && d.TestClosingParenExpectedFound != "" {
			// A word was there and was not the parenthesis, in the column
			// that names it — see Diagnostics.TestClosingParenExpectedFound.
			return d.TestClosingParenExpectedFound
		}
		return d.TestClosingParenExpected
	}
	return d.TestUnaryExpected
}

// groupedUnaryAlone is the shape
// [Semantics.TestGroupedUnaryAloneLosesTheClosingParen] refuses: a group
// holding a unary operator and nothing but its operand, standing as the whole
// expression behind any number of leading `!`s.
//
// Three words or four and no more, which is measured rather than tidy. One
// word further and the shell reads the expression again: `[ ( -n x -a y ) ]`
// is 0 there, where `[ ( -n x ) ]` and `[ ( -n ) ]` are both the refusal. The
// same group inside a longer expression is read as well — `[ ( -n x ) -a x ]`
// is 0 — which is why the whole list is what this looks at.
//
// The axis is asked only once the shape is found, so no other dialect is
// asked a question its own `test` never poses.
func (r *Runner) groupedUnaryAlone(args []string) bool {
	for len(args) > 0 && args[0] == "!" {
		args = args[1:]
	}
	if len(args) != 3 && len(args) != 4 {
		return false
	}
	if args[0] != "(" || args[len(args)-1] != ")" || !r.isTestUnary(args[1]) {
		return false
	}
	return r.ask(r.sem().TestGroupedUnaryAloneLosesTheClosingParen,
		"`[ ( -n x ) ]` refused as a group that never closed")
}

// emptyGroup is a `(` closed by the very next word, on the axis that reads
// one as false rather than as a list that ran out of words.
//
// Both readings of `test` reach it — the two-word count and the grammar — and
// both ask here, because the shape is the same one and the answer is the
// dialect's rather than the route's. The axis is asked only once the shape is
// found, so no dialect whose groups always hold something is asked a question
// its own `test` never poses.
//
// See Semantics.TestEmptyGroupIsFalse.
func (r *Runner) emptyGroup(open, next string) bool {
	if open != "(" || next != ")" {
		return false
	}
	return r.ask(r.sem().TestEmptyGroupIsFalse, "`[ ( ) ]` read as a false expression")
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
// than `-o`. That shape is unanimous across the panel; what is *inside* it is
// not, and one column reads the counts and the grammar differently enough to
// need a reader of its own — see the axis read on the first line of the body.
// unclosedGroup is the sentence a refusal takes when the reading gave up
// inside a group it had not closed, in the one column that replaces it.
//
// Every caller is a place where a group is *open*: the parser's own group
// primary, and the count-based readings, which have no group parser in them
// at all — so a leading `(` they refuse is a group nothing was ever going to
// close. See Semantics.TestFailureInsideAnUnclosedGroupIsTheParen.
func (r *Runner) unclosedGroup(err error) error {
	if err == nil {
		return nil
	}
	if !r.ask(r.sem().TestFailureInsideAnUnclosedGroupIsTheParen,
		"a refusal inside an unclosed `test` group named as the missing paren") {
		return err
	}
	return &testError{kind: errClosingParenExpected}
}

// countedLeadingGroup reports whether a refused expression was read by the
// argument counts *and* opened a group there.
//
// The counts are the whole of the reading for four words or fewer, and none
// of the four has a group parser in it: the three-word form matches `( x )`
// and the four-word one `( E )` by position, and everything else with a
// leading `(` is refused with the parenthesis still open. Four words whose
// last is not `)` fall past the counts to the parser, which closes its own
// groups and is why `[ ( x ) junk ]` names the leftover word in both columns.
func countedLeadingGroup(args []string) bool {
	switch len(args) {
	case 2, 3:
		return args[0] == "("
	case 4:
		return args[0] == "(" && args[3] == ")"
	}
	return false
}

// testExpr evaluates the expression and then says what a refusal is called,
// which is a question one column answers differently — see unclosedGroup.
func (r *Runner) testExpr(form testForm, args []string) (bool, error) {
	v, err := r.testExprRead(form, args)
	if err != nil && countedLeadingGroup(args) {
		return false, r.unclosedGroup(err)
	}
	return v, err
}

func (r *Runner) testExprRead(form testForm, args []string) (bool, error) {
	if r.sem().TestReadsOneExpressionOffTheOperands == Yes {
		// One column reads a single expression off the front of the list and
		// drops the rest, which is a different reader rather than a leniency
		// laid over this one — see interp/testfrontexpression.go. Read here
		// rather than asked, for the reason isTestUnary reads its letter
		// rather than asking it: this is the route, taken by every `test` in
		// every dialect, and the question it answers is settled for the core
		// in the preset.
		return r.frontExpr(form, args)
	}
	if r.groupedUnaryAlone(args) {
		// A group whose first word is a unary operator, standing as the
		// whole expression, in the one column that cannot close it. Before
		// the counts because it is both a four-word shape and a three-word
		// one, and behind neither: `[ ( -n ) ]` is the same refusal as
		// `[ ( -n x ) ]`.
		return false, &testError{kind: errClosingParenExpected}
	}
	switch len(args) {
	case 0:
		// No expression is false rather than an error.
		return false, nil
	case 1:
		return r.testOneOperand(args[0])
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
		if r.emptyGroup(args[0], args[1]) {
			// A group closed by its own next word, standing as the whole
			// expression. Before unaryTest because `(` is not a unary
			// operator and that is the refusal this replaces.
			return false, nil
		}
		return r.unaryTest(args[0], args[1])
	case 3:
		if ok, err, handled := r.binaryTest(form, args[0], args[1], args[2]); handled {
			return ok, err
		}
		if args[1] == form.and || args[1] == form.or {
			if args[0] == "!" && r.threeWordsNegateFirst() {
				// The one shape the two readings disagree about, and the
				// only place the axis is asked: a leading `!` ahead of the
				// connective, which is the order POSIX gives the
				// three-operand rule in and which two columns follow. So
				// `[ ! -a x ]` is the two-word `-a x` refused rather than
				// the both-set guard over the strings `!` and `x`. The
				// binary reading above still comes first in every column —
				// `[ ! = x ]` is a string comparison here too. See
				// Semantics.TestThreeWordsNegateBeforeAConnective.
				v, err := r.testExpr(form, args[1:])
				return !v, err
			}
			// The connectives, over two *strings* rather than over two
			// expressions: three words leave no room for an operator on
			// either side, so each side is true when it is non-empty and
			// `[ "$a" -a "$b" ]` is the both-set guard people write it for.
			//
			// The guard itself is unanimous — bash 5.3, bash-as-`sh`, bash
			// 3.2, ksh93, zsh 5.9.2, dash and BusyBox ash all answer
			// `[ x -a "" ]` false and `[ "" -o x ]` true — and the refusal
			// this replaces was a plain defect: the grammar below already
			// had both connectives and only the three-word form fell through
			// it to `binary operator expected` at status 2. A guard that is
			// meant to answer yes or no answered "this is not an
			// expression", and a script reading `$?` saw neither.
			//
			// What is **not** unanimous is where this reading stands against
			// a leading `!`, which is the branch above it: this comment used
			// to claim the whole three-word connective form was unanimous,
			// and `[ ! -a x ]` is a refusal in two of the seven columns
			// (#3717).
			//
			// Not folded into binaryTest, which the grammar also calls: the
			// parser reads these two as connectives with `-a` binding
			// tighter, and a primary that swallowed `a -o b` whole would
			// make `[ a -o b -a c ]` associate the other way.
			left, right := args[0] != "", args[2] != ""
			if args[1] == form.and {
				return left && right, nil
			}
			return left || right, nil
		}
		if args[0] == "!" {
			// The other order, where the connective above got first refusal.
			v, err := r.testExpr(form, args[1:])
			return !v, err
		}
		if args[0] == "(" && args[2] == ")" {
			return args[1] != "", nil
		}
		if v, err, handled := r.trailingConnectiveOver(func() (bool, error) {
			return r.testExpr(form, args[:2])
		}, args[2], args[0] != "("); handled {
			// Two words this reader has already refused once, and a
			// connective behind them with nothing behind it.
			return v, err
		}
		if (args[2] == "-a" || args[2] == "-o") && r.diag().TestNamesTheWordTheParseStoppedAt {
			// A connective with nothing behind it, in the column that parses
			// the words: an operator missing its right operand rather than a
			// word the parse stopped at, so nothing is named.
			return false, &testError{kind: errOperandExpected}
		}
		blamed := args[1]
		if r.diag().TestNamesTheWordTheParseStoppedAt && r.isTestUnary(args[0]) {
			// The parse took two words rather than one, so the word it
			// stopped at is the third. `[ -z a b ]` names `b` where
			// `[ a b c ]` names `b` as well — one past the expression each
			// time, which is the same rule and not two.
			blamed = args[2]
		}
		if r.diag().TestNamesFirstOperand {
			// dash names the last word of the expression that *did* parse
			// rather than the one that should have been an operator, which
			// for three words is the first — unless the first is a unary
			// operator, in which case its operand went with it. Measured
			// 2026-09-16: `test a b c` is `a: unexpected operator` and
			// `test -n a b` is `a`, where naming the first word alone would
			// say `-n`. See Diagnostics.TestNamesFirstOperand.
			blamed = args[0]
			if r.isTestUnary(args[0]) {
				blamed = args[1]
			}
		}
		return false, &testError{kind: errBinaryExpected, operand: blamed}
	case 4:
		if args[0] == "!" {
			v, err := r.testExpr(form, args[1:])
			if r.threeWordsReadAsANegation(form, args[1:]) &&
				r.ask(r.sem().TestFourWordsNegateANegationOnce,
					"a four-word `test` whose leading `!` stands in front of a negation") {
				// One column takes the three-word reading of the rest and
				// does not negate it again, so `[ ! ! -n x ]` there is
				// `[ ! -n x ]`. Only where the rest is itself a negation:
				// `[ ! x = x ]` negates in every column, and so does
				// `[ ! ! = x ]`, whose three words are a string comparison
				// with `!` as the left operand.
				return v, err
			}
			return !v, err
		}
		if args[0] == "(" && args[3] == ")" {
			return r.testExpr(form, args[1:3])
		}
	}
	p := &testParser{r: r, form: form, args: args}
	v, err := p.orExpr()
	if err != nil {
		return false, err
	}
	if p.pos != len(p.args) {
		// The operand is the last word the parse took, which is what the
		// one dialect with no "too many arguments" sentence names: dash
		// calls the leftover `<that word>: unexpected operator`. Measured
		// 2026-09-16 — `test a = b = c` names `b`, `test x = y z` names
		// `y`, `test a -a b c` names `b` and `test a b c d` names `a`, one
		// past the end of the expression each time. Every other dialect's
		// format ignores it.
		leftover := p.lastTaken()
		if r.diag().TestNamesTheWordTheParseStoppedAt {
			// The other reading of the same position: the first word the
			// parse did *not* take, rather than the last one it did.
			leftover = p.args[p.pos]
		}
		if stopped := p.args[p.pos]; strings.HasPrefix(stopped, "-") &&
			r.diag().TestLeftoverOperator != "" {
			// A leftover that is spelled like an **operator** is a different
			// sentence in one column, and it names that word rather than the
			// one the parse stopped after. See
			// Diagnostics.TestLeftoverOperator.
			return false, &testError{kind: errLeftoverOperator, operand: stopped}
		}
		return false, &testError{kind: errTooManyArguments, operand: leftover}
	}
	return v, nil
}

// threeWordsReadAsANegation reports whether the three-word reading of these
// words takes its `!` branch rather than one of the two that come before it.
//
// It mirrors the order in testExprRead's three-word case and has to stay with
// it: the binary operator is looked for first, and then a leading `!` and the
// two connectives in whichever order
// Semantics.TestThreeWordsNegateBeforeAConnective puts them.
// `[ ! = x ]` is the row that makes the binary step load-bearing — `=` binds
// the `!` as a left operand, so those three words are a string comparison and
// not a negation at all — and `[ ! -a x ]` is the row that makes the other
// two an axis rather than an order.
//
// Asked by the four-word reading, which one column is measured to *absorb*
// rather than negate. See Semantics.TestFourWordsNegateANegationOnce.
func (r *Runner) threeWordsReadAsANegation(form testForm, args []string) bool {
	if len(args) != 3 || args[0] != "!" || r.testBinaryOperatorWord(form, args[1]) {
		return false
	}
	if args[1] == form.and || args[1] == form.or {
		// The one shape the two readings disagree about, asked here for the
		// same reason it is asked there: where the negation is read first, a
		// middle `-a` or `-o` does not take these three words away from it,
		// and `[ ! ! -a x ]` is the refusal `[ ! -a x ]` gives.
		return r.threeWordsNegateFirst()
	}
	return true
}

// threeWordsNegateFirst reports whether a three-word `test` led by `!` is read
// as a negation of the other two ahead of the connective reading.
//
// See Semantics.TestThreeWordsNegateBeforeAConnective for the panel.
func (r *Runner) threeWordsNegateFirst() bool {
	return r.ask(r.sem().TestThreeWordsNegateBeforeAConnective,
		"a three-word `test` reading its leading `!` before a connective")
}

// testOneOperand is the one-word reading: a word, true when it is not empty.
//
// `-f` is *true* here rather than an operator with a missing operand, which
// is the whole of why the count comes before the grammar. The one exception
// is `-t`, which two shells read as `-t 1` — see bareTerminalTest.
//
// Its own function because both readers have this case and it is the one
// place an axis answers something inside it.
func (r *Runner) testOneOperand(word string) (bool, error) {
	if word == "-t" && r.bareTerminalTest() {
		return r.unaryTest("-t", "1")
	}
	return word != "", nil
}

// shortGroup is how many words stand between the parenthesis just consumed
// and a closing one, where that is **one, two or three** — and 0 for every
// other shape, the empty group and the unclosed one included.
//
// Three and no further, which is measured rather than tidy: four words inside
// is the grammar again, and the operator there does take the parenthesis. See
// Semantics.TestShortGroupIsReadByTheCounts for the rows.
func (p *testParser) shortGroup() int {
	for n := 1; n <= 3; n++ {
		if p.pos+n < len(p.args) && p.args[p.pos+n] == ")" {
			return n
		}
	}
	return 0
}

// lastTaken is the final word the parse consumed, for the complaint about
// what came after it.
//
// Empty where nothing was taken, which cannot happen on the path that asks —
// a parse that consumed nothing has already failed with a complaint of its
// own — and is written out rather than assumed.
func (p *testParser) lastTaken() string {
	if p.pos < 1 || p.pos > len(p.args) {
		return ""
	}
	return p.args[p.pos-1]
}

// testParser is the grammar that takes over past four arguments: `-a` binds
// tighter than `-o`, `!` binds tighter still, and `( )` groups.
type testParser struct {
	r    *Runner
	form testForm
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
	for p.more() && p.peek() == p.form.or {
		p.pos++
		if !p.more() && p.r.trailingConnectiveIsMissing() {
			// Nothing behind the connective, and this dialect reads that as
			// a right operand that is missing and therefore false.
			return left, nil
		}
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
	for p.more() && p.peek() == p.form.and {
		p.pos++
		if !p.more() && p.r.trailingConnectiveIsMissing() {
			// The same, and false rather than left: an `and` over a missing
			// right operand is false whatever stood in front of it.
			return false, nil
		}
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
		if p.more() && p.r.emptyGroup("(", p.peek()) {
			// Nothing between the parentheses, on the axis that reads that
			// as a value. Both words are taken, so the group stands where
			// any other operand would and the connectives around it compose
			// with it.
			p.pos++
			return false, nil
		}
		// A group holding one, two or three words is read by the **counts**
		// rather
		// than by the grammar, in the one column measured to do it: the
		// words inside stand exactly as they would with no parentheses
		// round them, so a lone operator is the word it is spelled with
		// and a `!` in front of one negates that word. See
		// Semantics.TestShortGroupIsReadByTheCounts.
		if n := p.shortGroup(); n > 0 && p.r.ask(p.r.sem().TestShortGroupIsReadByTheCounts,
			"a `test` group holding one or two words read by the argument counts") {
			content := p.args[p.pos : p.pos+n]
			p.pos += n + 1
			return p.r.testExpr(p.form, content)
		}
		if p.r.unspecified {
			return false, &testError{kind: errOperandExpected}
		}
		v, err := p.orExpr()
		if err != nil {
			return false, p.r.unclosedGroup(err)
		}
		if !p.more() || p.peek() != ")" {
			// The group's **own** missing parenthesis, which is a different
			// complaint from a failure inside one — see unclosedGroup, whose
			// axis is about replacing that. Four columns word this and they
			// do not agree: two name the parenthesis, two say `argument
			// expected` and reach it through the reading below. The word
			// that was found where the parenthesis belonged is carried for
			// the column that names it, and for `[` that word is the `]`
			// the builtin took off the end.
			if w := p.r.diag().TestClosingParenExpected; w != "" {
				found := ""
				if p.more() {
					found = p.peek()
				} else if p.r.inBuiltin == "[" {
					found = "]"
				}
				return false, &testError{kind: errClosingParenExpected, operand: found}
			}
			return false, p.r.unclosedGroup(&testError{kind: errOperandExpected})
		}
		p.pos++
		return v, nil
	}

	// A binary operator is decided by the *second* word, so it is looked for
	// before a unary one: `test -n = -n` compares two strings rather than
	// testing whether "=" is non-empty.
	if p.pos+2 < len(p.args) {
		if ok, err, handled := p.r.binaryTest(p.form, p.args[p.pos], p.args[p.pos+1], p.args[p.pos+2]); handled {
			p.pos += 3
			return ok, err
		}
	}
	if err := p.unknownOperator(); err != nil {
		return false, err
	}
	if p.r.isTestUnary(p.args[p.pos]) {
		if p.pos+1 >= len(p.args) {
			// Nothing left for it to be an operator over. Three columns read
			// the word the way the one-argument rule does and two refuse it
			// — see Semantics.TestTrailingUnaryOperatorIsAWord, and
			// testOneOperand, which is that rule and carries `-t`'s own
			// question inside it.
			if p.r.ask(p.r.sem().TestTrailingUnaryOperatorIsAWord,
				"an operator with nothing behind it at the end of a `test` expression read as a word") {
				word := p.args[p.pos]
				p.pos++
				return p.r.testOneOperand(word)
			}
			// An unanswered axis has already said so and runTestForm turns
			// that into the refusal status; the complaint below is the
			// answer for a dialect that said no.
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
	// An ordering operator this dialect does not have, standing where a
	// *binary* one belongs. It is the same fault the paragraph above
	// describes arriving by the other door: `<` is not spelled like a unary
	// operator, so without this the left operand becomes a bare string and
	// the count is reported instead of the one token that was wrong.
	// Measured — `test a '<' b -a b '>' a` is `test: <: unknown operator` in
	// ksh93 and `condition expected: <` in zsh, and neither says how many
	// arguments there were.
	if next := p.pos + 1; next < len(p.args) {
		if op := p.args[next]; (op == "<" || op == ">") && !p.r.stringOrderOperator(op) {
			// Which of the two words is blamed is the three-word complaint's
			// question, already answered.
			blamed := op
			if p.r.diag().TestNamesFirstOperand {
				blamed = p.args[p.pos]
			}
			return &testError{kind: errBinaryExpected, operand: blamed}
		}
	}
	word := p.args[p.pos]
	if len(word) < 2 || word[0] != '-' || p.r.isTestUnary(word) {
		return nil
	}
	switch next := p.pos + 1; {
	case next >= len(p.args):
		return nil
	case p.args[next] == p.form.and, p.args[next] == p.form.or, p.args[next] == ")":
		return nil
	}
	if (word == p.form.and || word == p.form.or) && p.r.diag().TestConnectiveIsALeftoverWord {
		// The same reading unaryTest gives the two-word form: a connective
		// this dialect has, standing where a primary begins and without the
		// file test behind it, is a string with a word left over.
		return &testError{kind: errTooManyArguments}
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
	switch s {
	case "-v":
		return r.lang().ParameterIsSetTest
	case "-R":
		return r.sem().TestHasTheNameReferenceOperator == Yes
	case "-a":
		// The connective under the same spelling, and the argument count is
		// what tells them apart — so this is asked only where a *primary*
		// begins, which is where the count has already said the word is an
		// operator. `[ x -a y ]` never reaches here: three words go straight
		// to the connective, and in the grammar past four the connective is
		// read between primaries rather than at the front of one.
		return r.sem().TestHasTheFileExistsLetter == Yes
	case "-o":
		return r.sem().TestHasTheShellOptionOperator == Yes
	case "-N":
		return r.sem().TestHasTheModifiedSinceReadOperator == Yes
	}
	return isTestUnary(s)
}

func isTestUnary(s string) bool {
	switch s {
	case "-n", "-z", "-e", "-f", "-d", "-s", "-r", "-w", "-x", "-L", "-h",
		"-b", "-c", "-p", "-S", "-g", "-u", "-k", "-t",
		// Ownership. Unanimous across the panel — bash 5.3, bash-as-`sh`,
		// bash 3.2, ksh93, zsh 5.9.2, dash and BusyBox ash all answer both,
		// which is a wider set than `[[ ]]` has because dash and ash have
		// the builtin without the keyword.
		"-O", "-G":
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
		if !r.lang().ParameterIsSetTest {
			break
		}
		// testParameterIsSet and not parameterIsSet, for the one thing the
		// builtin does that the condition does not: a subscript that
		// reached it as text is rounded here, and `shopt -s
		// assoc_expand_once` stops the round at this operator alone. See
		// Semantics.TestIsSetExpandsAFlatSubscript (#3298).
		return r.testParameterIsSet(operand)
	case "-R":
		// Whether the name is a **reference**, which is a question about the
		// binding rather than about what it points at: the reference answers
		// true and its target answers false. Gated here and at isTestUnary
		// for the reason `-v` is — the two-word route never consults the
		// operator table.
		if !r.ask(r.sem().TestHasTheNameReferenceOperator,
			"`test -R r` asking whether r is a name reference") {
			break
		}
		return r.isNameref(operand), nil
	case "-a":
		// The file test, not the connective: two words have already settled
		// which this is. Asked of the dialect at both gates for the reason
		// `-v` is — the two-word route never consults the operator table.
		if !r.ask(r.sem().TestHasTheFileExistsLetter, "`test -a f` asking whether f exists") {
			break
		}
		return r.fileTest("-e", operand), nil
	case "-o":
		// A shell option by the name `set -o` gives it. A name this shell
		// has never heard of is false rather than an error, measured in both
		// shells that have the operator — so `known` is deliberately
		// discarded.
		if !r.ask(r.sem().TestHasTheShellOptionOperator, "`test -o errexit` asking whether an option is set") {
			break
		}
		on, _ := r.NamedOption(operand)
		return on, nil
	case "-N":
		// Written since last read: the modification time against the access
		// time. The operator's presence is the axis and its answer is this
		// comparison — see Semantics.TestHasTheModifiedSinceReadOperator for
		// why the boolean is not what gets pinned.
		if !r.ask(r.sem().TestHasTheModifiedSinceReadOperator, "`test -N f` asking whether f was written since it was read") {
			break
		}
		return r.modifiedSinceRead(operand), nil
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
		if w := r.diag().TestTrailingBinaryOperandExpected; w != "" && r.hasTestBinaryOperator(operand) {
			// The two words are a left operand and a binary operator this
			// shell has, so what is missing is the operator's right operand
			// — see Diagnostics.TestTrailingBinaryOperandExpected. Asked
			// before the readings below, which all name the word in front.
			return false, &testError{kind: errTrailingOperandExpected, operand: operand}
		}
		if v, err, handled := r.trailingConnective(operand, op != "", op != "(" && op != ")"); handled {
			// The word behind is a connective with nothing behind *it*, so
			// the left side is the one word in front read as a string. Asked
			// before the refusals below, because in the columns that hold
			// the axis this is not a refusal at all.
			return v, err
		}
		if (op == "-a" || op == "-o") && r.diag().TestConnectiveIsALeftoverWord {
			// A connective this dialect has, standing where a unary operator
			// belongs and without the file test behind it: read as a string
			// with a word left over rather than as an operator nobody has.
			// See Diagnostics.TestConnectiveIsALeftoverWord.
			return false, &testError{kind: errTooManyArguments}
		}
		if r.diag().TestNamesTheWordTheParseStoppedAt {
			// One column parses the two words rather than reading the first
			// as the operator, so the word it has no operator for is the
			// *second* one — unless that word is a connective, which is an
			// operator missing its right operand and is said with nothing
			// named. See Diagnostics.TestNamesTheWordTheParseStoppedAt.
			if operand == "-a" || operand == "-o" {
				return false, &testError{kind: errOperandExpected}
			}
			return false, &testError{kind: errBinaryExpected, operand: operand}
		}
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
// ownDescriptorPath is the path of the file **this shell** has open at the
// descriptor a `/dev/fd/N` operand names, where it has one.
//
// A Runner keeps its own descriptor table, so the number a script writes is
// not the number the process holds: `exec 6>&1` opens the shell's 6 at
// whatever the Go runtime handed out. Statting the literal path asks the
// process about *its* 6, which is a descriptor no line of the script ever
// mentioned — so `test -p /dev/fd/6` over a pipe the script opened answered
// about something else entirely, and did it silently.
//
// Measured 2026-09-22 on bash 5.3.20 with standard output on a pipe:
// `test -p /dev/fd/6` is false with nothing open there and true after
// `exec 6>&1`, and true after `exec 6<>fifo` for a real named pipe. The
// mapping is what makes both rows the shell's answer rather than the
// runtime's.
//
// Only where the shell **has** a descriptor there. A number it has nothing
// open at is left as the path it was written as, which is the reading this
// already had, and it is deliberately not answered as "no such file" even
// though that is what the reference would say: a process substitution's
// `/dev/fd/N` is a **process** descriptor this table does not hold — see
// childFiles, where that is spelled out — so a rule that refused every number
// the table is missing would make `test -e <(echo hi)` false.
//
// What that leaves is a number nobody in the script opened answering about
// whatever the process has there, which is measurable and is not hypothetical:
// on a Linux runner the test binary really does hold a pipe at 6. It is the
// reading this had before and the narrower claim to make today.
func (r *Runner) ownDescriptorPath(operand string) (string, bool) {
	rest, ok := strings.CutPrefix(operand, devFdDir+"/")
	if !ok {
		return "", false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return "", false
	}
	sys, open := r.SystemDescriptor(n)
	if !open || sys == n {
		// Already the same number, so there is nothing to translate and the
		// path stands — which is every ordinary `/dev/fd/0`.
		return "", false
	}
	return inheritedPath(sys), true
}

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
	if own, ok := r.ownDescriptorPath(operand); ok {
		// A path naming one of **this shell's** descriptors is the file the
		// shell has open there, not the process's Nth. See
		// ownDescriptorPath.
		path = own
	}
	// Through the gate, like every stat; a denied one is err != nil here,
	// so every test below reads it as the file not existing.
	info, err := r.stat(path)
	switch op {
	case "-e", "-a":
		// `-a` is the older spelling of `-e` and asks the same question,
		// inside `[[ ]]` only: the `[` builtin reads the word as `and`
		// instead, which is why nothing routes it here from there.
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
	case "-O", "-G":
		// Ownership, against the *effective* identity rather than the real
		// one — which is what a shell running under setuid is asking about,
		// and the same pair `U` and `G` already compare a glob against.
		//
		// A file whose stat this platform cannot decompose answers false
		// rather than true: the question is "is this mine", and a shell that
		// could not tell must not say yes.
		if err != nil {
			return false
		}
		kind, want := byte('u'), uint64(uint32(osGeteuid()))
		if op == "-G" {
			kind, want = 'g', uint64(uint32(osGetegid()))
		}
		id, ok := fileOwner(info, kind)
		return ok && id == want
	}
	return false
}

// modifiedSinceRead is `test -N f`: the file has been written since it was
// last read.
//
// The modification time against the access time, to the nanosecond — a file
// written and read inside one second is the ordinary case, and the one column
// that compares whole seconds is the one column that gets it wrong. A file
// that is not there, an empty operand and a stat this platform cannot
// decompose are all false, which is measured for the first two and is the
// bargain `-O` and `-G` already strike for the third.
func (r *Runner) modifiedSinceRead(operand string) bool {
	if operand == "" {
		return false
	}
	info, err := r.stat(r.atDir(operand))
	if err != nil {
		return false
	}
	read, ok := fileAccessTime(info)
	if !ok {
		// A platform whose stat this build cannot decompose answers false,
		// the bargain `-O` and `-G` strike for the third: the question is
		// "was this written since it was read", and a shell that could not
		// tell must not say yes.
		return false
	}
	written := info.ModTime()
	if written.After(read) {
		return true
	}
	if read.After(written) {
		return false
	}
	// The tie, and the only part of the comparison the panel splits on —
	// asked here rather than in front of the whole function, so a file whose
	// two times differ never raises the question. See
	// Semantics.TestModifiedSinceReadCountsAnEqualTime for the measurement.
	return r.ask(r.sem().TestModifiedSinceReadCountsAnEqualTime,
		"`-N` on a file written and not read since, whose two times are equal")
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

// stringOrderOperator reports whether this dialect orders strings with this
// operator, resolving the axis once for everything that has to know.
//
// One function rather than the enum read at each site, because the sites
// disagree about what to *do* and must not come to disagree about what the
// answer is: the primary reader below names an operator the dialect lacks,
// and binaryTest hands the words back unread.
func (r *Runner) stringOrderOperator(op string) bool {
	switch r.testStringOrder() {
	case TestStringOrderBoth:
		return true
	case TestStringOrderGreaterOnly:
		return op == ">"
	}
	return false
}

// testBinaryOperatorWord reports whether a word stands as the *operator* of a
// three-word reading, which is exactly the set binaryTest below handles.
//
// It reads the vector rather than asking it, because it is consulted a second
// time — see threeWordsReadAsANegation — and asking twice would complain
// twice about one word. The two spellings a dialect may lack report through
// their own axes inside binaryTest, so an unanswered axis counts as an
// operator here: that way the refusal is reached rather than routed around.
func (r *Runner) testBinaryOperatorWord(form testForm, op string) bool {
	switch op {
	case "=", "!=", "-nt", "-ot", "-ef", "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		return true
	case "==":
		return form.patterns || r.sem().TestAcceptsDoubleEqual != No
	case "=~":
		return form.patterns
	case "<", ">":
		switch r.sem().TestStringOrder {
		case TestStringOrderBoth, TestStringOrderUnspecified:
			return true
		case TestStringOrderGreaterOnly:
			return op == ">"
		}
	}
	return false
}

// binaryTest is `a OP b`. The third return says whether the middle word was an
// operator at all, which is what lets the caller fall back to another reading
// rather than guessing.
func (r *Runner) binaryTest(form testForm, left, op, right string) (bool, error, bool) {
	if !r.testBinaryOperatorWord(form, op) {
		// Not an operator word at all, which is the same answer the switch
		// below falls out of — said once, in the one place a second reader
		// can ask the question too.
		return false, nil, false
	}
	if form.patterns {
		switch op {
		case "=", "==", "!=":
			// A pattern, and never a quoted literal: the words were expanded
			// and had their quotes removed before the builtin was called, so
			// there is nothing left to say which characters were quoted.
			got := r.matchPatternR(right, left, true)
			return got == (op != "!="), nil, true
		case "=~":
			ok, err := r.regexMatch(right, left)
			if err != nil {
				return false, errTestRegexDoesNotCompile, true
			}
			return ok, nil, true
		}
	}
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
	case "<", ">":
		// Byte order, and a *string* comparison — `test 10 '<' 9` is true.
		// Which of the two the shell has is one enum rather than two flags,
		// because ksh93 has `>` and refuses `<`: see TestStringOrderPolicy.
		//
		// Not handled where the shell lacks the operator, so the words fall
		// to whatever reading the caller has left — which is how a dialect
		// keeps the refusal it already gives for a word that is not an
		// operator, worded its own way. An unanswered axis has already
		// complained, and counts as handled so that one fault gets one
		// sentence.
		if !r.stringOrderOperator(op) {
			return false, nil, r.unspecified
		}
		if op == "<" {
			return left < right, nil, true
		}
		return left > right, nil, true
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

// trailingConnectiveIsMissing resolves
// [Semantics.TestTrailingConnectiveTakesAMissingOperand], and is asked only
// where a connective really has nothing behind it — so a dialect with no
// answer is not asked a question no expression posed.
func (r *Runner) trailingConnectiveIsMissing() bool {
	return r.ask(r.sem().TestTrailingConnectiveTakesAMissingOperand,
		"`test x -a` reading the connective with a right operand that is missing")
}

// trailingConnective answers the two-word form whose second word is a
// connective: `test x -a` and `test x -o`, where the left side is the one
// word in front read as a string and the right side is missing.
//
// The third result is whether the axis took the question. False leaves the
// caller's own refusal in place, which is what four of the six columns give.
//
// standsAlone is false where the word in front is a **grouping** token rather
// than a left side: `[ ( -a ]` is `closing paren expected` in dash and
// `argument expected` in zsh, so a parenthesis nothing closed is still a
// parenthesis nothing closed and the connective never gets that far.
func (r *Runner) trailingConnective(word string, left, grouped bool) (bool, error, bool) {
	return r.trailingConnectiveOver(func() (bool, error) { return left, nil }, word, grouped)
}

// trailingConnectiveOver is the same over a left side the caller evaluates —
// the three-word form, where the two words in front are an expression this
// reader has already tried once.
//
// The left side is evaluated only where the axis takes the question, so a
// dialect that refuses does not run an expression it is about to complain
// about; and its own refusal, where it has one, is what comes back.
func (r *Runner) trailingConnectiveOver(left func() (bool, error), word string, standsAlone bool) (bool, error, bool) {
	if word != "-a" && word != "-o" || !standsAlone {
		return false, nil, false
	}
	if !r.trailingConnectiveIsMissing() {
		return false, nil, false
	}
	v, err := left()
	if err != nil {
		return false, err, true
	}
	// The missing operand is **false**, which the statuses are what prove:
	// `-a` answers 1 over a true left side where `-o` answers 0, and both
	// answer 1 over a false one.
	if word == "-a" {
		return false, nil, true
	}
	return v, nil, true
}

// hasTestBinaryOperator reports whether a word is a binary operator this
// dialect has, which is what decides whether the two-word form ending in one
// is an operator missing its right operand or a word in the wrong place.
//
// The roster is read rather than asked: every arm below is a question the
// dialect has already answered in its vector, and a complaint about a missing
// operand must not itself produce an "unanswered axis" refusal about an
// operator the expression never got to use.
func (r *Runner) hasTestBinaryOperator(op string) bool {
	switch op {
	case "=", "!=", "-nt", "-ot", "-ef", "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		return true
	case "==":
		return r.sem().TestAcceptsDoubleEqual == Yes
	case "<":
		return r.sem().TestStringOrder == TestStringOrderBoth
	case ">":
		o := r.sem().TestStringOrder
		return o == TestStringOrderBoth || o == TestStringOrderGreaterOnly
	}
	return false
}
