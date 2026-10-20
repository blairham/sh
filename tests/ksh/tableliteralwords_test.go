// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A compound literal of **bare words** on a table is an index array's value
// here, and this shell will not put one in a table: it complains and the
// input ends. bash and zsh pair the words off as key, value, key, value,
// which is what makes it an axis rather than a correction — see
// interp.Semantics.BareElementsInATableLiteralEndTheScript.
//
// Measured 2026-09-13 against ksh93u+ 2012-08-01. The sentence says `append`
// even for a plain `=`, which is the tell that it is about the two kinds and
// not about the operator (#2611).
func TestAnIndexArrayLiteralOnATableEndsTheInput(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a declaration carrying it", `typeset -A m=(alpha one); echo after`},
		{"an assignment to a declared name", `typeset -A m; m=(alpha one); echo after`},
		{"the append spelling", `typeset -A m; m+=(alpha one); echo after`},
		{"more than one pair", `typeset -A m=(alpha one beta two); echo after`},
		{"a second declaration over a table that has elements", `typeset -A m=([a]=1); typeset -A m=(x y); echo after`},
		{"an append onto a table that has elements", `typeset -A m=([a]=1); m+=(x y); echo after`},
		// The written element is what counts, not what it came to.
		{"an element that expands to nothing", `e=; typeset -A m=($e); echo after`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			want := "ksh: cannot append index array to associative array m\n"
			if out != want || st != 1 {
				t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, want)
			}
		})
	}
}

// The other side of the same axis, so a shell that simply refused every table
// literal would fail here: a keyed literal is taken, and a literal with no
// element written in it is taken too — there is no index array in `()` to
// object to.
func TestAKeyedOrEmptyTableLiteralIsTaken(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -A m=([alpha]=one); echo "[${m[alpha]}] after"`, "[one] after\n"},
		{`typeset -A m=(); echo "st=$? after"`, "st=0 after\n"},
		{`typeset -A m=([a]=1); m+=([b]=2); echo "[${m[a]}][${m[b]}]"`, "[1][2]\n"},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The refusal costs the input and not the process: inside a subshell it ends
// that shell alone and the command after the subshell runs, at 0. Measured.
func TestTheRefusalEndsTheSubshellAndNotTheScript(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `(typeset -A m=(a b)); echo after`)
	want := "ksh: cannot append index array to associative array m\nafter\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// And the refusal is not every spelling of it: a **replacing** literal onto a
// table that already has an element converts the name to an index array,
// without a word. The table letter on the same command is what holds the name
// to its kind, and an append never converts — see
// interp.Runner.tableBecomesAnIndexArray for the measured rows.
func TestAReplacingLiteralOverATableThatHasElementsConvertsIt(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a plain assignment", "typeset -A m=([a]=1)\nm=(x y)\ntypeset -p m; echo after"},
		{"a declaration with no letter on it", "typeset -A m=([a]=1)\ntypeset m=(x y)\ntypeset -p m; echo after"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			want := "typeset -a m=(x y)\nafter\n"
			if out != want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, want)
			}
		})
	}
}

// The control for the cell above, and the one that says the *element* is what
// decides rather than the declaration: an empty table refuses the same
// replacing literal a populated one takes. That is the refusing shell's own
// reading and it is reproduced rather than tidied.
func TestAReplacingLiteralOverAnEmptyTableStillRefuses(t *testing.T) {
	for _, src := range []string{
		"typeset -A m\nm=(x y)\ntypeset -p m; echo after",
		"typeset -A m=()\nm=(x y)\ntypeset -p m; echo after",
		"typeset -A m\ntypeset m=(x y)\ntypeset -p m; echo after",
	} {
		out, st := runKsh(t, t.TempDir(), src)
		want := "ksh: line 2: cannot append index array to associative array m\n"
		if out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}
