// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$patchars` is the pattern characters this shell reads, in the order the
// module lists them.
//
// The list is a **constant**, which is measured rather than assumed and is
// the one thing about this parameter worth checking before writing it. The
// obvious reading is that it names the characters currently in force, so that
// `setopt extendedglob` would lengthen it and `setopt noglob` empty it. It
// does not move: measured 2026-09-27 on zsh 5.9.2 under `-f` from a script
// file with `zsh/parameter` loaded, the same fifteen come back under
// `extendedglob`, under `kshglob`, under `noglob` and under
// `nobareglobqual` alike. So it is what the shell's pattern language *has*
// rather than what this moment's options allow, and a view computed from the
// option vector would have been a different parameter wearing this name.
//
// Its disabled half is the one that varies, and it is already here and
// already empty: `$dis_patchars` fills as `disable -p` takes a character out,
// and `disable -p` is `not implemented yet` — see zshEmptyParams.
//
// The fifteen are this engine's too, which is what keeps the list from being
// a transcription. `|`, `(`, `?`, `*`, `[` and `<` are the common pattern
// language; `~`, `^` and `#` are the extended-glob operators this shell
// reads; and the six `?(`, `*(`, `+(`, `!(`, `\!(` and `@(` are the ksh-glob
// groups it reads under `kshglob`. Every one of them is a character this
// parser gives a meaning to, so there is nothing here this shell would have
// to grow to make true.
var zshPatternCharacters = []string{
	"|", "~", "(", "?", "*", "[", "<", "^", "#",
	"?(", "*(", "+(", "!(", `\!(`, "@(",
}

// registerPatternCharacters installs `$patchars`.
//
// Readonly and hidden, the pair every produced table in this dialect carries
// and for the reasons hideModuleParameter records: `${(t)patchars}` is
// `array-readonly-hide-hideval-special` in zsh 5.9.2, `patchars=(a b)` is
// `read-only variable: patchars`, and a bare `typeset` writes `array readonly
// patchars` with no value.
func registerPatternCharacters(r *interp.Runner) {
	r.SetDynamicArray("patchars", func(*interp.Runner) []string {
		return append([]string(nil), zshPatternCharacters...)
	})
	r.MarkReadonly("patchars")
	// Silent to `-p`, as zsh/parameter's frozen tables are; see
	// interp.Runner.SetSilentToPrint.
	r.SetSilentToPrint("patchars")
	hideModuleParameter(r, "patchars")
}
