// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An assignment through a name reference, as `set -x` writes it.
//
// One column writes the variable the store lands in, not the word as the
// script spelled it. Measured 2026-10-30 against ksh93u+ 2012-08-01
// (`/bin/ksh`), from `-c` and script files under `env -i PATH=/usr/bin:/bin`,
// with `typeset -A a; x=0; typeset -n n=a m=x` first (#6991):
//
//	m=1 :                         `+ x=1`, `+ :`
//	n[k]=1 :                      `+ a[k]=1`, `+ :`
//	m=1                           `+ x=1`, and `m+=2` is `+ x+=2`
//	m=(1 2)                       `+ x=( 1 2 )`
//	typeset -n l=m; l=5           `+ x=5` — through both references
//	typeset -n e='a[x y]'; e=1    `+ a['x y']=1` — an element, quoted as a key
//	m=1 f                         `+ x=1`, `+ f`, `f` a POSIX-form function
//	m=1 /usr/bin/true             `+ /usr/bin/true`, `+ x=1`
//	m=1 command /usr/bin/true     the same
//
// and the word as written where the prefix is a cell of the command's own,
// which no reference reaches:
//
//	m=2 true                      `+ true`, `+ m=2` — a regular builtin
//	m=1 command echo hi           `+ command echo hi`, `+ m=1`
//	function g { :; }; m=1 g      `+ g`, `+ m=1` — a `function`-form call
//
// and the word as written for a reference aimed at nothing, whose store aims
// it rather than landing anywhere: `typeset -n u; u=y` is `+ u=y`, and a
// second `u=z` is `+ y=z`. This column writes a bare assignment's line after
// its store, so the name is settled before the store and carried to the line
// (expandedAssign.tracedAs) — read at the line, it named the variable the
// store had just aimed the reference at, `+ y=y`.
//
// bash 5.3.20 writes the word as written in every row, `m=1` and `e[k]=1`
// alike. See Diagnostics.TraceAssignmentNamesTheReferencedVariable.
//
// A declaration's operand through a reference is not here: whether it lands
// in the reference's variable depends on whether the utility makes a local or
// re-aims the reference, and ksh93 writes it accordingly (#7102).

// tracedThroughTheReference is a traced assignment under the name its store
// lands in, in the dialect that writes it so, and the assignment itself
// everywhere else: a name that is no reference, one aimed at nothing or round
// a loop, and an element written through a reference that is itself aimed at
// an element, which nothing has measured.
func (r *Runner) tracedThroughTheReference(a *syntax.Assign) *syntax.Assign {
	if a == nil || a.Operand || !r.diag().TraceAssignmentNamesTheReferencedVariable || !r.isNameref(a.Name) {
		return a
	}
	target, cycle, aimed := r.namerefWalk(a.Name)
	if cycle || !aimed || target == a.Name {
		return a
	}
	named := *a
	if base, sub, ok := operandSubscriptOf(target); ok {
		if a.Index != nil || len(a.Leading) > 0 || a.IsArray {
			return a
		}
		d := r.diag()
		named.Name = base + "[" + r.traceQuote(sub, d.TraceQuoting, d.TraceMetacharacters) + "]"
		return &named
	}
	named.Name = target
	return &named
}
