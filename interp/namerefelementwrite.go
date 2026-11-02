// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// An element written through a reference that is itself aimed at an element:
// `typeset -A a; typeset -n e='a[x]'; e[k]=1`.
//
// One column takes it, and what it builds is what the two subscripts build
// written side by side. Measured 2026-11-02 on ksh93u+ 2012-08-01
// (`/bin/ksh`), from `-c` under `env -i PATH=/usr/bin:/bin` (#7113):
//
//	typeset -A a; typeset -n e='a[x]'
//	  e[k]=1                     0, `typeset -A a=([x]=(1) )`, `+ a[x][0]=1`
//	  e[1]=1                     `typeset -A a=([x]=([1]=1) )`
//	  e[k]=1; e[k]+=2; e[1]+=z   `typeset -A a=([x]=(12 z) )`
//	  typeset -n f=e; f[2]=1     `typeset -A a=([x]=([2]=1) )`
//	  e=1                        `typeset -A a=([x]=1)`, the control: no
//	                             subscript of the script's own
//	typeset -a a; typeset -n e='a[1]'; e[2]=z
//	                             `typeset -a a=([1]=([2]=z) )`
//	a=(p q r); typeset -n e='a[1]'; e[2]=z
//	                             `typeset -a a=(p ([0]=q [2]=z) r)`
//	typeset -A a; typeset -n e='a[x y]'; e[1]=v
//	                             `+ a['x y'][1]=v`
//
// Every row is the chained write `a[x][k]=1` answers, row for row: the
// element becomes an indexed array of its own, the script's subscript indexes
// that array and so is arithmetic whatever the name's attribute — `k`, unset,
// is 0 — a string already in the element is promoted to its first element,
// and the trace writes the chain as it resolved. So the write is rewritten
// into that chain and stored by Runner.assignChainedElement, which has all
// of those rows already, rather than given a store of its own.
//
// What a read through the same reference answers — `${e[0]}` and `${#e[@]}`
// are 1 and 1 there — is a second question and is not this.
//
// bash 5.3.20 refuses the write: `declare -A a; declare -n e='a[x]'; e[k]=1`
// is `` `a[x]': not a valid identifier `` and `a` keeps nothing, which is what
// every column did before the axis existed. See
// Semantics.ElementWriteThroughAnElementReferenceNests.

// elementWriteThroughAnElementReference is a rewritten as the chain it
// stores, where a writes an element through a reference aimed at an element
// in the column that nests one: with `e` aimed at `a[x]`, `e[k]=1` is
// `a[x][k]=1`. The second result is false for every other assignment, a
// reference aimed at a name included — `typeset -n e=v; e[1]=z` writes `v[1]`
// as it always has.
func (r *Runner) elementWriteThroughAnElementReference(a *syntax.Assign) (*syntax.Assign, bool) {
	if a.Index == nil || a.IsArray || a.IndexFlags != nil || a.IndexRange != nil || a.Member != "" ||
		a.Members != nil || r.sem().ElementWriteThroughAnElementReferenceNests != Yes || !r.isNameref(a.Name) {
		return nil, false
	}
	target, aimed := r.namerefTarget(a.Name)
	if !aimed {
		return nil, false
	}
	ref, ok := r.reference(target)
	if !ok || ref.IndexFlags != nil || ref.IndexRange != nil || len(ref.Leading) > 0 || r.wholeArrayIndex(ref) {
		return nil, false
	}
	chain := *a
	chain.Name = ref.Name
	chain.Leading = append([]syntax.LeadingIndex{{Index: ref.Index, Text: ref.IndexText}}, a.Leading...)
	return &chain, true
}

// assignThroughAnElementReference performs the chain an element write through
// an element reference stands for, as the write it replaces: a value or a
// subscript already expanded for a, by a trace waiting on it, is handed on to
// the chain rather than expanded a second time.
func (r *Runner) assignThroughAnElementReference(ctx context.Context, a, chain *syntax.Assign) {
	if r.expanded != nil && r.expanded.assign == a {
		r.expanded.assign = chain
	}
	r.assign(ctx, chain)
}
