// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// ksh93 writes a word back with **one** function: a trace, `printf %q`,
// `whence`'s not-found sentence and `typeset -p` agree on every word (#6582).
// This shell had three, and only the listing's knew that a leading `name=`
// stands bare, that an `=` anywhere else is quoted, and that `16#ff` is bare.
//
// Measured 2026-10-15 on ksh93u+ 2012-08-01 from a script file, each word
// written all four ways; the four agreed on all 96 words of that sweep, and
// every expected spelling here is that shell's.
func TestAWordIsWrittenBackAsItIsListed(t *testing.T) {
	for _, c := range []struct{ word, want string }{
		{`st=1 [4]`, `st='1 [4]'`}, // the issue's row
		{`x=y z`, `x='y z'`},       // the same head, another tail
		{`a b=c`, `'a b=c'`},       // no name in front of the `=`
		{`1=x y`, `'1=x y'`},       // nor here
		{`a[1]=x y`, `'a[1]=x y'`}, // nor a subscript
		{`a=b`, `a=b`},             // nothing to quote at all
		{`a=`, `a=`},               // and an empty tail is written as nothing
		{`a=b=c`, `a='b=c'`},       // a second `=` is quoted with the tail
		{`1=x`, `'1=x'`},           // and an `=` past the front quotes a word
		{`16#ff`, `16#ff`},         // a `#` after a non-name stands bare
		{`a#b`, `'a#b'`},           // and after a name does not
		{`a=x'y`, `a=$'x\'y'`},     // the tail takes the `$'…'` form
		{"a=x\ty", `a=$'x\ty'`},    // for a control byte as well
		{` a=b`, `' a=b'`},         // a blank in front is not a name
		{`a =b`, `'a =b'`},         // nor one behind it
	} {
		t.Run(c.word, func(t *testing.T) {
			q := strings.ReplaceAll(c.word, `'`, `'\''`)
			src := "w='" + q + "'\n" +
				`printf '%q\n' "$w"` + "\n" +
				`v=$w; typeset -p v` + "\n" +
				`type "$w"` + "\n" +
				"set -x\n: \"$w\"\n"
			got, _ := kshOut(t, src)
			want := c.want + "\n" +
				"v=" + c.want + "\n" +
				"ksh[4]: whence: " + c.want + ": not found\n" +
				"+ : " + c.want + "\n"
			if got != want {
				t.Errorf("%q:\n got %q\nwant %q", c.word, got, want)
			}
		})
	}
}

// The subscript an element assignment resolved to is a word written back too:
// `m["a b"]=w` traces as `m['a b']=w` and `m["k=1 2"]=v` as `m[k='1 2']=v`,
// the keys `typeset -p m` lists, where this wrote them raw.
func TestAResolvedSubscriptIsWrittenBackAsItIsListed(t *testing.T) {
	src := "typeset -A m\nset -x\n" +
		`m["k=1 2"]=v; m["a b"]=w; m[16#f]=1; m[k]=2` + "\n"
	want := "+ m[k='1 2']=v\n+ m['a b']=w\n+ m[16#f]=1\n+ m[k]=2\n"
	if got, _ := kshOut(t, src); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// A character above ASCII is a name character only where the locale has
// characters: `é=a b` is `é='a b'` under UTF-8 and `$'\xc3\xa9=a b'` under C,
// where the listing used to split it anyway and write the bytes raw.
func TestANonAsciiHeadIsANameOnlyWhereTheLocaleHasCharacters(t *testing.T) {
	for _, c := range []struct{ locale, want string }{
		{"en_US.UTF-8", `é='a b'`},
		{"C", `$'\xc3\xa9=a b'`},
	} {
		src := "LC_ALL=" + c.locale + "\nw='é=a b'\n" +
			`printf '%q\n' "$w"; v=$w; typeset -p v` + "\n"
		want := c.want + "\nv=" + c.want + "\n"
		if got, _ := kshOut(t, src); got != want {
			t.Errorf("LC_ALL=%s:\n got %q\nwant %q", c.locale, got, want)
		}
	}
}
