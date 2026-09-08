// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `return`'s operand is read four different ways, and the reading it gets is
// the same one `exit` gets — see Semantics.StatusArgument for the panel.
//
// The bug these pin is a silent wrong answer rather than a refusal: the
// operand was discarded and `$?` handed back in its place, so every status-0
// path looked right and a function meaning to return 3 returned whatever ran
// last. That is why every case here asserts the *number* — an assertion that
// the status is merely nonzero passes against the wrong nonzero value, which
// is exactly what `f(){ r=3; false; return r; }` used to produce.
func TestTheStatusOperandIsReadFourWays(t *testing.T) {
	sem := func(p StatusArgumentPolicy) func(*Runner) {
		s := permissive()
		s.StatusArgument = p
		// Not the subject here: a refusal's *fatality* is asked in
		// TestARefusedReturnOperandEndsTheScriptOnlyWhereAFailedSpecialBuiltinDoes
		// below, and leaving it fatal would stop the script before the echo
		// that reports the status.
		s.BadOptionToSpecialBuiltinFatal = No
		return withSem(s)
	}
	for _, tc := range []struct {
		operand                             string
		strict, numeric, digits, arithmetic int
	}{
		// A plain number in range is unanimous and never reaches the axis.
		{"3", 3, 3, 3, 3},
		// Leading zeros are decimal in all four, including the two that read
		// `$((010))` as 8 — so this is not the arithmetic reader.
		{"010", 10, 10, 10, 10},
		// A bare name: the whole point. Arithmetic finds r, the leading-digit
		// reader finds no digits and answers 0, and the two strict readings
		// refuse the word and leave 2.
		{"r", 2, 2, 0, 3},
		// An expression, which a shell that merely looked the name up would
		// still get wrong.
		{"r+1", 2, 2, 0, 4},
		// Parenthesized, so the operand is plainly going through the whole
		// arithmetic reader rather than a hand-rolled sum.
		{"(r+1)*2", 2, 2, 0, 8},
		// Digits then text: 3 where the leading digits are read, refused by
		// the two strict readings, and a math error under arithmetic — which
		// is reported and leaves 0, the same as `exit 3abc` leaves in zsh.
		// This row is the one that parts the leading-digit reading from the
		// arithmetic one; the bare name alone cannot, since both answer it
		// with a lookup that either finds a number or does not.
		{"3abc", 2, 2, 3, 0},
		// 256 exactly, which is where the panel parts and therefore where the
		// unanimous fast path has to stop: one more and it would answer 256
		// for bash and ksh93, which mask it to 0. A mutation run found this —
		// moving the bound to `<= 256` survived every other case here.
		{"256", 256, 0, 0, 256},
		// Over eight bits, which is the only place the mask can be seen.
		{"300", 300, 44, 44, 300},
		// And under zero.
		{"-1", 2, 255, 255, -1},
		// An empty operand is 0 where nothing is refused, and refused where
		// a number is required.
		{"", 2, 2, 0, 0},
		// An expression that evaluates but does not *finish*: ksh93 reads the
		// 1 and stops, arithmetic calls it a math error and leaves 0. This is
		// the row that reaches the evaluator's error path — every other
		// operand above either evaluates cleanly or never gets that far.
		{"1 +", 2, 2, 1, 0},
		// And one that does not even parse, which is the evaluator's *other*
		// error path and a different branch again. Both readings answer 0
		// here, so it is the branch rather than the answer that this row is
		// for; a mutation run reported the parse arm untouched without it.
		{"((", 2, 2, 0, 0},
	} {
		t.Run("operand "+tc.operand, func(t *testing.T) {
			for _, p := range []struct {
				name string
				pol  StatusArgumentPolicy
				want int
			}{
				{"strict", StatusArgStrict, tc.strict},
				{"numeric", StatusArgNumeric, tc.numeric},
				{"leading digits", StatusArgLeadingDigits, tc.digits},
				{"arithmetic", StatusArgArithmetic, tc.arithmetic},
			} {
				src := `f() { r=3; return "` + tc.operand + `"; }` + "\nf\necho \"st=$?\"\n"
				out, _ := run(t, src, sem(p.pol))
				want := "st=" + itoa(p.want)
				if !strings.Contains(out, want) {
					t.Errorf("%s: `return %q` said %q, want %q", p.name, tc.operand, out, want)
				}
			}
		})
	}
}

// The operand is discarded only when there is none, and then it is `$?` —
// which is the answer the whole panel gives and the one the bug produced for
// every operand.
//
// Without this the fix could take `return` with no operand away and nothing
// would notice, because every other case here wants a number instead.
func TestABareReturnStillHandsBackTheLastStatus(t *testing.T) {
	for _, p := range []StatusArgumentPolicy{
		StatusArgStrict, StatusArgNumeric, StatusArgLeadingDigits, StatusArgArithmetic,
	} {
		s := permissive()
		s.StatusArgument = p
		out, _ := run(t, "f() { false; return; }\nf\necho \"st=$?\"\n", withSem(s))
		if !strings.Contains(out, "st=1") {
			t.Errorf("%v: said %q, want st=1", p, out)
		}
	}
}

// `exit` and `return` read the operand identically in every shell in the
// panel, so they read it identically here — through one function rather than
// two that could drift.
//
// This is the guard on the fold. A second reader for `return` is the shape
// that has cost this tree seven bugs, and this exact axis is where it nearly
// happened again: `exit r` sat at 0 under the zsh preset while `return r` was
// being taught to answer 3.
func TestExitAndReturnReadTheSameOperandTheSameWay(t *testing.T) {
	for _, p := range []struct {
		name string
		pol  StatusArgumentPolicy
	}{
		{"strict", StatusArgStrict},
		{"numeric", StatusArgNumeric},
		{"leading digits", StatusArgLeadingDigits},
		{"arithmetic", StatusArgArithmetic},
	} {
		t.Run(p.name, func(t *testing.T) {
			for _, operand := range []string{"3", "r", "r+1", "3abc", "abc", "0x10"} {
				s := permissive()
				s.StatusArgument = p.pol
				s.BadOptionToSpecialBuiltinFatal = No

				// `exit` truncates to eight bits on its way out because a
				// process carries no more, so the comparison is made below
				// 256 on both sides.
				_, exitSt := run(t, `r=3; exit "`+operand+`"`, withSem(s))
				retSrc := `f() { r=3; return "` + operand + `"; }` + "\nf\nexit \"$?\"\n"
				_, retSt := run(t, retSrc, withSem(s))
				if exitSt != retSt {
					t.Errorf("operand %q: exit gave %d, return gave %d — the two readings have parted",
						operand, exitSt, retSt)
				}
			}
		})
	}
}

// The two routes to `return` — a function body and a sourced file — read the
// operand through the same builtin, so a fix on one is a fix on both.
//
// Named rather than assumed: the sourced-file route is where `~/.zi`'s
// plugin loading actually runs, and a fix that reached only a function body
// would leave the real config exactly as broken.
func TestASourcedFileReadsTheOperandLikeAFunctionBody(t *testing.T) {
	s := permissive()
	s.StatusArgument = StatusArgArithmetic
	dir := t.TempDir()
	src := "printf 'r=3\\nreturn r+1\\n' > " + dir + "/s.sh\n" +
		". " + dir + "/s.sh\n" +
		"echo \"st=$?\"\n"
	out, _ := run(t, src, withSem(s))
	if !strings.Contains(out, "st=4") {
		t.Errorf("said %q, want st=4 — the sourced route is not reading the operand", out)
	}
}

// A refused operand still returns from the function: bash skips the rest of
// the body and leaves 2 for the caller, and the script runs the next command.
func TestARefusedReturnOperandStillLeavesTheFunction(t *testing.T) {
	s := permissive()
	s.StatusArgument = StatusArgNumeric
	s.BadOptionToSpecialBuiltinFatal = No
	out, st := run(t, "f() { return abc; echo BODY; }\nf\necho \"st=$?\"\necho alive\n", withSem(s))
	if strings.Contains(out, "BODY") {
		t.Errorf("said %q, want the rest of the body skipped", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want st=2", out)
	}
	if !strings.Contains(out, "alive") {
		t.Errorf("said %q, want the script to carry on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// And where a special builtin's failure is fatal, the script ends there
// instead — which is dash, and bash called as `sh`. The two shells that read
// the operand leniently never reach this at all, because neither refuses any
// word.
func TestARefusedReturnOperandEndsTheScriptOnlyWhereAFailedSpecialBuiltinDoes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fatal Answer
		alive bool
		want  int
	}{
		{"fatal, and the script ends there", Yes, false, 2},
		{"reported, and the script carries on", No, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := permissive()
			s.StatusArgument = StatusArgStrict
			s.BadOptionToSpecialBuiltinFatal = tc.fatal
			out, st := run(t, "f() { return abc; }\nf\necho alive\n", withSem(s))
			if got := strings.Contains(out, "alive"); got != tc.alive {
				t.Errorf("said %q, want alive=%v", out, tc.alive)
			}
			if st != tc.want {
				t.Errorf("status = %d, want %d", st, tc.want)
			}
		})
	}
}

// With no answer recorded the operand is refused rather than guessed, and the
// complaint names `return` — not `exit`, which shares the axis and would send
// a reader to the wrong line.
func TestAnUnansweredStatusAxisNamesTheBuiltinThatAskedIt(t *testing.T) {
	s := PosixSemantics()
	s.StatusArgument = StatusArgUnspecified
	out, _ := run(t, "f() { return abc; }\nf\necho \"st=$?\"\n", withSem(s))
	if !strings.Contains(out, "return") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named against `return`", out)
	}
	if strings.Contains(out, "exit:") {
		t.Errorf("said %q, want `return` named rather than `exit`", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want st=2 — a refused axis must not answer 0", out)
	}
	// And said once. An unanswered axis is already reported by `ask`, so a
	// `return` that then fell through to its own refusal would complain twice
	// about one operand — naming a number as invalid when the real objection
	// is that nobody said how to read it.
	if strings.Contains(out, "invalid number") {
		t.Errorf("said %q, want the unanswered axis alone rather than a second complaint", out)
	}
}
