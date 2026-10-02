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
	// And the scope is the axis's, which the declaration word does not ask
	// on its own: where `readonly` declares no local, it is `typeset -gr`.
	// Measured 2026-10-02 on zsh 5.9.2, `f(){ X=1; readonly X; typeset -p
	// X }` is `typeset -r X=''` and leaves the outer `X=1` alone, and
	// under POSIX_BUILTINS the same call is `typeset -g -r X=1` and the
	// outer `X` is frozen.
	if !f.global && len(r.scopes) > 0 &&
		!r.ask(r.sem().ReadonlyDeclaresALocal, "`readonly` inside a function declaring a local") {
		f.global = true
	}
	if r.unspecified {
		return r.status, true
	}
	if f.print && len(args) == 0 && r.sem().ReadonlyListing == DeclareListingCommandWord {
		// The standard's listing, where the dialect's option moved this axis
		// there: `readonly name=value`, and only for scalars. Measured
		// 2026-10-02 on zsh 5.9.2 under POSIX_BUILTINS, `readonly foo=bar
		// novalue; readonly -p` writes `readonly foo=bar` and `readonly
		// novalue`, an integer and an export alike are `readonly n=3`, and
		// `readonly -a arr=(1 2); readonly -A as=(a 1); readonly -p` writes
		// nothing at all — which is also why zsh's own read-only tables do
		// not appear in it (#5157).
		return r.declarePrintForm(nil, DeclareListingCommandWord, true, func(d declaration) bool {
			return d.readonly && r.listsThisKind(d, DeclareListingCommandWord, f.letters)
		}), true
	}
	if f.print && len(args) > 0 {
		// `-p` beside operands, asked the axis `export` asks: where the
		// letter is inert the operands are frozen rather than listed.
		// Measured 2026-10-02 on zsh 5.9.2, under POSIX_BUILTINS `X=1;
		// readonly -p X` lists nothing and a later `X=2` is `read-only
		// variable: X`. Where it narrows, the declaration's own `-p` already
		// lists only the operands.
		if _, lists := r.exportPrintWithOperands(args); !lists {
			f.print = false
		}
		if r.unspecified {
			return r.status, true
		}
	}
	return r.declareNames(name, args, f), true
}
