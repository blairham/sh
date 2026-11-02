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
// A read through the same reference — `${e[0]}` and `${#e[@]}` are 1 and 1
// there — is the same chain read back, and is interp/namerefelementread.go.
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

// elementWriteThroughAnElementReferenceRefused refuses a where it writes an
// element through a reference aimed at an element in the column that does not
// nest one, and reports whether it did. It is the assignment statement's own
// refusal, so it costs what the statement's other reference refusals cost:
// the rest of the line goes with it.
//
// Measured 2026-11-02 on bash 5.3.20 (`/opt/homebrew/bin/bash`), from `-c`
// under `env -i PATH=/usr/bin:/bin` and from a script file, with `declare -A
// a; declare -n e='a[x]'` in front of each row and `; echo "after=$?"` behind
// it on the same line (#7367):
//
//	e[k]=1                    `` `a[x]': not a valid identifier ``, no
//	                          `after`, and the next line runs
//	e[k]+=1                   the same
//	e[1]=z over `declare -a a; declare -n e='a[1]'`
//	                          the same, `` `a[1]' ``
//	x=1 e[k]=1 y=2            the same, and `y` is never written
//	f() { e[k]=1; echo in; }; f
//	                          no `in` and no `after`: the line the call is on
//	                          goes, as it does for every give-up
//	set -o posix; e[k]=1      the shell ends at 127
//	e+=1                      the control: no subscript of the script's own,
//	                          `a[x]` becomes 1 and `after=0`
//	declare -n m; m=1         the control that already agreed: the same give-up
//	x=$(( e[k]=1 ))           the sentence, `after=0`: an arithmetic write is
//	                          not the statement's, and runs on
//	read 'e[k]' <<< v         the sentence, `after=1`: nor is a builtin's
//
// Every row the statement owns is Runner.refuseNamerefAim's assignedAlone
// answer, row for row — the give-up, the list it stops and the mode that
// sharpens it — so this reaches that answer rather than giving the refusal a
// cost of its own. It is asked where the store is, once the value, the
// subscript's words and its arithmetic are done, because bash does all three
// first: `i=0; e[i++]=1` leaves `i` at 1, and `e[1/0]=1` is the division's
// refusal and not this one. The other routes reach the same sentence at the
// store itself — see Runner.elementWriteRefusesAReferenceToAnElement.
func (r *Runner) elementWriteThroughAnElementReferenceRefused(a *syntax.Assign) bool {
	if !r.isNameref(a.Name) {
		return false
	}
	return r.elementWriteRefusesAReferenceToAnElement(r.throughNameref(a.Name), assignedAlone)
}
