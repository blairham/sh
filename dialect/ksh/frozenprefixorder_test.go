// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// When a frozen name in an assignment prefix is refused here, against when the
// command's redirections are opened.
//
// This shell is neither of the two answers the axis began with. Measured
// 2026-09-18 on ksh93u+ 2012-08-01, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null, each line its
// own `( … )` with `readonly V=0` in front of it:
//
//	V=1 : > /nope/f                 `V: is read only`, and no file complaint
//	V=1 typeset q=2 > /nope/f       the same
//	V=1 command eval : > /nope/f    the same — `command` is transparent here
//	V=1 command : > /nope/f         the same
//	V=1 f > /nope/f                 the same, with `f() { echo IN; }`
//	V=1 print hi > /nope/f          the file complaint alone, 1
//	V=1 nosuchcmd > /nope/f         the file complaint alone, 1
//
// So the name is checked ahead of the redirection in front of a function and a
// special builtin, and behind it in front of a regular builtin and an external
// — the same split the prefix's own persistence draws (#3314).
func TestAFrozenPrefixIsCheckedFirstOnlyWhereItPersistsHere(t *testing.T) {
	if got := ksh.Semantics().PrefixToAFrozenNameIsCheckedFirst; got != interp.FrozenPrefixCheckedFirstWhereItPersists {
		t.Errorf("PrefixToAFrozenNameIsCheckedFirst = %v, want the kind-keyed order", got)
	}
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		// The rows where the name is reached first. The refusal is fatal on
		// a special builtin and a function here, which is why the file is
		// never mentioned: the command is over before it is opened.
		{"a special builtin", `readonly V=0
V=1 : > /nope/f
print after`, "ksh: line 2: V: is read only\n", 1},
		{"the declaration builtin, special here", `readonly V=0
V=1 typeset q=2 > /nope/f
print after`, "ksh: line 2: V: is read only\n", 1},
		{"through command, which is transparent here", `readonly V=0
V=1 command eval : > /nope/f
print after`, "ksh: line 2: V: is read only\n", 1},
		{"a function", `readonly V=0
f() { print IN; }
V=1 f > /nope/f
print after`, "ksh: line 3: V: is read only\n", 1},
		// And the rows where the redirection is opened first. A regular
		// builtin is not refused at all in this column — that is
		// PrefixToARegularBuiltinIsRefused — so what says the order here is
		// that the *file* is what is reported.
		{"a regular builtin", `readonly V=0
V=1 print hi > /nope/f
print after`, "ksh[2]: /nope/f: cannot create [No such file or directory]\nafter\n", 0},
		{"a command nothing answers for", `readonly V=0
V=1 nosuchcmd > /nope/f
print after`, "ksh: line 2: /nope/f: cannot create [No such file or directory]\nafter\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("ran %q: got %q status %d, want %q status %d",
					tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// And the issue's own shape: the same first row seen through a redirection
// that *succeeds*.
//
// The complaint is written before the `2>&1` is in force, so it reaches the
// shell's own stderr rather than the file the command was going to write —
// measured, `( readonly V=0; V=1 export > /dev/null 2>&1; print after )` is
// `V: is read only` and the subshell ends there on ksh93u+, where this engine
// said nothing at all (#3314).
//
// The location is part of it. A complaint written before the redirections are
// opened is not a redirection's, and must not be located as one: the row below
// is the plain-assignment form `ksh: line N:` and not the builtin form
// `ksh[N]:` that this engine wrote once it had already recorded a builtin's
// redirection as being opened.
func TestARefusedPrefixOutrunsTheCommandsRedirections(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `readonly V=0
V=1 export > /dev/null 2>&1
print after`)
	const want = "ksh: line 2: V: is read only\n"
	if out != want || st != 1 {
		t.Errorf("got %q status %d, want %q status 1", out, st, want)
	}
}
