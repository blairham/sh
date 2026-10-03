// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheTypeFlagDescribesALetteredDeclarationHoldingNothing is #5439: the
// lettered half of what #5438 did for a bare `local var`. Under
// TYPESET_TO_UNSET, a declaration that carries a letter and no value leaves
// the name unset but still describable. Under POSIX_BUILTINS alone, an
// `export` or `readonly` leaves nothing to describe.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestTheTypeFlagDescribesALetteredDeclarationHoldingNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			`setopt typesettounset; f() { local -a la; local -A aa; local -F fl; local -i n; local -u U; local -x X; print "[${(t)la}][${(t)aa}][${(t)fl}][${(t)n}][${(t)U}][${(t)X}]"; }; f`,
			"[array-local][association-local][float-local][integer-local][scalar-local-upper][scalar-local-export]\n",
		},
		{
			`setopt typesettounset; typeset -r R; typeset -x X; typeset -i I; typeset -u U; typeset -L3 J; print "[${(t)R}][${(t)X}][${(t)I}][${(t)U}][${(t)J}]" $+U $+J`,
			"[scalar-readonly][scalar-export][integer][scalar-upper][scalar-left] 0 0\n",
		},
		{`setopt posixbuiltins; readonly RO; export EX; typeset -r R2; print "[${(t)RO}][${(t)EX}][${(t)R2}]"`, "[][][]\n"},
		{`setopt posixbuiltins typesettounset; typeset -i I3; typeset -r R3; print "[${(t)I3}][${(t)R3}]"`, "[integer][scalar-readonly]\n"},
		{`setopt typesettounset; typeset -i I4; unset I4; print "[${(t)I4}]"; f() { local -i q; unset q; print "[${(t)q}]"; }; f`, "[]\n[]\n"},
		{`setopt typesettounset; f(){ local -i z; }; f; print "[${(t)z}]"`, "[]\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
