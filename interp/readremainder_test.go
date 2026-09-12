// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// The last name on a `read` takes the remainder of the *line*, not the
// remaining fields joined back together.
//
// The difference is invisible under the default IFS, where the separator a
// rejoin writes is the separator the input had, and that is what made it a
// silent wrong answer: `IFS=: read -r user rest` on a passwd line gave the
// right number of words with every colon replaced by a space, at status 0 and
// with nothing said (#1208). Anything that then split `$rest` on `:` again
// found one field.
//
// Measured 2026-09-07 across dash, bash 5.3.15, bash as `sh`, bash 3.2.57,
// ksh93u+ and zsh 5.9.2. Every row below is unanimous across the six.
func TestTheLastNameOnAReadTakesTheRemainderOfTheLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the idiom, whole",
			`printf 'root:x:0:0:Root User:/root:/bin/sh\n' | { IFS=: read -r user rest; echo "[$user][$rest]"; }`,
			`[root][x:0:0:Root User:/root:/bin/sh]`,
		},
		{
			"the separators the input had survive",
			`printf 'a:b:c\n' | { IFS=: read -r x y; echo "[$x][$y]"; }`, `[a][b:c]`,
		},
		{
			"the control: under the default IFS both readings agree",
			`printf 'a b c\n' | { read -r x y; echo "[$x][$y]"; }`, `[a][b c]`,
		},
		{
			"and the control has teeth only with a run in it",
			`printf 'a   b   c\n' | { read -r x y; echo "[$x][$y]"; }`, `[a][b   c]`,
		},
		{
			"an empty field inside the remainder is a separator, not a word",
			`printf 'a::b\n' | { IFS=: read -r x y; echo "[$x][$y]"; }`, `[a][:b]`,
		},
		{
			"a leading separator opens a field and the remainder starts after it",
			`printf ':a:b\n' | { IFS=: read -r x y; echo "[$x][$y]"; }`, `[][a:b]`,
		},
		{
			"one name takes the line as it was written",
			`printf ':a:b:\n' | { IFS=: read -r line; echo "[$line]"; }`, `[:a:b:]`,
		},
		{
			"a closing run of separators is part of the remainder",
			`printf 'a:b:c::\n' | { IFS=: read -r x y; echo "[$x][$y]"; }`, `[a][b:c::]`,
		},
		{
			"more names than fields still clears the rest",
			`printf 'a:b\n' | { IFS=: read -r x y z; echo "[$x][$y][$z]"; }`, `[a][b][]`,
		},
		{
			"one name per field is one field each",
			`printf 'a:b:c\n' | { IFS=: read -r x y z; echo "[$x][$y][$z]"; }`, `[a][b][c]`,
		},
		{
			"raw or not, the remainder is the text after the escapes came off",
			`printf 'a:b\\:c:d\n' | { IFS=: read x y; echo "[$x][$y]"; }`, `[a][b:c:d]`,
		},
		{
			"an escaped separator does not open the remainder either",
			`printf 'a\\:b:c:d\n' | { IFS=: read x y; echo "[$x][$y]"; }`, `[a:b][c:d]`,
		},
		{
			"with -r the backslash is a character of the line",
			`printf 'a:b:c\\\n' | { IFS=: read -r x y; echo "[$x][$y]"; }`, `[a][b:c\]`,
		},
		{
			"an empty IFS hands the whole line to the first name",
			`printf 'a:b c\n' | { IFS= read -r x y; echo "[$x][$y]"; }`, `[a:b c][]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if !strings.Contains(out, tc.want) || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// What comes off the end of the remainder is the closing run of IFS
// *whitespace*, and nothing else.
//
// Three things have to be true at once, and each is a row a plausible fix gets
// wrong. A closing non-whitespace separator is kept — trimming "trailing IFS
// characters" would eat the `::` in the first row. A whitespace character that
// is not in IFS is kept too, so this is IFS's question and not
// unicode.IsSpace's. And the run comes off the *end* only: the whitespace
// between the fields of the remainder is text.
//
// Unanimous across the panel, measured 2026-09-07 — with one exception, which
// is the escaped closing space. Whether the mask reaches the trim is
// Semantics.ReadTrailingEscapedSeparator and is asserted both ways in
// readescapedseparator_test.go; the rows here are the ones the panel agrees
// about, and every one of them is written with `-r` or with no backslash in
// it so that none of them reaches the axis.
func TestAReadRemainderLosesOnlyItsClosingIFSWhitespace(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a closing separator run is not whitespace and stays",
			`printf 'a:b:c::\n' | { IFS=: read -r x y; echo "[$y]"; }`, `[b:c::]`,
		},
		{
			"a closing space with IFS holding one comes off",
			`printf 'a:b:c  \n' | { IFS=': ' read -r x y; echo "[$y]"; }`, `[b:c]`,
		},
		{
			"the same input with IFS not holding a space keeps it",
			`printf 'a:b:c  \n' | { IFS=: read -r x y; echo "[$y]"; }`, "[b:c  ]",
		},
		{
			"a closing run ending in a separator is kept whole",
			`printf 'a:b:c ::\n' | { IFS=': ' read -r x y; echo "[$y]"; }`, `[b:c ::]`,
		},
		{
			"whitespace inside the remainder is text",
			`printf 'a: b: c\n' | { IFS=': ' read -r x y; echo "[$y]"; }`, `[b: c]`,
		},
		{
			"leading whitespace is the splitter's and is gone before the remainder starts",
			`printf '  a b c  \n' | { read -r line; echo "[$line]"; }`, `[a b c]`,
		},
		{
			"a whitespace delimiter around a separator is one delimiter",
			`printf 'a : b : c\n' | { IFS=': ' read -r x y; echo "[$y]"; }`, `[b : c]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if !strings.Contains(out, tc.want) || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
