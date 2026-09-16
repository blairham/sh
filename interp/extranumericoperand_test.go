// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A word written *behind* the count of `break`, `continue`, `return`, `exit`
// or `shift`, which the panel does three different things with and which this
// shell took as silence in all five (#2298).
//
// One helper for the five builtins, so each row below is the same rule asked
// through a different one: a second copy beside the first is how a fix
// applied to one spelling and not its twin gets written here.
func TestAWordBehindANumericOperandIsOneTooMany(t *testing.T) {
	base := func(p ExtraNumericOperandPolicy) Semantics {
		s := CoreSemantics()
		s.ExtraNumericOperand = p
		s.LoopControlOutsideALoopIsFatal = No
		s.LoopControlPlaceIsJudgedBeforeTheCount = Yes
		s.ReturnOutsideAFunctionIsRefused = Yes
		s.ShiftPastEndFatal = No
		s.ShiftNamesAreArrays = No
		s.ShiftCountIsArithmetic = No
		s.ShiftOptionWords = ShiftOptionWordsNone
		s.BadOptionToSpecialBuiltinFatal = No
		return s
	}
	dg := Diagnostics{
		NumericOperandTooMany:   "%[1]s: one too many",
		NumericArgument:         "%[1]s: %[2]s: not a number",
		LoopControlCount:        "%[1]s: %[2]s: unreadable count",
		ReturnOutsideAFunction:  "return: nowhere to return to",
		LoopControlOutsideALoop: "%[1]s: not in a loop",
	}

	for _, tc := range []struct {
		name string
		p    ExtraNumericOperandPolicy
		src  string
		want string
		why  string
	}{
		{
			"the statement goes with it", ExtraNumericOperandGivesUpTheStatement,
			"set -- a b c\nshift 1 2; echo SAME\necho \"n=$#\"\n",
			"sh: shift: one too many\nn=3\n",
			"the rest of the list is given up, the shift did not happen, and the next line still runs",
		},
		{
			"and its status is the builtin's", ExtraNumericOperandGivesUpTheStatement,
			"set -- a b c\nshift 1 2\necho \"A=$?\"\n",
			"sh: shift: one too many\nA=2\n",
			"2, which is the status bash leaves behind for it",
		},
		{
			"a loop around it stops", ExtraNumericOperandGivesUpTheStatement,
			"set -- a b c\nfor i in 1 2; do shift 1 2; echo IN; done\necho TAIL\n",
			"sh: shift: one too many\nTAIL\n",
			"the give-up unwinds past the loop and is consumed at the statement, so the second pass never happens",
		},
		{
			"refused gives up nothing", ExtraNumericOperandRefused,
			"set -- a b c\nfor i in 1 2; do shift 1 2; echo IN; done\necho \"n=$#\"\n",
			"sh: shift: one too many\nIN\nsh: shift: one too many\nIN\nn=3\n",
			"the complaint is written once per pass and the loop runs to the end of its list, which is zsh's reading",
		},
		{
			"ignored takes the count", ExtraNumericOperandIgnored,
			"set -- a b c\nshift 1 2\necho \"A=$? n=$#\"\n",
			"A=0 n=2\n",
			"nothing is said and the shift happens, which is what ksh93, dash and BusyBox ash do",
		},
		{
			"`break` reaches it", ExtraNumericOperandGivesUpTheStatement,
			"for i in 1 2; do break 1 2; echo IN; done\necho TAIL\n",
			"sh: break: one too many\nTAIL\n",
			"and the loop is not left by a `break` that was refused — it is given up",
		},
		{
			"`continue` reaches it", ExtraNumericOperandGivesUpTheStatement,
			"for i in 1 2; do continue 1 2; echo IN; done\necho TAIL\n",
			"sh: continue: one too many\nTAIL\n",
			"the second of the pair, which a fix written into `break` alone would leave silent",
		},
		{
			"`return` reaches it", ExtraNumericOperandGivesUpTheStatement,
			"f() { return 1 2; echo BODY; }\nf\necho \"A=$?\"\n",
			"sh: return: one too many\nA=2\n",
			"the body after it does not run and the status is the refusal's, not the 1 the operand asked for",
		},
		{
			"`exit` reaches it", ExtraNumericOperandGivesUpTheStatement,
			"exit 1 2\necho TAIL\n",
			"sh: exit: one too many\nTAIL\n",
			"the shell does not leave: an `exit` that was refused is an `exit` that did not happen",
		},
		{
			"a count that will not read speaks first", ExtraNumericOperandGivesUpTheStatement,
			"set -- a b\nshift abc def\necho \"A=$?\"\n",
			"sh: shift: abc: numeric argument required\nA=2\n",
			"the operand is read before this is asked, so the script hears about the number and never about the count of words",
		},
		{
			"the place speaks first where the dialect judges it first", ExtraNumericOperandGivesUpTheStatement,
			"break 1 2\necho \"A=$?\"\n",
			"sh: break: not in a loop\nA=0\n",
			"outside a loop bash writes the place's sentence, which is LoopControlPlaceIsJudgedBeforeTheCount read at a second site",
		},
		{
			"one operand asks nothing", ExtraNumericOperandUnspecified,
			"set -- a b c\nshift 1\necho \"n=$#\"\n",
			"n=2\n",
			"a dialect is only ever asked where a second operand was actually written",
		},
		{
			"unanswered with two is refused by name", ExtraNumericOperandUnspecified,
			"set -- a b c\nshift 1 2\necho \"A=$? n=$#\"\n",
			"sh: a word written behind a `break`, `exit`, `return` or `shift` count: " +
				"the shells disagree here and no dialect was chosen\nA=2 n=3\n",
			"the strict core says which question it has no answer for rather than picking a reading",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := base(tc.p)
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// The refusal wins over both ends of `shift`'s range, which is measured: on
// three positional parameters `shift 5 2` is the count of words and not the
// count that overran, and `shift -2 3` is the same rather than the negative
// count's own sentence.
func TestOneTooManyWinsOverACountOutOfRange(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"past the end", "set -- a b c\nshift 5 2\necho \"A=$? n=$#\"\n"},
		{"below zero", "set -- a b c\nshift -2 3\necho \"A=$? n=$#\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.ExtraNumericOperand = ExtraNumericOperandGivesUpTheStatement
			sem.ShiftPastEndFatal, sem.ShiftNegativeIsOutOfRange = No, Yes
			sem.ShiftOptionWords = ShiftOptionWordsNone
			sem.ShiftNamesAreArrays, sem.ShiftCountIsArithmetic = No, No
			dg := Diagnostics{
				NumericOperandTooMany: "%[1]s: one too many",
				ShiftTooMany:          "shift: %[1]d: overran",
				ShiftNegativeCount:    "shift: %[1]d: below zero",
			}
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			want := "sh: shift: one too many\nA=2 n=3\n"
			if out != want {
				t.Errorf("got %q, want %q — the word count is refused before the number is judged", out, want)
			}
		})
	}
}

// Where `shift`'s operands are the names of arrays to shift, a second word is
// one of those and this is never asked — the two readings would otherwise
// both claim the same word. See Semantics.ShiftNamesAreArrays.
func TestNamedArrayOperandsAreNotOneTooMany(t *testing.T) {
	sem := CoreSemantics()
	sem.ExtraNumericOperand = ExtraNumericOperandRefused
	sem.ShiftNamesAreArrays, sem.ShiftPastEndFatal = Yes, No
	sem.ShiftCountIsArithmetic, sem.ShiftOptionWords = No, ShiftOptionWordsNone
	dg := Diagnostics{NumericOperandTooMany: "%[1]s: one too many"}
	out, _ := run(t, "set -- p q r\nshift 1 nosuch\necho \"A=$? n=$#\"\n",
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	want := "A=0 n=3\n"
	if out != want {
		t.Errorf("got %q, want %q — the second word is a name, so nothing is one too many and the "+
			"positional parameters are left alone", out, want)
	}
}

// The wording has a fallback, so a dialect that answers the axis and writes no
// sentence still says something rather than refusing in silence.
func TestOneTooManyHasASentenceWithoutADialectWording(t *testing.T) {
	sem := CoreSemantics()
	sem.ExtraNumericOperand = ExtraNumericOperandGivesUpTheStatement
	sem.ShiftNamesAreArrays, sem.ShiftCountIsArithmetic = No, No
	sem.ShiftOptionWords = ShiftOptionWordsNone
	out, _ := run(t, "set -- a b c\nshift 1 2\n", func(r *Runner) { r.Semantics = &sem })
	want := "sh: shift: too many arguments\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
