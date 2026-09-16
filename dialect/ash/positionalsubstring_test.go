// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/ash"
)

// A substring of `$@` or `$*` counts characters of the parameters joined
// together here, where bash, zsh and ksh93 slice the list (#2291).
//
// Measured 2026-09-16 against BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// and stdin from /dev/null. Every row prints the field count first, because
// one field and two fields that happen to print alike are the difference
// this is about.
func TestASubstringOfThePositionalsCountsCharacters(t *testing.T) {
	const show = `n() { printf '%s:' "$#"; printf '<%s>' "$@"; echo; }; `
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row that tells the two readings apart at a glance: the
			// list reading is `one two`.
			"a quoted star is a substring of the joined string",
			`set -- one two three four; n "${*:1:2}"`,
			"1:<ne>\n",
		},
		{
			"and so is a quoted at",
			`set -- one two three four; n "${@:1:2}"`,
			"1:<ne>\n",
		},
		{
			// Unquoted, the substring is then split like any expansion.
			"an unquoted at is split after it is cut",
			`set -- one two three four; n ${@:2}`,
			"4:<e><two><three><four>\n",
		},
		{
			// Offset 0 is the first character of `$1`: the shell's name
			// is not at the front of this string.
			"offset zero is the first parameter's first character",
			`set -- one two; n "${@:0:1}"`,
			"1:<o>\n",
		},
		{
			// A quoted `$@` keeps the parameters apart on either side of
			// the joint, which counts as one character.
			"a quoted at keeps the parameters it spans apart",
			`set -- 'a b' c 'd e'; n "${@:1}"`,
			"3:< b><c><d e>\n",
		},
		{
			"an offset on the joint begins with an empty field",
			`set -- 'a b' c 'd e'; n "${@:3}"`,
			"3:<><c><d e>\n",
		},
		{
			// The joint is IFS's first character for a quoted star, and
			// no character when IFS is empty.
			"a quoted star joins on IFS, and an empty IFS joins on nothing",
			`set -- 'a b' c 'd e'; IFS=; n "${*:1:4}"`,
			"1:< bcd>\n",
		},
		{
			// …where the at spelling still counts one character for the
			// joint and still gives two fields.
			"a quoted at still counts the joint under an empty IFS",
			`set -- 'a b' c 'd e'; IFS=; n "${@:1:4}"`,
			"2:< b><c>\n",
		},
		{
			"and so does an unquoted star",
			`set -- 'a b' c 'd e'; IFS=; n ${*:1:4}`,
			"2:< b><c>\n",
		},
		{
			"a quoted star with a set IFS joins on its first character",
			`set -- 'a b' c 'd e'; IFS=ab; n "${*:1:4}"`,
			"1:< bac>\n",
		},
		{
			"a negative offset counts back from the end of the string",
			`set -- one two three four; n "${*:(-3)}"`,
			"1:<our>\n",
		},
		{
			"and a negative length does too",
			`set -- 'a b' c 'd e'; n "${*:1:-1}"`,
			"1:< b c d >\n",
		},
		{
			// With no parameters at all it is empty, where the list
			// reading names the shell.
			"no parameters is empty",
			`set --; n "${*:0}"`,
			"1:<>\n",
		},
		{
			// The control: the length is already the joined string's,
			// which is the reading this row agrees with.
			"the length of the list is the joined string's",
			`set -- one two three four; n "${#*}"`,
			"1:<18>\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, show+tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

func TestTheSubstringOfThePositionalsAnswer(t *testing.T) {
	if got, want := ash.Semantics().SubstringOfPositionalsSlicesTheList, interp.No; got != want {
		t.Errorf("SubstringOfPositionalsSlicesTheList = %v, want %v", got, want)
	}
}
