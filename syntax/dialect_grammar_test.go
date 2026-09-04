// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

func mustFail(t *testing.T, src string, d Dialect, why string) {
	t.Helper()
	if _, err := Parse(src, d); err == nil {
		t.Errorf("%s: %q parsed, want a syntax error", why, src)
	}
}

func mustParse(t *testing.T, src string, d Dialect, why string) {
	t.Helper()
	if _, err := Parse(src, d); err != nil {
		t.Errorf("%s: %q: %v", why, src, err)
	}
}

// TestParenAfterAWordIsASyntaxError covers a rule the whole panel agrees on,
// so it is core rather than a dialect flag. Without it `[[ ( -n x ) ]]` in a
// dialect without `[[` ran as an ordinary command with surprising arguments
// instead of failing.
func TestParenAfterAWordIsASyntaxError(t *testing.T) {
	for _, src := range []string{`echo (`, `echo a (b)`, `function f() { echo x; }`} {
		mustFail(t, src, POSIX(), "paren after a word")
	}
	// In command position it still opens a subshell, and a pipeline or a
	// list may still be followed by one.
	for _, src := range []string{`(echo x)`, `echo x | (cat)`, `echo a; (echo b)`, `{ echo x; }`} {
		mustParse(t, src, Core(), "paren in command position")
	}
}

// TestStrayStopWordIsASyntaxError is why `function f { ...; }` fails in a
// dialect without the keyword: the `}` has nothing open, and parsing used to
// stop there quietly, silently discarding the rest of the script.
func TestStrayStopWordIsASyntaxError(t *testing.T) {
	for _, src := range []string{`}`, `echo hi; }`, `done`, `then`, `function f { echo kw; }`} {
		mustFail(t, src, POSIX(), "stray stop word")
	}
	// The same words still terminate the constructs that open them, and are
	// ordinary arguments where no command may begin.
	for _, src := range []string{
		`case a in a) echo x;; esac`,
		`if true; then echo x; fi`,
		`while false; do echo x; done`,
		`{ echo x; }`,
		`echo then done }`,
	} {
		mustParse(t, src, Core(), "stop word in its place")
	}
}

func TestArrayLiteralIsADialectQuestion(t *testing.T) {
	mustFail(t, `a=(x y)`, POSIX(), "dash has no arrays")
	mustParse(t, `a=(x y)`, Core(), "every other panel shell has arrays")
	// Not adjacent, and not a subshell either: dash, bash and zsh all call
	// `a= (echo x)` a syntax error. ksh93 alone accepts it, which is not
	// enough to make it core.
	mustFail(t, `a= (echo x)`, Core(), "a space does not make it a subshell")
}

// TestFunctionFormsAreSeparateFlags is the flag-level half of what the dialect
// packages assert per shell: the keyword and the hybrid are independent, so a
// dialect can have one without the other.
func TestFunctionFormsAreSeparateFlags(t *testing.T) {
	const kw, hybrid = `function f { echo x; }`, `function f() { echo x; }`
	keywordOnly := Core()
	both := Core()
	both.FunctionKeywordParens = true

	mustParse(t, kw, keywordOnly, "keyword alone")
	mustFail(t, hybrid, keywordOnly, "keyword alone")
	mustParse(t, kw, both, "keyword and parens")
	mustParse(t, hybrid, both, "keyword and parens")
	mustFail(t, kw, POSIX(), "neither")
	mustFail(t, hybrid, POSIX(), "neither")
}

// TestIndirectionIsAFlag likewise. Whether `${!x}` then means the name is a
// semantics question and belongs to the interpreter, not here.
func TestIndirectionIsAFlag(t *testing.T) {
	on := Core()
	on.ParamIndirection = true
	mustParse(t, `echo ${!x}`, on, "flag on")
	mustFail(t, `echo ${!x}`, Core(), "flag off")
	mustFail(t, `echo ${!x}`, POSIX(), "flag off")
}

// TestSelectIsADialectConstruct: `select` is a menu loop in three of the four
// shells and an ordinary word in dash, where the `do` that follows it has
// nothing to open. That makes it a grammar flag rather than a semantic one —
// a dialect *adds* it, and nothing about it conflicts.
func TestSelectIsADialectConstruct(t *testing.T) {
	for _, src := range []string{
		`select x in a b; do echo "$x"; done`,
		`select x; do echo "$x"; done`,
		`select x in a; do :; done < f`,
	} {
		mustParse(t, src, Core(), "select in a dialect that has it")
		mustFail(t, src, POSIX(), "select in a dialect that does not")
	}
	// The name is required and has to be a name, the same rule `for` has.
	mustFail(t, `select; do :; done`, Core(), "select with no name")
	mustFail(t, `select 1x in a; do :; done`, Core(), "select with a name that is not one")
}

// TestSelectKeepsTheAbsentListDistinct is the same distinction ForClause
// draws: without `in` the menu is the positional parameters, and with `in` and
// nothing after it there is no menu at all. A nil slice cannot say which.
func TestSelectKeepsTheAbsentListDistinct(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`select x in a b; do :; done`, "select x in(a,b) do[cmd[:]]"},
		{`select x; do :; done`, "select x no-list do[cmd[:]]"},
		{`select x in; do :; done`, "select x in() do[cmd[:]]"},
	} {
		if got := parse(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestArraySubscriptIsADialectConstruct: a subscript is a separate flag from
// the array literal because the two halves are separately reachable — a
// subscript can be written for a variable that was never an array, and dash
// rejects it there too. Without the flag `${a[@]}` on a plain variable read as
// an array of one instead of failing — it parses as a *deferred* bad
// substitution, because dash diagnoses it only when the expansion is reached:
// one inside a branch never taken prints nothing at all.
func TestArraySubscriptIsADialectConstruct(t *testing.T) {
	for _, src := range []string{`echo ${a[0]}`, `echo ${a[@]}`, `echo ${a[*]}`, `a=1; echo ${a[@]}`} {
		mustParse(t, src, Core(), "a subscript in a dialect with arrays")
		mustDefer(t, src, POSIX(), "a subscript in a dialect without them")
	}
}

// mustDefer asserts the source parses with the construct marked for a
// runtime refusal rather than read as something else.
func mustDefer(t *testing.T, src string, d Dialect, what string) {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Errorf("%s: %q failed to parse: %v", what, src, err)
		return
	}
	found := false
	walkParams(f, func(e *ParamExpr) {
		if e.Bad {
			found = true
		}
	})
	if !found {
		t.Errorf("%s: %q parsed without a deferred refusal", what, src)
	}
}

// TestCloseBraceAlwaysReserved is one rule with two visible halves, which is
// why the flag is about the word and not about brace groups: `}` reserved
// wherever a word may stand is what lets a group close with no terminator, and
// it is the same thing that stops `echo }` printing a brace.
func TestCloseBraceAlwaysReserved(t *testing.T) {
	reserved := Core()
	reserved.CloseBraceAlwaysReserved = true

	for _, src := range []string{`{ echo hi }`, `{ echo a; echo b }`, `f() { echo hi }`, `{ echo a } 2>/dev/null`} {
		mustParse(t, src, reserved, "a group closing without a terminator")
		mustFail(t, src, Core(), "the same group where the brace is only an argument")
	}
	// The other half: an argument that is a brace.
	mustParse(t, `echo }`, Core(), "`}` as an ordinary word")
	mustFail(t, `echo }`, reserved, "`}` where it is always reserved")
	// Quoting takes it out of the rule, and so does anything but a word.
	for _, src := range []string{`{ echo "a}" }`, `x=}`, `{ }`, `{ echo a; { echo b } }`} {
		mustParse(t, src, reserved, "a brace that is not a reserved word")
	}
}

// TestABraceGroupNamesTheWordItStoppedOn: a reserved word the group cannot use
// is reported as the word, not as a missing brace. The expectation rides along
// only when the group had something in it, which is the one shell that says
// both and the reason the two cases are separate.
func TestABraceGroupNamesTheWordItStoppedOn(t *testing.T) {
	for _, tc := range []struct{ src, token, expected string }{
		{`{ echo a; do :; done; }`, "do", "}"},
		{`{ echo a; esac; }`, "esac", "}"},
		{`{ fi; }`, "fi", ""},
		{`{ then; }`, "then", ""},
	} {
		_, err := Parse(tc.src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%s: got %v, want a syntax error", tc.src, err)
		}
		if se.Kind != ErrUnexpected {
			t.Errorf("%s: kind %v, want ErrUnexpected", tc.src, se.Kind)
		}
		if se.Token != tc.token {
			t.Errorf("%s: token %q, want %q", tc.src, se.Token, tc.token)
		}
		if se.Expected != tc.expected {
			t.Errorf("%s: expected %q, want %q", tc.src, se.Expected, tc.expected)
		}
	}
}

// TestArithFloatIsAGrammarFlag: whether a float literal exists is a question
// about what parses, so it lives on the dialect and not on the semantics
// vector — where it was also declared, unused, answering the same question
// twice. The interpreter reads the same flag for the half the parser cannot
// answer: a float arriving in a variable.
func TestArithFloatIsAGrammarFlag(t *testing.T) {
	float := Core()
	float.ArithFloat = true
	for _, src := range []string{`echo $((1.5))`, `echo $((.5))`, `echo $((1.5e2))`, `echo $((3.0/2))`} {
		mustParse(t, src, float, "a float literal where the dialect has them")
		mustFail(t, src, Core(), "a float literal where it does not")
	}
	// `1e-3` is the exception, and it is why the corpus case for exponents is
	// written with a point. It parses either way: the digit reader accepts
	// `e` as a hex digit, so without floats the text is `1e` minus `3` — a
	// perfectly good expression whose left operand is not a number, which
	// fails when it is evaluated rather than when it is read.
	mustParse(t, `echo $((1e-3))`, float, "an exponent where the dialect has floats")
	mustParse(t, `echo $((1e-3))`, Core(), "the same text read as a subtraction")
	// Integers parse either way, and a based literal is an integer whose
	// digits may include an `e` — reading `0x1e` as an exponent would make it
	// a different number.
	for _, src := range []string{`echo $((3/2))`, `echo $((0x1e))`, `echo $((16#ff))`} {
		mustParse(t, src, float, "an integer where the dialect has floats")
		mustParse(t, src, Core(), "an integer where it does not")
	}
}

// TestLeftoverTextIsBlamedForWhatItCouldHaveBeen: the two ways an expression
// can fail to use all its text. One shell words them differently, so the
// parser has to say which it was — it is a question about what the text could
// have been, which is grammar rather than wording.
func TestLeftoverTextIsBlamedForWhatItCouldHaveBeen(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind ErrorKind
	}{
		// An operand standing where an operator belonged.
		{`echo $((1 2))`, ErrArithOperator},
		{`echo $((1 x))`, ErrArithOperator},
		// Text that could be neither.
		{`echo $((1 @))`, ErrArithBadOperator},
		{`echo $((1.5))`, ErrArithBadOperator},
		// Nothing at all where a value belonged, which is a third thing.
		{`echo $((.5))`, ErrArithOperand},
		{`echo $((1 +))`, ErrArithOperand},
	} {
		_, err := Parse(tc.src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%s: got %v, want a syntax error", tc.src, err)
		}
		if se.Kind != tc.kind {
			t.Errorf("%s: kind %v, want %v", tc.src, se.Kind, tc.kind)
		}
	}
}

// TestPatternGroupsBelongToTheWord: a `(` inside a pattern is part of the word
// rather than the end of it, which the lexer has to decide before any parser
// sees a token. Three flags, because the three shells that allow one do not
// allow the same one.
func TestPatternGroupsBelongToTheWord(t *testing.T) {
	ext, alt := Core(), Core()
	ext.ExtendedPattern = true
	alt.PatternAlternation = true

	for _, src := range []string{`case x in @(a|b)) :;; esac`, `case x in ?(a)) :;; esac`, `case x in !(a)) :;; esac`} {
		mustParse(t, src, ext, "a quantified group where the dialect has them")
		mustFail(t, src, Core(), "a quantified group where it does not")
	}
	// A bare group is a different flag, and the quantified one does not
	// imply it.
	mustParse(t, `case x in a(b|c)) :;; esac`, alt, "a bare group where the dialect has them")
	mustFail(t, `case x in a(b|c)) :;; esac`, ext, "a bare group under the quantified flag")
	mustFail(t, `case x in a(b|c)) :;; esac`, Core(), "a bare group where there are none")

	// Two things a bare group must not swallow, both measured against the one
	// shell that has them: an empty `()` is a function definition, and a `(`
	// straight after `=` is an array literal.
	for _, src := range []string{`f() { echo hi; }`, `a=(x y)`, `f() { a=(x y); }`} {
		mustParse(t, src, alt, "not a group")
		mustParse(t, src, Core(), "not a group anywhere")
	}
}

// TestQuantifiedGroupsMayBeConditionOnly: where a group is allowed is a
// separate question from whether the shell has one. bash reads them inside
// `[[ ]]` and calls the same text a syntax error in a `case` pattern.
func TestQuantifiedGroupsMayBeConditionOnly(t *testing.T) {
	cond := Core()
	cond.ExtendedPatternInCondition = true

	mustParse(t, `[[ abc == @(abc|xyz) ]]`, cond, "a group inside a condition")
	mustFail(t, `case abc in @(abc|xyz)) :;; esac`, cond, "the same group in a case pattern")
	// And the flag that allows them everywhere allows them in a condition too.
	both := Core()
	both.ExtendedPattern = true
	mustParse(t, `[[ abc == @(abc|xyz) ]]`, both, "a group inside a condition")
	mustParse(t, `case abc in @(abc|xyz)) :;; esac`, both, "and in a case pattern")
	// The condition's rules end with the condition.
	mustFail(t, `[[ a == b ]]; case abc in @(abc|xyz)) :;; esac`, cond, "after the condition has closed")
}

// TestAnArithmeticSubscriptNeedsTheSameFlag: whether `a[i]` is a subscript is
// one question, asked in the two places that need it — `${a[i]}` and inside an
// expression. A dialect with no arrays has neither.
func TestAnArithmeticSubscriptNeedsTheSameFlag(t *testing.T) {
	for _, src := range []string{`echo $(( a[0] ))`, `echo $(( a[i+1] ))`, `echo $(( a[0] = 1 ))`} {
		mustParse(t, src, Core(), "a subscript where the dialect has them")
		mustFail(t, src, POSIX(), "a subscript where it does not")
	}
	// `(( … ))` is not the way to show it: a dialect without the arithmetic
	// command reads that as two nested subshells running a command called
	// `a[0]`, which parses perfectly well and means something else entirely.
	mustParse(t, `(( a[0] = 1 ))`, POSIX(), "nested subshells")
	// Without the brackets it parses either way, which is what makes the
	// flag about subscripts rather than about arithmetic.
	mustParse(t, `echo $(( a ))`, POSIX(), "a plain name")
}
