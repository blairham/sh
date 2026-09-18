// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// badSubscriptGivesUp writes a complaint about a subscript a builtin could not
// evaluate and then gives up exactly as much as the dialect gives up.
//
// One door for the three sites that ask, because they are the same failure
// seen from three builtins and the three outcomes have to mean the same thing
// at all of them: see BadSubscriptPolicy for what each one is and where it was
// measured. Before this, `unset` asked a bool that read "fatal" and the store
// asked nothing at all and was fatal unconditionally — so bash ended a script
// it carries on with, and ksh93 ended one it does not stop at all (#3485) —
// and a *declaration* reached Runner.fatal directly, so bash ended a script it
// runs to the end there too (#3495).
//
// The status is 1 wherever the shell is still running to see it, which is
// unanimous: the complaint leaves a failed builtin behind, and the next
// command reads 1 whether it got there by the builtin returning or by the
// abandoned command's line ending. The two give-up branches take the dialect's
// own fatal status instead, which is the same 1 everywhere it can be observed
// — only dash and BusyBox ash answer that axis differently and neither reaches
// here.
func (r *Runner) badSubscriptGivesUp(p BadSubscriptPolicy, what, sentence string) int {
	if p == BadSubscriptUnspecified {
		// The refusal alone, and not the sentence under it: a shell that was
		// never told what to do here has not decided to complain about the
		// subscript, it has failed to answer a question. Status 2 and the
		// unspecified flag are what every other refused axis leaves, and the
		// caller reads the flag to keep from reporting twice.
		r.diagf("%s\n", r.unanswered(what))
		r.status, r.unspecified = 2, true
		return 2
	}
	r.diagf("%s\n", sentence)
	switch p {
	case BadSubscriptEndsTheScript:
		r.fatalQuiet()
		return r.status
	case BadSubscriptAbandonsTheCommand:
		// The same door the sites that *expand* a subscript go through, `-c`
		// rule and all: see Runner.giveUpForABadSubscript, which is where
		// the rule this branch used to spell out by hand now lives. It is
		// what takes the rest of the line with it, so `unset 'q[b c]'; echo
		// x` prints no x and the next *line* runs.
		r.giveUpForABadSubscript()
		return r.status
	}
	return 1
}

// badSubscriptToADeclaration is the third site: a declaration whose operand
// names an element and whose subscript will not evaluate.
//
// `declare 'a[b c]'=v`, and `typeset`, `local`, `readonly` and `export` in
// whichever dialects let those take a subscript at all. It reached
// Runner.fatal until now, which is zsh's and ksh93's answer written on the
// common path: bash gives up the command it is running and carries on at the
// next top-level one, so a script bash runs to the end stopped here. See
// Semantics.BadSubscriptToADeclaration for the rows and for why this is a
// field of its own rather than either neighbor read a third time.
//
// The sentence is the language's and names no builtin, which is bash's and
// zsh's wording and is what the two sites next door already write. ksh93 puts
// the builtin in front of it and reports from the builtin's own location; that
// is #3496, which owns all three sites at once and is wording rather than
// unwinding.
//
// Nothing comes back, as nothing comes back from declareElement: the status is
// written here, and assignFailed is what keeps the builtin's own return value
// from zeroing it — the same pair changeCompoundKind's abandoning branch sets,
// one refusal over.
func (r *Runner) badSubscriptToADeclaration(sub string, err error) {
	// The builtin that was handed the operand: put out of the location by
	// declareElement and kept here for the two claims one column makes — the
	// name in the sentence, through Diagnostics.DeclarationBadSubscript, and
	// the builtin's own location, through keptBuiltinLocation. `typeset` lost
	// both where `unset` and `read` kept them (#3496).
	builtin := r.declarationSpeaker
	outer := r.inBuiltin
	r.inBuiltin = r.keptBuiltinLocation(builtin)
	defer func() { r.inBuiltin = outer }()
	r.status = r.badSubscriptGivesUp(r.sem().BadSubscriptToADeclaration,
		"how much a declaration gives up for an operand's unevaluable subscript",
		Wording(r.diag().DeclarationBadSubscript, "%[2]s",
			builtin, r.subscriptFailure(sub, err)))
	r.assignFailed = true
}

// valuelessSubscriptedOperand is what a declaration does with an operand that
// names an element and carries no value.
//
// It reports whether the operand is **finished**, and where it is not, the
// name and the letters the caller should declare instead — which is the base
// name with the array letter added, because the brackets are what say the name
// is an array. That was the `typeset`/`declare` site's own code and the other
// three spellings did not have it, so each of them declared a variable
// literally named `a[1]` — invisible to `${a[1]}` and to `typeset -p a`, frozen
// instead of the array, and exported under a name no environment can carry.
// #1380 fixed one spelling of four.
//
// One door for all four — `typeset`/`declare`, `local`, `readonly` and
// `export` — because the answer is about the declaration and not about which
// word spells it, and because a second copy that omitted a case is the failure
// this repository keeps making.
//
// See Semantics.ValuelessSubscriptedOperand for the three answers and where
// each was measured. Before this the brackets were never read in any dialect:
// `typeset 'a[b c]'` was silent at 0 where zsh and ksh93 both end the script,
// and in the zsh column the operand reached the valueless-declaration listing
// and printed the whole array on its way past (#3501).
func (r *Runner) valuelessSubscriptedOperand(base string, subs []string, f declareFlags, shadows bool,
) (name string, letters declareFlags, done bool) {
	sub := subs[len(subs)-1]
	// The array letter whether or not one was written — **unless the name is
	// already a table**, where the brackets say nothing of the kind and the
	// letter would ask for a conversion no shell performs here: measured
	// 2026-09-17, `typeset -A m; typeset 'm[b c]'` is silent at 0 in bash and
	// puts the key in with an empty value in zsh and ksh93, while this
	// refused it as `cannot convert associative to indexed array` at 1 — and
	// ended the script for it in the ksh column.
	letters = f
	letters.array = letters.array || (!letters.assoc && !r.assocDeclared(base))
	switch r.sem().ValuelessSubscriptedOperand {
	case ValuelessSubscriptedOperandDeclaresTheName:
		// Even the column that reads no subscript refuses an **empty** one,
		// and by the whole operand: measured, bash's `declare 'a[]'` is
		// ``declare: `a[]': not a valid identifier`` at 1 where `declare
		// 'a[3]'` is silent (#3509).
		if r.emptyDeclarationSubscript(base, sub, false) {
			return "", letters, true
		}
		return base, letters, false
	case ValuelessSubscriptedOperandReadsTheSubscript:
		if r.emptyDeclarationSubscript(base, sub, false) {
			return "", letters, true
		}
		if r.assocDeclared(base) {
			// A table's brackets hold a key rather than an expression, in
			// every column: measured 2026-09-17, `typeset -A m; typeset
			// 'm[b c]'` is silent at 0 in bash and puts the key in with an
			// empty value in zsh and ksh93 alike, with no arithmetic
			// anywhere. So this column writes the key too, which is the one
			// place its answer and the element-writing one coincide — and
			// the reason the constant's name is about the *subscript* rather
			// than about never writing anything.
			r.setAssocElem(base, sub, "")
			return "", letters, true
		}
		if _, err := r.subscriptValue(sub); err != nil {
			r.badSubscriptToADeclaration(sub, err)
			return "", letters, true
		}
		// **Twice**, and that is measured rather than a slip. The column
		// that reads a valueless operand's subscript reads it on two passes,
		// so a side effect in the brackets fires twice: measured 2026-09-17,
		// ksh93u+, `c=(1 2 3); i=0; typeset 'c[i++]'` leaves `i` at 2, and
		// so do the `readonly` and `export` spellings. It is this operand
		// shape alone — the same brackets with a value are read once
		// (`typeset 'c[i++]'=v` leaves 1), and so are `unset 'c[i++]'` and
		// the expansion `${c[i++]}`.
		//
		// The second pass is discarded, which is the whole of it: the value
		// was already found and a failure was already answered above, so
		// nothing but the side effect can come of it. See #3511, whose other
		// three rows are about what this column *records* and are not here.
		_, _ = r.subscriptValue(sub)
		return base, letters, false
	case ValuelessSubscriptedOperandWritesTheElement:
		// The empty-value form under another spelling, which is measured
		// rather than inferred — so it goes through the one path a value
		// goes through, refusals, table keys, array growth and all. The
		// letters there are the *element* declaration's, not the name's.
		r.declareElement(base, subs[:len(subs)-1], sub, "", f, shadows)
		return "", letters, true
	}
	r.diagf("%s\n", r.unanswered(
		"what a declaration does with a subscripted operand that carries no value"))
	r.status, r.unspecified = 2, true
	return "", letters, true
}

// emptyDeclarationSubscript is a declaration operand whose subscript is
// **empty** — `typeset 'a[]'=v`, which is what `typeset "a[$i]"=v` is once a
// blank `$i` has gone in, the parameters going in before the brackets are
// read. It reports whether the operand was refused here.
//
// Semantics.EmptyArithSubscript is the axis, read rather than a fourth field
// of its own: every shell that reaches a declaration gives it the disposition
// it gives the same brackets in an expression, which is the same reading
// Diagnostics.ArithEmptySubscriptTarget records for `(( a[] = 4 ))`. ksh93
// takes the brackets as the empty expression, which is element zero, and
// writes it; bash reports and writes nothing; zsh refuses and the script ends.
//
// Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin /dev/null,
// a script file, `a=(1 2 3)` in front of each:
//
//	                     typeset 'a[]'=v            typeset 'a[]'
//	bash 5.3.20          reported at 1, no write    reported at 1
//	bash 3.2.57          the same sentence at 0     silent at 0
//	zsh 5.9.2            the script ends at 1       the script ends at 1
//	ksh93u+              element zero is written    silent at 0
//
// Here it wrote element zero in **every** dialect and said nothing, so
// `typeset "a[$i]"=v` with a blank `$i` quietly replaced the array's first
// element at status 0 (#3509). bash 3.2's status is the one row not matched;
// the preset is 5.3's.
//
// Behind elementDeclarationRefused rather than in front of it, which is
// measured: zsh's `readonly 'a[]'=v` is `can't create readonly array
// elements` and not this sentence, so the attribute refusals answer first.
func (r *Runner) emptyDeclarationSubscript(base, sub string, hasValue bool) bool {
	if !r.operandEmptySubscript(sub) {
		// Either the brackets are not empty, or they hold an expression that
		// happens to be empty — which is zero, so the operand is element zero
		// and there is nothing to refuse. The one test every operand route
		// shares; see operandEmptySubscript. The caller carries on.
		return false
	}
	// The sentence names no builtin in its *location*, which is the rule the
	// store's complaints in declareElement follow one refusal over: measured,
	// zsh writes `./f.sh:2: not an identifier: a[]` where its own bad-name
	// refusal writes `./f.sh:typeset:1: not an identifier: 1x`. bash names
	// the builtin inside the sentence instead, where it names one at all.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	wording, fallback := r.diag().DeclarationEmptySubscript, "%[1]s[]: bad array subscript"
	if !hasValue {
		wording, fallback = r.diag().ValuelessDeclarationEmptySubscript,
			"%[2]s: `%[1]s[]': not a valid identifier"
	}
	switch r.sem().EmptyArithSubscript {
	case EmptyArithSubscriptIsReported:
		// Reported, and the element is not written: the operand costs the
		// builtin's status and the rest of the line still runs.
		r.diagf("%s\n", Wording(wording, fallback, base, outer))
		r.status, r.assignFailed = 1, true
		return true
	case EmptyArithSubscriptIsInvalid:
		r.fatal("%s\n", Wording(wording, fallback, base, outer))
		return true
	}
	r.diagf("%s\n", r.unanswered("a subscript written with nothing in it"))
	r.status, r.unspecified = 2, true
	return true
}

// operandEmptySubscript reports whether a **builtin's operand** carries a
// subscript that is empty and that this column does not read as the empty
// expression — `unset 'a[]'`, `read 'a[]'`, `printf -v 'a[]'` and
// `typeset 'a[]'=v`, which is what `unset "a[$i]"` and the rest are once a
// blank `$i` has gone in, the word being expanded before the builtin sees it.
//
// One test for all four routes, because the brackets are the same brackets and
// a route left out is a route that goes on writing element zero in silence. It
// says only that the operand names no element *here*; what each builtin does
// about that is measured per builtin below, and the three columns do not agree
// on any of it.
//
// Semantics.EmptyArithSubscript is the axis, read rather than a field of its
// own: every shell gives a builtin's operand the disposition it gives the same
// brackets in an expression. ksh93 takes them as an expression that happens to
// be empty, which is zero, and acts on element zero everywhere; bash acts on
// nothing and says so where there is something to say; zsh refuses the
// subscript. Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin
// /dev/null, a script file, `a=(1 2 3)` in front of each, with the status read
// on the *same* line and the array on the next:
//
//	                     unset 'a[]'                read 'a[]' / printf -v 'a[]'
//	bash 5.3.20          silent at 0, nothing gone  the builtin's bad-name
//	                                                refusal, nothing written
//	bash 3.2.57          `unset: `a[]': not a       the same as 5.3
//	                     valid identifier` at 1
//	zsh 5.9.2            `invalid subscript` at 1,  `not an identifier: a[]`
//	                     the line carries on        and the script ends
//	ksh93u+              element zero is removed    element zero is written
//
// bash 3.2's `unset` is the one row not matched; the preset is 5.3's.
//
// **The emptiness has to be in the operand as the builtin receives it**, which
// is a different construct from a subscript whose *text* expanded to nothing
// and is measured apart from it: `i=; unset 'a[$i]'`, single-quoted so the `$i`
// reaches the builtin, removes element **zero** in bash 5.3.20 where
// `unset 'a[]'` removes nothing, and zsh writes the arithmetic reader's
// `bad math expression: empty string` there where it writes `invalid subscript`
// here. That neighbor is Semantics.EmptySubscriptTextIsAMathError and this
// must not answer for it — hence the test on the operand's own text.
//
// **And a blank subscript is not an empty one.** `a[ ]` holds whitespace and
// reaches Semantics.BlankArithSubscriptIsTheEmptyExpression: measured, bash's
// `unset 'a[ ]'` removes element 0 where `unset 'a[]'` removes nothing, and
// zsh answers the first `operand expected at end of string` and the second
// `invalid subscript`. subscriptOperandText is what keeps the one from arriving
// as the other (#3509).
func (r *Runner) operandEmptySubscript(sub string) bool {
	return sub == "" && r.sem().EmptyArithSubscript != EmptyArithSubscriptIsTheEmptyExpression
}

// unsetEmptySubscript is `unset 'a[]'`, and reports whether the operand was
// answered here along with the status it leaves.
//
// A **delete** is the one route where the column that complains has nothing to
// complain about: the brackets name no element, and removing no element is not
// a failure. Measured, bash 5.3.20's `a=(1 2 3); unset 'a[]'` is silent at 0
// with all three elements standing, and `readonly a; unset 'a[]'` is silent at
// 0 as well — so the array is never reached, which is what tells that row from
// a refusal that happens to be quiet. The array's freeze is kept out of the
// way for it in unsetBuiltin, ahead of this.
//
// The refusing column writes the **read's** sentence here, not the write's:
// `unset` reads the brackets to find the element it is to remove, and zsh says
// `invalid subscript` of them — the same words Diagnostics.ArithEmptySubscript
// already holds for `$(( a[] ))`, where `read 'a[]'` one route over gets
// `not an identifier: a[]`, the write's. So the wording is read from the
// existing pair rather than duplicated into two more fields.
//
// How much the refusal gives up is the route's own axis and not this one:
// Semantics.BadSubscriptToUnset, which the unevaluable subscript next door
// already asks, answers `unset` with a failed builtin and the next command
// still running in the column that refuses — measured, zsh's line carries on
// to print its own status.
//
// Before this every dialect removed element **zero** and said nothing, so
// `unset "a[$i]"` with a blank `$i` quietly deleted the array's first element
// at status 0 (#3513).
func (r *Runner) unsetEmptySubscript(base, sub string) (handled bool, code int) {
	if !r.operandEmptySubscript(sub) {
		return false, 0
	}
	if r.sem().EmptyArithSubscript == EmptyArithSubscriptIsReported {
		// Nothing removed and nothing said. See above: the complaint this
		// column makes about an expression's empty brackets is about a value
		// it still has to produce, and a delete has none.
		return true, 0
	}
	// The sentence is the language's and names no builtin in its location,
	// which is what badSubscriptToUnset does one refusal over for the same
	// operand: measured, zsh writes `./f.sh:2: invalid subscript` where its
	// own `unset` bad-name refusal writes `./f.sh:unset:1: 1x: invalid
	// parameter name`.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	if r.sem().EmptyArithSubscript != EmptyArithSubscriptIsInvalid {
		r.diagf("%s\n", r.unanswered("a subscript written with nothing in it"))
		r.status, r.unspecified = 2, true
		return true, 2
	}
	return true, r.badSubscriptGivesUp(r.sem().BadSubscriptToUnset,
		"how much an `unset` operand's unevaluable subscript gives up",
		Wording(r.diag().UnsetBadSubscript, "%[1]s",
			Wording(r.diag().ArithEmptySubscript, "invalid subscript", base)))
}

// storeOperandEmptySubscript is `read 'a[]'` and `printf -v 'a[]'` — a
// builtin's **output operand** with an empty subscript — and reports whether
// the operand was refused here along with the status the builtin carries.
//
// In front of the table's key path rather than behind it, which is measured
// and is the opposite of what the delete does: bash's `typeset -A m; read
// 'm[]'` is refused as a name, sentence and all, and zsh's ends the script,
// where `unset 'm[]'` on the same table is silent at 0 in both and leaves an
// empty key standing in zsh. A store has nowhere to put the value either way,
// so the brackets are answered before anything asks what kind of array it is;
// only ksh93 reaches the key, and it reaches it as the empty expression.
//
// The column that acts on nothing refuses the **whole operand as a name**, and
// that is not an approximation of the bad-name refusal but the same refusal:
// measured 2026-09-17, bash 5.3.20 answers `read 'a[]'` and `read '1x'` with
// one sentence — `read: 'a[]': not a valid identifier`, with bash's own
// leading backquote in place of the first quote — at 1, and `printf -v 'a[]'`
// and `printf -v '1x'` with that sentence under printf's name at **2**, the
// same refusal in each pair. The status is the builtin's own, which is why it
// is read
// from Diagnostics.BuiltinBadNameStatusFor and not from this axis. It stands
// ahead of the freeze as well: `readonly a; read 'a[]'` is the name refusal
// and not `a: readonly variable`, which frozenReadName is what arranges.
//
// The refusing column writes the **write's** sentence — zsh's `not an
// identifier: a[]`, which Diagnostics.ArithEmptySubscriptTarget already holds
// for `(( a[] = 4 ))` — and gives up as much as
// Semantics.BadSubscriptToAnOutputOperand says, which in that column is the
// script. Both read from the fields the expression's write already uses; the
// two sentences and the two give-ups are what part `unset` from a store here.
//
// Before this every dialect wrote element **zero**, so `read "a[$i]"` with a
// blank `$i` quietly replaced the array's first element at status 0 (#3513).
func (r *Runner) storeOperandEmptySubscript(base, operand, sub, builtin string) (status int, refused bool) {
	if !r.operandEmptySubscript(sub) {
		return 0, false
	}
	switch r.sem().EmptyArithSubscript {
	case EmptyArithSubscriptIsReported:
		// The builtin's bad-name refusal, quoting the operand back whole and
		// carrying that builtin's own status — and not fatal, which is what
		// this axis value says everywhere: the complaint is made and the
		// input survives it. The one column that ends a script here answers
		// the axis below instead.
		return r.badBuiltinName(builtin, operand, operand, No), true
	case EmptyArithSubscriptIsInvalid:
		return r.storeRefusalStatus(builtin, r.badSubscriptGivesUp(
			r.sem().BadSubscriptToAnOutputOperand,
			"how much a store through a builtin's operand gives up for an unevaluable subscript",
			Wording(r.diag().StoreOperandBadSubscript, "%[2]s", builtin,
				Wording(r.diag().ArithEmptySubscriptTarget,
					"not an identifier: %[1]s[]", base)))), true
	}
	r.diagf("%s\n", r.unanswered("a subscript written with nothing in it"))
	r.status, r.unspecified = 2, true
	return 2, true
}

// keptBuiltinLocation is the speaker a site leaves in place while it puts the
// builtin's name out of the *sentence*.
//
// Three sites report a complaint the **language** makes through a builtin —
// `unset 'a[b c]'`, `read 'a[b c]'` and `typeset 'a[b c]'=v` — and all three
// cleared the speaker outright. That is two claims at once: the name leaves
// the sentence, and the location falls back from the builtin's style to the
// shell's. One column makes only the first, so the second is a value and this
// is the one door it is read through.
//
// See Diagnostics.BadSubscriptKeepsTheBuiltinsLocation for the rows and for
// the control line that tells the two claims apart.
func (r *Runner) keptBuiltinLocation(builtin string) string {
	if r.diag().BadSubscriptKeepsTheBuiltinsLocation {
		return builtin
	}
	return ""
}
