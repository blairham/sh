// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subshell owns the tables that say what a command *name* means, on the
// same terms as the tables that say what a variable name means.
//
// The core rather than an axis, and measured rather than assumed: dash, bash
// 5.3, bash 3.2, ksh93u+ and zsh 5.9.2 all four answer the same way on every
// shape below, in both directions. Sharing the maps made a definition made in
// a subshell the parent's *and* a removal made in a subshell the parent's —
// two failures from one line, both at status 0 with nothing said (#1125).
//
// These name no shell, because there is nothing here for a shell to answer.

// TestASubshellOwnsTheFunctionTable is the three things a subshell can do to a
// function, and the parent must see none of them.
func TestASubshellOwnsTheFunctionTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A definition. The control for it is the row below: the subshell
		// must still be able to *read* the parent's table, or this would
		// pass by having copied nothing at all.
		{"defined in one", `(g(){ echo in; }); command -v g >/dev/null 2>&1 && echo leaked || echo gone`, "gone\n"},
		{"read from the parent", `g(){ echo mine; }; (g)`, "mine\n"},
		// A redefinition, which is the shape a script uses on purpose: run a
		// helper in `( … )` so the version it needs stays out of the way.
		{"redefined in one", `g(){ echo old; }; (g(){ echo new; }); g`, "old\n"},
		{"redefined and used in one", `g(){ echo old; }; ( g(){ echo new; }; g )`, "new\n"},
		// A removal. This is the direction #1125 did not measure and the one
		// that was wrong the other way round: the parent's function was gone
		// afterwards.
		{"removed in one", `f(){ echo yes; }; (unset -f f); f`, "yes\n"},
		{"removed and gone in one", `f(){ echo yes; }; ( unset -f f; command -v f >/dev/null 2>&1 && echo there || echo gone )`, "gone\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestASubshellOwnsTheAliasTable is the same for aliases, where the
// consequence is a *parse* difference rather than a value one — an alias
// defined in a subshell changed how the parent's later lines were read.
//
// Every row defines an alias in the parent first, and that is not decoration:
// the writers allocate lazily, so with no alias anywhere the subshell built
// the map the parent's field never pointed at and the leak did not appear.
// That masking is why #1125's own measurement of the definition row could not
// see it, and why this one has to set the table up before it can ask.
//
// Through aliasRun because `alias` reads its options only where a dialect
// says so, and the whole listing is asked for rather than one name: a
// not-found lookup is three more axes and none of them is the subject.
func TestASubshellOwnsTheAliasTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"defined in one", `alias z=echo; (alias q=ls); alias`, "z='echo'\n"},
		{"read from the parent", `alias z=echo; (alias z)`, "z='echo'\n"},
		{"removed in one", `alias z=echo; (unalias z); alias`, "z='echo'\n"},
		{"redefined in one", `alias z=echo; (alias z=printf); alias`, "z='echo'\n"},
		// The control: inside the subshell the change is real.
		{"changed and seen in one", `alias z=echo; ( alias z=printf; alias )`, "z='printf'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := aliasRun(t, nil, Diagnostics{}, tc.src); got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestASubshellOwnsTheDisabledBuiltinTable is the third such table, and it was
// leaking for the same reason and behind the same lazy-allocation mask.
//
// Listed rather than exercised, because switching a builtin off does not stop
// the *name* working — it stops resolving to the builtin and is looked up on
// PATH like any other word, and `printf` is on PATH. So a test that ran the
// name would pass whichever table it read.
func TestASubshellOwnsTheDisabledBuiltinTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"switched off in one", "enable -n :\n(enable -n printf)\nenable -n", "enable -n :\n"},
		{"switched on in one", "enable -n :\n(enable :)\nenable -n", "enable -n :\n"},
		{"seen from inside one", "enable -n :\n(enable -n)", "enable -n :\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
