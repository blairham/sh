// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `set -A name value …`, which assigns an array through a name a variable
// holds.
//
// It is the thing `name=(…)` cannot do: the name is a literal there, so a
// script with the name in a variable has no other spelling. ksh93 and zsh
// have the letter and bash and dash refuse it, which makes it a dialect's
// answer rather than an axis — and the reason it is on the release bar is
// that zsh's own `add-zsh-hook` is written with it:
//
//	typeset -ga $hook
//	set -A $hook ${(P)hook} $fn
//
// The assignment itself is `Runner.setArray`, the same whole-array store
// `name=(…)` reaches, so the array base, the unique attribute, the scalar
// view, a tied scalar and the readonly refusal all come from the one place.
// What is new here is only the *parse*: a letter that takes an operand, and
// which of the words after it are values.

// setArrayOperands is the assignment, once the name and the values are known.
//
// front is the plus spelling, which replaces from the front and leaves the
// rest of the array standing — a different operation from the minus form
// rather than the same one, and the half a single implementation gets wrong.
// Measured 2026-09-06 in both shells: `set -A b 1 2 3 4 5; set +A b Q R`
// leaves `Q R 3 4 5`, and a longer list simply extends.
func (r *Runner) setArrayOperands(name string, front bool, values []string) int {
	if _, _, subscripted := r.subscriptOperand(name); subscripted {
		// Both shells take a subscripted name here and place the values from
		// that subscript on. Nothing measured needs it and the placement
		// interacts with the array base, so it is named as missing rather
		// than guessed at — the base is the whole of what could go wrong
		// silently.
		r.diagf("set: -A with a subscripted name is not implemented yet\n")
		return 2
	}
	rest, status, ended := r.builtinNames("set", []string{name}, true)
	if ended {
		// One operand and nothing behind it, so there is nothing to declare
		// before the give-up: raised straight away, and everything below
		// reads the control flag as it always did.
		r.endAfterABadName(status)
	}
	if r.ctl == controlExit || len(rest) == 0 {
		if status == 0 {
			status = 2
		}
		// The refusal still ends the shell — the words after it do not run —
		// and the number it leaves behind is 0 in the one column that
		// answers so. Where that 0 is raised back to 1 is the same
		// measurement the declaration's store next door reads, and the same
		// one this used to get wrong by asking about the route: measured
		// 2026-09-17, `( set -A 1bad v; echo x ); echo "next=$?"` is 0 in a
		// *script file* as well as under `-c`, and `set -A 1bad v && echo
		// yes` is 1 under `-c` as well as in a file (#3504).
		return r.refusalLeavesZero(r.sem().SetArrayBadNameLeavesZero,
			"a `set -A` bad name leaving 0 behind", status)
	}
	if r.assocDeclared(name) {
		// An association is a different operation under the same spelling and
		// the two shells do not agree which: zsh reads the operands as
		// key-and-value pairs — `set -A m k1 v1 k2 v2` is two entries — where
		// ksh93 stores four elements counted from zero. Neither is guessable
		// from the other, and a shell that picked one would silently answer
		// the other shell's script wrongly, so it is named as missing.
		r.diagf("set: -A over an association is not implemented yet\n")
		return 2
	}
	// The refusal stands in front of the store, the same way it stands in
	// front of `name=(…)`: storeArray keeps the scalar view in step and that
	// call *is* guarded, so a check made afterwards would print its complaint
	// with the array already written. See refuseReadonly.
	if r.refuseReadonly(name, assignedByDeclaration) {
		return 1
	}
	if front && len(values) == 0 {
		// Nothing to put at the front, which is a store that may not happen
		// at all — see emptyPrependStores, where the two columns part. Ahead
		// of the re-creation question only where it does not: a store that
		// does not happen takes nothing off the name, and `export e=(p q);
		// set +A e` lists `typeset -ax e=( p q )` in zsh and `typeset -x -a
		// e=(p q)` in ksh93.
		stores := r.emptyPrependStores(name)
		if r.unspecified {
			return r.status
		}
		if !stores {
			return 0
		}
	}
	if r.setArrayStartsTheNameOver(name, front) {
		// A re-created name keeps neither the letters that say what its
		// values are nor the export attribute, and the clearing is ahead of
		// the store so that the values land untyped: ksh93 answers `typeset
		// -i b=1; set -A b 5+5` with `typeset -a b=(5+5)` rather than folding
		// it to 10. See setArrayStartsTheNameOver.
		r.clearAttributesAReCreationDrops(name)
	}
	if r.unspecified {
		return r.status
	}
	switch {
	case front:
		r.storeArray(name, r.prependedArray(name, values))
		if r.unspecified {
			return r.status
		}
	case len(values) == 0:
		if r.ask(r.sem().SetArrayWithNoValuesUnsetsTheName,
			"`set -A name` with no values unsetting the name") {
			r.unsetName(name)
			return 0
		}
		if r.unspecified {
			return r.status
		}
		r.setArray(name, nil)
	default:
		r.setArray(name, values)
	}
	if r.assignFailed {
		return 1
	}
	return 0
}

// setArrayWithoutAName is `set -A` with nothing after it.
//
// ksh93 refuses it and says which operand is missing; zsh answers with a
// listing of every array it has, which this engine does not build. So a
// dialect that words the refusal gets its words, and one that does not has
// the listing named as missing — the listing being the thing it would have
// had to write.
func (r *Runner) setArrayWithoutAName(on bool) int {
	sign := "+"
	if on {
		sign = "-"
	}
	if msg := r.diag().SetArrayNeedsAName; msg != "" {
		r.saySetRefusal(Wording(msg, "", sign+"A"), true, false)
		// The letter's pair and not the name's: `-A` is a letter, and what
		// was refused is the letter's own usage. ksh93 and zsh are the only
		// dialects with `set -A` at all and both answer the two spellings
		// alike, so nothing measurable rides on it — which is the reason to
		// write down which one this is rather than take whichever was
		// nearest. See Semantics.BadSetOptionLetterFatal.
		r.setRefusalStatus(refusedOptionLetter, "a `set -A` with no name ending the script")
		return r.setOptionFailure()
	}
	r.diagf("set: %sA: a listing is not implemented yet\n", sign)
	return 2
}

// setArrayLetter reports whether this dialect's `set` has the `-A` letter at
// all.
//
// Read rather than asked, because where the answer is not yes the letter is
// somebody else's *invalid option* and the refusal already there is that
// shell's real answer — `set: -A: invalid option` and a usage block in bash,
// `Illegal option -A` in dash. Asking an axis would replace a correct answer
// with a complaint about a missing dialect.
func (r *Runner) setArrayLetter() bool { return r.sem().SetArrayLetter == Yes }

// setArrayStartsTheNameOver reports whether this store re-creates the name
// rather than replacing its elements, having asked the dialect.
//
// `set -A` reaches the array store by a road of its own, and until #4768 that
// road never asked the question at all: `export a=1; set -A a x y` kept the
// export attribute here where both shells that have the letter take it off.
// The same three axes answer it as answer the literal — the disagreement is
// about what counts as a *re-creation*, not about which spelling wrote it —
// and the two routes are measured here rather than assumed to agree.
//
// Measured 2026-09-27, `env -i PATH=/usr/bin:/bin`, from a script file, on
// zsh 5.9.2 (aarch64-apple-darwin25.4.0) and ksh93u+ 2012-08-01, each read
// back with that shell's own `typeset -p`. `x` is the export attribute
// surviving; `·` is it gone:
//
//	                                              zsh   ksh93
//	export a=1;     set -A a x y                   ·      ·
//	export a=(p q); set -A a x y                   x      ·
//	typeset -a a; export a; set -A a x             x      ·
//	export a=1;     set +A a x y                   ·      x
//	export a=(p q); set +A a x y                   x      x
//
// Row one is ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver (both yes),
// rows two and three are ArrayLiteralAssignmentStartsTheNameOver (zsh no,
// ksh93 yes), and row four is
// AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver (zsh yes, ksh93
// no). Row five is the shape every column agrees about, as it is for `a+=(…)`.
// The type letters move with the export attribute in every one of the ten
// cells — `typeset -i b=1; set -A b 5+5` is `typeset -a b=(5+5)` in ksh93 and
// `typeset -l c=AB; set -A c CD` is `typeset -a c=( CD )` in zsh — which is
// what says this is the re-creation question and not an export rule.
//
// **One thing the literal does that this does not**, and it is measured
// rather than inherited: arrayLiteralStartsTheNameOver never asks about a
// name whose array letter was written and which is holding nothing yet,
// because the first literal such a name receives keeps the letter in both
// shells. `set -A` is not that — ksh93 drops the attribute on row three above
// where `typeset -a a; export a; a=(x)` keeps it — so the axis is asked for
// any array here, and only the *append* form is exempt. Carrying the
// literal's guard across would have left row three answering keep in both
// columns, which is the wrong answer in one of them.
func (r *Runner) setArrayStartsTheNameOver(name string, front bool) bool {
	if !r.nameCarriesAnAttributeAReCreationDrops(name) {
		// Nothing on the name to lose, so the question cannot be seen and is
		// not put — the same guard the literal keeps, and the same list, so
		// the two cannot come apart.
		return false
	}
	if !r.nameIsAnArray(name) {
		if front {
			return r.ask(r.sem().AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
				"a `set +A` re-creating a name that is not an array")
		}
		return r.ask(r.sem().ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
			"a `set -A` re-creating a name that is not an array")
	}
	if front {
		// An append over an array it is already holding never re-creates it,
		// which is unanimous for `a+=(…)` and measured here for `set +A`.
		return false
	}
	return r.ask(r.sem().ArrayLiteralAssignmentStartsTheNameOver,
		"a `set -A` re-creating the name it writes")
}

// prependedArray is the plus form's store: the values written laid over the
// front of what the name is holding, with the rest left standing.
//
// **The values go through the name's folding attributes**, which is the half
// this took a road of its own around. `setArray` next door runs every element
// it is given through compoundElemsFolded, and the array literal reaches the
// same fold; this built its elements by hand, so the one store that did not
// ask was the one spelling nothing else in the tree shares — the second-helper
// shape, a store written beside the one that carries the fold and without it.
//
// Measured 2026-09-27 on `/bin/ksh`, `Version AJM 93u+ 2012-08-01`, from a
// script file under `env -i PATH=/usr/bin:/bin`, each row read back with
// `typeset -p`:
//
//	typeset -u d=ab;      set +A d cd     typeset -a -u d=(CD)
//	typeset -a -u e=(ab); set +A e cd     typeset -a -u e=(CD)
//	typeset -l h=AB;      set +A h CD     typeset -a -l h=(cd)
//	typeset -i i=1;       set +A i 5+5    typeset -a -i i=(10)
//
// and the append spelling of the first two, which is the control and has
// always agreed: `typeset -u d=ab; d+=(cd)` is `typeset -a -u d=(AB CD)`.
// This shell wrote `(cd)`, `(CD)` and `(5+5)` — the value unfolded in every
// row while the letter it was supposed to go through survived beside it
// (#4809).
//
// The fold reaches the **arriving** values and not the standing ones: what
// the name is already holding was folded when it was stored, and in the one
// column that keeps a width attribute's presentation in the store those
// elements are already presented. Both readings agree on every row above,
// which is why this is stated rather than left to whichever was nearer.
//
// zsh is the other column and is quiet here for a reason of its own rather
// than by not being asked: its case letters fold on the *read* — see
// Semantics.CaseAttributeFoldsWhenRead — so there is nothing for a store to
// do, and its `set +A` over a name that is not already an array re-creates
// the name and takes the letter off before the values land.
func (r *Runner) prependedArray(name string, values []string) Array {
	front := NewArray(len(values))
	for i, v := range values {
		front.Set(i, Scalar(v))
	}
	front = r.compoundElemsFolded(name, front)
	a := NewArray(r.Arrays[name].Len())
	for k, v := range r.Arrays[name].All() {
		a.Set(k, v)
	}
	for k, v := range front.All() {
		a.Set(k, v)
	}
	return a
}

// emptyPrependStores reports whether `set +A name` with no values behind it
// writes anything at all.
//
// Over a name that is **already an array** it does not, in either column, and
// that is the row interp/setarray.go recorded as unanimous for the whole
// question: `export e=(p q); set +A e` leaves `typeset -ax e=( p q )` in zsh
// and `typeset -x -a e=(p q)` in ksh93, the export attribute and every element
// untouched.
//
// Over a name that is **not** an array the two columns part, and that row had
// never been measured. Both references from a script file under `env -i
// PATH=/usr/bin:/bin`, 2026-09-27 — `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f`, and `/bin/ksh`, Version AJM 93u+
// 2012-08-01 — each row read back with that shell's own `typeset -p`:
//
//	                                zsh                 ksh93
//	s=v;          set +A s          typeset -a s=(  )   s=v
//	export e=1;   set +A e          typeset -a e=(  )   typeset -x e=1
//	typeset -i n=3; set +A n        typeset -a n=(  )   typeset -i n=3
//	unset u;      set +A u          typeset -a u=(  )   the name is absent
//	export f=(p q); set +A f        typeset -ax f=( p q )  typeset -x -a f=(p q)
//
// So zsh makes the name an empty array — dropping the export attribute and
// the integer letter doing it, which is the re-creation
// AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver already records
// — and ksh93 leaves the name exactly as it was, down to a name that is not
// there at all. The last row is the control and is what keeps this narrow.
//
// The empty prepend is a spelling nobody writes on purpose: it is what a loop
// produces when its list came out empty, and the difference is one empty array
// against an untouched scalar (#4810).
func (r *Runner) emptyPrependStores(name string) bool {
	if r.nameIsAnArray(name) {
		return false
	}
	return r.ask(r.sem().SetArrayEmptyPrependMakesANonArrayAnEmptyArray,
		"a `set +A name` with no values making a name that is not an array an empty array")
}
