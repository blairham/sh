// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// TypeExternalSentence is the line `type`, `command -V` and one dialect's
// `whence` write for a name that resolves to a file.
//
// One function because there are **two** wordings and the choice between them
// is a fact about the operand rather than about the caller: a word that
// already held a slash was never searched for, so a dialect whose sentence
// says how the name was found has nothing to say about it. See
// Diagnostics.TypePathnameOperand, which is empty in every column whose two
// wordings are the same string.
//
// Exported because the dialect with the second wording has a `whence` of its
// own, and a copy of this choice there is a copy that would drift: it already
// had the tracked-alias sentence written out as a literal in two places, and
// the pathname operand was wrong in both.
func (r *Runner) TypeExternalSentence(name, path string) string {
	if strings.ContainsRune(name, '/') {
		if w := r.diag().TypePathnameOperand; w != "" {
			return Wording(w, "%[1]s is %[2]s", name, path)
		}
	}
	return Wording(r.diag().TypeExternal, "%[1]s is %[2]s", name, path)
}
