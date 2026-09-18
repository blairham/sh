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
	r.status = r.badSubscriptGivesUp(r.sem().BadSubscriptToADeclaration,
		"how much a declaration gives up for an operand's unevaluable subscript",
		r.subscriptFailure(sub, err))
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
	if sub != "" {
		return false
	}
	if r.sem().EmptyArithSubscript == EmptyArithSubscriptIsTheEmptyExpression {
		// The brackets hold an expression that happens to be empty, which is
		// zero — so the operand is element zero and there is nothing to
		// refuse. The caller carries on.
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
