// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `compset`, measured through a pseudo-terminal against zsh 5.9.2 on
// 2026-09-15 from inside a `zle -C` widget's function with `git foo=che`
// typed — so `$PREFIX` is `foo=che`, `$SUFFIX` is empty, `$words` is
// `(git foo=che)` and `$CURRENT` is 2. The table in compset.go's file comment
// is this one.
func TestCompsetMovesTheWordsBoundaries(t *testing.T) {
	for _, c := range []struct{ name, call, want string }{
		{"-p by count", "compset -p 2", "st=0 PREFIX=o=che IPREFIX=fo w=git|foo=che C=2"},
		// More characters than `$PREFIX` has: 1, and nothing moves.
		{"-p past the end", "compset -p 99", "st=1 PREFIX=foo=che IPREFIX= w=git|foo=che C=2"},
		// The longest match anchored at the start, which is what makes
		// `_arguments` able to take `--opt=` off a word in one call.
		{"-P longest anchored match", "compset -P '*='", "st=0 PREFIX=che IPREFIX=foo= w=git|foo=che C=2"},
		// Anchored: `ch` is in the word and does not begin it.
		{"-P must begin the word", "compset -P ch", "st=1 PREFIX=foo=che IPREFIX= w=git|foo=che C=2"},
		// The status is "did the pattern match" and not "did anything
		// change": `*` matches the empty `$SUFFIX`, so this is 0 with
		// nothing moved.
		{"-S matches the empty suffix", "compset -S '*'", "st=0 PREFIX=foo=che IPREFIX= w=git|foo=che C=2"},
		// And `-s` by count against an empty `$SUFFIX` is 1, which is the
		// same answer zsh gives for the same state. See compset.go for why
		// `$SUFFIX` is always empty here.
		{"-s against an empty suffix", "compset -s 1", "st=1 PREFIX=foo=che IPREFIX= w=git|foo=che C=2"},
		// `-n` and `-N` renumber the line and move nothing into `$IPREFIX`.
		{"-n drops the words before", "compset -n 2", "st=0 PREFIX=foo=che IPREFIX= w=foo=che C=1"},
		{"-N finds the word by pattern", "compset -N git", "st=0 PREFIX=foo=che IPREFIX= w=foo=che C=1"},
		// One word split into one word, which is what `-q` leaves behind
		// when nothing is quoted.
		{"-q with nothing quoted", "compset -q", "st=0 PREFIX=foo=che IPREFIX= w=foo=che C=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, widgetOf(
				c.call+`; local s=$?; compadd -U -Q -- "st=$s PREFIX=$PREFIX `+
					`IPREFIX=$IPREFIX w=${(j:|:)words} C=$CURRENT"`,
			), "git foo=che")
			// The reading is read off the *end* of the offered word, because
			// what `compset` moved into `$IPREFIX` is inserted in front of
			// every match — which is the next case, and is why an assertion
			// on the whole word would be asserting two things at once.
			if len(got) != 1 || !strings.HasSuffix(got[0], c.want) {
				t.Errorf("%s left\n %q\nwant it to end\n %q", c.call, got, c.want)
			}
		})
	}
}

// What `compset` moved into `$IPREFIX` is still inserted, which is the point
// of moving it rather than dropping it: the text is on the line and a
// replacement word that left it out would delete what a person typed.
//
// Measured on zsh 5.9.2: `compset -P '*='` then `compadd bar` against
// `git foo=che` completes the word to `foo=bar`.
func TestWhatCompsetMovedAsideIsStillInserted(t *testing.T) {
	got := completionFor(t, widgetOf("compset -P '*='; compadd bar baz"), "git foo=b")
	want := []string{"foo=bar", "foo=baz"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("offered %q, want %q", got, want)
	}
}

// TestCompsetTakesTheLongestPrefixAndTheLastWord is the pair of rows the table
// above cannot discriminate, and it is here because the table's own case
// cannot tell the two readings apart: `*=` has exactly one match in `foo=che`,
// so a `-P` that took the shortest match would pass every row above.
//
// Measured on zsh 5.9.2 on 2026-09-15, through a pseudo-terminal from inside
// a `zle -C` widget:
//
//	git a=b=che   compset -P '*='   PREFIX=che  IPREFIX=a=b=
//	a x a che     compset -N a      words=(che) CURRENT=1
//
// So `-P` takes the longest match and `-N` the last word before the current
// one that matches, and both would be wrong the other way round on a line a
// person really types — `--opt=a=b` and a command after a second `;`.
func TestCompsetTakesTheLongestPrefixAndTheLastWord(t *testing.T) {
	for _, c := range []struct{ name, call, line, want string }{
		{
			"-P takes the longest", "compset -P '*='", "git a=b=che",
			"st=0 PREFIX=che IPREFIX=a=b= w=git|a=b=che C=2",
		},
		{
			"-N takes the last", "compset -N a", "a x a che",
			"st=0 PREFIX=che IPREFIX= w=che C=1",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, widgetOf(
				c.call+`; local s=$?; compadd -U -Q -- "st=$s PREFIX=$PREFIX `+
					`IPREFIX=$IPREFIX w=${(j:|:)words} C=$CURRENT"`,
			), c.line)
			if len(got) != 1 || !strings.HasSuffix(got[0], c.want) {
				t.Errorf("%s on %q left\n %q\nwant it to end\n %q", c.call, c.line, got, c.want)
			}
		})
	}
}
