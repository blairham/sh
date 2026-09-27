// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `readonly` as the declaration builtin under a second name, which is what
// one dialect's is — see Semantics.ReadonlyIsTheDeclarationWord.
//
// It is the shape `integer` already has and exists for the same reason: the
// attributes, the scope, the value a later assignment means, the kind gate a
// special parameter meets and every option letter's meaning come from the one
// place. The alternative was a second table of letters over a second operand
// loop, and this repository keeps finding what that costs — `readonly -a`
// froze a name and recorded nothing about what it was (#1554), `readonly -f`
// took the letter and reached nothing (#3192), and the special-parameter gate
// that biDeclare and biLocal both meet was walked past by this word's own
// loop because the loop was its own (#4853).
//
// What it is not is the whole of `readonly` everywhere. Five of the six
// columns read the word as POSIX's freeze — an attribute put on a name the
// shell already has — and they keep the loop in biReadonly, which carries the
// axes that are that reading's alone: ReadonlyReferenceLetter,
// ReadonlyRecordsTheCompoundAttribute, ReadonlyElement and the listing pair.
// The axis is what decides between the two, rather than the width of
// ReadonlyOptions, because a letter set is a fact about a word's spelling and
// this is a fact about what the word *is*.

// readonlyAsTheDeclaration runs `readonly` as the declaration word where the
// dialect reads it that way, and reports whether it did.
//
// The letters are the caller's, already defaulted, so the one place that
// decides which letters this builtin takes stays the one place.
func (r *Runner) readonlyAsTheDeclaration(_ context.Context, args []string, letters string) (int, bool) {
	if r.sem().ReadonlyWord != ReadonlyWordIsTheDeclaration {
		// POSIX's reading, which is the zero value: the loop in biReadonly is
		// the word, and this function decided nothing.
		return 0, false
	}
	name := r.inBuiltin
	if name == "" {
		name = "readonly"
	}
	args, f, code := r.parseDeclareFlags(name, args, letters)
	if code != 0 {
		// The same fatality `typeset` and `integer` have, asked in the same
		// place: a dialect that counts its declaration builtins among the
		// special ones ends the script on a bad letter, and this word is one
		// of them wherever this axis is yes.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalUsageQuiet()
		}
		return code, true
	}
	// The word *is* the `r` letter, so the attribute is an addition however
	// the line was written: measured 2026-09-27 on zsh 5.9.2, `v=1; readonly
	// +x v` leaves `scalar-readonly` where `typeset +x v` leaves `scalar`, so
	// a plus word takes its own letter off and still freezes.
	f.readonly = true
	// And the letter recorded the way `integer` records its own, because the
	// *company* rules read f.letters rather than the flag: this word is
	// `typeset -r` under another name, and a pair one dialect refuses must be
	// as visible under this spelling as under the other.
	if !strings.ContainsRune(f.letters, 'r') {
		f.letters += "r"
		f.letterSigns += "-"
	}
	// And `added` with it, which is the same statement said to the listing:
	// a letter naming an attribute was written under a **minus**, so a bare
	// `readonly` is the filtered listing carrying values rather than the bare
	// names the plus form selects. Without it the word lost the values it had
	// always written — measured, `v=1; readonly v; readonly` is `v=1` here
	// and in zsh 5.9.2 alike, where the name-only branch wrote `v`.
	f.added = true
	return r.declareNames(name, args, f), true
}
