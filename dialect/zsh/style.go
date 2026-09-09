// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/syntax"

// Style is how a formatter lays this shell's scripts out.
//
// One field differs from the core, and it is the only place any dialect
// differs: this shell alone spells a body with braces —
//
//	if [[ -n $x ]] { … } else { … }
//	for f ( a b c ) { … }
//	while (( i-- )) { … }
//	repeat 3 { … }
//
// — and those parse to exactly the tree the keyword spelling parses to. There
// is no AST field to tell them apart, so a formatter that prints from the tree
// rewrites every one of them into `; then … fi` and `; do … done` without
// anything reporting that it did. Measured on this machine: 323 `if … {`, 122
// `} else {` and 64 `for … ( ) {` in the zsh corpus, and none outside it —
// checked, because the same probe's 40 apparent bash hits were all embedded
// awk.
//
// So the spelling is the author's and is read back from the source. The
// alternative is defensible — expanding is portable and loses no behavior —
// and it is one word in this file, which is where a taste belongs.
//
// The indent needs no such argument. Two independent zsh trees, sharing no
// maintainer, indent two spaces in 92% and 97% of their files.
func Style() syntax.Style {
	s := syntax.CoreStyle()
	s.BraceShortForm = syntax.PreserveShortForm
	return s
}
