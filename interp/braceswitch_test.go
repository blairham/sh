// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether `-B` is the short spelling of `braceexpand` (#1856).
//
// Two shells in the panel say yes. A third has the letter and means the
// terminal bell by it, which is why a no here falls through to the dialect's
// own refusal rather than to a silent no-op — a letter accepted and ignored
// would let a script believe it had asked for something. The fourth has no
// braces and no letter.
//
// The long name is deliberately in the table and deliberately not the axis:
// `set +o braceexpand` means the same in every shell that declares the name,
// which is what makes it the spelling a portable script writes.
func TestSetBTurnsOffBraceExpansionIsAnAxis(t *testing.T) {
	answer := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.SetBTurnsOffBraceExpansion = a
			r.Semantics = &s
			r.AddSetOptions("braceexpand")
		}
	}
	for _, tc := range []struct {
		name string
		a    Answer
		src  string
		want string
	}{
		{"the letter stops them", Yes, `set +B; echo {a,b}`, "{a,b}"},
		{"and puts them back", Yes, `set +B; set -B; echo {a,b}`, "a b"},
		{
			// The refusal is the dialect's, and the letter reaching it is
			// the point: a shell that means something else by `-B` has to
			// go on saying so. The second line is this base vector leaving
			// the fatality of a refused letter unanswered, which is a
			// question about refusals rather than about this letter.
			"a shell that means something else by the letter refuses it",
			No, `set +B; echo {a,b}`,
			"sh: set: +B: invalid option\n" +
				"sh: a refused `set` option letter ending the script: " +
				"the shells disagree here and no dialect was chosen\na b",
		},
		{"the long name is not the axis", Yes, `set +o braceexpand; echo {a,b}`, "{a,b}"},
		{"either way round", No, `set +o braceexpand; echo {a,b}`, "{a,b}"},
		{"and unanswered", Unspecified, `set +o braceexpand; echo {a,b}`, "{a,b}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, answer(tc.a))
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// `$-` loses the letter while the option is off, which is the reporting half
// of the same switch.
//
// A startup letter is the one kind `$-` cannot derive from a field — the
// option is on before any script runs, so the dialect's string says so — and
// that is exactly why turning it off has to take the letter back out.
// Measured on bash 5.3.15, 2026-09-11: `set +B; echo $-` answers `hc`.
func TestTheStartupLetterGoesWhenTheOptionDoes(t *testing.T) {
	setup := func(r *Runner) {
		s := *r.Semantics
		s.SetBTurnsOffBraceExpansion = Yes
		s.DefaultOptionLetters = "hB"
		r.Semantics = &s
		r.AddSetOptions("braceexpand")
	}
	for _, tc := range []struct{ name, src, want string }{
		{"on at startup", `echo $-`, "hB"},
		{"gone once it is off", `set +B; echo $-`, "h"},
		{"and back once it is on", `set +B; set -B; echo $-`, "hB"},
		// The long name writes the same state, so it moves the letter too:
		// two spellings of one question cannot report differently.
		{"through the long name as well", `set +o braceexpand; echo $-`, "h"},
		// A letter this runner holds no state for is passed through
		// untouched, which is the honest answer for one we only record.
		{"a letter with nothing behind it stays", `set +o histexpand; echo $-`, "hB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				setup(r)
				r.AddSetOptions("histexpand")
			})
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
