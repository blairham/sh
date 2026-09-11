// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(t)` expansion flag: what a name *is*, in place of what it holds.
//
// It is the one read that separates "the value changed" from "the name is
// still an array", and a shell without it cannot be asked that question from
// a script — which is exactly the shape that hides bugs. `[[ ${(t)x} ==
// *array* ]]` is how a function checks what it was handed, and a fix whose
// own tests had to fall back to a *listing* said less and said it in a form
// that changes when the printer changes (#1657, #1645).
//
// Measured on zsh 5.9.2, 2026-09-10. The whole of the rule is that the flag
// **replaces the base with the type word** and the rest of the group then
// runs on that word, exactly as it would on a value:
//
//	v=abc;    ${(t)v}      scalar
//	          ${(Ut)v}     SCALAR      the letters act on the word
//	          ${(t)#v}     6           and so does the length
//	          ${(t)v#s}    calar       and the operator
//	          ${(t)v:-D}   scalar      which does not fire: the name is set
//	w=(a b);  ${(t)w}      array
//	          ${(t)w[1]}   a           the subscript reads the *word*,
//	          ${(t)w[2]}   r           character by character —
//	          ${(t)w[1,2]} ar          `array` is what is being indexed
//	          ${(t)w[@]}   array
//	h=arr;    ${(Pt)h}     array       behind a `(P)`: the name it resolved
//	s=hello;  ${(t)${s}}   hello       a nested inner is a value, and a
//	                                   value has no name to describe
//	unset u;  ${(t)u}      ``          an unset name is empty and *unset*:
//	          ${(t)u-D}    D           the colon-less test fires
//	          ${(t)+u}     0
//
// The vocabulary itself is a shell's and this package holds nobody's, so the
// word comes from the dialect — see Runner.SetParameterTypeWord, and
// ParameterAttributes for the facts it is built from.

// typeFlagBase is the words a `(t)` group's base comes to: the type of the
// name the expansion is about, subscripted as a string if it was subscripted.
//
// base and set are what the pipeline had, and they are returned unchanged for
// the one shape that has no name to ask about — a nested expansion standing
// where the name would.
func (r *Runner) typeFlagBase(e *syntax.ParamExpr, indirect *indirectTarget,
	base []string, set bool,
) (words []string, held, ok bool) {
	if e.Inner != nil {
		// A value rather than a name. Measured: `s=hello; ${(t)${s}}` is
		// `hello`, so the flag does nothing here rather than describing the
		// expansion that produced the value.
		return base, set, true
	}
	name := e.Name
	if indirect != nil {
		// `(P)` moved the expansion to a different parameter, and that is the
		// one being described.
		name = indirectName(indirect.text)
	}
	// A name the shell does not have is the empty word, and *unset* with it:
	// measured, `${(t)u}` is empty and `${(t)u-D}` is `D`, so the colon-less
	// test fires. ParameterAttributes answers for a name registered to
	// refuse by name as well as for one holding a value, and both are
	// parameters this shell has.
	var word string
	if a, has := r.ParameterAttributes(name); has {
		word = r.parameterTypeWord(a)
	} else if !isNameLike(name) && name != "" {
		// A *special* parameter — `$@`, `$0`, `$#`, a positional. The shell
		// has these and describes them, and this one does not keep the facts
		// they would be described from: they are not in the table
		// ParameterAttributes reads, and "special" is narrower here than
		// there. Refused by name rather than answered empty, because empty
		// is the word for a name the shell does not have — `${(t)0-D}` would
		// have substituted `D` for a parameter every shell has.
		r.diagf("${%s}: the (t) expansion flag is not implemented for a special parameter\n", e.Src)
		r.expandErr = true
		return nil, false, false
	}
	if e.Index == nil {
		return []string{word}, word != "", true
	}
	// A subscript reads the type *word*, character by character — `${(t)w[2]}`
	// is `r`, the second character of `array`, and not the second element of
	// `w`. The same machinery a subscript on a nested expansion's value uses,
	// because it is the same question: a subscript over one string.
	elems, sok := r.subscriptAgainst(e, subscriptSource{elems: []string{word}, scalar: true})
	if !sok {
		return nil, false, false
	}
	return []string{strings.Join(elems, "")}, word != "", true
}
