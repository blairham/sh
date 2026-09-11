// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `argv` — this shell's name for the positional parameters.
//
// It is a real array and not a synonym for `$@`: it is read, subscripted,
// counted, assigned to whole, appended to, written one element at a time and
// unset, and every one of those reaches `$@`. Measured on zsh 5.9.2,
// 2026-09-10, with `set -- 'a b' c`:
//
//	$argv               a b c   the bare name, joined exactly as `"$arr"` is
//	${argv[1]}          a b     subscripts count from one, like any array
//	${argv[2]}          c
//	${#argv}            2       the element count
//	"${argv[@]}"        two fields, `a b` and `c`
//	${(t)argv}          array-special
//	typeset -p argv     typeset -a argv=( 'a b' c )
//	argv[1]=zz          leaves `$1` as `zz`
//	argv=(n1 n2 n3)     leaves `$#` at 3
//	argv+=(d)           leaves `$#` at 4 and `$4` as `d`
//	argv[5]=e           leaves `$#` at 5
//	unset argv          leaves `$#` at 0
//	f() { print $argv[1] }; f q w   →  q, the *frame's* parameters
//
// The bare name joining is what says it is an array rather than `@`: `"$@"`
// keeps its fields where `"$argv"` does not, so this is an ordinary array
// whose storage happens to be the parameters, and the axes that decide what a
// bare array name means decide this too.
//
// The panel is unanimous the other way — `set -- "a b" c; echo "[$argv]"` is
// `[a b c]` in zsh 5.9.2 and `[]` in bash 5.3, bash 3.2, ksh93 and dash alike
// — so it belongs to this dialect and not to the core.
//
// Without it a script reading `argv` saw an ordinary unset name, and a *write*
// to it was worse than nothing: `argv[1]=zz` created a stray global that every
// later `$argv` in the shell then read, including inside a function whose own
// parameters it should have been. That is the shape of #1633 and #1622's
// reduction, where `${(q)argv}` came back as `”`.
//
// A producer and a writer rather than a stored array seeded at each `set --`:
// a stored array is what a read finds first, so it would have to be kept in
// step with `$@` at every one of the dozen places the parameters move — a
// function call, a `shift`, a `set --`, a trap, a `source` with words. The
// producer reads them where they are.
func registerArgv(r *interp.Runner) {
	r.SetDynamicArray("argv", func(rr *interp.Runner) []string { return rr.Params })
	r.SetDynamicArrayWriter("argv", func(rr *interp.Runner, values []string) {
		rr.Params = values
	})
}
