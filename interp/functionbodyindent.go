// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// `-x num`: how wide one level of a printed function body is, for the length of
// one command.
//
// The letter belongs to four names rather than one — `functions -x2 f`,
// `whence -x2 -f f`, `which -x2 f` and `where -x2 f` all write the same body
// with the same indentation, measured 2026-09-29 on zsh 5.9.2 — and they share
// this rather than each reading the letter, because the reading is the part
// with the trap in it.
//
// **The number may be attached to the letter or the word after it**, so a scan
// that has not taken `-x` cannot tell an operand from an argument: `which -x 2
// f` has three words and two of them belong to the option. That is why this
// runs before the letters are parsed and hands back what is left.
//
// The second result restores the arrangement, and is never nil: a caller can
// defer it without asking whether the letter was there.
//
// Exported for dialect/zsh's `whence`, `which` and `where`, which had the
// letter in their unimplemented list. Folding it here rather than writing a
// second reader is the point: the two would have to agree about an attached
// number, about `-x` with nothing after it, and about the wording — three
// chances to drift for one letter.
func (r *Runner) FunctionBodyIndentOption(name string, args []string) (rest []string, restore func(), code int) {
	rest, indent, found, code := r.functionsIndentLetter(name, args)
	if code != 0 || !found {
		return rest, func() {}, code
	}
	// The listing's own arrangement for the length of this call: what `-x`
	// moves is how one level of structure is written, and the printer already
	// knows where the levels are.
	saved := r.functionLayout.Indent
	r.functionLayout.Indent = strings.Repeat(" ", indent)
	return rest, func() { r.functionLayout.Indent = saved }, 0
}
