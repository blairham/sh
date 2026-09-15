// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// NamerefBuiltin is `typeset -n` under a second word.
//
// ksh93 spells a name reference two ways and they are one command: measured
// 2026-09-15 on ksh93u+, `nameref r=v` and `typeset -n r=v` leave the same
// state, `nameref -x` answers with `typeset`'s whole usage block, and
// `nameref 1x=v` is `typeset: 1x=v: is not an identifier` — the *other*
// word's name in the complaint, which is what says the second spelling is a
// front for the first rather than a builtin of its own.
//
// Registered rather than written out in the dialect, for the reason
// [FunctionsBuiltin] is: two words reaching one declaration cannot come to
// disagree about the listing, the refusals, or the status a name leaves
// behind.
//
// `whence nameref` answers `not found` in the real shell, where this shell
// says it is a builtin — that word is a declaration *command* there and not
// something its own command table holds. Recorded rather than worked around:
// the divergence is in what `whence` says about the name, and closing it
// would mean a builtin the shell has and hides, which is a wider notion than
// this spelling is worth.
func NamerefBuiltin() Builtin { return biNameref }

func biNameref(r *Runner, ctx context.Context, args []string) int {
	typeset, ok := r.Builtin("typeset")
	if !ok {
		// A dialect that registered this word and unregistered the one it
		// stands for. Refused rather than reimplemented here, which is the
		// whole point of the word being a front.
		r.diagf("nameref: this shell has no declaration builtin behind it\n")
		return 2
	}
	return typeset(r, ctx, append([]string{"-n"}, args...))
}
