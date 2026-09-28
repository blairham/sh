// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `read -E` writes the values it read and assigns them; `read -e` writes the
// same values and assigns nothing — #4963.
//
// The letters are not bash's, where the pair opens a line editor and does
// nothing at all off a terminal. Every want below is zsh 5.9.2 with `-f`,
// measured 2026-09-27, and the rows are chosen so that each one rules out a
// reading the others would let through:
//
//   - **the values, not the record.** Two names over three words writes *two*
//     lines, the second holding the remainder as it stood — so what is echoed
//     is the fields as they would be assigned. The bare-name row is the same
//     rule from the other side: the record goes to the shell's own name whole,
//     and that whole record is the one value written.
//   - **a name with no field still gets its line**, so the count is the
//     names' and not the fields'.
//   - **`-e` leaves a name as it was.** Every other failed or partial read
//     here clears; this one does not, so it cannot be "assign, then undo".
//   - **`-e` attempts no write at all**: a readonly name is silence at 0
//     rather than the frozen-name complaint.
//   - **`-e` wins over `-E`** where both are written, which is why one axis
//     carries the pair.
//   - **the echo is standard output**, shown from both sides — with stderr
//     closed the line is still there, with stdout closed it is gone.
func TestTheReadEchoLettersWriteTheValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"-E writes the fields and assigns them",
			`printf "  a   b   c  \n" | { read -E x y; print "x=[$x] y=[$y]" }`,
			"a\nb   c\nx=[a] y=[b   c]\n",
		},
		{
			"-e writes the same fields and assigns none",
			`printf "  a   b   c  \n" | { read -e x y; print "x=[$x] y=[$y]" }`,
			"a\nb   c\nx=[] y=[]\n",
		},
		{
			"an array's elements are the values",
			`printf "a b c\n" | { read -E -A r; print "n=${#r} j=${(j:,:)r}" }`,
			"a\nb\nc\nn=3 j=a,b,c\n",
		},
		{
			"and -e writes them without filling the array",
			`printf "a b c\n" | { read -e -A r; print "n=${#r}" }`,
			"a\nb\nc\nn=0\n",
		},
		{
			"a name with no field still gets its line",
			`printf "a\n" | { read -E x y z; print "x=[$x] y=[$y] z=[$z]" }`,
			"a\n\n\nx=[a] y=[] z=[]\n",
		},
		{
			"the shell's own name takes the record whole and echoes it whole",
			`printf "  a  b  \n" | { read -E; print "[$REPLY]" }`,
			"a  b\n[a  b]\n",
		},
		{
			"-e leaves a name as it was rather than clearing it",
			`printf "a\n" | { x=keep; read -e x; print "st=$? x=[$x]" }`,
			"a\nst=0 x=[keep]\n",
		},
		{
			"-e attempts no write, so a readonly name is not a refusal",
			`printf "a\n" | { typeset -r x=orig; read -e x; print "st=$? x=[$x]" }`,
			"a\nst=0 x=[orig]\n",
		},
		{
			"end of input still writes the value it would have assigned",
			`printf "" | { read -e x; print "st=$?" }`,
			"\nst=1\n",
		},
		{
			"-e wins over -E",
			`printf "a\n" | { read -eE x; print "x=[$x]" }`,
			"a\nx=[]\n",
		},
		{
			"the echo survives a closed standard error",
			`printf "a\n" | { read -e x 2>/dev/null }`,
			"a\n",
		},
		{
			"and goes with a closed standard output",
			`printf "a\n" | { read -e x 1>/dev/null; print done }`,
			"done\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
