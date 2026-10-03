// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A `local OPTIND` gives the names after it nothing of its own.** zsh holds
// OPTIND as an integer, and a fresh local of it takes that kind — but only it:
// measured 2026-10-03 on zsh 5.9.2, `f() { local OPTIND o p; typeset -p o p; }`
// lists both as plain empty scalars, and the function that parses its options with
// `local OPTIND=1 o` reads `[a][b]`. The integer letter had been written onto
// the whole line's flags, so `o` became an integer and `getopts` stored 0.
func TestALocalOptindLeavesTheNamesAfterItPlain(t *testing.T) {
	src := `f() { local OPTIND=1 o p; typeset -p o p; while getopts ab o; do printf "[%s]" "$o"; done; echo; }; f -a -b`
	out, _, err := presets["zsh"].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "typeset o=''\ntypeset p=''\n[a][b]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// **dash refuses to unset OPTIND**, in a sentence that names no name, and the
// refusal is a special builtin's error: measured 2026-10-03 on dash 0.5.12,
// `unset OPTIND` is `unset: Illegal number: ` at 2 and ends the script, and
// under `command` the script goes on with OPTIND as it was. `unset -f OPTIND`
// is about a function and is not refused.
func TestDashRefusesToUnsetOptind(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"unset OPTIND; echo reached\n", "dash: 1: unset: Illegal number: \n"},
		{"OPTIND=3; command unset OPTIND; echo \"st=$? [${OPTIND-U}]\"\n", "dash: 1: unset: Illegal number: \nst=2 [3]\n"},
		{"command unset -f OPTIND; echo st=$?\n", "st=0\n"},
	} {
		out, _, err := presets["dash"].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%q: got %q, want %q", c.src, out, c.want)
		}
	}
}
