// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An operand of `functions -M` that says how many arguments a registration
// takes is refused *before* the rule that a `-Ms` registration takes one, and
// the two rules are two different complaints.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), run `-f`
// with `env -u FPATH` over a script file. `functions -M -s mf sf` is `-M:
// invalid min number of arguments: sf` there and was `-Ms: must take a single
// string argument` here — the arity rule reached for an operand the reference
// never gets that far with (#4791).
//
// The two controls are the ones that make this the order and not either
// sentence. `functions -M -s mf 2` is the arity rule in both, so the wording
// for that rule is right and it is where it is reached from that was wrong;
// and `functions -M mf sf` without the letter is the count complaint in both,
// so that sentence exists here and this route did not reach it.
func TestACountOperandIsRefusedBeforeTheStringArityRule(t *testing.T) {
	for _, c := range []struct{ name, spec, want string }{
		{"the issue's row", "-M -s mf sf", "-M: invalid min number of arguments: sf"},
		{"a negative minimum", "-M -s mf -1", "-M: invalid min number of arguments: -1"},
		{"a minimum that is not a count, with a maximum behind it", "-M -s mf sf 1", "-M: invalid min number of arguments: sf"},
		{"a maximum that is not a count", "-M -s mf 1 sf", "-M: invalid max number of arguments: sf"},
		{"a maximum that is empty", `-M -s mf 1 ""`, "-M: invalid max number of arguments: "},

		// The arity rule itself, which must not move: each of these is a
		// count the reference reads and then refuses for its value.
		{"a control: the arity rule on a minimum", "-M -s mf 2", "-Ms: must take a single string argument"},
		{"a control: the arity rule on a nought", "-M -s mf 0", "-Ms: must take a single string argument"},
		{"a control: the arity rule on a maximum", "-M -s mf 1 2", "-Ms: must take a single string argument"},
		{"a control: an unbounded maximum is still not one", "-M -s mf 1 -1", "-Ms: must take a single string argument"},

		// And the same sentence through the route that always reached it.
		{"a control: no letter, so no arity rule to reach", "-M mf sf", "-M: invalid min number of arguments: sf"},
		{"a control: no letter, a bad maximum", "-M mf 1 sf", "-M: invalid max number of arguments: sf"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(),
				"functions "+c.spec+"\nprint \"st=$?\"\nfunctions -M\n")
			if !strings.Contains(out, c.want) {
				t.Errorf("functions %s: output = %q, want %q", c.spec, out, c.want)
			}
			if !strings.Contains(out, "st=1\n") || st != 0 {
				t.Errorf("functions %s: output = %q, want the registration refused at 1", c.spec, out)
			}
			if strings.Contains(out, "functions -M mf") || strings.Contains(out, "functions -Ms mf") {
				t.Errorf("functions %s: output = %q, want nothing registered", c.spec, out)
			}
		})
	}
}

// The arity rule is applied to each count operand as it is read rather than
// once after both, which two rows tell apart: a maximum below the minimum is
// the maximum's complaint where the minimum itself was acceptable, and the
// arity's where it was not.
//
// Measured the same day: `functions -Ms mf 1 0` is `-M: invalid max number of
// arguments: 0` and `functions -Ms mf 2 1` is `-Ms: must take a single string
// argument`, where a single check at the end would give the same answer to
// both. Without the letter the pair is the maximum's complaint either way,
// which is the control.
func TestTheStringArityRuleIsAppliedPerOperand(t *testing.T) {
	for _, c := range []struct{ spec, want string }{
		{"-M -s mf 1 0", "-M: invalid max number of arguments: 0"},
		{"-M -s mf 2 1", "-Ms: must take a single string argument"},
		{"-M mf 1 0", "-M: invalid max number of arguments: 0"},
		{"-M mf 2 1", "-M: invalid max number of arguments: 1"},
	} {
		out, _ := runZsh(t, t.TempDir(), "functions "+c.spec+"\nprint \"st=$?\"\n")
		if !strings.Contains(out, c.want) {
			t.Errorf("functions %s: output = %q, want %q", c.spec, out, c.want)
		}
	}
}

// And a count operand is read in whichever base its spelling announces, with
// leading blanks skipped and an optional sign, where a spelling that runs out
// before any digit is nought rather than a refusal.
//
// This is not decoration on the rule above: it is what the rule needs to be
// true of, because "refuse an operand that is not a count" is only as right as
// the notion of a count under it. Every row was measured 2026-09-27 on zsh
// 5.9.2 through `functions -M mf <operand>` and the listing it leaves — the
// listing rather than the status, because an operand read as the wrong number
// is accepted either way and only what is registered says which number it was.
func TestACountOperandIsReadInTheBaseItsSpellingAnnounces(t *testing.T) {
	for _, c := range []struct {
		name, operand, listed string
	}{
		{"hexadecimal", "0x10", "functions -M mf 16"},
		{"hexadecimal, capital", "0X10", "functions -M mf 16"},
		{"binary", "0b1", "functions -M mf 1"},
		{"octal by a leading nought", "007", "functions -M mf 7"},
		{"octal, one digit", "01", "functions -M mf 1"},
		{"a bare nought", "0", "functions -M mf 0 0"},
		{"a signed nought", "-0", "functions -M mf 0 0"},
		{"a signed one", "+1", "functions -M mf 1"},
		{"leading blanks", "  0x1f", "functions -M mf 31"},
		{"a leading tab", "\t1", "functions -M mf 1"},
		{"a leading newline", "\n1", "functions -M mf 1"},
		{"nothing at all", "", "functions -M mf 0 0"},
		{"blanks alone", " ", "functions -M mf 0 0"},
		{"a sign alone", "+", "functions -M mf 0 0"},
		{"a base with no digits behind it", "0x", "functions -M mf 0 0"},

		// And the refusals, which are the other half of the same reader:
		// anything left standing after the digits is not a count.
		{"a digit that is not in the base", "08", "-M: invalid min number of arguments: 08"},
		{"letters behind the digits", "1abc", "-M: invalid min number of arguments: 1abc"},
		{"an underscore", "1_0", "-M: invalid min number of arguments: 1_0"},
		{"a trailing blank", " 1 ", "-M: invalid min number of arguments:  1 "},
		{"an expression", "1+1", "-M: invalid min number of arguments: 1+1"},
		{"the shell's own base spelling", "8#10", "-M: invalid min number of arguments: 8#10"},
		{"a fraction", "1.5", "-M: invalid min number of arguments: 1.5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(),
				"functions -M mf "+singleQuoted(c.operand)+"\nfunctions -M\n")
			if !strings.Contains(out, c.listed) {
				t.Errorf("functions -M mf %q: output = %q, want %q", c.operand, out, c.listed)
			}
		})
	}
}
