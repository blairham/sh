// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A declaration's own array literal over a name that is not an array, and
// carries a type letter from an earlier line, re-creates the name exactly as
// the bare `z=(…)` does — Semantics.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
// whose answer it reuses.
//
// Measured 2026-10-03, script files, `typeset -p z` after each:
//
//	                                       zsh 5.9.2        ksh93u+          bash 5.3.20
//	typeset -i z; typeset z=(1 2)          -a, letter gone  -a, letter gone  -ai, kept
//	typeset -i z=3; typeset z=(1 2)        the same         the same         the same
//	typeset -i z; typeset -a z=(1 2)       -a               -a               -ai
//	typeset -l z; typeset -x z=(A B)       -ax              -x -a            -axl, folded
//	typeset -i z; export z=(1 2)           -ax              -x -a            -aix
//	typeset -i z; typeset -i z=(1 2)       refused          -a -i            -ai
//
// So it is the same split as the bare assignment's and the same answers. The
// last row is the bound: a line that writes a type letter of its own is not
// this question — ksh93 keeps the letter the line wrote and zsh refuses the
// pair outright, which is Semantics.TypeLetterAndAnArrayLiteralIsAnInconsistentType's.
//
// **The export is not part of it.** A bare `z=(1 2)` over an exported typed
// name drops the export in both re-creating shells, and a declaration splits
// them: `typeset -ix z; typeset z=(1 2)` lists `-ax` in zsh and `-a` in
// ksh93, and both keep an `x` the line writes itself. So the operand form takes
// the type letters away and leaves the export where it was, which is zsh's
// answer and the one this engine already gave in both; ksh93's dropped export
// on that one shape is not modeled.
//
// Decided before the utility runs, because by the time the literal is stored
// the line's own `-a` has made the name an array and the "not an array"
// question can no longer be asked of it.

// typeLettersOfALine are the letters that say what a name's values are, which
// a line writing any of them under a minus keeps out of this question.
const typeLettersOfALine = "iluFEXLRZc"

// literalOperandsOverANonArray is the array-literal operands that are not an
// array before the line runs and carry an attribute a re-creation drops, where
// the line writes no type letter itself.
func (r *Runner) literalOperandsOverANonArray(argv []string) map[string]bool {
	if len(r.literalOperands) == 0 || lineWritesATypeLetter(argv) {
		return nil
	}
	var out map[string]bool
	for name := range r.literalOperands {
		if r.nameIsAnArray(name) || !r.nameCarriesAnAttributeAReCreationDrops(name) {
			continue
		}
		if out == nil {
			out = map[string]bool{}
		}
		out[name] = true
	}
	return out
}

// lineWritesATypeLetter reports a minus option word, ahead of the first
// operand, carrying one of typeLettersOfALine.
func lineWritesATypeLetter(argv []string) bool {
	for _, w := range argv[1:] {
		if w == "--" || len(w) < 2 || (w[0] != '-' && w[0] != '+') {
			return false
		}
		if w[0] == '-' && strings.ContainsAny(w[1:], typeLettersOfALine) {
			return true
		}
	}
	return false
}

// operandLiteralStartsTheNameOver is arrayLiteralStartsTheNameOver for a
// declaration's own operand.
func (r *Runner) operandLiteralStartsTheNameOver(a *syntax.Assign) bool {
	if !r.operandsOverANonArray[a.Name] {
		return false
	}
	if a.Append {
		return r.ask(r.sem().AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
			"an appended array literal re-creating a name that is not an array")
	}
	return r.ask(r.sem().ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
		"an array literal re-creating a name that is not an array")
}
