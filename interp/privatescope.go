// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A declaration a deeper **function frame** reads straight past.
//
// One shell in the panel has a second declaration word beside `local`, and
// what it asks for is the one thing a `local` cannot be wrapped into doing: a
// name the declaring call can use and the functions it calls cannot. See
// [Runner.DeclaringPrivateName] and privatebuiltin.go for the word.
//
// ## The rule is not "the callee cannot see it"
//
// This is the whole design and it is not what the name suggests. **A callee
// reads *past* the declaration, to whatever it displaced.** Measured
// 2026-09-27 against zsh 5.9.2 (`aarch64-apple-darwin25.4.0`), `-f` under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME`:
//
//	v=9
//	f() { private v=1; g; print "in-f=[$v]" }
//	g() { print "g=[$v] set=${+v}" }
//	f                       g=[9] set=1        in-f=[1]
//	print "[$v]"            [9]
//
// The same lines with `local` in place of `private` print `g=[1]`. With no
// outer `v` at all the callee reads **nothing** — `g=[] set=0` — and the
// declaring call still has its `1`. So a private binding is not a hidden
// name: it is a binding a deeper frame steps over, revealing whatever was
// underneath it, and `${+name}` answers about that rather than about the
// private.
//
// ## The boundary is the frame, not the scope
//
// Measured the same day, one row per construct, `v=9` outside and
// `private v=1` in the declaring call:
//
//	the declaring call itself        sees it       [1]
//	a function it calls              reads past    [9]
//	a function that function calls   reads past    [9]
//	a subshell ( … ) inside it       sees it       [1]
//	a command substitution inside it sees it       [1]
//	an anonymous function (){ … }    reads past    [9]
//	a file it sources                sees it       [1]
//	eval "g", g being a function     reads past    [9]
//	a callee declaring private v     its own       [2]
//	a callee declaring typeset v     its own       []
//
// The `eval` row and the anonymous-function row are the two that settle it.
// `eval` is not a boundary and a function called from inside one still is, so
// what the rule keys on is the **call frame** and not the scope, not the
// nesting, and not the construct. That is why this is a pair of hooks on the
// two ends of a function call — see compound.go — and not a question asked
// when a name is read.
//
// ## Which makes it affordable, and that was the design constraint
//
// Asked as "may this frame see this name" on every read, this would be a
// question on every scalar, every array, every association, every `${+name}`,
// every `unset` and every assignment in the hottest path of the interpreter,
// paid by every script whether or not it has ever written the word. Asked as
// a **swap at the frame boundary** it costs, in a shell that has never
// declared one, a single bool compared against false once per function call:
// [Runner.privateDeclared] is what sealPrivateNames reads before anything
// else. Nothing on any read path knows this file exists.
//
// In a shell that *has* declared one it costs, per function call, a walk of
// the enclosing scopes' private sets — which is empty for every call below
// the declaring one — and, per name actually in scope, one binding captured
// and one installed on the way in and the reverse on the way out. That is the
// same work [Runner.sealCallerLocals] does for the one dialect whose bodies
// read past *every* caller local, and this is deliberately built on its
// machinery rather than beside it: captureBinding, installBinding and
// writeBindingToScope already know what a name carries, and a second copy of
// that list is where the next attribute would go missing from one of them.
//
// ## Two rows this shape does not get for free
//
// **A callee may write through the seal, and what it writes stays written.**
//
//	v=9; g() { v=7 }; f() { private v=1; g; print "back=[$v]" }
//	f                    back=[1]
//	print "[$v]"         [7]
//
// That is the swap-not-copy property [sealedName] already states for the
// static seal: what the callee wrote is the *shell's* `v`, so it has to be
// left in the declaring scope's saved copy rather than thrown away with the
// seal. unsealPrivateNames writes it back through the same seam.
//
// **A callee may not write a name the private displaced nothing under.** With
// no outer `v`, the seal leaves the callee with no such parameter — and an
// assignment there is not the creation of a global it looks like:
//
//	g() { v=7 }; f() { private v=1; g }
//	f                    g: v: can't change parameter attribute, exit 1
//
// Measured under `-c` and from a script file, for a scalar, an array and a
// `+=` append alike, and with `typeset -g v=7` in place of the assignment.
// It ends the script, which is the same answer that shell gives a readonly
// reassignment. A callee that *declares* the name first is untouched, which
// is the control: `typeset v` and `private v` in the callee both make a
// binding of their own and assign it happily. So the refusal is about a write
// reaching a name the frame can see the shape of and not the value, and
// privateShield is the set it is asked against — non-nil only while such a
// seal stands, which is what keeps the question off the assignment path in
// every other shell and in this one before the word is used.

// DeclaringPrivateName runs f with every declaration it makes marked private
// to the call it is standing in.
//
// The seam a dialect's `private` word is built on, and it is a *bracket*
// rather than a "mark this name" call because the marking has to happen where
// the shadow is taken. A declaration makes its binding in one place —
// [Runner.shadow] — whatever the word in front of it, whatever letters it
// carries, and whichever of the three tables the name ends up in; marking
// from outside would mean naming every operand again after the builtin had
// already worked out what they were, and getting `private -A m` or a
// subscripted operand wrong the first time somebody wrote one.
//
// Nested, because a dialect may build the word on another and because the
// restore has to be the caller's value rather than false.
func (r *Runner) DeclaringPrivateName(f func()) {
	outer := r.declaringPrivate
	r.declaringPrivate = true
	defer func() { r.declaringPrivate = outer }()
	f()
}

// privateShadowTaken records that the shadow just taken belongs to a private
// declaration. Called from [Runner.shadow] and nowhere else.
//
// The runner-wide flag is set here rather than at the word, and that is what
// makes the fast path honest: a `private` written at the **top level** takes
// no shadow at all — measured, `private x=1; print $x` is `1` at status 0
// there with nothing hidden from anything — so a shell whose only `private`
// is outside a function goes on paying nothing.
func (r *Runner) privateShadowTaken(sc *scope, name string) {
	if sc.private == nil {
		sc.private = map[string]bool{}
	}
	sc.private[name] = true
	r.privateDeclared = true
}

// privateHolders is, per name any *enclosing* call declared private, the
// innermost such call's scope.
//
// The innermost, for [sealedName.holder]'s reason: that scope's saved copy is
// what the shell's own name was before the private displaced it, so it is
// both what the callee should read and where the callee's own writes belong.
//
// The running call's own scope is deliberately not walked. A call sees the
// names it declared private itself — that is the first row of the table above
// — so the seal is over what the calls *below* this one declared.
func (r *Runner) privateHolders() map[string]*scope {
	var holders map[string]*scope
	for i := len(r.scopes) - 2; i >= 0; i-- {
		for name := range r.scopes[i].private {
			if _, taken := holders[name]; taken {
				continue
			}
			if holders == nil {
				holders = map[string]*scope{}
			}
			holders[name] = r.scopes[i]
		}
	}
	return holders
}

// sealPrivateNames puts the enclosing calls' private declarations aside, so
// that this frame reads what they displaced.
//
// The first line is the whole of what a shell with no private pays, and it is
// read rather than asked: there is no axis here because no other shell in the
// panel has the word at all, so there is no disagreement to model — the
// mechanism is reachable only through a declaration that a dialect without
// the builtin can never make.
func (r *Runner) sealPrivateNames(sc *scope) {
	if !r.privateDeclared || len(r.scopes) < 2 {
		return
	}
	holders := r.privateHolders()
	if len(holders) == 0 {
		return
	}
	sc.privateSealed = make(map[string]sealedName, len(holders))
	for name, holder := range holders {
		outer := bindingFromScope(holder, name)
		sc.privateSealed[name] = sealedName{holder: holder, held: r.captureBinding(name)}
		r.installBinding(name, outer)
		if bindingIsAbsent(outer) {
			// The private displaced nothing, so this frame can see that the
			// name is spoken for and cannot have it. See privateShield.
			r.shieldPrivateName(name)
			sc.privateShielded = append(sc.privateShielded, name)
		}
	}
}

// unsealPrivateNames is the return: the declaring call gets its private
// binding back, and whatever this frame wrote to the shell's own name under
// it is left where the declaring call's return will find it.
func (r *Runner) unsealPrivateNames(sc *scope) {
	// What the seal exposed *nothing* for, taken before the shields come off
	// because it is also what decides the branch below.
	exposedNothing := make(map[string]bool, len(sc.privateShielded))
	for _, name := range sc.privateShielded {
		exposedNothing[name] = true
		r.unshieldPrivateName(name)
	}
	sc.privateShielded = nil
	for name, sn := range sc.privateSealed {
		now := r.captureBinding(name)
		writeBindingToScope(sn.holder, name, now)
		if !exposedNothing[name] && bindingIsAbsent(now) {
			// The frame **unset** the name the seal exposed, and that takes
			// the private with it rather than leaving it standing underneath.
			// Measured 2026-09-27 on zsh 5.9.2:
			//
			//	v=9; g(){ unset v }
			//	f(){ private v=1; g; print "back=[$v] set=${+v}" }
			//	f                  back=[] set=0
			//	print "${+v}"      0
			//
			// Both halves are the finding. The outer `v` is gone, which the
			// write-back above already accounts for — and the declaring
			// call's own binding is gone too, which nothing else here would
			// do. `unset` in that shell takes the *parameter* away, not a
			// value out of one binding, so there is nothing left for the
			// return to put back.
			//
			// Conditioned on the seal having **exposed something**, which is
			// the control row and is not the same question as the private
			// having held something: with no outer `v` the callee has no
			// parameter to unset, `unset v` there changes nothing, and the
			// declaring call's private is still `1` when it returns.
			//
			// Keyed on the wrong one of those two to begin with — on whether
			// the *private* held a value rather than on whether the seal had
			// anything to expose — and the two agree on every row but that
			// control, where the private is dropped and `in-f=[]` comes back
			// for a line that measures `in-f=[1]`. A rule written as "the
			// name is absent now" alone loses it the same way.
			continue
		}
		r.installBinding(name, sn.held)
	}
	sc.privateSealed = nil
}

// refusePrivateRedeclaration reports whether this `private` declaration is
// refused because the running call has already declared the name, having said
// so.
//
// Measured 2026-09-27 on zsh 5.9.2 under `-f`, inside a function:
//
//	private v=1; private v=6      f:private: can't change scope of existing
//	                              param: v, status 1, `v` still 1
//	local v=1;   private v=6      the same sentence, the same 1, `v` still 1
//	private v=1; local v=6        0, and `v` is 6
//	private v=1; typeset v=6      0, and `v` is 6
//	v=9; f(){ private v=1 }       0 — a *global* underneath is not a
//	                              redeclaration
//	private v=1; private w=2      0 — one name at a time
//
// It reports and the script carries on, which is what separates it from the
// write refusal above: that one ends the script.
//
// The asymmetry in the middle four rows is the whole rule and is worth
// stating as one sentence. A declaration may *narrow* what a binding is —
// `local` and `typeset` over a private both take the value and leave the
// binding where it is — and `private` may not *move* a binding that already
// exists in this call, whichever word made it. Which is why this is asked of
// the innermost scope and of nothing else: a name the shell holds, or one a
// *calling* function declared, is something this declaration displaces rather
// than something it would have to move.
func (r *Runner) refusePrivateRedeclaration(name string) bool {
	if !r.declaringPrivate || !r.localInTheInnermostScope(name) {
		return false
	}
	// The refusal names `private` and **not the word as written**, which is
	// measured rather than a simplification: `local -P v=1; local -P v=2` is
	// ``f:private: can't change scope of existing param: v`` in the shell
	// with the letter, naming the request rather than the spelling that
	// asked for it.
	//
	// Said by moving the *builtin* rather than by writing the name into the
	// sentence, because this dialect puts a builtin's name in the location
	// (Diagnostics.NamesBuiltinInLocation) and strips it off the front of a
	// message that also opens with it. Written into the sentence alone,
	// `local -P` came out as ``p13:local: private: can't …`` — the location
	// naming the spelling and the sentence naming the request, which is the
	// name twice and neither of them where the shell puts it.
	outer := r.inBuiltin
	r.inBuiltin = "private"
	r.diagf("%s\n", Wording(r.diag().PrivateRedeclaresName,
		"%[1]s: can't change scope of existing param: %[2]s", "private", name))
	r.inBuiltin = outer
	r.status = 1
	return true
}

// privateHere reports whether the binding the running frame sees under this
// name is a private one.
//
// The walk stops at the first scope that says something, and which of the two
// things it says is what makes this different from asking whether the name is
// private anywhere. A scope that **declared** it private answers yes; a scope
// that **sealed** it answers no, because what this frame is looking at is
// then the thing the private displaced — measured, a callee's `typeset -p v`
// over an outer `v=9` writes `typeset -g v=9` at 0, which is the global and
// is not private to anybody.
//
// Declared before sealed, and the order is load-bearing: a callee that
// declares its own `private v` has a scope holding both, and the one that
// governs what it sees is its own.
func (r *Runner) privateHere(name string) bool {
	if !r.privateDeclared {
		return false
	}
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if r.scopes[i].private[name] {
			return true
		}
		if _, sealed := r.scopes[i].privateSealed[name]; sealed {
			return false
		}
	}
	return false
}

// privateKindWouldChange reports whether this assignment would retype a
// private binding, having refused it.
//
// **A private keeps the kind its declaration gave it**, where an ordinary
// local is retyped by an array literal without a word. Measured 2026-09-27 on
// zsh 5.9.2 under `-f`, with the module loaded:
//
//	typeset -a at=(t l); (){ private at; at=(in fn) }
//	                     (anon): at: attempt to assign array value to
//	                     non-array, and the shell ends at 1
//	                     (){ private at=x; at+=(p q) }   the same
//	(){ local at; at=(p q); print ${(t)at} }
//	                     array-local — the control, taken at 0
//	(){ private -a at; at=(p q) }    taken at 0: the kind is the one the
//	                                 declaration asked for
//	(){ private -a at; at=plain }    taken at 0, and still `array-…`
//
// The control is what makes this a statement about `private` rather than
// about this engine's retyping: the same two lines with `local` retype the
// name and print `array-local`, in that shell and in this one.
//
// Rows three and four are the other control, and they are why this asks about
// the *declared kind* rather than about the literal: a private declared with
// `-a` takes an array literal, so the refusal is not "no literal over a
// private" — it is a kind that may not move.
//
// The scalar direction is deliberately left alone and is not an omission: row
// five is that case measured, and the shell takes it. Whatever it does to the
// value, `${(t)}` still says `array-…` afterwards, so nothing was retyped
// there either.
func (r *Runner) privateKindWouldChange(a *syntax.Assign) bool {
	if !a.IsArray || len(a.Members) > 0 || a.Index != nil {
		return false
	}
	if !r.privateHere(a.Name) || r.nameIsAnArray(a.Name) || r.assocDeclared(a.Name) {
		return false
	}
	r.fatal("%s\n", Wording(r.diag().ArrayValueToNonArray,
		"%[1]s: attempt to assign array value to non-array", a.Name))
	return true
}

// bindingIsAbsent reports whether a captured binding holds no parameter at
// all — nothing in any of the three tables.
//
// The three together and not Vars alone, because a private may be any of the
// kinds: measured, `private -a arr` is `array-local-hide-special` in that
// shell and a callee reads past it to the global array exactly as it reads
// past a scalar.
func bindingIsAbsent(b staticBinding) bool {
	return !b.valueExists && !b.arrayExists && !b.assocExists
}

// shieldPrivateName marks a name as one this frame may read the absence of
// and may not write.
//
// A count rather than a bool, because seals nest: two calls deep under one
// private declaration each seal the same name, and the inner one's return
// must not clear a shield the outer one still needs.
func (r *Runner) shieldPrivateName(name string) {
	if r.privateShield == nil {
		r.privateShield = map[string]int{}
	}
	r.privateShield[name]++
}

// unshieldPrivateName is the return's half of the pair.
func (r *Runner) unshieldPrivateName(name string) {
	if r.privateShield[name] <= 1 {
		delete(r.privateShield, name)
		return
	}
	r.privateShield[name]--
}

// refusePrivateWrite reports whether a write to this name is refused because
// an enclosing call declared it private over nothing, having said so.
//
// Asked at the top of [Runner.refuseReadonly], which is the one gate every
// route to a stored name already goes through — a bare assignment, an append,
// an array literal, an element, a declaration's value. A second gate beside
// it would be the shape of bug this tree has found repeatedly: the copy is
// written from the same understanding and then only one of them gets the fix.
//
// **The map is nil in every shell that has never sealed such a name**, which
// is what makes this free: the length check is the whole cost on the
// assignment path, and it is zero in every dialect but one and in that one
// until a script writes the word.
//
// A name this frame has declared itself is not refused, and that is measured
// rather than reasoned: a callee writing `typeset v` or `private v` first
// gets a binding of its own and assigns it at status 0. The declaration takes
// its shadow before any value is stored, so by the time a value reaches this
// gate the name is local here and the shield no longer applies to it.
func (r *Runner) refusePrivateWrite(name string) bool {
	if len(r.privateShield) == 0 || r.privateShield[name] == 0 {
		return false
	}
	if r.localInTheInnermostScope(name) {
		return false
	}
	r.fatal("%s\n", Wording(r.diag().PrivateParameterWrite,
		"%[1]s: can't change parameter attribute", name))
	return true
}
