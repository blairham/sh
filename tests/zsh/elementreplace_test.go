// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `${a:/pattern/replacement}` in this dialect, measured on zsh 5.9.2 with
// `-f` — see docs/spec/grammar/parameter-expansion.md (#1617).
//
// The operator is what `compaudit:60` and `compdump:26` build their file list
// with — `_i_files=( ${^~fpath:/.}/^([^_]*|*~|*.zwc)(N) )`, which is `.` being
// struck out of `fpath` — and what `_p9k_must_init:22` reads `$parameters`
// with. Reading the `:/` as an arithmetic offset made all three of them
// `bad math expression: operand expected`, so the dump was never written and
// no prompt was drawn.
func TestTheWholeElementReplacementIsThisDialects(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the elements a pattern matches whole become the replacement",
			`a=(foo bar); print -r -- "[${(@)a:/foo/Z}]"`,
			"[Z bar]\n",
		},
		{
			// Not the span replacement with a colon in front, which is the
			// wrong turn a fix takes: `foo` is a prefix of `foobar` here.
			"a pattern matching part of a value replaces nothing",
			`x=foobar; print -r -- "[${x:/foo/Z}][${x/foo/Z}]"`,
			"[foobar][Zbar]\n",
		},
		{
			"a value the pattern does not match is left alone and says nothing",
			`x=/a/b/c; print -r -- "[${x:/b/Z}]"; print -r -- "st=$?"`,
			"[/a/b/c]\nst=0\n",
		},
		{
			"an array without (@) in double quotes joins first, so it matches nothing",
			`a=(foo bar); print -r -- "[${a:/foo/Z}]"`,
			"[foo bar]\n",
		},
		{
			"and unquoted it is one element at a time",
			`a=(foo bar); print -rl -- ${a:/foo/Z}`,
			"Z\nbar\n",
		},
		{
			"the offset it sits beside is untouched",
			`x=abcdef; print -r -- "[${x:2}][${x:2:3}][${x: -2}][${x:-2}][${x:3/2}]"`,
			"[cdef][cde][ef][abcdef][bcdef]\n",
		},
		{
			// The shape both completion files depend on: `.` out of `fpath`,
			// spelled with no replacement at all.
			"an empty replacement drops the elements it matched",
			`fpath=(/x . /y); print -rl -- ${^fpath:/.}`,
			"/x\n/y\n",
		},
		{
			"a scalar emptied by the operator is still a field",
			`x=foo; b=("${x:/foo}"); print -r -- "n=$#b [${b[1]}]"`,
			"n=1 []\n",
		},
		{
			"where a list every element matches is no field at all",
			`a=(foo); b=("${(@)a:/foo}"); print -r -- "n=$#b"`,
			"n=0\n",
		},
		{
			"a quoted [*] joins before the pattern is tested",
			`a=(foo bar baz); print -r -- "[${a[*]:/ba*/Z}][${a[@]:/ba*/Z}]"`,
			"[foo bar baz][foo Z Z]\n",
		},
		{
			// The `M` flag reads `:#` from the other side and has nothing to
			// reverse here, so it changes nothing.
			"the matching flag is ignored",
			`a=(foo bar); print -r -- "[${(M@)a:/foo/Z}]"`,
			"[Z bar]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// `(#m)` and `(#b)` report into the replacement, and the positions they report
// are counted from this dialect's array base.
//
// Under `extendedglob` and not without it: the same line with the option off
// reads the flag group as an ordinary pattern, matches nothing, and leaves the
// array as it was. Both halves are here because the pair is what says the
// reporting is the pattern's doing.
func TestTheWholeElementReplacementReportsIntoItsReplacement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"$MATCH is the element the pattern took",
			`setopt extendedglob; a=(foo bar); print -r -- "[${(@)a:/(#m)*/<$MATCH>}]"`,
			"[<foo> <bar>]\n",
		},
		{
			"without extendedglob there is no flag to report",
			`a=(foo bar); print -r -- "[${(@)a:/(#m)*/<$MATCH>}]"`,
			"[foo bar]\n",
		},
		{
			"$MBEGIN and $MEND are counted from one",
			`setopt extendedglob; a=(foo); print -r -- "[${(@)a:/(#m)*/$MBEGIN-$MEND}]"`,
			"[1-3]\n",
		},
		{
			"$match is filled again for every element",
			`setopt extendedglob; a=(ab cd); print -r -- "[${(@)a:/(#b)(?)(?)/<$match[2]$match[1]>}]"`,
			"[<ba> <dc>]\n",
		},
		{
			"a pattern that reports nothing reads the replacement once",
			`a=(x y z); i=0; print -r -- "[${(@)a:/*/$((++i))}]"`,
			"[1 1 1]\n",
		},
		{
			"and one that reports reads it again for each match",
			`setopt extendedglob; a=(x y z); i=0; print -r -- "[${(@)a:/(#m)*/$((++i))}]"`,
			"[1 2 3]\n",
		},
		{
			"the pattern itself is expanded once for the whole list",
			`a=(foo bar baz); print -r -- "[${(@)a:/$(print -n ran >&2; print -n bar)/Z}]"`,
			"ran[foo Z baz]\n",
		},
		{
			// The pattern is built from the operand's *word*, so whether a
			// metacharacter out of an expansion is one is the same axis that
			// keeps `p='t*'; echo $p` from globbing — and in this dialect it
			// is not.
			"a metacharacter out of a parameter is two literal characters",
			`p='ba*'; a=(foo bar baz); print -r -- "[${(@)a:/$p/Z}]"`,
			"[foo bar baz]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
