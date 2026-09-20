// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// An index array's listing writes its elements as **bare words**, so an
// element holding `a=1` would re-read as the keyed element `[a]=1` — or, under
// the compound reading, as a body. ksh93 answers that by backslashing the `=`
// that ends the value's bare `name=` head, and only where the value stands as
// a bare word inside parentheses.
//
// Every row was measured on ksh93u+ 2012-08-01, 2026-09-20, each one its own
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/ksh x.sh` with
// standard input on `/dev/null` and the output read through `sed -n l`. The
// rows are written out in docs/spec/semantics.md, "The `=` of a bare
// assignment head inside a list" (#3863).
func TestAnIndexArrayElementEscapesItsAssignmentHead(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The issue's own two rows.
		{`typeset -a c=(a=1 b=2); typeset -p c`, "typeset -a c=(a\\=1 b\\=2)\n"},
		{`typeset -a d=("x y" "a=1" "p:q" "-n"); typeset -p d`, "typeset -a d=('x y' a\\=1 p:q -n)\n"},
		// The head is the only part that changes: the tail keeps whichever
		// of the three answers the style gives it, quoted for a blank, a
		// `$`, a paren or a quote of its own.
		{`typeset -a g=("a=1 b" "c=d"); typeset -p g`, "typeset -a g=(a\\='1 b' c\\=d)\n"},
		{`typeset -a i=('a=$y'); typeset -p i`, "typeset -a i=(a\\='$y')\n"},
		{`typeset -a b=('a=(x)'); typeset -p b`, "typeset -a b=(a\\='(x)')\n"},
		{`typeset -a b=("a=it's"); typeset -p b`, "typeset -a b=(a\\=$'it\\'s')\n"},
		{`typeset -a e=('a='); typeset -p e`, "typeset -a e=(a\\=)\n"},
		{`typeset -a f=('_x9=1'); typeset -p f`, "typeset -a f=(_x9\\=1)\n"},
		// Once and at the front, which is the head rule this rides on: the
		// second `=` is inside the quoted tail.
		{`typeset -a h=('a=b=c=d'); typeset -p h`, "typeset -a h=(a\\='b=c=d')\n"},
		// The shapes with no name in front of the first `=` take no head and
		// so no backslash — the whole word is quoted instead.
		{
			`typeset -a j=('=lead' '=' '9x=1' 'a.b=1' 'a-b=1' 'a b=1'); typeset -p j`,
			"typeset -a j=('=lead' '=' '9x=1' 'a.b=1' 'a-b=1' 'a b=1')\n",
		},
		// A word with no `=` at all is untouched, which is the control that
		// says the rule is the head and not the position.
		{`typeset -a k=('a.b' '9x' 'a-b' 'p:q'); typeset -p k`, "typeset -a k=(a.b 9x a-b p:q)\n"},
		// A nested array's elements are bare words in parentheses too.
		{`typeset -a n; n[0]=('a=1' x); typeset -p n`, "typeset -a n=((a\\=1 x) )\n"},
		// A compound body's member **value** takes it, where the member's
		// own `=` does not.
		{`typeset -C co=(p=1 q='a=1'); typeset -p co`, "typeset -C co=(p=1;q=a\\=1)\n"},
		{`typeset -C co=(q='a=1'); printf '[%s]' "$co"`, "[(\n\tq=a\\=1\n)]"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The positions that take **no** backslash, which are what say this is the
// bare-word position rather than the character. Measured in the same run: a
// key sits inside brackets and a subscripted element's value after a `]=`, and
// both are already unambiguous where they stand.
func TestAnAssignmentHeadOutsideAListKeepsItsPlainEquals(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`v=a=1; typeset -p v`, "v=a=1\n"},
		{`v='a=1 b'; typeset -p v`, "v=a='1 b'\n"},
		{`typeset -A m=([k]="a=1"); typeset -p m`, "typeset -A m=([k]=a=1)\n"},
		{`typeset -A t=(['j=2']=v); typeset -p t`, "typeset -A t=([j=2]=v)\n"},
		{`e=(x y z); unset 'e[1]'; e[5]='a=1'; typeset -p e`, "typeset -a e=([0]=x [2]=z [5]=a=1)\n"},
		// And the neighbors of the two rows above that never had an `=` in
		// them, unmoved.
		{`c=(1 2); typeset -p c`, "typeset -a c=(1 2)\n"},
		{`typeset -a c=("x y"); typeset -p c`, "typeset -a c=('x y')\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The two whole-shell listings, which have no name to pass and so are read out
// of the whole output: `set` writes an array's elements as bare words and
// takes the backslash with them, and `export -p` writes a scalar's value with
// nothing around it and does not.
func TestTheWholeShellListingsAgreeAboutTheHead(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -a y=('a=1'); set`, "y=(a\\=1)\n"},
		{`export v9='a=1'; export -p`, "export v9=a=1\n"},
		// The control for each: the same listing over a value with no `=` in
		// it, which neither rule can move.
		{`typeset -a y=('a b'); set`, "y=('a b')\n"},
		{`export v9='a b'; export -p`, "export v9='a b'\n"},
	} {
		if out, st := kshOut(t, c.src); !strings.Contains(out, c.want) || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant a line %q at 0", c.src, out, st, c.want)
		}
	}
}
