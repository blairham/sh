// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `${a[lo..hi]}` names a range of elements. Every row measured 2026-09-16 on
// ksh93u+ 2012-08-01 from a script file under `env -i` (#3408). Two of these
// spellings — `..2` and `i..i+2` — used to answer one wrong element at status
// 0, because the text reached the arithmetic, which read `.2` and `i.` as
// floating-point constants.
func TestADotRangeNamesTheElementsFromLoThroughHi(t *testing.T) {
	const five = `a=(a b c d e); `
	for _, c := range []struct{ src, want string }{
		{five + `echo "${a[1..3]}"`, "b c d\n"},
		{five + `printf '[%s]' "${a[1..3]}"; echo`, "[b][c][d]\n"},
		{five + `set -- "${a[1..3]}"; echo $#`, "3\n"},
		{five + `x="${a[1..3]}"; echo "$x"`, "b c d\n"},
		{five + `IFS=:; x=${a[1..2]}; echo "$x"`, "b c\n"},
		{five + `echo "${a[..2]}"`, "a b c\n"},
		{five + `i=1; echo "${a[i..i+2]}"`, "b c d\n"},
		{five + `echo "${a[$((1))..$((3))]}"`, "b c d\n"},
		{five + `echo "${a["1..3"]}"`, "b c d\n"},
		// An empty high end is 0, and a range whose high end is below its
		// low one is the first element alone.
		{five + `echo "${a[2..]}"`, "c\n"},
		{five + `echo "${a[..]}"`, "a\n"},
		{five + `echo "${a[3..1]}"`, "d\n"},
		{five + `echo "${a[-1..-3]}"`, "e\n"},
		{five + `echo "${a[1..-9]}"`, "b\n"},
		// Past the last is the last; counting back works from either end.
		{five + `echo "${a[1..9]}"`, "b c d e\n"},
		{five + `echo "${a[-3..-1]}"`, "c d e\n"},
		{five + `echo "${a[1..-1]}"`, "b c d e\n"},
		// Nothing at or above the low end is element 0.
		{five + `echo "${a[5..9]}"`, "a\n"},
		{five + `echo "${!a[5..9]}"`, "0\n"},
		{`a=(a b c d e); unset 'a[0]'; set -- "${a[7..9]}"; echo $#`, "0\n"},
		// A sparse array: the first *set* subscript at or above lo, then
		// every set one through hi.
		{`a[2]=c a[5]=f; echo "${a[3..4]}"`, "f\n"},
		{`a[2]=c a[5]=f; echo "${a[..1]}"`, "c\n"},
		{`a[2]=c a[5]=f; echo "${a[0..9]}"`, "c f\n"},
		{`a[2]=c a[5]=f; echo "<${a[6..9]}>"`, "<>\n"},
		{`a[1]=b a[3]=d; echo "${a[0..3]}"`, "b d\n"},
		{`a[2]=c a[5]=f; echo "${a[-1..5]}" "${a[-4..5]}"`, "f c f\n"},
		// The subscripts, and the element operators applied to each.
		{five + `echo "${!a[1..3]}"`, "1 2 3\n"},
		{five + `echo "${a[1..3]/c/X}"`, "b X d\n"},
		{five + `echo "<${a[1..3]#?}>"`, "<  >\n"},
		{five + `echo "${a[1..3]:+s}"`, "s\n"},
		// A string is an array of one, with nothing to fall back to.
		{`s=hello; echo "<${s[0..3]}>" "<${s[1..3]}>" "<${s[..]}>"`, "<hello> <> <hello>\n"},
		// Every `..` after the first ends the high end again.
		{five + `echo "${a[0..9..3]}"`, "a b c d\n"},
		{five + `echo "${a[1..1+..3]}"`, "b c d\n"},
		{five + `echo "${a[1...3]}"`, "b\n"},
		// A substring's offset replaces the low end.
		{five + `echo "<${a[1..3]:2}>" "<${a[2..4]:1}>" "<${a[1..3]:0}>" "<${a[1..3]:1:2}>"`, "<c d> <b c d e> <b c d> <b c>\n"},
		{five + `echo "<${a[1..3]:4}>" "<${a[1..3]: -1}>" "<${a[1..3]:9}>" "<${a[1..3]:1:0}>"`, "<e> <e> <> <>\n"},
		// An association ranges over its keys in order.
		{`typeset -A h; h[a]=1 h[b]=2 h[c]=3 h[d]=4; echo "<${h[b..c]}>" "<${h[bb..c]}>" "<${h[..b]}>" "<${h[e..f]}>" "<${h[c..b]}>"`, "<2 3> <3> <1 2> <> <3>\n"},
		{`typeset -A h; h[a]=1 h[b]=2 h[c]=3; echo "${!h[b..c]}"`, "b c\n"},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s\n got %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// A range is the construct only when its `..` was written: one produced by a
// substitution, or escaped, is left to the arithmetic, which refuses it.
func TestDotsNotWrittenAreNoRange(t *testing.T) {
	for _, src := range []string{
		`a=(a b c d e); x=1..3; echo "${a[$x]}"; echo after`,
		`a=(a b c d e); x=..; echo "${a[1${x}3]}"; echo after`,
	} {
		out, st := runKsh(t, t.TempDir(), src)
		if want := "ksh: .: invalid character in expression - 1..3\n"; out != want || st != 1 {
			t.Errorf("%s\n got %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}

// What a range refuses: its length, a low end counting back past the first
// element, and an end that is no expression. Each ends the script at 1; the
// length only when it is reached.
func TestADotRangeRefuses(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(a b c d e); echo "${#a[1..3]}"; echo after`, "ksh: \"${#a[1..3]}\": bad substitution\n"},
		{`a=(a b c d e); echo "${a[-9..1]}"; echo after`, "ksh: a: subscript out of range\n"},
		{`a=(a b c d e); echo "${a[1..3+]}"; echo after`, "ksh: 3+: more tokens expected\n"},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s\n got %q (status %d), want %q at 1", c.src, out, st, c.want)
		}
	}
	if out, st := runKsh(t, t.TempDir(), `if false; then echo "${#a[1..3]}"; fi; echo ok`); out != "ok\n" || st != 0 {
		t.Errorf("an untaken length: got %q (status %d), want %q at 0", out, st, "ok\n")
	}
}
