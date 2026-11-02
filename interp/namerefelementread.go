// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An element read through a reference that is itself aimed at an element:
// `typeset -A a; typeset -n e='a[x]'; e[k]=1; echo "${e[0]}"`.
//
// The read half of interp/namerefelementwrite.go, and the same sentence: in
// the column that nests the element on the write, a subscript the script
// writes on the reference indexes what the element holds, so the read is the
// chain the two subscripts spell. Measured 2026-11-02 on ksh93u+ 2012-08-01
// (`/bin/ksh`), from `-c` under `env -i PATH=/usr/bin:/bin` (#7366), each row
// beside the direct chain it stands for:
//
//	typeset -A a; typeset -n e='a[x]'; e[k]=1; e[3]=q
//	  ${e[0]} ${#e[@]} ${e[@]} ${!e[@]}   1 2 `1 q` `0 3`, as ${a[x][…]}
//	  ${e[0]:-d} ${#e[0]} ${e[0]/1/Z}     1 1 Z, the operators on the chain
//	  typeset -n f=e; ${f[0]} ${#f[@]}    1 1, through a second reference
//	typeset -a a; typeset -n e='a[1]'; e[2]=z
//	  ${e[2]} ${#e[@]}                    z 1
//	a=(p q r); typeset -n e='a[1]'
//	  ${e[0]} ${e[1]} ${#e[@]}            q, empty, 0: a string element is
//	                                      itself at the base and holds no
//	                                      elements, which is ${a[1][0]}'s rule
//
// In every row the reference and the chain written out answer alike, so the
// node is rewritten into that chain and read by the path that already serves
// `${a[x][0]}` (interp/nestedchainsub.go), rather than given a reading of its
// own.
//
// The plain `${e}`, with no subscript of the script's own, is not this: it
// reads the element, `a[x]`, in both shells, and never comes here. See
// Runner.readThroughNamerefElement.
//
// bash 5.3.20 has no nested element to read: `declare -A a; a[x]=s; declare
// -n e='a[x]'; echo "${e[0]} ${#e[@]}"` is ` 0` there, which is what every
// column answered before this. The axis is the write's, because no shell
// measured parts the two: Semantics.ElementWriteThroughAnElementReferenceNests.

// elementReadThroughAnElementReference is e rewritten as the chain it reads,
// where e subscripts a reference aimed at an element in the column that nests
// one: with `e` aimed at `a[x]`, `${e[k]}` is `${a[x][k]}`, and `${!e[@]}` the
// nested element's subscripts. The second result is false for every other
// expansion, a reference aimed at a name included — `typeset -n e=v; ${e[1]}`
// reads `v[1]` as it always has.
func (r *Runner) elementReadThroughAnElementReference(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if e == nil || e.Index == nil || e.Inner != nil || e.Prefix != 0 || e.Member != "" ||
		e.IndexFlags != nil || e.IndexRange != nil || e.IndexDots != nil ||
		r.sem().ElementWriteThroughAnElementReferenceNests != Yes || !r.isNameref(e.Name) {
		return nil, false
	}
	if e.Indirect && !r.wholeArrayIndex(e) {
		// `${!e[0]}` is an indirection and not a listing; only the listing
		// was measured as the chain's.
		return nil, false
	}
	target, aimed := r.namerefTarget(e.Name)
	if !aimed {
		return nil, false
	}
	ref, ok := r.reference(target)
	if !ok || ref.IndexFlags != nil || ref.IndexRange != nil || len(ref.Leading) > 0 || r.wholeArrayIndex(ref) {
		return nil, false
	}
	chain := *e
	chain.Name = ref.Name
	chain.Leading = append([]syntax.LeadingIndex{{Index: ref.Index, Text: ref.IndexText}}, e.Leading...)
	return &chain, true
}
