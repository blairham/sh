// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The two subscripted shapes `set -u` reaches by a route of their own: a
// **length** taken of a missing element, and a **list-shaped** subscript on a
// name that holds nothing at all. Runner.checkNounsetElement answers neither,
// and says so — the first is counted before any refusal is reached and the
// second is one of the three shapes that check skips on purpose.
//
// Measured 2026-09-18, `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
// script file under `set -u` with `a=(x y z)`, each row in a subshell:
//
//	                  bash 5.3.20     bash 3.2.57   ksh93u+      zsh 5.9.2
//	${#a[9]}          0 at 0          0 at 0        0 at 0       a[9]: …
//	${#nope[9]}       nope[9]: …      nope: …       nope[9]: …   nope[9]: …
//	${nope[@]}        [] at 0         nope[@]: …    [] at 0      nope[@]: …
//	${nope[*]}        [] at 0         nope[*]: …    [] at 0      nope[*]: …
//	b=(); ${b[@]}     [] at 0         b[@]: …       —            [] at 0
//
// Row two is the first question's control and it is unanimous among the four:
// a length of an element of a name that is **not there** is refused wherever
// arrays are. So what divides them is only the missing *element* of a name
// that is, and that is one axis with zsh on one side.
//
// Row five is the second question's control and it is what makes that
// question about **existence** rather than about emptiness: an array that
// exists and holds no elements is no fields and no complaint in the two
// columns that answer row three with `[]`, and in zsh as well — so zsh's
// refusal of rows three and four is about the name being absent, which is the
// same fact Semantics.UnsetNameAtIsOneEmptyField measures from the other
// side. bash 3.2.57 refuses row five too and is a third reading; it is not a
// preset here, and that column's row is recorded rather than modeled.
//
// Two further rows are measured and not matched, and both are named here
// rather than left to be rediscovered.
//
// `${#nope[@]}` is `nope: …` in both bash columns, `nope[@]: …` in zsh and
// `0` at status 0 in ksh93u+ — three answers for one line, and a subject that
// is neither of the two these checks produce. It keeps ksh93's reading in
// every dialect, which is what the list-shaped skip below leaves standing.
//
// And a **declared table holding nothing** parts bash from ksh93 under a
// length: measured 2026-09-18, `typeset -A t; ${#t[q]}` is `t[q]: unbound
// variable` in bash 5.3.20 and `0` at status 0 in ksh93u+, where the same two
// agree on `typeset -A u; u[k]=v; ${#u[q]}` being `0`. So bash asks whether
// the name holds an **element** and ksh93 asks whether the name is **there**,
// which is a second axis on top of the one above. This takes ksh93's reading
// for both — see Runner.nameHoldsSomething — which leaves bash's empty-table
// row as the one cell that does not match, unchanged from before any of this.

// checkNounsetLength reports a `${#a[i]}` whose subscript named no element,
// where the dialect counts that as an unset parameter.
//
// The count is taken in front of every other refusal — the length branch
// returns before paramSource is reached — so this is the only place the
// question can be put, which is the half #2980 filed rather than modeled.
//
// Three shapes are skipped, and each of them is a place this could have been
// made too wide:
//
//   - a list-shaped subscript. `${#a[@]}` is a *count* rather than a length,
//     and the panel gives it three answers rather than two; see the table
//     above.
//   - a subscript on a name holding one string, where the dialect reads the
//     brackets as characters. A character past the end of a string is empty
//     rather than missing — the same exemption checkNounsetElement makes, and
//     for the same measurement.
//   - an expansion that has already failed. A second sentence about an
//     element nobody could name would be the first one's aftermath.
func (r *Runner) checkNounsetLength(e *syntax.ParamExpr, elems []string) {
	if !r.nounset || elems != nil || e.Index == nil || e.Inner != nil {
		return
	}
	if r.expandErr || r.ctl == controlExit {
		return
	}
	if r.subscriptYieldsAList(e) || r.subscriptReadsCharacters(e) {
		return
	}
	if !r.nameHoldsSomething(e.Name) {
		// The name itself is absent, which every column that has arrays
		// refuses under a length. No axis: row two of the table.
		r.checkNounset(e)
		return
	}
	if r.ask(r.sem().LengthOfAMissingElementIsRefused,
		"`${#a[9]}` of an element that is not there being an unset parameter") {
		r.checkNounset(e)
	}
}

// checkNounsetWholeArray reports a `${nope[@]}` — a list-shaped subscript on a
// name that holds nothing at all — where the dialect counts that as an unset
// parameter.
//
// A question about **existence** and not about emptiness, which is why the
// name is asked about and the elements are not: an array that exists and
// holds none is quiet in every column, row five of the table above.
//
// checkNounsetElement skips every list-shaped subscript on purpose and
// interp/nounsetelement_test.go has the rows that pin the skip, so this moves
// the question rather than deleting that guard: the element check still never
// fires for `[@]`, and this one never fires for anything else.
func (r *Runner) checkNounsetWholeArray(e *syntax.ParamExpr) {
	if !r.nounset || e.Index == nil || e.Inner != nil || e.Indirect {
		return
	}
	if r.expandErr || r.ctl == controlExit {
		return
	}
	if e.Length || !r.wholeArrayIndex(e) {
		return
	}
	if r.nameHoldsSomething(e.Name) {
		return
	}
	if r.ask(r.sem().UnsetNameWithAWholeArraySubscriptIsRefused,
		"`${nope[@]}` on a name holding nothing being an unset parameter") {
		r.checkNounset(e)
	}
}
