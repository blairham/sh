// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"unicode"

	"github.com/blairham/sh/internal/eastasian"
)

// How many cells a terminal draws a character in.
//
// This is the one thing in this repository that is **not** settled by running
// a real shell. What is being modeled here is the terminal emulator rather
// than the shell: under a pty there is no terminal, and asking another
// shell's line editor where it wrapped only reports what its own width table
// says — a second implementation, not ground truth. So this follows the
// standard, and the nearest thing to a check is agreeing with the other
// implementations of the same standard. That is a weaker claim than every
// other behavioral claim here makes, and it is written down rather than left
// to look like an oversight.
//
// Three classes, and the third is where implementations part company:
//
//   - Two cells: East Asian Width W and F, from the generated table beside
//     this file. *Ambiguous* is not among them — it is one cell in a Western
//     locale and two in an East Asian one, which makes it a property of the
//     terminal rather than of the character.
//   - Zero cells: the combining marks, Mn and Me. Those come from the
//     standard library, so only the wide half needed generating.
//   - One cell: everything else, and that includes the format characters,
//     Cf. Zero-width space, soft hyphen and the joiners are neither Mn nor
//     Me, and implementations disagree about them — the common wcwidth
//     lineage gives U+200B zero and glibc gives soft hyphen one. Mn and Me
//     alone is the line that can be pointed at in the standard; anything
//     else is picking a side in somebody else's disagreement.
func runeWidth(r rune) int {
	if r < 0x20 || r == 0x7f {
		// A control character occupies no cell of its own. What a terminal
		// does with it is its business, and none of it is a column.
		return 0
	}
	if r < 0x300 {
		// Below the first combining mark and far below the first wide
		// range, so it is one cell without consulting either table — the
		// whole of Latin, and the common case by a long way. The bound is
		// 0x300 rather than the first wide range because U+0300 is where
		// the zero-width half begins, and cutting above it measured a
		// combining acute as a column of its own.
		return 1
	}
	if unicode.In(r, unicode.Mn, unicode.Me) {
		return 0
	}
	if eastasian.Wide(r) {
		return 2
	}
	return 1
}
