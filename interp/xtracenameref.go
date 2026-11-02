// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

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
// second `u=z` is `+ y=z`. Through a reference to one aimed at nothing it is
// the one the value aims: `typeset -n m; typeset -n l=m; l=y` is `+ m=y`, and
// `typeset l=y` the same (#7114). This column writes a bare assignment's line after
// its store, so the name is settled before the store and carried to the line
// (expandedAssign.tracedAs) — read at the line, it named the variable the
// store had just aimed the reference at, `+ y=y`.
//
// bash 5.3.20 writes the word as written in every row, `m=1` and `e[k]=1`
// alike. See Diagnostics.TraceAssignmentNamesTheReferencedVariable.
//
// A declaration's operand is written the same way where the utility stores
// it through the reference: measured 2026-11-02 against the same ksh93u+,
// from `-c` under `env -i PATH=/usr/bin:/bin`, with `x=0; typeset -n m=x`
// first (#7102):
//
//	typeset m=3                   `+ x=3`, `+ typeset m`
//	export m=4; readonly m=6      `+ x=4`, `+ export m`; `+ x=6`, …
//	typeset -i m=2+2              `+ x=2+2`, `+ typeset -i m`
//	typeset +n m=5                `+ x=5` — the value goes through first
//	typeset -a m=(1 2)            `+ x=( 1 2 )`, `+ typeset -a m`
//	typeset -A m=([k]=v)          `+ x[k]=v`
//	typeset o[1+1]=z              `+ b[2]=z`, `o` aimed at an array `b`
//	g() { typeset m=7; }; g       `+ x=7` — a POSIX-form body makes no local
//	function f { export m=4; }    `+ x=4` — nor does `export`
//
// and as the script spelled it where the operand does not go through the
// reference at all — it re-aims it, or it makes a local of the running
// function, whose fresh binding no reference reaches:
//
//	typeset -n m=y; nameref m=y   `+ m=y`, `+ typeset -n m`
//	function f { typeset m=7; }   `+ m=7`, `+ typeset m`
//
// A reference the function made itself is not replaced, so the operand goes
// through it there: `function f { typeset -n m=x; typeset m=3; }` is `+ x=3`
// and `+ typeset m`, and leaves the global `x` holding 3 (#7352).
//
// bash 5.3.20 writes `declare m=3` and `export m=4` through `declare -n m=x`
// as it always has, on the command's own line. See
// Runner.operandStoresThroughTheReference.

// tracedThroughTheReference is a traced assignment under the name its store
// lands in, in the dialect that writes it so, and the assignment itself
// everywhere else. operandOf is the declaration utility's words where a is
// one of its operands, and nil for an assignment of the shell's own; an
// operand handed over without them is left as written, since only the words
// say whether its store goes through the reference. See
// Runner.nameTracedThroughTheReference.
func (r *Runner) tracedThroughTheReference(a *syntax.Assign, operandOf []string) *syntax.Assign {
	if a == nil || (a.Operand && operandOf == nil) {
		return a
	}
	whole := a.Index == nil && len(a.Leading) == 0 && !a.IsArray
	name := r.nameTracedThroughTheReference(a.Name, whole, operandOf)
	if name == a.Name {
		return a
	}
	named := *a
	named.Name = name
	return &named
}

// nameTracedThroughTheReference is the name a traced store to name is written
// under: the variable a reference lands it in, in the dialect that writes it
// so, and name itself everywhere else — a name that is no reference, one aimed
// at nothing or round a loop, a declaration's operand the utility does not
// store through the reference, and an element written through a reference
// that is itself aimed at an element, which nothing has measured. whole says
// the store writes the whole variable rather than an element or a literal of
// it; operandOf is as tracedThroughTheReference has it.
func (r *Runner) nameTracedThroughTheReference(name string, whole bool, operandOf []string) string {
	if !r.diag().TraceAssignmentNamesTheReferencedVariable || !r.isNameref(name) {
		return name
	}
	target, cycle, aimed := r.namerefWalk(name)
	if cycle {
		return name
	}
	if !aimed {
		// Aimed at nothing, so the store aims a reference rather than
		// landing anywhere, and the line names the one it aims: the name
		// itself, or the end of the chain where the value follows it there
		// — `typeset -n m; typeset -n l=m; l=y` is `+ m=y` (#7114).
		target = r.referenceAValueAims(name, target)
	}
	if target == name {
		return name
	}
	if operandOf != nil && !r.operandStoresThroughTheReference(name, operandOf) {
		return name
	}
	base, sub, ok := operandSubscriptOf(target)
	if !ok {
		return target
	}
	if !whole {
		return name
	}
	d := r.diag()
	return base + "[" + r.traceQuote(sub, d.TraceQuoting, d.TraceMetacharacters) + "]"
}

// operandStoresThroughTheReference reports whether the declaration utility
// argv names stores an operand through the reference name holds. It does not
// where it carries the reference letter under a minus — `typeset -n m=y`, and
// `nameref`, which is that — since the operand then re-aims the reference, nor
// where it gives name a fresh binding of the running function's own, which
// drops the reference. ksh93u+ writes `typeset -n +n m=8` as `+ m=8` too, so
// any minus `n` among the letters counts.
func (r *Runner) operandStoresThroughTheReference(name string, argv []string) bool {
	for _, w := range argv[min(traceDeclarationUtilityWord(argv)+1, len(argv)):] {
		if w == "--" || len(w) < 2 || (w[0] != '-' && w[0] != '+') {
			break
		}
		if w[0] == '-' && strings.ContainsRune(w[1:], 'n') {
			return false
		}
	}
	return !r.operandMakesAFreshLocal(name, argv)
}
