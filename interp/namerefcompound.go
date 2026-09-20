// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// namerefCompoundBodyTarget is the name a **compound variable's body** is
// stored under, where the name written is a reference.
//
// A compound body is the one operand shape that did not follow a reference.
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh`), script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device, over `typeset -n c=zz` — every row lands on `zz` there and
// landed on `c` here, silently and at status 0, so `typeset -p zz` reported
// nothing at all:
//
//	typeset c=(a=1)                   ${zz.a} is 1
//	c=(a=1)                           1, with no declaration word at all
//	typeset c=(a=1); c+=(b=2)         ${zz.b} is 2 and the `a` is kept
//	typeset c=(a=1 b=(y=2))           ${zz.b.y} is 2
//	typeset c=(typeset -i n=5)        ${zz.n} is 5
//	typeset c=(a=1); typeset c=(d=3)  the body is replaced: `a` gone, `d` is 3
//	through a chain of two references  the far end takes it
//
// The scalar and the array literal were already right — `typeset c=plain` and
// `typeset c=(1 2)` both reach `zz` here — which is what says this is one
// operand shape and not "namerefs are broken". They were right for two
// different reasons: a scalar goes through [Runner.setVarAs], which resolves a
// reference itself, and an array literal is redirected in [Runner.assign] by
// [Runner.namerefArrayLiteralTarget]. The compound body's branch stands ahead
// of the array literal's — both answer to `Assign.IsArray` — and had no
// redirect of its own.
//
// **Why the redirect is at the assignment and not in the declaration
// builtin.** `typeset` already replaces a reference's name with its target for
// the attributes ([Runner.attributeFollowsTheReference]) and for the value
// ([Runner.referenceValueTarget]). A compound body reaches neither: it is
// performed by Runner.assignOperands *after* callBuiltin, so that the
// declaration decides the scope its members land in — the order
// interp/compoundoperandorder.go states and #3824 depends on. Redirecting at
// the assignment covers the declaration word and the bare `c=(a=1)` with one
// answer, which is what the second row above requires.
//
// # One shape this deliberately does not reach
//
// A reference aimed at an **element**:
//
//	a=(p q r); typeset -n e='a[1]'; typeset e=(x=1)
//
// puts the compound *in* the element in ksh93u+ — `"${a[1]}"` renders the body
// and `typeset -p a` is `typeset -a a=(p (x=1) r)`. Here the element keeps `q`.
// Storing it needs an [elementAddress], which is built from a parsed subscript
// and not from a reference's resolved text, so nothing in this shell can point
// an element at a compound from a name alone yet. The *members* still land in
// the right place, because [Runner.compoundMemberThroughAReference] below
// resolves `e.x` to `a[1].x` — so `${a[1].x}` is 1 there and here. It is the
// element's own value that is left behind.
//
// There is deliberately **no guard for that shape here**: one was written and
// it is dead, which mutation testing is what said. Removing it changes no
// answer, because the members reach the element through the member path below
// whichever name this hands back, and the element's value is not written
// either way. A guard that cannot change an answer is worse than none — it
// reads as a decision somebody made.
func (r *Runner) namerefCompoundBodyTarget(name string) string {
	if !r.isNameref(name) {
		return name
	}
	target, cycle, aimed := r.namerefWalk(name)
	if cycle || !aimed {
		// A cycle has already been reported where a read of it would report
		// one, and a reference aimed at nothing is not aimed anywhere to
		// carry a body to — `typeset -n u; typeset u=(a=1)` is `u: no
		// reference name` at 1 in ksh93u+, which this shell does not raise
		// and which is its own row rather than this one.
		return name
	}
	return target
}

// compoundMemberThroughAReference is the rule above at a **member path**: the
// name `c.a` denotes where `c` is a reference is `zz.a`.
//
// A member of a compound is an ordinary name with a dot in it — that is the
// whole of interp/compoundvariable.go's design, and it is why a member carries
// its own attributes and is read by every route that reads a name. The
// reference walk is keyed on whole names, so it never saw `c.a`: the walk
// asked about `c.a`, found nothing, and the read and the write both landed on
// a name literally called `c.a`. Measured over `typeset zz=(a=1 b=2);
// typeset -n c=zz`:
//
//	${c.a}      1 there, empty here
//	${c.b}      2 there, empty here
//	c.a=99      ${zz.a} is 99 there; here it stayed 1 and a `c.a` appeared
//
// It is the same defect as the store above seen from the reading side, and the
// two have to land together: with the body following the reference and the
// member path not, `${c.a}` would read nothing at all — which is the row that
// made the store going astray invisible in practice.
//
// **The base goes through the function above rather than through a walk of its
// own**, which is the whole shape of this file: one rule, two entry points. A
// second copy is how the element answer would come to differ between the store
// and the read, and this tree has paid for that three times over.
//
// An element target is followed here, and the comment above says why that is
// the measurement's asymmetry and not an oversight:
//
//	a=(p q); a[1]=(x=1); typeset -n e='a[1]'
//	    ${e.x}   1 there, and 1 here now; empty before
//	    e.x=9    ${a[1].x} is 9 there, and 9 here now; 1 before
//
// Applied at the two funnels every name passes through, [Runner.storedValue]
// and [Runner.setVarAs], rather than at the member paths themselves. There is
// no one member path: `${c.a}`, `${c.a:-D}`, `${#c.a}`, `c.a=1`, `c.a+=x`,
// `typeset c.a` and `unset c.a` are all the plain name `c.a` by the time they
// get here, which is exactly the economy that made members work in the first
// place.
//
// A leading dot is not a base — `${.sh.level}` and a `namespace` member both
// begin with one — so the empty base is declined rather than walked.
func (r *Runner) compoundMemberThroughAReference(name string) string {
	dot := strings.IndexByte(name, '.')
	if dot <= 0 {
		return name
	}
	return r.namerefCompoundBodyTarget(name[:dot]) + name[dot:]
}
