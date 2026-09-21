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
// There is deliberately **no guard for that shape here**, and mutation testing
// says so twice over, from both sides:
//
//   - A guard that declines an element **in this function** is *wrong*. The
//     member path below shares this rule, so declining here would take the
//     element's members away with it — four rows of
//     dialect/ksh/namerefcompoundbody_test.go fail.
//   - A guard that declines an element **at the one call site that stores a
//     body** is *dead*. Nothing moves: the members reach the element through
//     the member path whichever name this hands back, and the element's own
//     value is not written either way.
//
// So the shape has no guard anywhere, which is the fold paying for itself: one
// rule cannot hold two answers, and the answer it holds is the one both
// entry points want. A guard that cannot change an answer is worse than none —
// it reads as a decision somebody made.
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

// namerefCompoundBodyStore is the rule above at the one site that **stores** a
// body, and it differs from the read in exactly one state: a reference with
// nothing to point at is refused there rather than storing under the name
// that was written.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh`), script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device:
//
//	written                          ksh93u+                    here, before
//	typeset -n u; typeset u=(a=1)    u: no reference name, 1    st=0, a `u`
//	typeset -n u; u=(a=1)            the same                   the same
//	typeset -n u; u+=(a=1)           the same                   the same
//	f(){ typeset -n u; typeset u=(a=1); }; f   the same, and no line
//	                                 of the script after the call runs
//
// and the two controls that say it is the compound body alone, both of which
// agreed before this and must go on agreeing: `typeset -n u; typeset u=plain`
// is silent at 0 with `$u` empty, and `typeset -n u; typeset u=(1 2)` stores
// the array under `u` itself — which is what [Runner.namerefArrayLiteralTarget]
// already does, warning wording and all. So an unaimed reference is not a
// refusal in general there; the parenthesized body is the one operand shape
// that will not take one.
//
// **Not folded into [Runner.namerefCompoundBodyTarget].** That rule is shared
// with the member path below, and the costs differ: a read through the same
// reference refuses as an *expansion* and an `unset` of it refuses without
// ending the script, so one rule cannot carry all three. The state they share
// is [Runner.unaimedReferenceBase]; what it costs is each site's own (#3955).
//
// The fatality is not written in as a number: the refusal ends the script
// through [Runner.fatalQuiet], so the status is Semantics.FatalErrorStatusIsOne's
// like every other fatal error's. What gates the whole refusal is the
// Diagnostics wording being set, which is the shape a refusal only one column
// makes takes everywhere in this tree — see Runner.refuseNamerefAim.
func (r *Runner) namerefCompoundBodyStore(name string) (string, bool) {
	if aimless, unaimed := r.unaimedReferenceBase(name); unaimed {
		r.refuseUnaimedReference(aimless, "")
		r.fatalQuiet()
		return "", false
	}
	return r.namerefCompoundBodyTarget(name), true
}

// unaimedReferenceBase reports the reference a name is read or written
// *through* where that reference has nothing to point at, and answers no
// where the dialect has nothing to say about it.
//
// The base and not the whole name, because a member path is a use of the
// reference its base is: `${u.a}` is refused in the words `${u}` is, naming
// `u`. A leading dot is not a base — `${.sh.level}` and a namespace's member
// both begin with one — so the empty base is declined, exactly as
// [Runner.compoundMemberThroughAReference] declines it.
//
// The rows, the states that stay silent, and why each site answers the cost
// itself are in Diagnostics.NamerefUnaimedUse.
func (r *Runner) unaimedReferenceBase(name string) (string, bool) {
	if r.diag().NamerefUnaimedUse == "" {
		return "", false
	}
	base := name
	if dot := strings.IndexByte(name, '.'); dot > 0 {
		base = name[:dot]
	}
	if !r.isNameref(base) {
		return "", false
	}
	if _, cycle, aimed := r.namerefWalk(base); cycle || aimed {
		// A cycle has already been reported where a read of it would report
		// one, and an aimed reference is the ordinary case.
		return "", false
	}
	return base, true
}

// refuseUnaimedReference writes the sentence and leaves the cost to the
// caller, which is the whole reason it is not one function with the check.
// Five sites ask [Runner.unaimedReferenceBase] and they hold three answers
// between them: an expansion sets expandErr and lets
// Semantics.FailedExpansionAbandonsTheLine say what that costs, a write and a
// compound body end the script, and `unset` reports at 1 and lets the next
// line run. One function with the cost inside it could only have held one of
// the three.
//
// The speaker is the builtin whose sentence this is, and it is "" for the two
// sites that have none — an expansion runs in no builtin, and a compound
// body is stored after the declaration has returned. `unset` is the one that
// names itself: measured, `typeset -n u; unset u` is `unset: u: no reference
// name` where `${u}` on the line above it is `u: no reference name`.
//
// Written into the sentence rather than taken from the location, because
// that is where this shell's other `unset` refusals put it — see
// Diagnostics.BuiltinBadName, whose `unset` entry is `%[1]s: %[2]s: invalid
// variable name`.
func (r *Runner) refuseUnaimedReference(name, speaker string) {
	line := Wording(r.diag().NamerefUnaimedUse, "", name)
	if speaker != "" {
		line = speaker + ": " + line
	}
	r.diagf("%s\n", line)
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
// # Where it is applied, and what that does and does not reach
//
// At the funnels that decide **which name** an act is about, rather than at
// the member paths themselves: [Runner.storedValue] for a read,
// [Runner.setVarAs] for a write, [Runner.unsetName] for a removal, and the
// prefix listing in Runner.expandSpan. Each of those already resolves a whole
// name through a reference and could not see a dotted one.
//
// One call at the read funnel covers every operator, because there is no one
// member path — they are all the plain name `c.a` by the time they get there.
// Measured against ksh93u+ over `typeset zz=(a=1 b=2); typeset -n c=zz`, and
// each of these was empty or wrong before:
//
//	${c.a} ${c.b}     1 and 2            ${c.a:-D} ${c.nope:-D}  1 and D
//	${#c.a}           1                  ${c.a+SET}              SET
//	c.a=99            ${zz.a} is 99      c.a+=x                  ${zz.a} is 1x
//	unset c.a         ${zz.a} empty, ${zz.b} still 2
//	${!c.@}           zz.a zz.b          ${c.a.y}, nested        7
//
// **One route is measured and deliberately not taken**: an attribute letter
// over a member path. `typeset -u c.b; c.b=hi` leaves `${zz.b}` as `HI` in
// ksh93u+ and `hi` here, so the letter lands on a name called `c.b` and the
// value lands on `zz.b`. That is the declaration builtin's operand loop, which
// takes a shadow and consults [Runner.attributeFollowsTheReference] before any
// of this — a different mechanism from the four funnels above, and its own row.
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
