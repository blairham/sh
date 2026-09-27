// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// braceGroupSyntaxIsFreed reports whether the `(`, `)` and `|` of the value
// being expanded are the syntax of a pattern group rather than four ordinary
// characters, which they are behind a `{` the script wrote unquoted.
//
// Outside this rule the column that reads it globs the result of an expansion
// and refuses to let that result build a **group** — that is
// Semantics.ExpansionResultSuppliesGroupSyntax, and it is answered `No` here.
// Behind a written brace the same three characters go the other way.
//
// Measured 2026-09-27 against /bin/ksh `Version AJM 93u+ 2012-08-01`, each
// case in a directory of its own holding `za`, `ya`, `zb`, `yb`, `zq`, `z@a`,
// `z*a`, `1a`, `2a`, `{z}` and `{z}a`, created by the shell under test and
// confirmed with `ls`, `f` printing its count and its arguments:
//
//	                          ksh93u+                    ours, before
//	g='?(a)'; f z$g (control) 1 | [z?(a)]                same
//	g='?(a)'; f {z,y}$g       2 | [za] [ya]              2 | [z?(a)] [y?(a)]
//	g='+(a)'; f {z,y}$g       2 | [za] [ya]              2 | [z+(a)] [y+(a)]
//	g='!(q)'; f {z,y}$g       6 | [z*a] [z@a] [za] …     2 | [z!(q)] [y!(q)]
//	g='a|b'; f {z,y}@($g)     4 | [za] [zb] [ya] [yb]    2 | [z@(a|b)] …
//	f {z,y}$(printf '?(a)')   2 | [za] [ya]              2 | [z?(a)] [y?(a)]
//
// The control is what says this is the brace and not the value: with no brace
// in the word the same value is text, which is the reading the axis beside
// this one records. `z?(a)` reaches `za`, `z+(a)` reaches `za`, `z!(q)`
// reaches everything under `z` that is not `zq`, and the last row is the bar
// alone in a group whose parentheses the script wrote — so every row has a
// file the live reading hits and the text reading does not.
//
// **The keys are BraceStopsFieldSplitting's**, and a `{z}` that is no list
// does it just the same:
//
//	g='?(a)'; f {z}$g            2 | [{z}] [{z}a]   the character, not a list
//	g='?(a)'; f "{z}"$g          1 | [{z}?(a)]      a quoted brace is exempt
//	g='?(a)'; f \{z\}$g          1 | [{z}?(a)]      and an escaped one
//	b='{'; g='?(a)'; f ${b}z,y}$g
//	                             2 | [z?(a)] [y?(a)]
//	                                                and a produced one, which
//	                                                expands and frees nothing
//	g='?(a)'; f $g{z}            1 | [?(a){z}]      and it is the rest of the
//	                                                word, not the word
//
// `{z}` and `{z}a` are in the directory for the first row and `a{z}` for the
// last, which is what makes those two falsifiable rather than a pattern
// passing through: the first row's answer *is* two files, and the last one's
// live reading would have reached `{z}` and `a{z}`.
//
// **The openers `*` and `@` stay text**, and that is not an exception. A
// pattern group is introduced by one of `? * + @ !` and a bare `(…)` is
// ordinary text, so an opener that is itself marked takes its parentheses
// with it — `*` by Semantics.BraceMakesAProducedStarOrBracketText and `@` by
// the entry beside it in producedPatternLeavesABraceMarks. With `za`, `z*a`
// and `z@a` in the directory, `g='*(a)'; f {z,y}$g` and `g='@(a)'; f
// {z,y}$g` are `[z*(a)] [y*(a)]` and `[z@(a)] [y@(a)]` there, reaching
// neither the `za` a live group would nor the `z*a` and `z@a` a marked
// opener in front of a live group would.
//
// Read rather than asked, like the two rules either side of it: the extent is
// braceGlobStopSpans', which arms it only on the road a vector that answered
// BraceFanExpandsEachNameOnItsOwn takes.
func (r *Runner) braceGroupSyntaxIsFreed() bool {
	return r.braceStopsGlob && r.sem().BraceFreesProducedGroupSyntax == Yes
}
