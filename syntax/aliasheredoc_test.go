// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document whose body is text of an alias value.
//
// Substitution is textual in every column: the value stands in the input where
// the alias word stood, and a body is read from the lines after the operator's
// — which, for a value holding newlines, are the value's own, then the text of
// any value it was spliced into, then the input. The rows are the shapes
// measured against bash 5.3.20, ksh93u+ and dash on 2026-09-16, which all three
// answer alike; the program each one reads is compared as the printer writes it.
func TestAHereDocumentBodyIsReadFromTheAliasText(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	for _, c := range []struct {
		name  string
		table syntax.Aliases
		src   string
		want  string
	}{
		{
			name:  "the value holds the whole body, delimiter last",
			table: table("hd", "cat <<EOF\nhello\nEOF"),
			src:   "hd\necho after\n",
			want:  "cat <<EOF\nhello\nEOF\necho after",
		},
		{
			name:  "the value holds the whole body and a command after it",
			table: table("hd", "cat <<EOF\nhello\nEOF\necho two"),
			src:   "hd\necho after\n",
			want:  "cat <<EOF\nhello\nEOF\necho two\necho after",
		},
		{
			name:  "two documents on the operator's line",
			table: table("hd", "cat <<A <<B\na\nA\nb\nB\n"),
			src:   "hd\necho after\n",
			want:  "cat <<A <<B\na\nA\nb\nB\necho after",
		},
		{
			name:  "the body goes on into the input",
			table: table("hd", "cat <<EOF\nin alias\n"),
			src:   "hd\nfrom file\nEOF\necho after\n",
			want:  "cat <<EOF\nin alias\n\nfrom file\nEOF\necho after",
		},
		{
			name:  "the rest of the alias word's line is a body line",
			table: table("hd", "cat <<EOF\nin alias\nEOF"),
			src:   "hd; echo same\necho after\nEOF\necho end\n",
			want:  "cat <<EOF\nin alias\nEOF; echo same\necho after\nEOF\necho end",
		},
		{
			name:  "the operator in one value, the body in the value around it",
			table: table("Y", `cat <<\END`, "X", "Y\ntext\nEND\necho inX"),
			src:   "X\necho after\n",
			want:  "cat <<\\END\ntext\nEND\necho inX\necho after",
		},
		{
			name:  "a body split between the two values and the input",
			table: table("Y", "cat <<END\nin Y", "X", "Y\nin X"),
			src:   "X\nin file\nEND\necho after\n",
			want:  "cat <<END\nin Y\nin X\nin file\nEND\necho after",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedWith(t, d, c.table, c.src); got != c.want {
				t.Errorf("%q read as\n%s\nwant\n%s", c.src, got, c.want)
			}
		})
	}
}

// The controls, which read the same before and after.
func TestAliasHeredocControls(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	for _, c := range []struct {
		name  string
		table syntax.Aliases
		src   string
		want  string
	}{
		{
			name:  "the operator alone in the value reads the body from the input",
			table: table("hd", "cat <<EOF"),
			src:   "hd\nhello\nEOF\necho after\n",
			want:  "cat <<EOF\nhello\nEOF\necho after",
		},
		{
			name:  "no here-document at all",
			table: table("two", "echo a\necho b"),
			src:   "two\necho after\n",
			want:  "echo a\necho b\necho after",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedWith(t, d, c.table, c.src); got != c.want {
				t.Errorf("%q read as\n%s\nwant\n%s", c.src, got, c.want)
			}
		})
	}
}

// Where the seam between a value and the input reads as a blank — zsh 5.9.2 —
// a body line the value ends in the middle of gains one, unless the input
// already has one there.
func TestTheSeamIsABlankWhereABackslashDoesNotJoin(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = false
	for _, c := range []struct {
		name, value, src, want string
	}{
		{"a line the value ends mid-way", "cat <<END\nin Y", "hd\nEND\n", "cat <<END\nin Y \nEND"},
		{"a blank already there", "cat <<END\nin Y", "hd x\nEND\n", "cat <<END\nin Y x\nEND"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedWith(t, d, table("hd", c.value), c.src); got != c.want {
				t.Errorf("%q read as\n%s\nwant\n%s", c.src, got, c.want)
			}
		})
	}
}
