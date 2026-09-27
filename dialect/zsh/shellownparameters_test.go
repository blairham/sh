// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What this shell's own stored parameters say about themselves.
//
// `${(t)UID}` was a bare `scalar` here where the reference says
// `integer-special`, and so was every other name the shell stores for itself
// rather than producing — the seam, not the four names (#4488). Measured
// 2026-09-26 on zsh 5.9.2 under `-f`; see dialect/zsh/shellownparameters.go
// for the whole table and for the reference's own version.
func TestTheShellsOwnStoredParametersDescribeThemselves(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the real uid", `print -r -- ${(t)UID}`, "integer-special\n"},
		{"the effective uid", `print -r -- ${(t)EUID}`, "integer-special\n"},
		{"the real gid", `print -r -- ${(t)GID}`, "integer-special\n"},
		{"the effective gid", `print -r -- ${(t)EGID}`, "integer-special\n"},
		{"the field separator", `print -r -- ${(t)IFS}`, "scalar-special\n"},
		{"a tie's scalar half", `export PATH; print -r -- ${(t)PATH}`, "scalar-tied-export-special\n"},
		{"and its array half", `print -r -- ${(t)path}`, "array-tied-special\n"},
		{"an unexported tie", `print -r -- ${(t)FPATH} ${(t)fpath}`, "scalar-tied-special array-tied-special\n"},
		{"the last of the eight pairs", `print -r -- ${(t)FIGNORE} ${(t)fignore}`, "scalar-tied-special array-tied-special\n"},
		// The table `${(t)}` reads is the one `$parameters` renders, so the
		// two spellings of the question cannot answer it differently.
		{"and the table agrees", `print -r -- $parameters[UID] $parameters[IFS]`, "integer-special scalar-special\n"},
		// The integer letter rides on a *declaration* rather than on the
		// attribute table, so a listing writes it and an assignment is
		// untouched — which is the half #4476 measured and left alone.
		{
			// The letter and the base, without the *number*, which is the
			// machine's: a row spelling `501` passes on the laptop it was
			// written on and fails on a runner. The value is checked in the
			// same row, against `$UID` rather than against a constant.
			"the listing carries the letter and the base",
			`x=$(typeset -p UID); print -r -- "${x%=*}"; [[ ${x##*=} = $UID ]] && print -r -- "value ok"`,
			"typeset -i10 UID\nvalue ok\n",
		},
		{"and the separator's does not", `typeset -p IFS`, "typeset IFS=$' \\t\\n\\C-@'\n"},
		{"an assignment is still stored as written", `EGID=1+1; print -r -- $EGID`, "1+1\n"},
		// The controls. A produced parameter was right throughout — that is
		// what said the gap was the stored names' — and the one name this
		// hook stores that the reference does *not* call special is the row
		// that says the hook may not do the marking itself.
		{"a produced parameter was already right", `print -r -- ${(t)RANDOM}`, "integer-special\n"},
		{"a stored name the reference calls ordinary", `print -r -- ${(t)HOST}`, "scalar\n"},
		{"an ordinary exported name", `export ord=1; print -r -- ${(t)ord}`, "scalar-export\n"},
		// And the mark is a parameter's rather than a record's: `unset`
		// takes the name away and takes the word with it.
		{"unset takes the mark too", `unset UID; print -r -- "[${(t)UID}][${+UID}]"`, "[][0]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
