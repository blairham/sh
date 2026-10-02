// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestALocalHidesTheCallersValueUnderTypesetToUnset is #5157's E03posix front
// `KSH_TYPESET option`, whose first line of output depends on it. Under
// TYPESET_TO_UNSET a function's declaration with no value leaves its own
// binding unset, and that binding hides the caller's value. It did not here,
// so the caller's value showed through.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
// The emulation rows are the same under `emulate sh` and `emulate ksh`.
func TestALocalHidesTheCallersValueUnderTypesetToUnset(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`setopt typesettounset; g=1; f() { local g; echo ${g-UNSET}; }; f; echo $g`, "UNSET\n1\n"},
		{`setopt typesettounset; g=1; f() { typeset g; print $+g; }; f`, "0\n"},
		{`setopt typesettounset; g=1; f() { local -x g; print $+g; }; f`, "0\n"},
		{`setopt typesettounset; g=1; f() { local -i g; print $+g; }; f`, "0\n"},
		// A caller's own local is the value hidden, one frame further in.
		{`setopt typesettounset; g=9; k() { local g=2; m; }; m() { local g; print $+g; }; k`, "0\n"},
		// And a second declaration in the same frame has nothing to hide: the
		// value it would hide is the one this frame just gave it.
		{`setopt typesettounset; f() { local g; g=3; local g; print $+g$g; }; f`, "g=3\n13\n"},
		{`emulate sh; setopt typesettounset; g=1; f() { local g; echo ${g-UNSET}; }; f`, "UNSET\n"},
		{`emulate ksh; setopt typesettounset; g=1; f() { local g; echo ${g-UNSET}; }; f`, "UNSET\n"},
		// The control: without the option the local holds the empty string,
		// which hides the caller's value by holding one of its own.
		{`g=1; f() { local g; echo "[${g-UNSET}]"; }; f; echo $g`, "[]\n1\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
