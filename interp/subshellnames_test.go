// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
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
	// readonly's own control, which needs its diagnostic rather than only a
	// value: a subshell inherits the attribute, so the assignment in it is
	// refused and the parent's value is the one that survives. Written apart
	// from the table because the assertion is on both halves at once.
	out, _ := run(t, `readonly x=1; (x=2); echo "[$x]"`, nil)
	if !strings.Contains(out, "readonly") || !strings.Contains(out, "[1]") {
		t.Errorf("a subshell must inherit the attribute and leave the parent alone: %q", out)
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

// TestASubshellOwnsTheAttributeTables is the same rule for what the shell
// knows about a *variable* name besides its value, and it is #1384.
//
// clone copied twelve tables by hand and left twenty-two shared, so an
// attribute a subshell declared was the parent's afterwards: `x=1;
// (readonly x); x=2` refused the assignment where every shell in the panel
// assigns 2. That is the half a script can see. The half it cannot is that a
// process substitution is a subshell running on a *goroutine*, so the same
// sharing is a data race that ends the process with a runtime fatal error no
// recover can catch — which is why the fix is a rule about the struct rather
// than about the tables anybody had thought of.
//
// A process substitution is here for that reason and not for symmetry: it is
// the boundary where sharing stops being a leak and starts being a crash.
func TestASubshellOwnsTheAttributeTables(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// readonly, which is the one that showed. The control beside it is
		// the row below: the subshell must still *see* the parent's
		// attribute, or this would pass by having copied nothing at all.
		{"readonly declared in one", `x=1; (readonly x); x=2; echo "$x"`, "2\n"},
		// The attribute tables behind `typeset`, on the same terms. The
		// second row is the control for the first: a subshell has to still
		// *see* the parent's attribute, or the row above would pass by
		// having copied nothing at all.
		{"integer declared in one", `n=5; (typeset -i n); n=1+1; echo "$n"`, "1+1\n"},
		{"integer read from the parent", `typeset -i n; n=1+1; (echo "$n")`, "2\n"},
		// And a name a subshell took away. `unset` records the removal in a
		// table of its own, which is the table the crash was on.
		{"unset in one", `x=1; (unset x); echo "$x"`, "1\n"},
		{"unset and gone in one", `x=1; ( unset x; echo "[$x]" )`, "[]\n"},
		// The goroutine boundary. Same tables, and now two writers with no
		// schedule between them.
		{"readonly in a substitution", `x=1; cat <(readonly x) >/dev/null; x=2; echo "$x"`, "2\n"},
		{"unset in a substitution", `x=1; cat <(unset x) >/dev/null; echo "$x"`, "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
