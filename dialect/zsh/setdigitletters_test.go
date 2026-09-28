// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The twelve letters this shell's `set` table had no row for: the ten digits,
// `f` and `t` (#5057).
//
// Every one was refused — `no such option` through `[[ -o <letter> ]]` and
// `bad option` through `set` — while `set -o <name>` already took and moved
// the option behind it. A letter has **three** readers and all three were
// blind: the condition, `set -<l>`/`set +<l>`, and `$-`.
//
// **The mapping was found by a probe that does not ask `[[ -o ]]` at all.**
// The issue's own probe toggled each option and read the letter back through
// `[[ -o ]]`; it resolved nine of the twelve and went silent on `3`, `6` and
// `t` — a silence about *which* option, not about whether the letter was
// real. Toggling every name in `$options` and watching which letter of `$-`
// moves answers all three: `nomatch` moves `3`, `bgnice` moves `6`, and `t`
// is `singlecommand`, which `$-` carries when the shell is started `-t`.
//
// Measured 2026-09-28 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version
// -m` *not a Go executable*, from script files under `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`.
func TestTheTwelveSetLettersThisShellHadNoRowFor(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The nine positives: the letter turns the option on and `$-` grows.
		{"correct", `set -0; [[ -o correct ]]; echo "o=$? [$-]"`, "o=0 [0569X]"},
		{"printexitvalue", `set -1; [[ -o printexitvalue ]]; echo "o=$? [$-]"`, "o=0 [1569X]"},
		{"globdots", `set -4; [[ -o globdots ]]; echo "o=$? [$-]"`, "o=0 [4569X]"},
		{"ignoreeof", `set -7; [[ -o ignoreeof ]]; echo "o=$? [$-]"`, "o=0 [5679X]"},
		{"markdirs", `set -8; [[ -o markdirs ]]; echo "o=$? [$-]"`, "o=0 [5689X]"},
		// **Three name a negative**, exactly as `-F` names `noglob`. With the
		// option on — which is its default — the letter is *off*, and turning
		// the option off turns the letter on. A row written the other way
		// round would pass on the `set -2` line and fail here.
		{"nobadpattern is off while badpattern is on", `[[ -o 2 ]]; echo "o=$?"`, "o=1"},
		{"and on once it is off", `unsetopt badpattern; [[ -o 2 ]]; echo "o=$? [$-]"`, "o=0 [2569X]"},
		{"nonomatch the same", `unsetopt nomatch; [[ -o 3 ]]; echo "o=$? [$-]"`, "o=0 [3569X]"},
		{
			"and norcs, which is why `zsh -f` answers `[[ -o f ]]` with 0",
			`set -f; [[ -o rcs ]]; echo "o=$? [$-]"`, "o=1 [569Xf]",
		},
		// **The condition asked of the letter itself**, which is a different
		// reader from the two rows either side of it. Deleting the `f` row
		// from the table killed nothing until this row existed: `set -f` is a
		// letter the substrate already takes and `$-` already had `f` through
		// Semantics.SetFLetterOption, so both of those went on agreeing with
		// the reference while `[[ -o f ]]` was refused.
		{"and the condition asks the letter too", `set -f; [[ -o f ]]; echo "o=$?"`, "o=0"},
		{"which is refused by nobody now", `[[ -o f ]]; echo "o=$?"`, "o=1"},
		// The three that are on at startup, where the letter has to be taken
		// *away*. These are the rows `$-` could not answer before: the string
		// carried `569` unconditionally, so no script could shift them.
		{"notify withdraws", `unsetopt notify; [[ -o 5 ]]; echo "o=$? [$-]"`, "o=1 [69X]"},
		{"bgnice withdraws", `unsetopt bgnice; [[ -o 6 ]]; echo "o=$? [$-]"`, "o=1 [59X]"},
		{"autolist withdraws", `unsetopt autolist; [[ -o 9 ]]; echo "o=$? [$-]"`, "o=1 [56X]"},
		// And the name brings the letter, which is what makes a letter the
		// option's state rather than a note that the letter was typed.
		{"the name brings the digit", `setopt correct; echo "[$-]"`, "[0569X]"},
		{"and `set +` takes it back", `set -0; set +0; [[ -o correct ]]; echo "o=$? [$-]"`, "o=1 [569X]"},
		// `singlecommand`: the condition answers instead of refusing, and the
		// letter still refuses to *move*, which is a different sentence from
		// a letter nobody has. Both halves are the measurement.
		{"singlecommand answers", `[[ -o t ]]; echo "o=$?"`, "o=1"},
		{"and still refuses to move", `set -t 2>&1; echo "st=$?"`, "zsh:set:1: can't change option: -t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if out != tc.want+"\n" {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
			}
		})
	}
}

// All ten digits lead `$-`, in order, ahead of every letter.
//
// The order string named `569` and left the other seven digits to keep their
// produced place, which put them *behind* the capitals. Measured on one
// binary, and the third row is what settles it — every digit at once.
func TestEveryDigitLeadsDollarDash(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -8 -0 -3 -T -y -1; echo "[$-]"`, "[0135689TXy]"},
		{`set -9 +9 -7 -2 -B -w -4; echo "[$-]"`, "[24567BXw]"},
		{`set -0 -1 -2 -3 -4 -7 -8; echo "[$-]"`, "[0123456789X]"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if out != tc.want+"\n" {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
		}
	}
}
