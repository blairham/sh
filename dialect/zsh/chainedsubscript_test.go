// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A second subscript on a subscripted name — `${m[k][2]}` — end to end as
// this dialect.
//
// Measured 2026-09-08 on zsh 5.9.2 against bash 5.3.15, that binary as `sh`,
// bash 3.2.57, ksh93 and dash: five different answers to `${a[1][2]}`, and
// this is the one that reads both subscripts. The interp package holds what
// the reading *is*; this holds that the preset turns it on and that the
// spellings a real script writes come out right.
//
// `~/.zi/bin/zi.zsh` writes `${ICE[atload][1]}` ten times, in the function
// that sources a plugin: it asks whether the ice's value begins with a `!`,
// which is how it decides whether the load needs tracking (#1516).
func TestASecondSubscriptReadsWhatTheFirstNamed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the shape a plugin manager writes",
			`typeset -A ICE=(atload "!y"); echo "[${ICE[atload][1]}]"`, "[!]\n",
		},
		{"characters of an element", `a=(one two three); echo "[${a[1][2]}]"`, "[n]\n"},
		{"characters of a key's value", `typeset -A m=(k abc); echo "[${m[k][2]}]"`, "[b]\n"},
		{"counted back from the end", `typeset -A m=(k abc); echo "[${m[k][-1]}]"`, "[c]\n"},
		{"a range of characters", `a=(one two three); echo "[${a[3][2,4]}]"`, "[hre]\n"},
		{"elements of a range", `a=(one two three four five); echo "[${a[2,4][1]}]"`, "[two]\n"},
		{"elements of the whole array", `a=(one two three); echo "[${a[@][2]}]"`, "[two]\n"},
		{"a range of a range", `a=(one two three four five); echo "[${a[2,4][1,2]}]"`, "[two three]\n"},
		{"three deep", `a=(one two three four five); echo "[${a[2,4][2][3]}]"`, "[r]\n"},
		{"a count and a width", `a=(one two three); echo "[${#a[1,3][1,2]}][${#a[1,3][2]}]"`, "[2][3]\n"},
		{"unset when a link named nothing", `a=(x y); echo "[${a[9][1]-none}]"`, "[none]\n"},
		{"a search then a character", `a=(one two three); echo "[${a[(r)two][1]}]"`, "[t]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The chain is the braced spelling's. Written bare this shell reads one
// subscript and leaves the rest as ordinary text — measured, `$a[1][2]` on
// `(hello world)` is `hello[2]` — so the two spellings are not the same node
// here, where every other subscript reading they share is.
func TestABareChainIsOneSubscriptAndThenText(t *testing.T) {
	out, st := answersRun(t, `a=(hello world); echo "[$a[1][2]]"`)
	if want := "[hello[2]]\n"; out != want || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0", out, st, want)
	}
}

// A chain on a nested expansion is the two constructs at once, and this tree
// refuses it by name rather than answering with the last subscript alone.
func TestAChainOnANestedExpansionIsRefusedHere(t *testing.T) {
	out, st := answersRun(t, `a=(hello world); echo "[${${a}[1][2]}]"`)
	const want = "${${a}[1][2]}: a chain of subscripts on a nested expansion is not implemented"
	if !strings.Contains(out, want) {
		t.Errorf("gave %q, want a refusal naming %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the unbuilt shape refused")
	}
}
