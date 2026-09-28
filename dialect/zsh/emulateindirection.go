// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/syntax"

// `${!name}` — the construct this shell's own grammar has not and whose
// **mode** gives it back.
//
// It is the first entry in emulationGrammar and the evidence that table was
// built for: an emulation is a grammar **vector per mode** and not a
// narrowing of zsh's, because here a mode *adds* a construct rather than
// taking one away. Measured on `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — run `-f` over a script file, 2026-09-28:
//
//	                           emulate zsh       emulate sh        emulate ksh
//	x=y; y=V; echo "${!x}"     bad substitution  bad substitution  y
//	typeset -A m; m[k]=v
//	echo "${!m[@]}"            bad substitution  bad substitution  k
//
// **And no option name moves it**, which is what puts it here rather than in
// setopt.go beside `shglob`, `ignorebraces` and `multifuncdef`. Every one of
// zsh's option names was set and then unset in turn — the whole of
// `${(k)options}`, a name at a time — inside `emulate zsh` with each of the
// seven corpus snippets that reach this in front of it, and again inside
// `emulate ksh`; nothing moved any of them in either direction.
// `setopt ksharrays`, the obvious candidate, leaves `bad substitution`
// exactly where it was.
//
// What the construct then *means* is
// interp.Semantics.IndirectionIsTheSubscriptFlag, which this dialect answers
// once in Semantics() rather than per mode: no other mode can reach it, since
// the other two refuse the text at the parse.
func paramIndirectionAxis() grammarAxis {
	return grammarFlag("ParamIndirection",
		func(d *syntax.Dialect) *bool { return &d.ParamIndirection },
		map[string]bool{"zsh": false, "sh": false, "ksh": true, "csh": false})
}
