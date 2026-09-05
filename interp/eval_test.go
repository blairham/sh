// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

func TestArithmetic(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"precedence follows C", `echo $((1+2*3))`, "7\n"},
		{"grouping", `echo $(((1+2)*3))`, "9\n"},
		{"integer division truncates", `echo $((3/2))`, "1\n"},
		{"modulo", `echo $((7%3))`, "1\n"},
		{"comparison yields one or zero", `echo $((1<2)) $((2<1))`, "1 0\n"},
		{"logical yields one, not an operand", `echo $((2 && 3))`, "1\n"},
		{"ternary", `echo $((1?2:3))`, "2\n"},
		{"a bare name is a variable", `x=5; echo $((x*2))`, "10\n"},
		{"unset is zero", `echo $((u+1))`, "1\n"},
		{"shifts", `echo $((1<<3)) $((8>>2))`, "8 2\n"},
		{"bitwise", `echo $((6&3)) $((6|3)) $((6^3))`, "2 7 5\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestArithmeticBasesAreDecidedAtEvaluation(t *testing.T) {
	// The parser kept literals as written precisely so this could decide.
	// Whether a leading zero means octal is a dialect question.
	if got, _ := run(t, `echo $((0x10)) $((010)) $((0100)) $((2#101))`, nil); got != "16 8 64 5\n" {
		t.Errorf("got %q, want 16 8 64 5", got)
	}
}

func TestArithmeticAssignmentOutlivesTheExpression(t *testing.T) {
	if got, _ := run(t, `echo $((x=7)); echo $x`, nil); got != "7\n7\n" {
		t.Errorf("got %q", got)
	}
	if got, _ := run(t, `x=1; echo $((x+=4)); echo $x`, nil); got != "5\n5\n" {
		t.Errorf("compound assignment gave %q", got)
	}
	// ++ and -- differ in what they evaluate to, not in what they do.
	if got, _ := run(t, `x=1; echo $((x++)) $x`, nil); got != "1 2\n" {
		t.Errorf("postfix gave %q, want 1 2", got)
	}
	if got, _ := run(t, `x=1; echo $((++x)) $x`, nil); got != "2 2\n" {
		t.Errorf("prefix gave %q, want 2 2", got)
	}
}

func TestArithmeticShortCircuitIsObservable(t *testing.T) {
	// Assignment is an operator whose effect outlives the expression, so
	// evaluation order is part of the specification rather than a detail.
	if got, _ := run(t, `x=0; echo $((0 && (x=9))) $x`, nil); got != "0 0\n" {
		t.Errorf("&& evaluated its right side, got %q", got)
	}
	if got, _ := run(t, `x=0; echo $((1 || (x=9))) $x`, nil); got != "1 0\n" {
		t.Errorf("|| evaluated its right side, got %q", got)
	}
}

func TestArithmeticErrors(t *testing.T) {
	// Division by zero is a runtime error; the expression parsed.
	out, _ := run(t, `echo $((1/0))`, nil)
	if out == "" || out == "\n" {
		t.Errorf("dividing by zero should say so, got %q", out)
	}
	// A variable holding a name is re-evaluated as an expression, which is
	// what bash and zsh do; dash and ksh93 error instead.
	if got, _ := run(t, `x=abc; echo $((x+1))`, nil); got != "1\n" {
		t.Errorf("got %q, want 1", got)
	}
	// And that recursion is bounded.
	if _, st := run(t, `x=x; echo $((x+1))`, nil); st == -1 {
		t.Error("self-referential value should not hang or crash")
	}
}

func TestArithmeticCommandInvertsTheConvention(t *testing.T) {
	// `(( expr ))` exits 0 when the expression is non-zero.
	if _, st := run(t, `(( 1+1 ))`, nil); st != 0 {
		t.Errorf("status %d, want 0 for a non-zero expression", st)
	}
	if _, st := run(t, `(( 0 ))`, nil); st != 1 {
		t.Errorf("status %d, want 1 for a zero expression", st)
	}
}

func TestConditions(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"unquoted right side is a pattern", `[[ abc == a* ]] && echo m`, "m\n"},
		{"quoted right side is a literal", `[[ abc == "a*" ]] && echo m || echo no`, "no\n"},
		{"inequality", `[[ abc != x* ]] && echo m`, "m\n"},
		{"numeric comparison", `[[ 10 -gt 9 ]] && echo m`, "m\n"},
		// The sharpest trap: > compares strings, so 10 sorts before 9.
		{"string comparison", `[[ 10 > 9 ]] && echo gt || echo lt`, "lt\n"},
		{"regex", `[[ abc =~ ^a.c$ ]] && echo m`, "m\n"},
		{"quoted regex is a literal", `[[ abc =~ "^a.c$" ]] && echo m || echo no`, "no\n"},
		{"unary", `[[ -n x && -z "" ]] && echo m`, "m\n"},
		{"negation", `[[ ! -z x ]] && echo m`, "m\n"},
		{"grouping", `[[ ( -n a || -n b ) && -n c ]] && echo m`, "m\n"},
		// && binds tighter than || inside [[ ]], unlike the command language.
		{"precedence is C's here", `[[ -n a || -n b && -z x ]] && echo t || echo f`, "t\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditionsDoNotSplitOrGlob(t *testing.T) {
	// Every case where the `[` builtin needs its argument quoted and this
	// does not, because the words never become arguments.
	if got, _ := run(t, `x="a b"; [[ $x == "a b" ]] && echo m`, nil); got != "m\n" {
		t.Errorf("field splitting happened inside [[ ]], got %q", got)
	}
	if got, _ := run(t, `[[ -z $u ]] && echo m`, nil); got != "m\n" {
		t.Errorf("an unset variable needed quoting, got %q", got)
	}
}

func TestCommandSubstitution(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"basic", `echo "[$(echo sub)]"`, "[sub]\n"},
		{"trailing newlines are stripped", "echo \"[$(printf 'a\\n\\n')]\"", "[a]\n"},
		{"in an assignment", `x=$(echo v); echo $x`, "v\n"},
		{"nested", `echo "[$(echo "$(echo deep)")]"`, "[deep]\n"},
		{"backticks", "echo \"[`echo t`]\"", "[t]\n"},
		// Unquoted, the result is field-split like any other expansion.
		{"unquoted result is split", `printf "[%s]" $(printf "a b")`, "[a][b]"},
		{"quoted result is not", `printf "[%s]" "$(printf "a b")"`, "[a b]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	// A substitution is a subshell: nothing it assigns escapes.
	if got, _ := run(t, `x=outer; y=$(x=inner; echo $x); echo "$x $y"`, nil); got != "outer inner\n" {
		t.Errorf("a substitution leaked state, got %q", got)
	}
}

func withSem(s Semantics) func(*Runner) { return func(r *Runner) { r.Semantics = &s } }

func TestSemanticsAxesHaveTwoSides(t *testing.T) {
	// Each row is an axis, run with its field answered both ways on the same
	// base. Asserting only one side asserts a default rather than a behavior.
	tests := []struct {
		axis, src string
		set       func(*Semantics, Answer)
		wantYes   string
		wantNo    string
	}{
		{
			"a leading zero means octal",
			`echo $((0100))`,
			func(s *Semantics, a Answer) { s.ArithLeadingZeroIsOctal = a },
			"64\n", "100\n",
		},
		{
			"echo interprets escapes",
			`echo 'a\tb'`,
			func(s *Semantics, a Answer) { s.EchoInterpretsEscapes = a },
			"a\tb\n", "a\\tb\n",
		},
		{
			"${#@} is the count",
			`set -- p q r; echo ${#@}`,
			func(s *Semantics, a Answer) { s.LengthOfSpecialIsCount = a },
			"3\n", "5\n",
		},
		{
			"an unquoted expansion is split",
			`x="a b"; printf "[%s]" $x`,
			func(s *Semantics, a Answer) { s.SplitParamExpansion = a },
			"[a][b]", "[a b]",
		},
		{
			"quoting a regex makes it a literal",
			`[[ abc =~ "^a.c$" ]] && echo m || echo no`,
			func(s *Semantics, a Answer) { s.RegexQuotingMakesLiteral = a },
			"no\n", "m\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.axis, func(t *testing.T) {
			yes := permissive()
			tc.set(&yes, Yes)
			if got, _ := run(t, tc.src, withSem(yes)); got != tc.wantYes {
				t.Errorf("Yes side: got %q, want %q", got, tc.wantYes)
			}
			no := permissive()
			tc.set(&no, No)
			if got, _ := run(t, tc.src, withSem(no)); got != tc.wantNo {
				t.Errorf("No side: got %q, want %q", got, tc.wantNo)
			}
		})
	}
}

func TestHeredocBodies(t *testing.T) {
	// The delimiter's quoting decides whether the body is expanded, which is
	// a property of how it was *written* — the reason the lexer recorded it
	// rather than resolving it.
	expanded := "x=VAL\ncat <<EOF\n[$x]\nEOF\n"
	if got, _ := run(t, expanded, nil); got != "[VAL]\n" {
		t.Errorf("an unquoted delimiter should expand the body, got %q", got)
	}
	literal := "x=VAL\ncat <<'EOF'\n[$x]\nEOF\n"
	if got, _ := run(t, literal, nil); got != "[$x]\n" {
		t.Errorf("a quoted delimiter should not, got %q", got)
	}
	// A here-string is one line and its word is expanded like any other.
	if got, _ := run(t, `x=v; cat <<< "[$x]"`, nil); got != "[v]\n" {
		t.Errorf("here-string gave %q", got)
	}
}

func TestCoreRefusesWhatTheShellsDisagreeAbout(t *testing.T) {
	// The counterpart of syntax.Core(), built the same way: that refuses
	// constructs not every shell has, this refuses behaviors not every shell
	// shares. A script that runs under it depends on nothing contested.
	core := CoreSemantics()
	for _, tc := range []struct{ name, src, axis string }{
		{"octal", `echo $((0100))`, "leading zero"},
		{"splitting", `x="a b"; printf "[%s]" $x`, "splitting"},
		{"quoted regex", `[[ a =~ "x" ]]`, "regex"},
		{"echo escapes", `echo 'a\tb'`, "escapes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(core))
			if st != 2 {
				t.Errorf("status = %d, want 2 — the command must not run", st)
			}
			if !strings.Contains(out, "no dialect was chosen") {
				t.Errorf("the refusal should say what is missing, got %q", out)
			}
			if !strings.Contains(out, tc.axis) {
				t.Errorf("the refusal should name the axis, got %q", out)
			}
		})
	}
}

func TestCoreOnlyRefusesWhenTheInputDependsOnAnAxis(t *testing.T) {
	// What keeps the core usable rather than refusing everything: an axis is
	// consulted only when the answer could change the result.
	core := CoreSemantics()
	for _, tc := range []struct{ src, want string }{
		{`echo hi`, "hi\n"},                // no escapes, so no question about them
		{`echo $((1+2))`, "3\n"},           // no leading zero
		{`x=ab; printf "[%s]" $x`, "[ab]"}, // nothing to split on
		{`printf "[%s]" "a b"`, "[a b]"},   // quoted, so splitting never arises
	} {
		out, st := run(t, tc.src, withSem(core))
		if st != 0 || out != tc.want {
			t.Errorf("%s: got %q status %d, want %q status 0", tc.src, out, st, tc.want)
		}
	}
}

func TestCoreAgreesWithTheShellsWhereTheyAgree(t *testing.T) {
	// Only two axes survive into the core, and that is not a defect of the
	// panel: the axes exist because they diverge, so everything the shells
	// agree about never became one.
	core := CoreSemantics()
	if got, _ := run(t, `printf "[%s]" $(printf "a b")`, withSem(core)); got != "[a][b]" {
		t.Errorf("an unquoted command substitution splits everywhere, got %q", got)
	}
	if got, _ := run(t, `set -- p q r; echo ${#@}`, withSem(core)); got != "3\n" {
		t.Errorf("${#@} is the count in every core shell, got %q", got)
	}
}

func TestArithmeticErrorsFailTheCommand(t *testing.T) {
	// `echo $((1/0))` fails rather than echoing an empty string. Reporting
	// the error and then running the command was the silent wrong answer —
	// the diagnostic went to stderr and the exit status said everything had
	// gone fine.
	//
	// *Which* non-zero status is an axis, not a constant: it is
	// FatalErrorStatusIsOne. Asserting one number would have written one
	// shell's policy into the core, so both sides are asserted here and the
	// core is asserted to refuse.
	one := permissive()
	one.FatalErrorStatusIsOne = Yes
	for _, src := range []string{`echo $((1/0))`, `echo $((08))`, `echo $((1%0))`} {
		for _, tc := range []struct {
			name string
			sem  Semantics
			want int
		}{
			{"status one", one, 1},
			{"posix", PosixSemantics(), 2},
		} {
			out, st := run(t, src, withSem(tc.sem))
			if st != tc.want {
				t.Errorf("%s under %s: status = %d, want %d", src, tc.name, st, tc.want)
			}
			if strings.Contains(out, "\n\n") || out == "\n" {
				t.Errorf("%s under %s: the command ran anyway, got %q", src, tc.name, out)
			}
		}
		// The core has no answer and must say so rather than pick.
		if _, st := run(t, src, withSem(CoreSemantics())); st != 2 {
			t.Errorf("%s under the core: status = %d, want a refusal", src, st)
		}
	}
}
