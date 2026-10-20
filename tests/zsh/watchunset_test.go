// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `unset` of one half of `$WATCH`/`$watch` — the pair the shell joins without
// tying, and the one join in this engine that comes apart.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0) — run `-f`
// from a script file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a
// scratch `HOME`, **one shell per row**, and with `${+name}` on every row
// rather than the value: the value alone cannot tell a name holding nothing
// from a name that is gone.

// watchProbe is what every row below prints, so that a row is the two lines
// in front of it and nothing else.
const watchProbe = `print -r -- "+W=${+WATCH} +w=${+watch} W=[$WATCH] ` +
	`w=[${(j:,:)watch}] tW=[${(t)WATCH}] tw=[${(t)watch}]"`

// TestUnsettingOneHalfOfTheWatchPairLeavesTheOther is the row #4999 filed.
//
// **The surviving half is emptied and keeps `special`**, which is the half a
// set test cannot see on its own: `${+WATCH}` of 1 says the name is there and
// `W=[]` says the pair's value went with the half that was removed.
func TestUnsettingOneHalfOfTheWatchPairLeavesTheOther(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the array half", "unset watch",
			"+W=1 +w=0 W=[] w=[] tW=[scalar-special] tw=[]\n",
		},
		{
			"the scalar half", "unset WATCH",
			"+W=0 +w=1 W=[] w=[] tW=[] tw=[array-special]\n",
		},
		// Both, one line at a time, which is the row that says the emptying
		// above is not a re-creation: the second `unset` finds the other
		// half already gone and leaves it gone.
		{
			"the array half then the scalar", "unset watch\nunset WATCH",
			"+W=0 +w=0 W=[] w=[] tW=[] tw=[]\n",
		},
		{
			"the scalar half then the array", "unset WATCH\nunset watch",
			"+W=0 +w=0 W=[] w=[] tW=[] tw=[]\n",
		},
		{
			"the same half twice", "unset watch\nunset watch",
			"+W=1 +w=0 W=[] w=[] tW=[scalar-special] tw=[]\n",
		},
		// The control: with no `unset` in front of it the pair is whole, so
		// every row above is saying something about the removal rather than
		// about the pair.
		{
			"no unset at all", ":",
			"+W=1 +w=1 W=[a:b] w=[a,b] tW=[scalar-special] tw=[array-special]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), "watch=(a b)\n"+tc.src+"\n"+watchProbe)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// And what a write does afterwards, which is the half of the measurement the
// removal alone does not state.
//
// One sentence: **a write re-creates the half it is written to, and mirrors
// into the other half only if that half still exists.** The grid varies which
// half was removed *and* which half is then written, which is the pair of
// nouns the rule is keyed on — a reading that mirrored unconditionally gets
// the first and last rows wrong, and one that stopped mirroring after an
// `unset` gets the middle two wrong.
func TestAWriteAfterHalfThePairIsRemovedMirrorsOnlyIntoAHalfThatIsThere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the array gone, the scalar written",
			"unset watch\nWATCH=x:y",
			"+W=1 +w=0 W=[x:y] w=[] tW=[scalar-special] tw=[]\n",
		},
		{
			"the array gone, the array written",
			"unset watch\nwatch=(q r)",
			"+W=1 +w=1 W=[q:r] w=[q,r] tW=[scalar-special] tw=[array-special]\n",
		},
		{
			"the scalar gone, the scalar written",
			"unset WATCH\nWATCH=x:y",
			"+W=1 +w=1 W=[x:y] w=[x,y] tW=[scalar-special] tw=[array-special]\n",
		},
		{
			"the scalar gone, the array written",
			"unset WATCH\nwatch=(q r)",
			"+W=0 +w=1 W=[] w=[q,r] tW=[] tw=[array-special]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), "watch=(a b)\n"+tc.src+"\n"+watchProbe)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// The nine **worded** pairs are the control, and they are not an afterthought:
// #4999 was left unfixed on the reading that half a pair is a state nothing in
// this engine can be in, which would have made this a change to what `unset`
// means for every pair. It is not — `interp.Runner.PairNames` has one user and
// the tie table has the rest — and these rows are what says so.
//
// Measured in the same run: every one of them answers the same in both
// shells, before this change and after it.
func TestUnsettingHalfOfATiedPairStillRemovesBoth(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a tie a script made", "typeset -T SCA sca\nSCA=a:b\nunset SCA\n" +
				`print -r -- "+S=${+SCA} +s=${+sca} t=[${(t)sca}]"`,
			"+S=0 +s=0 t=[]\n",
		},
		{
			"the shell's own, scalar half", "unset PATH\n" +
				`print -r -- "+P=${+PATH} +p=${+path}"`,
			"+P=0 +p=0\n",
		},
		{
			"the shell's own, array half", "unset path\n" +
				`print -r -- "+P=${+PATH} +p=${+path}"`,
			"+P=0 +p=0\n",
		},
		// And the pairing is kept, which is what parts a tie from the pair
		// above: both names went, and a write to either re-makes the two.
		{
			"a write after the array half went", "unset path\nPATH=/y\n" +
				`print -r -- "+P=${+PATH} +p=${+path} p=[${(j:,:)path}]"`,
			"+P=1 +p=1 p=[/y]\n",
		},
		{
			"a write after the scalar half went", "unset PATH\npath=(/q)\n" +
				`print -r -- "+P=${+PATH} +p=${+path} P=[$PATH]"`,
			"+P=1 +p=1 P=[/q]\n",
		},
		{
			"another of the eight", "unset cdpath\nCDPATH=/z\n" +
				`print -r -- "+C=${+CDPATH} +c=${+cdpath} c=[${(j:,:)cdpath}]"`,
			"+C=1 +c=1 c=[/z]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}
