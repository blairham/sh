// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// Two surfaces a compound variable touches on its way *out* of the shell: the
// environment, which has no representation of one at all, and the listing's
// letters. Measured on ksh93u+ 2012-08-01, 2026-09-14, under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, over `-c`, a script file
// and standard input alike (#2736).

// The export letter over a compound is refused, and fatally — this shell
// counts both words among its special builtins.
func TestACompoundMayNotBeExported(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The literal on the export line itself.
		{`export c=(a=1); echo after`, "export: c: only simple variables can be exported\n"},
		// The attribute arriving over a name that is already one.
		{`c=(a=1); export c; echo after`, "export: c: only simple variables can be exported\n"},
		// The same through the declaration word, which names itself instead.
		{`c=(a=1); typeset -x c; echo after`, "typeset: c: only simple variables can be exported\n"},
		// The `C` letter making one on the line that exports it.
		{`typeset -Cx c=(a=1); echo after`, "typeset: c: only simple variables can be exported\n"},
		// An *empty* parenthesized value is a compound here, not an empty
		// array, which is what its own listing says — so it is refused too.
		{`typeset -x a=(); echo after`, "typeset: a: only simple variables can be exported\n"},
		// The copy form names the operand and not the name copied from.
		{`c=(a=1); typeset -Cx d=c; echo after`, "typeset: d: only simple variables can be exported\n"},
	} {
		out, st := kshOut(t, c.src)
		if !strings.HasSuffix(out, c.want) || st == 0 || strings.Contains(out, "after") {
			t.Errorf("%s\n got %q at %d\nwant it to end %q, be non-zero, and not reach `after`",
				c.src, out, st, c.want)
		}
	}
}

// And the two controls, which are what keep the refusal about the *compound*
// rather than about every parenthesized value: an index array and a table are
// both exportable here.
func TestAnArrayAndATableAreStillExportable(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -x a=(p q); echo after; typeset -p a`, "after\ntypeset -x -a a=(p q)\n"},
		{`typeset -Ax m; m[k]=v; echo after; typeset -p m`, "after\ntypeset -x -A m=([k]=v)\n"},
		// And a plain scalar, which is the shape the refusal must never
		// reach.
		{`a=1; export a; echo after; typeset -p a`, "after\ntypeset -x a=1\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The `C` letter is the compound's *default* spelling rather than a letter it
// carries: a listing with any other letter to write drops it, and the
// parentheses are what still say the kind.
func TestTheCompoundLetterIsDroppedBesideAnother(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The letter with nothing beside it, which is where it is written.
		{`c=(a=1); typeset -p c`, "typeset -C c=(a=1)\n"},
		{`typeset -C c; typeset -p c`, "typeset -C c=()\n"},
		// And every spelling that puts a second letter on the name drops it,
		// however the compound was made.
		{`readonly c=(a=1 b=2); typeset -p c`, "typeset -r c=(a=1;b=2)\n"},
		{`c=(a=1); readonly c; typeset -p c`, "typeset -r c=(a=1)\n"},
		{`c=(a=1); typeset -r c; typeset -p c`, "typeset -r c=(a=1)\n"},
		// The `-C` written on the command is no different: it is the letter
		// the *listing* would write that is dropped, not one the declaration
		// carried.
		{`typeset -rC c=(a=1); typeset -p c`, "typeset -r c=(a=1)\n"},
		{`typeset -Cr c=(a=1); typeset -p c`, "typeset -r c=(a=1)\n"},
		// The empty compound takes the same rule, which is the row a reading
		// that treated `()` as an empty array would get wrong.
		{`readonly c=(); typeset -p c`, "typeset -r c=()\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}
