// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `PROMPT` and `PS1` are two spellings of one parameter, and so are the
// three pairs behind them.
//
// This shell had them as two independent variables, and what that cost was
// the whole of a prompt theme. Measured on this machine, 2026-09-10: a
// theme sets `PROMPT` in its `precmd`, the line editor reads `PS1`, and the
// two never met — so a session drew the built-in `%m%#` while the theme sat
// in a variable nobody read. Nothing was reported, because nothing was
// wrong with either half.
//
// # Which pairs, measured
//
// One fresh shell per pair, because `unset` breaks a tie in zsh and a
// script that unsets both before testing reports every pair as untied:
//
//	PROMPT=XX   -> $PS1 is XX        PS1=YY  -> $PROMPT is YY
//	PROMPT2=XX  -> $PS2 is XX        PS2=YY  -> $PROMPT2 is YY
//	PROMPT3=XX  -> $PS3 is XX        PS3=YY  -> $PROMPT3 is YY
//	PROMPT4=XX  -> $PS4 is XX        PS4=YY  -> $PROMPT4 is YY
//
// And which pairs are **not**, though their names say they should be —
// this is the half that has to be measured rather than assumed, because a
// rule inferred from the four above would have added all three:
//
//	RPROMPT=XX  -> $RPS1 is unset    RPS1=YY  -> $RPROMPT is unset
//	RPROMPT2=XX -> $RPS2 is unset    RPS2=YY  -> $RPROMPT2 is unset
//	SPROMPT=XX  -> $SPS1 is unset    SPS1=YY  -> $SPROMPT is unset
//
// So the right-hand prompt and the spelling prompt have one name each that
// this shell already answers to, and only the four left-hand ones are
// tied.
//
// # `PS` is the store and `PROMPT` is the spelling
//
// The pair is one parameter in zsh and two names here, with the `PS` name
// holding the value: the line editor reads `PS1`, the driver's defaults
// fill `PS1` and `PS2` in, and every other reader in this tree was written
// against those names. Making `PROMPT` the produced side is what leaves
// all of them correct without a second question about which name won.
//
// It is the produced-scalar seam rather than a tie in the core because the
// core's tie is the `typeset -T` one — a scalar joined to an *array* by a
// separator, which is what `PATH` and `path` are (see builtintie.go). Two
// scalars that are the same parameter is a different relation and this is
// the whole of it: a read produces the other name's value and a write goes
// to the other name, so neither can hold a value the other does not.
func registerPromptNames(r *interp.Runner) {
	for _, pair := range [...][2]string{
		{"PROMPT", "PS1"},
		{"PROMPT2", "PS2"},
		{"PROMPT3", "PS3"},
		{"PROMPT4", "PS4"},
	} {
		spelling, store := pair[0], pair[1]
		r.SetDynamic(spelling, func(rr *interp.Runner) string {
			v, _ := rr.GetVar(store)
			return v
		})
		// Required rather than decorative: without a writer the assignment
		// would land in a stored variable of the same name, which every
		// later read finds before the producer — so `PROMPT=…` would
		// silently stop being the same parameter at the moment a script
		// used it, which is exactly the failure this exists to fix. See
		// [interp.Runner.SetDynamicWriter].
		r.SetDynamicWriter(spelling, func(rr *interp.Runner, value string) {
			rr.SetVar(store, value)
		})
	}
}
