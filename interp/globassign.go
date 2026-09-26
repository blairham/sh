// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A plain scalar assignment whose right-hand side is a pattern, for the one
// shell that has a switch for it — `GLOB_ASSIGN`, and
// [Semantics.ScalarAssignmentValueIsGlobbed] is the axis it moves.
//
// **The subject is the assignment and not the value**, which is the whole of
// why this is a place in the store rather than a flag on the word pipeline.
// Measured on zsh 5.9.2 under `-f`, 2026-09-26, in a directory holding
// `a.txt b.txt c.txt one.only plain` and a `dir`: `a=*.txt` comes back
// `typeset -a a=( a.txt b.txt c.txt )` and `typeset a=*.txt` comes back
// `typeset a='*.txt'` — same value, same directory, same option. So the rule
// is written where the *statement* form is performed, and a declaration
// utility's operand never reaches it: `typeset`, `declare`, `export`,
// `local` and `readonly` each expand their own word through
// Runner.expandAssignArg and store what it came to.
//
// Three things are deliberately *not* here, because they are not this
// question:
//
//   - what makes a word a pattern at all. A written metacharacter counts, a
//     quoted one does not, and one a value carries counts only where it was
//     asked to — so `a='*.txt'` and `v='*.txt'; a=$v` keep their characters
//     under the option, and `a=${~v}` and the same `a=$v` under `setopt
//     globsubst` match. That difference is carried by the glob marks, which
//     is why the value is expanded with them left in.
//   - what a miss means. `NOMATCH`, `nullglob` and a `(N)` qualifier decide
//     it exactly as they do for any other pattern: the default reports `no
//     matches found` and nothing is stored, `nullglob` leaves the name an
//     empty scalar, and `unsetopt nomatch` leaves the characters standing.
//   - `a=(*.txt)`. An array literal's elements are ordinary words and glob
//     in **either** state, which is the control that says the option is
//     about assignments rather than about assignment-shaped syntax.

// scalarAssignmentGlobs reports whether a plain scalar assignment's value is
// read as a pattern.
//
// Compared against Yes rather than asked through Runner.ask, the arrangement
// Runner.rcExpandOn gives its reasons for: every shell in the panel answers
// No and the disagreement exists only inside the one that has the option, so
// a core with no dialect chosen has no conflict to be told about — and asking
// here would put the question to every `x=1` in a shell that has not answered
// it.
func (r *Runner) scalarAssignmentGlobs() bool {
	return r.sem().ScalarAssignmentValueIsGlobbed == Yes
}

// scalarAssignValue is a plain assignment's right-hand side, expanded once,
// in both readings the store may need: the text, and — where the option is on
// and the value turned out to be a pattern — the fields it matched.
//
// It hands back whatever the caller expanded already, for the reason
// Runner.assignValue gives: expanding a second time runs the second time's
// side effects (#1915). The fields travel with the value for the same reason,
// which is why expandedAssign carries them.
func (r *Runner) scalarAssignValue(a *syntax.Assign) (string, []string, bool) {
	if r.expanded != nil && r.expanded.assign == a {
		return r.expanded.value, r.expanded.fields, r.expanded.fieldsSet
	}
	return r.expandScalarAssignValue(a.Value)
}

// expandScalarAssignValue performs that one expansion.
//
// The ordinary path is untouched where the option is off, which is every
// other dialect and this one by default: the word goes through
// Runner.expandAssignValue exactly as it did before, marks removed a span at
// a time. Only the shell that asked for the reading takes the marked road.
func (r *Runner) expandScalarAssignValue(w *syntax.Word) (string, []string, bool) {
	if w == nil || !r.scalarAssignmentGlobs() {
		return r.expandAssignValue(w), nil, false
	}
	marked := r.expandAssignValueMarked(w)
	if r.noglob || r.globSuspended || !r.describesRatherThanSpells(marked) {
		// `unsetopt glob` is the first of these and is measured: `setopt
		// globassign; unsetopt glob; a=*.txt` keeps the six characters. The
		// last is the control the whole rule turns on — a value with no
		// pattern in it is stored as itself, so `integer a; a=2+2` is still
		// `typeset -i a=4` and `integer a; a=plain` is still `typeset -i
		// a=0`.
		return globUnescape(marked), nil, false
	}
	fields := r.globFields([]string{marked})
	return strings.Join(fields, " "), fields, true
}

// storeGlobbedScalarAssign puts the match where the name is.
//
// **How many names matched decides the kind, and the numeric attributes go
// whichever it is.** Measured: `a=one.*` is `typeset a=one.only`, `a=*.txt`
// is `typeset -a a=( a.txt b.txt c.txt )`, and prefixing either with
// `integer a` changes neither the value nor the kind — the `-i` is simply
// gone. A single match is a *scalar* and not a one-element array, which is
// the row an implementation that rewrote the assignment as `a=( … )` gets
// wrong.
//
// The append is not an append. `a=x; a+=one.*` is `typeset a=one.only` and
// `a=(q w); a+=*.txt` is the three matches alone, so a match replaces the
// name whichever operator was written — which is why a.Append is not
// consulted here. The joining `+=` is still the one below in Runner.assign,
// reached whenever the value was not a pattern: `a=x; a+=plain` is `xplain`
// in both states.
func (r *Runner) storeGlobbedScalarAssign(a *syntax.Assign, fields []string) {
	if r.expandErr || r.ctl == controlExit || r.unspecified {
		// A miss the shell refused — `no matches found`. **The name is
		// gone**, not left holding what it held: measured on zsh 5.9.2,
		// 2026-09-26, `setopt globassign; a=kept; eval 'a=*.nomatch'` leaves
		// `${a-UNSET}` as `UNSET` and `typeset -p a` as `no such variable`,
		// and `integer a=5` in front of the same eval loses the `-i` with
		// the value. An `eval` is what makes the row observable at all,
		// since the refusal ends the script it is written in.
		//
		// Keyed on this road and not on a failed expansion in general: the
		// same directory and the same miss with the option **off** stores
		// `*.nomatch`, and `a=kept; eval 'a=${q?bad}'` is a different shape
		// that never reaches here. So the removal belongs to the glob's
		// refusal rather than to assignment.
		r.unsetName(a.Name)
		return
	}
	r.clearTypeAttributes(a.Name)
	if len(fields) > 1 {
		r.setArray(a.Name, fields)
		return
	}
	value := ""
	if len(fields) == 1 {
		value = fields[0]
	}
	r.setVarAs(a.Name, value, assignedAlone)
	r.markForAllexport(a.Name)
}

// globbedElementValue is the right-hand side of a store that **names a
// subscript**, expanded once, with the match where the option asked for one.
//
// `GLOB_ASSIGN` reaches a subscripted assignment as well as the statement form
// #4638 built, and there it is a *splice* rather than a store: measured on zsh
// 5.9.2 (`-f -c`, 2026-09-26) in a directory holding `a.txt b.txt c.txt
// one.only`, with the option on throughout,
//
//	a=(q w e); a[2]=*.txt    typeset -a a=( q a.txt b.txt c.txt e )
//	a=(q w e); a[2]=one.*    typeset -a a=( q one.only e )
//	a=(q w e); a[2]=plain    typeset -a a=( q plain e )
//
// So one subscript goes in and three elements come out, and a single match is
// the road where the splice and a plain store coincide. The controls are the
// third row and the same first row with the option off, which is `( q '*.txt'
// e )` in the reference and here.
//
// **It is expanded here and nowhere else on the road below**, which is what
// the second result is for: `zero` is an expansion this function did not
// perform, so the caller falls back to Runner.assignValue and nothing is
// expanded twice (#1915). With the option off nothing here runs at all, so no
// other dialect's ordering moves.
type globbedValue struct {
	value  string
	fields []string
	// list says the match came to more than one field, which is the splice.
	list bool
	// read says this value has been expanded already.
	read bool
}

func (r *Runner) globbedElementValue(a *syntax.Assign) globbedValue {
	if a.Value == nil || !r.scalarAssignmentGlobs() {
		return globbedValue{}
	}
	value, fields, globbed := r.expandScalarAssignValue(a.Value)
	return globbedValue{value: value, fields: fields, list: globbed && len(fields) > 1, read: true}
}

// elementValue is that value, or the ordinary expansion where this road did
// not take one.
func (r *Runner) elementValue(a *syntax.Assign, g globbedValue) string {
	if g.read {
		return g.value
	}
	return r.assignValue(a)
}

// globbedElementIsAList performs the splice where the match is a list, and
// reports whether it did.
//
// A list has no place to go in a name that holds one value per position, and
// the reference refuses rather than storing anything: measured the same day,
//
//	typeset -A h; h=(k v); h[k]=*.txt   h: attempt to set slice of associative
//	                                    array, status 1
//	v=abcdef; v[2]=*.txt                v: attempt to assign array value to
//	                                    non-array, status 1
//	typeset -A h; h=(k v); h[k]=one.*   [k]=one.only, taken
//	v=abcdef; v[2]=one.*                aone.onlycdef, taken
//
// The last two are what keep the refusal keyed on the **list** rather than on
// the option or on the pattern: one match is one value and every target can
// hold it. Both sentences are the ones Runner.spliceTargetIsAnArray already
// writes for the literal spelling of the same shape — `h[k]=(p q)` and
// `v[2]=(p q)` — which is why this goes through it rather than wording them
// again.
func (r *Runner) globbedElementIsAList(a *syntax.Assign, text string, g globbedValue) bool {
	if !g.list {
		return false
	}
	r.spliceWordsIntoElement(a, text, func() ([]string, bool) { return g.fields, true })
	return true
}
