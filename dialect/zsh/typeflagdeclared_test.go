// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheTypeFlagDescribesANameDeclaredHoldingNothing is #5157's E03posix
// front `(t) returns correct type`. Under TYPESET_TO_UNSET, a declaration with
// no value brings a name into being that holds nothing. `${(t)name}`
// describes it, although `${+name}` is 0. Here it was empty.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestTheTypeFlagDescribesANameDeclaredHoldingNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`setopt typesettounset; f() { local var; print ${(t)var} $+var; }; f`, "scalar-local 0\n"},
		{`setopt typesettounset; typeset top; print ${(t)top} $+top`, "scalar 0\n"},
		// A caller's value the local hides does not change what it is.
		{`setopt typesettounset; g=1; f() { local g; print ${(t)g}; }; f; print ${(t)g}`, "scalar-local\nscalar\n"},
		// An `unset` is not that state: the name is gone.
		{`setopt typesettounset; typeset T; unset T; print "[${(t)T}]"`, "[]\n"},
		{`f() { local v; unset v; print "[${(t)v}]"; }; f`, "[]\n"},
		{`setopt typesettounset; f() { local q; unset q; print "[${(t)q}]"; }; f`, "[]\n"},
		// Nor is a POSIX_BUILTINS declaration that leaves the name as it was.
		{`setopt posixbuiltins; readonly RO; export EX; print "[${(t)RO}][${(t)EX}]"`, "[][]\n"},
		// The control: without the option the local holds the empty string.
		{`f() { local var; print ${(t)var} $+var; }; f`, "scalar-local 1\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
