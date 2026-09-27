// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// What `$dirstack` says about *itself*.
//
// Every behavioral row about the directory stack already agreed with the
// reference; what did not was the parameter's own description, and it showed
// on four routes at once (#4615). The three attribute words are one fact
// between them: `special` is the shell maintaining the parameter, `hideval`
// is what keeps the values out of a listing, and `hide` is what makes a local
// declaration of the name an ordinary parameter.
//
// Measured 2026-09-26 on zsh 5.9.2 under `-f`; see
// dialect/zsh/shellownparameters.go for the table and for the one row of the
// issue that was wrong.
func TestTheDirectoryStackDescribesItself(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The sharpest row: nothing declared the name here until the
			// first push, where the reference has an array from startup.
			"in a shell that has pushed nothing",
			`print -r -- "${(t)dirstack} n=${#dirstack}"`,
			"array-hide-hideval-special n=0\n",
		},
		{
			"and after one",
			`setopt autopushd; cd sub; print -r -- "${(t)dirstack} n=${#dirstack}"`,
			"array-hide-hideval-special n=1\n",
		},
		{
			// `hideval` is what writes the bare name: a listing that would
			// have written `=value` writes the name alone.
			"a bare listing writes the name and not the values",
			`setopt autopushd; cd sub
			 set | while IFS= read -r l; do case $l in (dirstack*) print -r -- "[$l]";; esac; done`,
			"[dirstack]\n",
		},
		{
			// The `cd` is not a reference and the read is, which is the
			// pair rather than a precaution: this parameter arrives on the
			// script's first reference, and the *shell* filling the stack
			// is not one. Measured 2026-09-27 on zsh 5.9.2 — `setopt
			// autopushd; cd sub; typeset -p dirstack` is nothing at 0
			// there, and the same line with `${#dirstack}` in front of it
			// writes the row. See interp/deferredparam.go (#4899).
			"and the declaration printer writes the kind and no value",
			`setopt autopushd; cd sub; : ${#dirstack}; typeset -p dirstack; print -r -- "st=$?"`,
			"typeset -a dirstack\nst=0\n",
		},
		{
			// And the other half of that pair, which is what says the read
			// above is arming the row rather than hiding one: with the `cd`
			// alone the listing writes nothing, at 0.
			"and a shell-driven push writes no row at all",
			`setopt autopushd; cd sub; typeset -p dirstack; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// `hide` is the third word, and this is the row that separates
			// it from the other two: a local declaration of the name is an
			// ordinary parameter, so none of the three survives the shadow.
			"a local declaration of the name is an ordinary parameter",
			`setopt autopushd; cd sub
			 f() { local dirstack; print -r -- "${(t)dirstack} n=${#dirstack}"; }
			 f
			 print -r -- "after=${#dirstack}"`,
			"scalar-local n=0\nafter=1\n",
		},
		{
			// The control: the stack itself still works, which is what says
			// the rows above are about the description and not about the
			// parameter.
			"and the stack still works",
			`cd sub; pushd deep >/dev/null
			 print -r -- "n=${#dirstack}"
			 popd >/dev/null
			 print -r -- "after=${#dirstack}"`,
			"n=1\nafter=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
				t.Fatal(err)
			}
			out, st := runZshPrelude(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
