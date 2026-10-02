// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// registerTheSpecialParameterListing makes the parameters that are not
// variables — `!`, `#`, `$`, `*`, `-`, `0`, `?` and `@` — rows of every
// listing that walks the table, the way this shell's other own parameters
// are (#5157).
//
// Measured 2026-10-01 on zsh 5.9.2 under `-f -c`, `env -i
// PATH=/usr/bin:/bin`. A bare `typeset` opens on
//
//	integer 10 readonly !=0
//	integer 10 readonly '#'=0
//	integer 10 readonly '$'=85112
//	array readonly '*'=(  )
//	readonly -=569Xf
//	0=/opt/homebrew/bin/zsh
//	integer 10 readonly '?'=0
//	array readonly @=(  )
//
// and `typeset +`, `typeset +m '*'`, `typeset -i`, `-r`, `-a`, `+r` and a
// bare `readonly` name the same parameters by the same letters. A named
// `typeset -p '#'` writes nothing at 0, and a bare `typeset -p` names `0`
// alone, which is the Silent answer for the seven readonly ones. `${(t)…}`
// agrees: `integer-readonly-special` for `!`, `#`, `$` and `?`,
// `array-readonly-special` for `*` and `@`, `scalar-readonly-special` for
// `-` and `scalar-special` for `0`.
//
// The values are the expansion's own, read through Runner.SpecialParameter,
// so a listing cannot disagree with `$#`. bash 5.3's `declare` and ksh93u+'s
// `typeset` list none of these names, which is why this is the dialect's.
func registerTheSpecialParameterListing(r *interp.Runner) {
	for _, name := range [...]string{"!", "#", "$", "?"} {
		r.SetDynamic(name, specialValue(name))
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{Integer: true, Base: 10, Silent: true})
		r.MarkReadonly(name)
	}
	r.SetDynamic("-", specialValue("-"))
	r.SetDynamicDeclaration("-", interp.ProducedDeclaration{Silent: true})
	r.MarkReadonly("-")
	for _, name := range [...]string{"*", "@"} {
		r.SetDynamicArray(name, positionalParameters)
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{Array: true, ListsItsElements: true, Silent: true})
		r.MarkReadonly(name)
	}
	r.SetDynamic("0", specialValue("0"))
	r.SetDynamicDeclaration("0", interp.ProducedDeclaration{})
	// And two produced names that are ordinary scalars to a listing: the
	// second spelling of `histchars`, and `$_`, whose row is the running
	// command's own last word — `typeset` writes `_=typeset` and `typeset -p
	// 0` writes `typeset _=0`. Both are in `typeset -p` as well as the bare
	// listing, so neither is Silent.
	r.SetDynamicDeclaration(historyCharactersAlias, interp.ProducedDeclaration{})
	r.SetDynamicDeclaration("_", interp.ProducedDeclaration{})
}

func specialValue(name string) func(*interp.Runner) string {
	return func(rr *interp.Runner) string {
		v, _ := rr.SpecialParameter(name)
		return v
	}
}

// positionalParameters is `$@` as a produced array: empty rather than nil
// where there are none, because nil is a produced array that is not there and
// these always are — `typeset -a` writes `'*'=(  )` in a shell with no
// parameters, measured 2026-10-01.
func positionalParameters(rr *interp.Runner) []string {
	return append([]string{}, rr.Params...)
}
