// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A `private` declaration makes a slot of a **fixed kind**, exactly as the
// shell holds its own parameters in one.
//
// interp/parameterkindfixed.go is the same rule over the names the shell owns
// and this is the same rule over the names a call declares private; the only
// difference is where the allowed set comes from. There the dialect writes it
// out per name, because a slot can take two kinds — `SECONDS` is an integer
// or a float — and nothing about the name says which. Here the set is a
// single kind and the declaration is what chose it, so it is read off the
// binding rather than tabled.
//
// ## The grid
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours, so these are two
// programs — one run per cell, `-f` from a script file under `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`, every row behind `zmodload
// zsh/param/private`. Five declared kinds against seven letter spellings,
// each written as `(){ private <d> at; typeset <l> at; print ${(t)at} }`:
//
//	declared   -a     -A     -i     -F     none   +a     +i
//	(scalar)   type   type   type   type   take   take   take
//	-a         take   type   type   type   take   type   take
//	-A         type   take   type   type   take   take   take
//	-i         type   type   take   type   take   take   type
//	-F         type   type   type   take   take   take   take
//
// where *type* is `at: can't change type of a special parameter`, naming the
// word that was written and ending the shell at 1.
//
// That is [parameterKinds] exactly: a minus letter asks for its own kind and
// is refused when the slot has not got it, and a plus letter takes that kind
// away, which is a refusal only where the slot holds nothing else. `+a` over
// an `-A` private is taken and `+a` over an `-a` private is refused, which is
// the pair that says the plus half is read the same way here.
//
// **The control is the ordinary local, and it takes every cell.** The same
// grid written with `local` in place of `private` is 35 takes in both shells,
// so this is the second declaration word's and not a rule about redeclaring.
//
// ## And the value is asked first
//
// Two rules can fire on one line and the **value's** wins, which is measured
// rather than chosen:
//
//	private at; typeset -a at=(p q)   at: can't assign array value to
//	                                  non-array special
//	private at; typeset -a at=plain   at: can't change type of a special
//	                                  parameter
//	private -A m; typeset -a m=(p q)  the type sentence — a table takes an
//	                                  array literal, so the value agrees
//	                                  with the slot and only the letter is
//	                                  left to refuse
//	private -i n; typeset -a n=(p q)  the assign sentence — an integer does
//	                                  not take one
//
// So it is not "a literal beats a letter": the question is whether the value
// this line carries is one the slot would take, and only where it is not does
// the store's refusal come first. The shell's own parameters are the same
// shape read from the other side — `typeset -i path=(1 2)` is the type
// sentence there, because an array slot takes the literal.
//
// ## What is not here
//
// The identical shape over a name the **shell** holds is open and is not
// this: `path=plain` at the top level is `array-tied-special` holding one
// element in the reference and a five-character scalar here, measured the
// same day. That is interp/parameterkindfixed.go's table rather than a
// private's, and it is filed on its own.

// fixedSlotKinds is the set of kinds a slot of a fixed kind
// will take, and whether the name is in one at all.
//
// **Two sources and one rule**, which is what #4879 folded together. A
// `private` binding's set is the single kind its declaration gave it, read
// off the binding — a private is declared and then may not move, so what it
// is now is what it was declared as, and that stays true only while the rules
// below hold. A name the *shell* holds carries its set in `r.kindFixed`,
// written out per name by the dialect that measured it, because a shell-held
// slot can take two kinds and nothing about the name says which:
// interp/parameterkindfixed.go has that table and the sweep behind it.
//
// The private is asked first. The two sets never both answer for one name in
// practice — `private path` is refused by the scope rule before it declares
// anything — and asking the nearer one first is the order every other
// binding question in this package takes.
func (r *Runner) fixedSlotKinds(name string) (parameterKinds, bool) {
	if allowed, private := r.privateSlotKindsOnly(name); private {
		return allowed, true
	}
	if allowed, tied := r.scriptTieSlotKinds(name); tied {
		return allowed, true
	}
	if r.localInTheInnermostScope(name) && r.shadowIsHidden(name) {
		// A hidden shadow makes an ordinary parameter that merely happens to
		// be spelled like the shell's, so there is no slot left to keep a
		// kind — the same exemption
		// Runner.kindLetterOverAShellParameterRefused records, reached from
		// the value side.
		return 0, false
	}
	if !r.specialNames.mayHold(name) {
		// Never fixed: MarkParameterKindFixed records every name it fixes in the
		// filter. See specialnames.go.
		return 0, false
	}
	allowed, fixed := r.kindFixed[name]
	return allowed, fixed
}

// scriptTieSlotKinds is the third source: a half of a tie a **script** made
// with `typeset -T`, which holds its kind against a value exactly as the
// shell's own pairs do. The scalar half takes a scalar and the array half an
// array. Measured 2026-10-15 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`,
// `-f` from a script file, after `typeset -T FOO=a:b foo` (#6468):
//
//	FOO=(x)              FOO: attempt to assign array value to non-array,
//	                     and the script ends
//	FOO+=(x), FOO=()     the same
//	set -A FOO x y       the same, with no `set:` in it
//	read -A FOO          the same
//	: ${(A)FOO::=x y}    the same
//	foo=plain            foo `array-tied` holding one element, FOO `plain`
//	f(){ local foo; FOO=(x) }   refused — the half written is still the tie's
//	f(){ local FOO; FOO=(x) }   array-local: a local in front is an ordinary
//	                            name, which is tieDescribesTheBinding's no
//	x=s; x=(a)           array — the control, an untied scalar converts
//
// A **declaration** carrying the literal is not refused but unties, as a kind
// letter does — `typeset FOO=(x)` leaves FOO an array and foo gone — and that
// is Runner.kindLetterUntiesAScriptTie's, so fixedSlotRefusesThisOperand
// leaves a script's tie to it.
//
// The shell's own pairs are kindFixed's and are left to it: their sets are
// measured per name and a `-h` local detaches them where a plain one does
// not. A joined pair with no word on it is the shell's too.
func (r *Runner) scriptTieSlotKinds(name string) (parameterKinds, bool) {
	t, tied := r.tieOf(name)
	if !tied || t.special || t.wordless || !r.tieDescribesTheBinding(name) {
		return 0, false
	}
	if name == t.array {
		return kindBit(ArrayParameter), true
	}
	return kindBit(ScalarParameter), true
}

// privateSlotKindsOnly is the private half of fixedSlotKinds, for the one
// caller that must not answer for a name the shell holds.
func (r *Runner) privateSlotKindsOnly(name string) (parameterKinds, bool) {
	if !r.privateDeclared || !r.privateHere(name) {
		return 0, false
	}
	return kindBit(r.parameterKind(name)), true
}

// fixedSlotRefusesThisOperand reports whether a declaration word's operand
// over a private binding is refused, having said so and ended the script.
//
// Both refusals, in one place and in this order, because the order is the
// measurement: the **value** this line carries is asked about first and the
// kind **letter** second. Asked inside the builtin rather than at the store,
// which is what makes the order reachable at all — a letter applied first
// would have turned the name into the kind the literal wanted and left
// nothing to refuse.
//
// The kind half is [Runner.kindLetterOverAShellParameterRefused] with the
// allowed set read off the private instead of out of the dialect's table, and
// it is worded by the same field: the `special` in that sentence is the word
// `${(t)}` already gives a private, so the shell that has both writes one
// sentence for two sources of one rule.
func (r *Runner) fixedSlotRefusesThisOperand(name string, f declareFlags) bool {
	allowed, fixed := r.fixedSlotKinds(name)
	if !fixed {
		return false
	}
	if _, tied := r.scriptTieSlotKinds(name); tied {
		// A declaration's literal over a script's tie unties the pair rather
		// than being refused — see scriptTieSlotKinds — and a kind letter on
		// one is not a private's letter either.
		return false
	}
	if r.literalOperands[name] &&
		!allowed.has(ArrayParameter) && !allowed.has(AssocParameter) {
		// An array literal over a slot that is not a container. The word is
		// named in the location and the sentence is not the bare
		// assignment's — see Diagnostics.ArrayValueToANonArraySpecial, and
		// the ordering rows above for why this is ahead of the letter.
		r.fatal("%s\n", Wording(r.diag().ArrayValueToANonArraySpecial,
			Wording(r.diag().ArrayValueToNonArray,
				"%[1]s: attempt to assign array value to non-array", name), name))
		return true
	}
	if _, private := r.privateSlotKindsOnly(name); !private {
		// The **letter** half is the private's alone here. A name the shell
		// holds reaches it through
		// Runner.kindLetterOverAShellParameterRefused, which runs ahead of
		// this on every route that has a letter record to read and carries
		// the hidden-shadow exemptions a shell-held slot has and a private
		// has not. Asking it twice would answer those rows by the wrong one
		// of the two.
		return false
	}
	kind, plus, written := f.kindLetterWritten()
	if !written {
		return false
	}
	if plus {
		if !allowed.onlyHolds(kind) {
			return false
		}
	} else if allowed.has(kind) {
		return false
	}
	r.fatal("%s\n", Wording(r.diag().SpecialParameterKind,
		"%s: can't change type of a special parameter", name))
	return true
}

// fixedContainerKeepsItsKind reports whether a plain value written over a
// slot whose kind is a **container** was dealt with by this rule rather than
// by the ordinary scalar store.
//
// The two containers answer differently and both are measured, over a private
// and over a name the shell holds alike:
//
//	private -a at; at=plain            array-local-hide-special, one element
//	private -a at; at=$(echo "x y")    the same, and the element holds both
//	                                   words — nothing is split
//	private -A m;  m=plain             m: attempt to set slice of
//	                                   associative array, and the shell ends
//	private -A m;  m+=plain            the same
//	path=plain                         array-tied-special, one element
//	fpath=plain                        the same
//	fignore=plain                      the same
//
// and the control is the ordinary name, which is retyped by the identical
// line in both shells: `local -a at; at=plain`, `typeset -a at; at=plain` and
// a bare `q=plain` over a `typeset -a q` are all `scalar` holding five
// characters. `private -i n; n=abc` and `SECONDS=plain` are the other
// control — a slot whose kind is not a container takes the value and stays
// itself, in both — so this is about the container and not about every fixed
// slot.
//
// The append is the array's one exception and it is not an exception to the
// rule: `private -a at=(p q); at+=x` is three elements in both shells, which
// is the element append the operator already means over an array.
func (r *Runner) fixedContainerKeepsItsKind(a *syntax.Assign) bool {
	if a.IsArray || a.Index != nil || len(a.Members) > 0 || a.Member != "" {
		return false
	}
	allowed, fixed := r.fixedSlotKinds(a.Name)
	if !fixed {
		return false
	}
	if allowed.has(AssocParameter) && r.assocDeclared(a.Name) {
		outer := r.inBuiltin
		// The store is speaking and not the word in front of it: measured,
		// the sentence carries the function's name and no builtin's.
		r.inBuiltin = ""
		r.fatal("%s\n", Wording(r.diag().SliceOfAnAssociativeArray,
			"%[1]s: attempt to set slice of associative array", a.Name))
		r.inBuiltin = outer
		return true
	}
	if a.Append || !allowed.has(ArrayParameter) || !r.nameIsAnArray(a.Name) {
		return false
	}
	value, _, globbed := r.scalarAssignValue(a)
	if r.assignmentGaveUp() {
		// Nothing to store — the value would not expand. See the scalar
		// store in Runner.assign, which this stands in front of (#6544).
		return true
	}
	if globbed {
		// The right-hand side was a pattern and the option that reads it as
		// one replaces the name outright, kind included, which is a road
		// this rule is not on. Left to the ordinary store.
		return false
	}
	r.setArray(a.Name, []string{value})
	r.markForAllexport(a.Name)
	return true
}

// fixedSlotRefusesABareArrayLiteral reports whether an array literal written
// as a **plain assignment** — no declaration word in front of it — over a
// slot that is not a container is refused, having said so and ended the
// script.
//
// **A fixed slot keeps its kind**, where an ordinary name is retyped by an
// array literal without a word. Measured 2026-09-27 on zsh 5.9.2 under `-f`:
//
//	typeset -a at=(t l); (){ private at; at=(in fn) }
//	                     (anon): at: attempt to assign array value to
//	                     non-array, and the shell ends at 1
//	                     (){ private at=x; at+=(p q) }   the same
//	(){ local at; at=(p q); print ${(t)at} }
//	                     array-local — the control, taken at 0
//	(){ private -a at; at=(p q) }    taken at 0: the kind is the one the
//	                                 declaration asked for
//
// **The declaration's own operand is not a retype, and that is the exemption
// the two flags buy.** Every row above is a *later* line writing over a
// binding some earlier line declared, and the kind that may not move is the
// one that declaration gave. A literal written on the declaration itself is
// what gives it — measured the same day:
//
//	(){ private q=(1 2); print ${(t)q} $#q }   array-local-hide-special 2
//	(){ local -P q=(1 2); print ${(t)q} $#q }  the same
//	private topq=(1 2)                         `array`, at the top level
//
// so there is nothing to keep the kind of yet. r.declaringPrivate is the span
// of the builtin and r.privateDeclarationRan the `name=( … )` operand, which
// Runner.simple stores after the builtin has returned. Both are one command
// line wide, so every later line above runs outside them.
//
// The neighbor of fixedSlotRefusesThisOperand above, and the two are told
// apart by the sentence rather than by the rule: measured 2026-09-27 on zsh
// 5.9.2, with a `private v` standing and over the shell's own scalars alike,
//
//	v=(p q)             v: attempt to assign array value to non-array
//	local v=(p q)       local: v: can't assign array value to non-array special
//	HOME=(p q)          HOME: attempt to assign array value to non-array
//	typeset HOME=(p q)  typeset: HOME: can't assign array value to non-array …
//	IFS=(p q)           the bare sentence, naming IFS
//	RANDOM=(p q)        the bare sentence, naming RANDOM
//	SECONDS=(p q)       the bare sentence, naming SECONDS
//
// The control is a slot that **is** a container, which takes the literal in
// both shells: `path=(a b)` is `array-tied-special` holding two elements, and
// so is `typeset -a path=(a b)`. The other control is the ordinary name,
// which is retyped by the identical line: `q=1; q=(p q)` is an array
// everywhere.
func (r *Runner) fixedSlotRefusesABareArrayLiteral(a *syntax.Assign) bool {
	if !a.IsArray || len(a.Members) > 0 || a.Index != nil {
		return false
	}
	if r.declaringPrivate || r.privateDeclarationRan {
		return false
	}
	return r.fixedSlotRefusesAnArrayValue(a.Name)
}

// fixedSlotRefusesAnArrayValue reports whether an array value written over a
// slot that is not a container is refused, having said so and ended the
// script. It is the store's refusal, and every road that writes a whole array
// by a name asks it: the bare literal above, `set -A`, `read -A` and the
// `(A)` assignment flag. An assignment prefix asks the question half alone,
// because its cost is not always the script — see
// fixedSlotTakesNoArrayValue (#6485). Measured 2026-10-15 on zsh 5.9.2, over a script's
// tie, a `private` and `HOME` alike:
//
//	set -A HOME x y            HOME: attempt to assign array value to non-array
//	read -A HOME <<< x         the same
//	: ${(A)HOME::=x}           the same
//	f(){ private v; set -A v x }   f: v: attempt to assign …
//
// The sentence is the store's and names no builtin, so the word in front is
// taken out of the location as fixedContainerKeepsItsKind does.
func (r *Runner) fixedSlotRefusesAnArrayValue(name string) bool {
	if !r.fixedSlotTakesNoArrayValue(name) {
		return false
	}
	r.reportAnArrayValueToAFixedSlot(name)
	r.fatalQuiet()
	return true
}

// fixedSlotTakesNoArrayValue is the question fixedSlotRefusesAnArrayValue
// acts on, asked without acting: whether name is a slot of a fixed kind that
// is not a container.
//
// Asked on its own by the one road whose refusal is **not** always fatal: an
// array value as a command's assignment prefix. That refusal costs what a
// prefix to a frozen name costs, which the command it stands in front of
// decides, so the prefix walk names the entry and the cost is
// Runner.refusePrefixesNow's. See Runner.prefixEntryIsFrozen.
func (r *Runner) fixedSlotTakesNoArrayValue(name string) bool {
	allowed, fixed := r.fixedSlotKinds(name)
	return fixed && !allowed.has(ArrayParameter) && !allowed.has(AssocParameter)
}

// reportAnArrayValueToAFixedSlot writes the store's sentence and nothing
// else; what the refusal costs is the caller's.
func (r *Runner) reportAnArrayValueToAFixedSlot(name string) {
	outer := r.inBuiltin
	r.inBuiltin = ""
	r.diagf("%s\n", Wording(r.diag().ArrayValueToNonArray,
		"%[1]s: attempt to assign array value to non-array", name))
	r.inBuiltin = outer
}
