// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// printfCount is the `%n` directive: it writes nothing of its own and stores
// the number of bytes the format has produced so far in this pass into the
// parameter its operand names.
//
// The count is read off the pass's own writer rather than kept beside it,
// which is what makes it the right number without a second accounting to keep
// in step: printfWriter.mark is already `written + len(pending)`, the place a
// rewind names, and a new writer is built for every pass — so `printf '%s%n'
// a n1 b n2` leaves 1 in each name rather than 1 and 2, which is what the
// three columns that have the directive do.
//
// present parts an operand that is absent from one that is there and empty.
// They are the same string and they are not the same question: an absent
// operand is silent everywhere, and an empty one is refused in two of the
// three columns. See Semantics.PrintfCountEmptyNameIsIgnored.
func (r *Runner) printfCount(name string, present bool) (int, bool) {
	if !present {
		// `printf 'abc%n'` — nothing to store, nothing to say, and the
		// format runs on. Unanimous in bash 5.3.20, zsh 5.9.2 and ksh93u+,
		// so no dialect is consulted for it.
		return 0, false
	}
	if name == "" && r.ask(r.sem().PrintfCountEmptyNameIsIgnored,
		"an empty `%n` operand being nothing to store rather than a name that is not one") {
		return 0, false
	}
	if r.unspecified {
		return r.status, true
	}
	if !r.isPrintfCountName(name) {
		if r.unspecified {
			return r.status, true
		}
		code := r.badPrintfCountName(name)
		return code, r.ask(r.sem().PrintfCountBadNameStopsThePass,
			"a `%n` operand that is not a name giving up the rest of the format")
	}
	if !r.readMayWrite(name) {
		// The freeze has already been reported, and in the one dialect whose
		// refusal ends the script it has already said so. What is left is
		// what the *builtin* does with it, which is the axis below: one
		// column finishes the format and reports the format's own status.
		if r.ask(r.sem().PrintfCountFrozenNameStopsThePass,
			"a frozen `%n` operand giving up the rest of the format") {
			return 1, true
		}
		return 0, false
	}
	// The attribute is decided before the store, because one reading of it
	// turns on whether the shell already had the name and the store is what
	// makes it. Resolved here rather than after, so a name that is new is
	// still new when the question is asked.
	attribute := r.countAttribute()
	if r.unspecified {
		return r.status, true
	}
	fresh := !r.nameIsSet(name)
	count := 0
	if r.printfOut != nil {
		// The pass's own writer already holds the number: mark is
		// `written + len(pending)`, the place a rewind names, and a new
		// writer is built for every pass. A second tally kept beside it
		// would be a second thing to keep in step.
		count = r.printfOut.mark()
	}
	if st, refused := r.storeThroughOperand(name, itoa(count)); refused {
		return st, true
	}
	if attribute == PrintfCountIntegerAlways ||
		(attribute == PrintfCountIntegerOnANewName && fresh) {
		// Only a plain name: where the operand named an element, the
		// attribute belongs to the array and no column that takes a
		// subscript here also gives one out. The one that takes subscripts
		// is the one whose reading is "a new name", and an element of an
		// array the script wrote is not a new name.
		if isPlainName(name) {
			if r.integer == nil {
				r.integer = map[string]bool{}
			}
			r.integer[name] = true
		}
	}
	return 0, false
}

// isPrintfCountName is the name question at `%n`, and it is the same rule as
// every other output operand's with a subscript gate of its own.
//
// **The gate is why this is not isPrintfName.** bash fills `printf -v 'a[0]'`
// and refuses `printf 'abcd%n' 'arr[2]'`, so StoreOperandTakesASubscript —
// which is Yes there, and is what isPrintfName reads — would answer this
// route wrongly. It is the shape isGetoptsName already has, for the same
// reason. See Semantics.PrintfCountOperandTakesASubscript.
func (r *Runner) isPrintfCountName(name string) bool {
	if base, _, subscripted := r.subscriptOperandRead(name,
		r.outputOperandBracketsAreLexed(name)); subscripted && isPlainName(base) &&
		r.sem().PrintfCountOperandTakesASubscript != No {
		return true
	}
	return r.isBuiltinName("printf", name, r.sem().ReadNameOperands)
}

// badPrintfCountName reports an operand `%n` cannot store through, through the
// same wording table and the same fatality gate as every other bad name.
//
// Two things are the route's rather than the builtin's, and both are measured
// rather than inherited. The **status** is Diagnostics.PrintfCountBadNameStatus
// where a dialect has one, because bash's `printf -v` bad name is that
// builtin's usage number and this one is not. And a **subscripted** operand is
// a sentence of its own in one column, which is
// Diagnostics.PrintfCountNameIsAnArray.
func (r *Runner) badPrintfCountName(name string) int {
	d := r.diag()
	if w := d.PrintfCountNameIsAnArray; w != "" && strings.ContainsRune(name, '[') {
		r.diagf("%s\n", Wording(w, "printf: %[1]s: cannot be an array", name))
		return orDefault(d.PrintfCountBadNameStatus, 1)
	}
	status := r.badBuiltinName("printf", name, name, r.sem().BadNameToPrintfFatal)
	own := d.PrintfCountBadNameStatus
	if own == 0 || own == status {
		return status
	}
	if r.ctl == controlExit {
		// The refusal ended the script, so the number it left behind is the
		// one the shell exits with and has to move with the return.
		r.status = own
	}
	return own
}
