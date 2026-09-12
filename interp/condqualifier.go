// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The one word inside `[[ … ]]` that matches against the filesystem.
//
// Nothing in a condition is split or globbed — that is the whole reason
// `[[ -z $u ]]` needs no quotes — with one exception, and the vendor manual
// states it as a rule rather than leaving it to be found: within conditions
// using the `[[` form, a parenthesized `(#q…)` expression at the end of a
// string says that globbing should be performed; the expression may hold
// qualifiers, and `(#q)` on its own is valid too. It does not apply to the
// right-hand side of a pattern-match operator, where the syntax already means
// something else.
//
// Measured on zsh 5.9.2, in a directory holding `g/a.txt` and `g/b.txt`:
//
//	[[ -n g/a.txt(#qN) ]]        true    the file is there
//	[[ -n g/zz(#qN) ]]           false   so the word came to nothing
//	[[ -n g/zz ]]                true    without the group, it is text
//	[[ -n g/zz(#q) ]]            error   `no matches found`, the `N` being
//	                                     what makes a miss empty instead
//	[[ g/a.txt(#qN) == g/a.txt ]]        true, so the *left* side globs
//	[[ -e g/a.txt(#qN) ]]        true    and a file test's operand does
//	[[ -n g/*.txt(#qN) ]]        true
//	[[ g/*.txt(#qN) == 'g/a.txt g/b.txt' ]]  true — two matches come back
//	                                     as one operand joined with a space
//
// And four probes that say the group has to be **written**, at the end, and
// unquoted — each of them true, meaning the word stayed the text it was:
//
//	[[ -n "g/zz(#qN)" ]]         quoted whole
//	[[ -n g/zz"(#qN)" ]]         or only the group
//	V='g/zz(#qN)'; [[ -n $V ]]   held in a parameter
//	[[ -n ${~V} ]]               even with the tilde flag, which makes the
//	                             value a pattern everywhere else
//	[[ -n g/zz(#qN)x ]]          not at the end
//
// The `${~V}` row is the one that settles the shape: the group is read off
// the word as the script wrote it, and not off the text the expansion came
// to. A parameter holding `(#qN)` is a value in this position however live
// its pattern characters are, so this question is asked of the spans.
//
// This is why powerlevel10k's directory segment shortens. Its `prompt_dir`
// asks `[[ -n $dir/${~MARKER}(#qN) ]]` of each component to decide whether
// that component is an *anchor* — a directory holding `.git`, `go.mod` and
// the like — and an anchor is never shortened. A shell that reads the word as
// text answers true for every component, so every component is an anchor,
// and the segment is drawn at full length at every width with no arithmetic
// anywhere having gone wrong (#2119).

// condWordQualifies reports whether a condition operand ends in a glob
// qualifier group written literally, which is what turns filename generation
// on for it.
//
// Gated on the dialect having glob qualifiers at all rather than on a
// semantics axis, because the panel does not disagree about this rule: a
// shell without `(#q…)` reads those six characters as text, which is the same
// answer it gives for the whole construct. An axis would be asking which of
// two readings applies where only one shell has a reading.
//
// The extended-pattern option is read for the same reason fieldQualifiers
// reads it: `(#q…)` is the extended spelling, and with `extendedglob` off it
// is not a qualifier group anywhere else either.
func (r *Runner) condWordQualifies(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	if !r.dialect().GlobQualifiers || !r.MatchOption(ExtendedPatternOperators) {
		return false
	}
	last := w.Spans[len(w.Spans)-1]
	if last.Kind != syntax.Literal || last.Quoting != syntax.Unquoted {
		return false
	}
	_, list, ok := splitGlobQualifiers(last.Value)
	return ok && strings.HasPrefix(list, "#q")
}

// condGlobbedOperand expands such a word and matches it, as one operand.
//
// Through globFields, which is the same pathname expansion an ordinary word
// gets — so a miss is this dialect's miss, fatal or empty or the pattern
// itself, decided in one place rather than in a second copy of the rule here.
// The fields it comes back with are joined with a space, measured above:
// `[[ -n … ]]` is asking about one string, and two matching files are one
// string with a space in it rather than a condition with two operands.
func (r *Runner) condGlobbedOperand(w *syntax.Word) string {
	return strings.Join(r.globFields([]string{r.wordTextGlobMarked(w)}), " ")
}
