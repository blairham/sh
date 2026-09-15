// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `BASH_ARGC` and `BASH_ARGV` are the arguments of each call the shell is
// inside, kept while extended debugging is on. They are the third item of
// #2476's list and the last of it.
//
// `BASH_ARGC` is one count per call, innermost first. `BASH_ARGV` is every
// argument of every call in one list and it is a **stack**, so the innermost
// call's *last* argument is element 0 — which is the half a reader is most
// likely to get backwards.
//
// Every row measured on bash 5.3.15, 2026-09-14, through `-c`.
func TestTheArgumentsOfEachCallAreKeptWhileExtendedDebuggingIsOn(t *testing.T) {
	const show = `echo "[${BASH_ARGV[@]}] [${BASH_ARGC[@]}]"`
	for _, tc := range []struct{ name, src, want string }{
		{
			// The control, and the reason this is a record rather than a
			// view of the call stack: with the option off there is nothing
			// in it at all.
			"nothing is kept with the option off",
			`f(){ ` + show + `; }; f a b`, "[] []\n",
		},
		{
			"a call and the top level under it",
			`shopt -s extdebug; f(){ ` + show + `; }; f a b`, "[b a] [2 0]\n",
		},
		{
			"two calls, the innermost first and its last argument first",
			`shopt -s extdebug; g(){ ` + show + `; }; f(){ g x y z; }; f a b`,
			"[z y x b a] [3 2 0]\n",
		},
		{
			// The top level's own entry is its positional parameters, which
			// under `-c` with no operands is none — a count of zero rather
			// than no entry.
			"the top level on its own",
			`shopt -s extdebug; ` + show, "[] [0]\n",
		},
		{
			"a call with no arguments is still a call",
			`shopt -s extdebug; f(){ ` + show + `; }; f`, "[] [0 0]\n",
		},
		{
			"a call that has returned is out of it again",
			`shopt -s extdebug; f(){ :; }; f a b; ` + show, "[] [0]\n",
		},
		{
			// Turning the record on records the frame it is turned on in,
			// and nothing below it: there is no `0` here for the top level.
			"turning it on inside a call records that call",
			`f(){ shopt -s extdebug; ` + show + `; }; f a b`, "[b a] [2]\n",
		},
		{
			// And turning it on where the record already reaches adds
			// nothing, which is what keeps `2 2 0` from becoming `2 2 2 0`.
			"turning it on again inside a call already recorded adds nothing",
			`shopt -s extdebug; f(){ shopt -u extdebug; g(){ echo "[${BASH_ARGC[@]}]"; }; ` +
				`shopt -s extdebug; g x y; }; f a b`, "[2 2 0]\n",
		},
		{
			"a call entered with the option off stays out of it",
			`shopt -s extdebug; f(){ echo "[${BASH_ARGC[@]}]"; }; shopt -u extdebug; f a b`,
			"[0]\n",
		},
		{
			// The entry the *enabling* took is not the call's to remove, so
			// it outlives the call. This is the row that says only a call
			// which pushed pops.
			"an entry taken by the enabling outlives the call",
			`f(){ shopt -s extdebug; }; f a b; ` + show, "[b a] [2]\n",
		},
		{
			"and the next call stacks on top of it",
			`f(){ shopt -s extdebug; }; f a b; g(){ ` + show + `; }; g z`,
			"[z b a] [1 2]\n",
		},
		{
			"a subshell inherits the record",
			`shopt -s extdebug; f(){ ( ` + show + ` ); }; f a b`, "[b a] [2 0]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The top level's entry is the shell's own positional parameters, which is
// what makes the trailing `0` above an answer rather than a placeholder:
// given operands, it is their count and they are in `BASH_ARGV` too.
// Measured, `bash -c 'shopt -s extdebug; …' name p1 p2` answers `2` and
// `p2 p1`.
func TestTheTopLevelEntryIsThePositionalParameters(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`set -- p1 p2; shopt -s extdebug; echo "[${BASH_ARGV[@]}] [${BASH_ARGC[@]}]"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "[p2 p1] [2]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}
