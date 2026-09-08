// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// substitutingNothing is the grammar these rows need: the flag group, and
// arrays for the rows that ask what a field count is.
func substitutingNothing(d *syntax.Dialect) {
	ordering(d)
	d.NestedParamExpansion = true
	d.ParamAssignAlways = true
}

// A word branch that substituted nothing, which the backslash quoting flag
// alone can see.
//
// Every row is a measurement on zsh 5.9.2 taken 2026-09-08 with `y=""`, run
// through `printf '%s|'` so that the difference between two empty-looking
// answers is a difference in the printed text. Five shells cannot express
// `${(q)…}` at all — bash 5.3, bash 3.2 and bash-as-sh call it a bad
// substitution, dash calls it `Bad substitution` and ksh93 a syntax error —
// so there is nothing here for a dialect flag or a semantics axis to hold
// apart, and the reading is the one shell's or it is nobody's (#1549).
//
// What the rows separate is a value that is empty from a *word branch* that
// came to nothing. A reading that stopped quoting empty values would pass the
// first block below and fail the second, and the reading that was here before
// — quote everything empty as `”` — passes the second and fails the first.
func TestAWordBranchThatSubstitutedNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The bug as reported: nested, with a default, everything empty.
		{"a nested expansion with an empty default", `x=(); y=""; printf "[%s]" "${(q)${x}:-$y}"`, "[]"},
		{"through a subscript", `x=(); y=""; printf "[%s]" "${(q)${x[1]}:-$y}"`, "[]"},
		{"and over a plain scalar", `y=""; printf "[%s]" "${(q)${y}:-$y}"`, "[]"},
		// The nesting is not what does it. This row is the one that says the
		// shape reported was a symptom rather than the rule.
		{"the nesting is not needed", `y=""; printf "[%s]" "${(q)y:-$y}"`, "[]"},
		{"nor is the parameter set", `printf "[%s]" "${(q)nope:-$nope}"`, "[]"},
		{"nor is the colon", `printf "[%s]" "${(q)nope-$nope}"`, "[]"},
		// The alternate operator, whose branch is the other one.
		{"the alternate branch, taken", `y="a"; printf "[%s]" "${(q)y:+$nope}"`, "[]"},
		{"the alternate without a colon", `y="a"; printf "[%s]" "${(q)y+$nope}"`, "[]"},
		{"but not the alternate untaken", `y=""; printf "[%s]" "${(q)y:+$nope}"`, "['']"},

		// An empty value is still `''`, which is the half a general fix
		// breaks. These are the rows that fail if the flag simply stops
		// quoting empties.
		{"an empty value is a pair of quotes", `y=""; printf "[%s]" "${(q)y}"`, "['']"},
		{"an unset one likewise", `printf "[%s]" "${(q)nope}"`, "['']"},
		{"an empty array likewise", `x=(); printf "[%s]" "${(q)x}"`, "['']"},
		{"and a value trimmed to empty", `y=""; printf "[%s]" "${(q)y#*}"`, "['']"},
		// The distinction inside the operator itself: the word has to be
		// written. `:-` with nothing behind it substitutes no word.
		{"a branch with no word written", `y=""; printf "[%s]" "${(q)y:-}"`, "['']"},
		{"the alternate with no word written", `y="a"; printf "[%s]" "${(q)y:+}"`, "['']"},
		// The assigning forms substitute what they stored, not the word.
		{"the assigning form is a value", `printf "[%s]" "${(q)nope:=$nope}"`, "['']"},
		{"and so is the unconditional one", `printf "[%s]" "${(q)nope::=$nope}"`, "['']"},

		// A word that came to something is quoted as that something, which
		// is the row a reading keyed on "the operator fired" fails.
		{"a non-empty word is quoted", `y=""; printf "[%s]" "${(q)y:-a b}"`, `[a\ b]`},
		{"a word that came to one space is not nothing", `y=""; printf "[%s]" "${(q)y:-$nope $nope}"`, `[\ ]`},
		{"nor is one with a word beside it", `y=""; printf "[%s]" "${(q)y:-$nope x}"`, `[\ x]`},
		{"and an untaken branch keeps its value", `y="a"; printf "[%s]" "${(q)${y}:-zz}"`, "[a]"},

		// Only this style. The wrapping styles write their own empty wrapper,
		// which is what says the value reaching them is empty rather than
		// absent — and is the block that catches a fix put one level too high.
		{"qq wraps it anyway", `y=""; printf "[%s]" "${(qq)y:-$y}"`, "['']"},
		{"qqq likewise", `y=""; printf "[%s]" "${(qqq)y:-$y}"`, `[""]`},
		{"qqqq likewise", `y=""; printf "[%s]" "${(qqqq)y:-$y}"`, "[$'']"},
		{"and the minimal style likewise", `y=""; printf "[%s]" "${(q-)y:-$y}"`, "['']"},
		{"as does the extended one", `y=""; printf "[%s]" "${(q+)y:-$y}"`, "['']"},

		// Only inside double quotes. Unquoted the shell writes the same two
		// characters an ordinary empty value gives.
		{"unquoted it is a pair of quotes again", `y=""; v=${(q)y:-$y}; printf "[%s]" "$v"`, "['']"},
		{"quoted it is not", `y=""; v="${(q)y:-$y}"; printf "[%s]" "$v"`, "[]"},

		// The field is still there. This is the question a printed `[]`
		// cannot answer, and the answer is one empty field rather than none.
		{"the field survives", `y=""; set -- "${(q)y:-$y}"; printf "[%s]" "n=$#"`, "[n=1]"},
		{"and it is empty rather than absent", `y=""; set -- "${(@q)y:-$y}"; printf "[%s]" "n=$# len=${#1}"`, "[n=1 len=0]"},

		// The other flags in the group leave it alone, which says the state
		// belongs to the value and not to the flag that happened to be first.
		{"case conversion does not restore it", `y=""; printf "[%s]" "${(Uq)y:-$y}"`, "[]"},
		{"nor does a join", `y=""; printf "[%s]" "${(qj.-.)y:-$y}"`, "[]"},

		// It does not survive being the name half of another expansion,
		// which is where a state stored on the runner rather than beside the
		// value would leak.
		{"an enclosing flag sees an empty value", `y=""; printf "[%s]" "${(q)${y:-$y}}"`, "['']"},
		{"and so does an assignment", `y=""; v="${y:-$y}"; printf "[%s]" "${(q)v}"`, "['']"},
		{"two of them in one word", `y=""; printf "[%s]" "${(q)y:-$y}${(q)y}"`, "['']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, substitutingNothing, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
