// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// The dialect turns the grammar on, so the real line that needs it runs here.
//
// The substrate's tests name the flag — `NamelessParamExpansion` — and this
// one names the shell, which is the only place that is allowed. It is worth
// having for the reason the nested-expansion one is: a flag nothing turns on
// is a construct nobody can write, and nothing in `syntax` or `interp` can
// notice that.
func TestANamelessExpansionIsThisDialects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"a default over the name that is not there",
			`printf "[%s]" "${:-abc}" "${:+abc}" "${}"`,
			"[abc][][]",
		},
		{
			"the operand is a word and it nests",
			`v=q; printf "[%s]" "${:-x${v}y}" "${:-${:-a}}"`,
			"[xqy][a]",
		},
		{
			// The pair that says the flag group renders rather than reads:
			// the same expansion with and without one.
			"a flag group beside no flag group",
			`printf "[%s]" "${(U):-abc}" "${:-abc}" "${(q):-a b}"`,
			`[ABC][abc][a\ b]`,
		},
		{
			"a length over it",
			`set -- p q; printf "[%s]" "${#:-word}" "${#}"`,
			"[4][2]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The line a plugin manager actually writes, in the dialect that has to run
// it.
//
// `~/.zi/bin/zi.zsh` builds the argv of every non-zsh plugin out of this
// block, and its second-to-last word is a nameless expansion nested inside a
// nameless expansion:
//
//	${(s: :):-${${:-${(@s: :):--o}" "${(s: :)^ICE[opts]}}:#-o }}
//
// Until the empty name read, that word was a bad substitution and the whole
// `precm=(…)` assignment stopped — which is the state a real startup was in
// with the rc-expand flag beside it already fixed (#1517, #1529).
//
// Asserted as the whole argv rather than as the one word, because the word is
// only correct in company: the option letters it produces have to interleave
// with the ones the earlier lines produce, and an implementation that
// substituted a plausible fragment would still hand `emulate` a broken
// command line.
func TestThePluginArgvLineRuns(t *testing.T) {
	const src = `typeset -A ICE
ICE=(bash "" opts "noglob extendedglob")
local -a precm
precm=(
  builtin emulate
  ${${(M)${ICE[(i)(\!|)(sh|bash|ksh|csh)]}#\!}:+-R}
  ${${${ICE[(i)(\!|)(sh|bash|ksh|csh)]}#\!}:-zsh}
  ${${ICE[(i)(\!|)bash]}:+-${(s: :):-o noshglob -o braceexpand -o kshglob}}
  ${(s: :):-${${:-${(@s: :):--o}" "${(s: :)^ICE[opts]}}:#-o }}
  -c
)
printf "[%s]" "${precm[@]}"`
	want := "[builtin][emulate][bash][-o][noshglob][-o][braceexpand][-o][kshglob]" +
		"[-o][noglob][-o][extendedglob][-c]"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
