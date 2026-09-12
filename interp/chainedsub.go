// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A chain of subscripts — `${m[k][2]}`, `${a[2,4][1]}` — where each one reads
// what the one before it named.
//
// One grammar in the panel has the construct, and what it means there is one
// rule applied twice rather than a rule of its own: a subscript counts
// *elements* when it is handed a list and *characters* when it is handed one
// string, and a chain only changes where the thing it is handed came from.
// Measured on zsh 5.9.2:
//
//	typeset -A m=(k abc); ${m[k][2]}   b     the key names one value, and
//	                                         the second subscript counts its
//	                                         characters
//	a=(one two three); ${a[1][2]}      n     the same, on an element
//	a=(one two three); ${a[1,3][2]}    two   a range names three elements,
//	                                         and the second counts those
//	a=(one two three); ${a[@][2]}      two   so does `[@]`
//	a=(one two three); ${a[1,3][2][3]} o     and the third counts the
//	                                         characters of what `[2]` named
//
// It is not the nested spelling with the braces left out. `${${a[1,3]}[2]}`
// is `two` unquoted and `n` quoted, because quoting joins what the *inner*
// expansion came to before the subscript is read; `${a[1,3][2]}` is `two`
// either way, because there is no inner expansion for the quotes to join.
// Measured, and the reason this is its own reading rather than a rewrite into
// interp/nestedsub.go's.
//
// `~/.zi/bin/zi.zsh` writes `${ICE[atload][1]}` ten times, in the function
// that sources a plugin: it is asking whether the ice's value begins with a
// `!`, which is how it decides whether the load needs tracking (#1516).

// chainLink is the node for the i-th subscript of a chain: the same parameter,
// that subscript, and the subscripts before it.
//
// Every reading in this package takes a *ParamExpr and asks the node its own
// questions, so a link is handed to them as a node rather than as an index
// into someone else's. Leading is carried so the link is a chain in its own
// right — which is what lets both walks below be one step and a recursion
// instead of a loop that has to re-derive where it is.
func (r *Runner) chainLink(e *syntax.ParamExpr, i int) *syntax.ParamExpr {
	return &syntax.ParamExpr{
		Name:       e.Name,
		Src:        e.Src,
		Index:      e.Leading[i].Index,
		IndexFlags: e.Leading[i].Flags,
		IndexRange: e.Leading[i].Range,
		IndexText:  e.Leading[i].Text,
		Leading:    e.Leading[:i],
	}
}

// chainedSubscriptSource is what the last subscript of a chain reads: the
// values the one before it named, and whether those are a list or one string.
//
// ok is false when the previous link named nothing at all, which makes the
// whole expansion unset — measured, `a=(x y); ${a[9][1]-none}` is `none`
// where `${a[1][2]-none}` is empty, because the element is there and only the
// character is missing.
//
// Nothing named is `nil` and not an empty slice, which is the same
// distinction the route without a chain already reads: a range that fell off
// the end *answered*, with no elements — `${a[9,10]-none}` is empty rather
// than `none` — and a link that answered that way is a source the next
// subscript reads, not a miss. Testing the length instead lost that, and lost
// it silently: every ordinary reading of an empty source comes to nothing
// either way, and only a search over one tells them apart.
func (r *Runner) chainedSubscriptSource(e *syntax.ParamExpr) (subscriptSource, bool) {
	prev := r.chainLink(e, len(e.Leading)-1)
	if r.assocSearchSubscript(prev) {
		// A search over an *association* behind a chain is refused by name.
		// The shell with the grammar does not read the rest of the chain
		// against what the search named at all: measured on zsh 5.9.2,
		// `typeset -A m=(k1 vA k2 vB); ${m[(I)k1][1]}` is `1` and
		// `${m[(I)k1][2]}` is `2` — the subscript itself, whatever the keys
		// and the values are — and `${m[(r)vA][1]}` is the whole of `vA`
		// rather than its first character. Reading the matches as an
		// ordinary source instead answers a plausible key at status 0, which
		// is worse than saying so.
		r.diagf("${%s}: a chain behind a search over an association is not implemented\n", r.paramSubject(e))
		r.expandErr = true
		return subscriptSource{}, false
	}
	elems, ok := r.arraySubscript(prev)
	if !ok || elems == nil {
		return subscriptSource{}, false
	}
	if r.subscriptYieldsAList(prev) {
		return subscriptSource{elems: elems}, true
	}
	// One value, and a subscript on one value counts its characters. The
	// join is for the shape rather than for a case that reaches it: a link
	// that is not a list named exactly one element.
	return subscriptSource{elems: []string{strings.Join(elems, "")}, scalar: true}, true
}

// subscriptAgainst answers one subscript over values already in hand: the
// search a flag group asked for, or the ordinary reading.
//
// It is what a subscript written on something other than a name reads — the
// result of the subscript before it, and the result of a nested expansion —
// so that both reach the same code. arraySubscript is the third caller of the
// same pair and cannot use this one: a *name* is more than its values, and the
// group there has an association's keys to search and an attribute to consult
// before any of them are read.
func (r *Runner) subscriptAgainst(e *syntax.ParamExpr, src subscriptSource) ([]string, bool) {
	if e.IndexFlags != nil {
		search, ok := r.subscriptSearch(e)
		if !ok {
			// A letter the group does not carry, refused by name inside.
			return nil, true
		}
		if search != 0 {
			return r.searchSubscript(e, search, src)
		}
	}
	return r.subscriptOver(e, src)
}
