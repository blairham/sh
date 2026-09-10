// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `$(( m[$w] ))` with an empty `$w` is `$(( m[] ))` by the time the expression
// exists, because an arithmetic expansion substitutes its parameters before it
// parses. This shell answers that two ways and the two part on whether the
// name is *there*, not on whether it is an array. Measured against zsh 5.9.2
// (2026-09-10); found in a real interactive startup, where a completion
// plugin's `bind_count=$((_ZSH_AUTOSUGGEST_BIND_COUNTS[$widget]))` reached it
// with `$widget` empty (#1745).
func TestAnEmptyArithmeticSubscriptOnAnUnsetNameIsZero(t *testing.T) {
	for _, src := range []string{
		`echo $(( m[] )); echo after`,
		`w=; echo $(( m[$w] )); echo after`,
		`w=; n=$((nodecl[$w])); echo "n=$n"; echo after`,
		`echo $(( notset[] + 1 )); echo after`,
		// Once declared and then taken away again it is unset once more.
		`typeset -gA m; unset m; w=; n=$((m[$w])); echo "n=$n"; echo after`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if st != 0 {
			t.Errorf("%s = %q (status %d), want 0", src, out, st)
		}
		if want := "after\n"; len(out) < len(want) || out[len(out)-len(want):] != want {
			t.Errorf("%s = %q, want the script to run on", src, out)
		}
	}
	// The values, stated rather than implied: a name nothing declared reads
	// as the plain unset operand `$(( nosuchvar ))` is.
	for _, tc := range []struct{ src, want string }{
		{`echo $(( m[] ))`, "0\n"},
		{`w=; echo $(( m[$w] ))`, "0\n"},
		{`echo $(( notset[] + 1 ))`, "1\n"},
		{`echo $(( 1 + notset[] ))`, "1\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The subscript is not looked at at all, which is what makes the row above a
// consequence rather than a special case: a subscript that would divide by
// zero does not, and one that would step a counter does not.
func TestASubscriptOfAnUnsetNameIsNeverEvaluated(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $(( nodecl[1/0] ))`, "0\n"},
		{`i=0; echo $(( nodecl[i++] )); echo "i=$i"`, "0\ni=0\n"},
		{`echo $(( nodecl[nodecl2[]] ))`, "0\n"},
		{`echo $(( nodecl[x=5] )); echo "x=[$x]"`, "0\nx=[]\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
	// And it *is* looked at once the name exists, which is the other half:
	// set-ness and not emptiness decides it.
	for _, src := range []string{
		`e=; echo $(( e[1/0] ))`,
		`typeset -ga a; a=(1 2 3); echo $(( a[1/0] ))`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if out != "zsh:1: division by zero\n" || st != 1 {
			t.Errorf("%s = %q (status %d), want the division refused at 1", src, out, st)
		}
	}
}

// On a name that is there, the complaint is the subscript machinery's own and
// not the expression parser's: `invalid subscript`, with no `bad math
// expression` in front of it and no name after it, and the expression produces
// no value at all. Every kind the name can be, because the question is whether
// it exists and not whether it is an array — a plain scalar and an integer are
// refused exactly as a table is.
func TestAnEmptyArithmeticSubscriptOnASetNameIsInvalid(t *testing.T) {
	for _, src := range []string{
		`typeset -gA m; echo $(( m[] )); echo after`,
		`typeset -gA m; m[k]=3; echo $(( m[] )); echo after`,
		`typeset -gA m; w=; echo $(( m[$w] )); echo after`,
		`typeset -ga a; echo $(( a[] )); echo after`,
		`typeset -ga a; a=(1 2 3); w=; echo $(( a[$w] )); echo after`,
		`s=hello; echo $(( s[] )); echo after`,
		`nodecl=; echo $(( nodecl[] )); echo after`,
		`integer i=5; w=; echo $(( i[$w] )); echo after`,
		`w=; echo $(( PATH[$w] )); echo after`,
		`typeset -gA m; w=; print -r -- $(( m[$w] + 1 )); echo after`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if out != "zsh:1: invalid subscript\n" || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1",
				src, out, st, "zsh:1: invalid subscript\n")
		}
	}
}

// An operator writing through the same brackets makes the same complaint, and
// writes nothing. The array is the assertion that matters: the target has no
// index, so a write that fell through would store the number under the *bare
// name* and replace the array with it.
func TestAnEmptyArithmeticSubscriptRefusesAnIncrement(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -gA m; w=; (( m[$w]++ )); echo "keys=[${(k)m}]"`, "keys=[]\n"},
		{`typeset -ga a; a=(1 2 3); w=; (( a[$w]++ )); echo "a=[${a[*]}]"`, "a=[1 2 3]\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		want := "zsh:1: invalid subscript\n" + tc.want
		if out != want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, want)
		}
	}
}

// The neighbouring rows this must not have moved: they already agreed, and a
// fix that reached them would have been a fix in the wrong place.
func TestTheSubscriptsThatWereAlreadyRight(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -gA m; m[k]=7; echo $(( m[nokey] ))`, "0\n"},
		{`typeset -ga a; a=(1 2 3); echo $(( a[9] ))`, "0\n"},
		{`echo $(( nosuchvar ))`, "0\n"},
		{`typeset -gA m; m[abc]=7; echo $(( m[abc] ))`, "7\n"},
		{`typeset -ga a; a=(1 2 3); echo $(( a[3] ))`, "3\n"},
		// A quoted empty subscript is a key and not an empty pair of
		// brackets, which is why the refusal above cannot be about the text
		// between them being blank.
		{`typeset -gA m; w=; echo "[${m[$w]}]"`, "[]\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
