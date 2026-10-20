// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `C_BASES` decides how the mark in front of a value written in an output base
// is spelled: C's own — `0x6C` — or zsh's `16#6C`.
//
// It was accepted and then ignored until #4502 — the option went into the
// recorded store, the renderer went on writing `16#`, and both states of it
// produced byte-identical output. So every case below is run in **both**
// states, and the expectations differ between them wherever real zsh's do: a
// shell that ignores the option fails one half of a pair rather than passing a
// row that only ever asked it one question.
//
// **The rule is keyed on the mark, not on the base**, and two rows here hold
// the base fixed at sixteen and move something else to say so. `[##16]` writes
// no mark at all and is unmoved by the option; `[#16]` writes one and moves.
// Base eight is the other control and is the one the reduction in #4502 opens
// with: it agrees in both states, so a probe that looked only at octal would
// have concluded the option was already implemented.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`, `setopt cbases` and `unsetopt
// cbases` around each expression.
func TestCBasesSpellsTheBaseMarkTheWayCDoes(t *testing.T) {
	for _, c := range []struct {
		name, src, on, off string
	}{
		// The issue's own reduction.
		{"base sixteen", `print $(( [#16] 108 ))`, "0x6C\n", "16#6C\n"},
		{"a wider value", `print $(( [#16] 255 ))`, "0xFF\n", "16#FF\n"},
		{"one hex digit", `print $(( [#16] 10 ))`, "0xA\n", "16#A\n"},
		// The sign stays outside the mark in both spellings, which is
		// IntegerBaseNegativeIsTwosComplement answering No either way.
		{"a negative", `print $(( [#16] -108 ))`, "-0x6C\n", "-16#6C\n"},
		{"zero", `print $(( [#16] 0 ))`, "0x0\n", "16#0\n"},
		// The base held fixed at sixteen, and the mark taken away: the two
		// states agree, which is what says the option reaches the *mark*.
		{"two hashes at the same base", `print $(( [##16] 108 ))`, "6C\n", "6C\n"},
		{"two hashes, negative", `print $(( [##16] -108 ))`, "-6C\n", "-6C\n"},
		// The controls. Only sixteen has a C literal of its own to borrow.
		{"base eight is the control", `print $(( [#8] 8 ))`, "8#10\n", "8#10\n"},
		{"base eight, zero", `print $(( [#8] 0 ))`, "8#0\n", "8#0\n"},
		{"base two", `print $(( [#2] 5 ))`, "2#101\n", "2#101\n"},
		{"base thirty-six", `print $(( [#36] 108 ))`, "36#30\n", "36#30\n"},
		{"base ten marks nothing either way", `print $(( [#10] 108 ))`, "108\n", "108\n"},
		// Grouping is orthogonal and survives the other spelling.
		{"grouped digits", `print $(( [#16_4] 1048575 ))`, "0xF_FFFF\n", "16#F_FFFF\n"},
		// The integer attribute renders through the same renderer, which is
		// why the two constructs are one: the option moves both by the same
		// amount.
		{"the integer attribute", "typeset -i16 a=108\nprint $a\n", "0x6C\n", "16#6C\n"},
		{"the attribute at base eight", "typeset -i8 b=8\nprint $b\n", "8#10\n", "8#10\n"},
		{"the attribute, negative", "typeset -i16 d=-108\nprint $d\n", "-0x6C\n", "-16#6C\n"},
		// And the listing prints the value in decimal in both states, so the
		// spelling reaches what a read sees and not what `-p` writes.
		{
			"typeset -p is decimal either way",
			"typeset -i16 a=108\ntypeset -p a\n",
			"typeset -i16 a=108\n", "typeset -i16 a=108\n",
		},
		// Reading is unmoved: `0x6C` and `010` are read the same with the
		// option on and off, so this is an output spelling alone.
		{"input is unmoved", `print $(( 0x6C )) $(( 16#6C ))`, "108 108\n", "108 108\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"on", "setopt cbases\n", c.on},
				{"off", "unsetopt cbases\n", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := answersRun(t, state.setopt+c.src)
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// Base eight's C spelling is a leading zero, and it is only C's spelling in a
// shell that *reads* a leading zero as octal — so it is `C_BASES` and
// `OCTAL_ZEROES` together and neither one alone.
//
// This is the grid that keeps the two halves from being collapsed into one
// answer. Three of the four cells write `8#10`; only the corner where both
// options are on writes `010`, and base sixteen moves on `cbases` alone in
// every column.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f`.
func TestTheOctalHalfOfCBasesNeedsOctalZeroesToo(t *testing.T) {
	for _, c := range []struct{ cbases, octal, eight, zero, sixteen string }{
		{"setopt cbases", "setopt octalzeroes", "010", "00", "0x6C"},
		{"setopt cbases", "unsetopt octalzeroes", "8#10", "8#0", "0x6C"},
		{"unsetopt cbases", "setopt octalzeroes", "8#10", "8#0", "16#6C"},
		{"unsetopt cbases", "unsetopt octalzeroes", "8#10", "8#0", "16#6C"},
	} {
		t.Run(c.cbases+" "+c.octal, func(t *testing.T) {
			src := c.cbases + "\n" + c.octal + "\n" +
				`print $(( [#8] 8 )) $(( [#8] 0 )) $(( [#16] 108 ))` + "\n"
			want := c.eight + " " + c.zero + " " + c.sixteen + "\n"
			out, st := answersRun(t, src)
			if out != want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// The state is the semantics vector's rather than a bit in the recorded store,
// which is what makes a subshell's change stay in the subshell.
//
// Measured on zsh 5.9.2, 2026-09-25: the expression inside the subshell is
// C-spelled and the one after it is not.
func TestCBasesIsSubshellLocal(t *testing.T) {
	const src = "(setopt cbases; print $(( [#16] 108 )))\nprint $(( [#16] 108 ))\n"
	const want = "0x6C\n16#6C\n"
	out, st := answersRun(t, src)
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// The option still reports itself on every surface it shows on, which is the
// half that already worked and must not be traded away: moving the state onto
// the axis would be no gain if `[[ -o … ]]` and the listing then described a
// shell that no longer exists.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f` throughout.
func TestCBasesReportsItsState(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"off by default", "[[ -o cbases ]] && print on || print off\n", "off\n"},
		{
			// The namespace's own spellings reach the axis too — the
			// underscored one, and a `no` prefix.
			"the underscored spelling",
			"setopt C_BASES\n[[ -o c_bases ]] && print on || print off\n",
			"on\n",
		},
		{
			"the negated spelling",
			"setopt cbases\nsetopt nocbases\n[[ -o cbases ]] && print on || print off\n",
			"off\n",
		},
		{
			"the options parameter",
			"setopt cbases\nzmodload zsh/parameter\nprint -r -- \"${options[cbases]}\"\n",
			"on\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// An option that defaults off is named in the `setopt` listing once it is on,
// and the listing reads the axis now that the axis is where the state lives.
// Measured on zsh 5.9.2, 2026-09-25: `setopt cbases; setopt` writes `cbases`
// among the deviations.
func TestCBasesIsListedWhenItIsOn(t *testing.T) {
	out, st := answersRun(t, "setopt cbases\nsetopt\n")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if !strings.Contains(out, "cbases\n") {
		t.Errorf("the `setopt` listing is %q; an option turned on is named there", out)
	}
}
