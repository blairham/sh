// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// parameterIsSet answers `[[ -v name ]]` and `test -v name`: whether the
// parameter that name names is set, which is a question about the parameter
// and never about its value. A name holding the empty string is set.
//
// The two constructs are one function because the three shells that have the
// operator give them the same answer — `x=1; [ -v x ]` and `x=1; [[ -v x ]]`
// agree in bash 5.3, ksh93u+ and zsh 5.9.2, and so does every row below.
// They differ only in how the operand is obtained, which is their callers'
// business.
//
// **The set-ness itself is paramSource's**, the same call `${name+word}`
// makes. That is deliberate rather than convenient: an array element, an
// associative key, and the base an index counts from are all decided there
// already, so `[[ -v 'a[2]' ]]` cannot come to a different answer from
// `${a[2]+s}` — and it is why zsh's `a[0]` is unset here while bash's is set,
// with no knowledge of array bases in this file.
//
// Measured 2026-09-07 across bash 5.3.15, ksh93u+ and zsh 5.9.2, each probe
// under `-c`. Unanimous, and therefore not modeled here at all:
//
//	x=1                 set        x=                  set
//	unset x             unset      never assigned      unset
//	a=(1 2 3); a[1]     set        a[9]                unset
//	h[k] present        set        h[k] absent         unset
//	whole array `a`     set        PATH                set
//	`` (empty name)     unset      `a b`, `x `         unset
//	`@`                 unset      3 past `$#`         unset
//
// One row looked like a fourth disagreement and is not, which is the reason
// it is written down: `typeset x` with no value answers *set* in zsh and
// unset in bash and ksh93. That is not this operator — zsh's `typeset x`
// really assigns the empty string, and `${x+s}` says so in the same three
// columns. Reading it as an axis here would have modeled a declaration rule
// as a set-ness rule, which is the shape of #989.
func (r *Runner) parameterIsSet(name string) (bool, error) {
	base, sub, subscripted := r.subscriptOperand(name)
	if !subscripted {
		base = name
	}
	if !r.isSetNameKind(base) {
		return false, nil
	}
	e := &syntax.ParamExpr{Name: base}
	if subscripted {
		e.Index = literalWord(sub)
	}
	_, set, _ := r.paramSource(e)
	return set, nil
}

// isSetNameKind reports whether this shell lets `-v` ask about a name of this
// kind at all. A name it will not ask about is *unset* rather than an error:
// all three answer `[[ -v 'a b' ]]` with a plain false.
//
// Two classes are where the panel splits, and both are the shell declining to
// treat the name as a parameter rather than looking and finding nothing —
// `${1+s}` and `${?+s}` are `s` in every one of the three, so the parameters
// are there and it is the operator that does not see them.
//
//	           `-v 1`, `-v 0`   `-v ?`, `-v #`, `-v $`, `-v !`, `-v *`, `-v -`
//	bash 5.3   set              unset
//	ksh93u+    unset            unset
//	zsh 5.9.2  set              set
//
// `@` is the exception that keeps the second column from being "the specials":
// it is unset in all three, with positional parameters set and in the shell
// that answers for every other one. It is excluded by being absent from
// isSpecialParamName and by nothing else — an `@` guard stood here as well
// for a while, and the pair was mutually redundant in the way that hides a
// bug rather than guards against one: with the guard present, *adding* `@` to
// the specials set changed no answer and no test could see it. One place, so
// one mutant.
func (r *Runner) isSetNameKind(name string) bool {
	if isPositionalName(name) {
		return r.sem().ParameterIsSetSeesPositionals
	}
	if isSpecialParamName(name) {
		return r.sem().ParameterIsSetSeesSpecials
	}
	// isNameLike rather than a second name test of this file's own. It
	// trims blanks before it looks, which this question does not want — but
	// the trim cannot leak an answer, because the *lookup* below is on the
	// untrimmed name: `[[ -v "x " ]]` passes the shape test and then finds
	// nothing under `x `, which is the unset all three shells report.
	return isNameLike(name)
}

// isPositionalName reports whether a name is a positional parameter or `$0`,
// which is a run of digits and nothing else.
func isPositionalName(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return name != ""
}

// isSpecialParamName reports whether a name is one of the parameters spelled
// as a single punctuation character. `@` is deliberately not here; see
// isSetNameKind.
func isSpecialParamName(name string) bool {
	return len(name) == 1 && strings.IndexByte("?#$!*-", name[0]) >= 0
}

// literalWord is one unquoted literal span as a word, for handing text that
// has already been expanded to something that wants a syntax.Word.
func literalWord(s string) *syntax.Word {
	return &syntax.Word{Spans: []syntax.Span{{Kind: syntax.Literal, Value: s}}}
}
