// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A command substitution's child carries the part of its parent's trace line
// that is not written yet, and three more of the lines it opens take that text
// the way an assignment list and a `case` arm already did (#6623). Measured
// 2026-10-30 against zsh 5.9.2 under `-f -c`, both streams on one pipe (#6711).
//
// A prefix's line starts with the inherited text, and a substitution of its own
// takes the text with it. A substitution run inside a compound command or a
// redirection takes it as well, where one in a plain command's word leaves it
// for the command's own line. A `[[ … ]]` is written as far as it got ahead of
// a substitution in an operand, and a pattern operand is expanded a second
// time for the line. And a prefix that stopped the child on an exit request
// is not written, where the inherited text goes out ahead of the complaint.
func TestASubstitutionCarriesItsParentsOpenLine(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a prefix", `set -x; x=$( a=1 true )`, "+zsh:1> x=+zsh:1> a=1 +zsh:1> true\n+zsh:1> x='' \n"},
		{
			"a prefix to a program",
			`set -x; x=$( a=1 /usr/bin/true )`,
			"+zsh:1> x=+zsh:1> a=1 /usr/bin/true\n+zsh:1> x='' \n",
		},
		{
			"a substitution in a prefix",
			`set -x; x=$( a=$(echo s >&2) true )`,
			"+zsh:1> x=+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' +zsh:1> true\n+zsh:1> x='' \n",
		},
		{
			"two substitutions in a prefix",
			`set -x; x=$( a=$(echo s >&2) b=$(echo t >&2) true )`,
			"+zsh:1> x=+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' b=+zsh:1> echo t\nt\n" +
				"+zsh:1> a='' b='' +zsh:1> true\n+zsh:1> x='' \n",
		},
		{
			"a command after the prefix",
			`set -x; x=$( a=$(echo s >&2) true; echo u >&2 )`,
			"+zsh:1> x=+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' +zsh:1> true\n+zsh:1> echo u\nu\n+zsh:1> x='' \n",
		},
		{"a prefix stopped by a request", `set -x; x=$( a=1 b=${q?b} true )`, "+zsh:1> x=zsh:1: q: b\n+zsh:1> x='' \n"},
		{
			"a prefix stopped by a failure",
			`set -x; x=$( a=1 b=$((1/0)) true )`,
			"+zsh:1> x=zsh:1: division by zero\n+zsh:1> a=1 b=+zsh:1> x='' \n",
		},
		{
			"a for word list",
			`set -x; x=$( for i in $(echo a); do :; done )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> i=a\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"two substitutions in a for word list",
			`set -x; x=$( for i in $(echo a) $(echo b); do :; done )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> echo b\n+zsh:1> i=a\n+zsh:1> :\n+zsh:1> i=b\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a group",
			`set -x; x=$( { : $(echo a); } )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> : a\n+zsh:1> x='' \n",
		},
		{
			"a redirection",
			`set -x; x=$( : >$(echo /dev/null) )`,
			"+zsh:1> x=+zsh:1> echo /dev/null\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a pattern operand",
			`set -x; x=$( [[ a == $(echo a) ]] )`,
			"+zsh:1> x=+zsh:1> [[ a == +zsh:1> echo a\n+zsh:1> x=+zsh:1> [[ a == a+zsh:1> echo a\n" +
				"+zsh:1> x=+zsh:1> [[ a == a ]]\n+zsh:1> x='' \n",
		},
		{
			"a condition in an if",
			`set -x; x=$( if [[ $(echo a) == a ]]; then :; fi )`,
			"+zsh:1> x=+zsh:1> [[+zsh:1> echo a\n+zsh:1> [[ a == a ]]\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a pattern operand at the top",
			`set -x; [[ a == $(echo a) ]]`,
			"+zsh:1> [[ a == +zsh:1> echo a\n+zsh:1> [[ a == a+zsh:1> echo a\n+zsh:1> [[ a == a ]]\n",
		},
		{"a unary operand", `set -x; [[ -n $(echo a) ]]`, "+zsh:1> [[+zsh:1> echo a\n+zsh:1> [[ -n a ]]\n"},
		{
			"behind an and",
			`set -x; [[ -n a && -n $(echo b) ]]`,
			"+zsh:1> [[ -n a &&+zsh:1> echo b\n+zsh:1> [[ -n a && -n b ]]\n",
		},
		{"the pattern expanded twice", `n=0; set -x; [[ a == $((n++)) ]]; echo $n`, "+zsh:1> [[ a == 0 ]]\n+zsh:1> echo 2\n2\n"},
		{
			"the line holds the first and the match the second",
			`n=0; set -x; [[ 2 == $((++n)) ]]; echo st=$? n=$n`,
			"+zsh:1> [[ 2 == 1 ]]\n+zsh:1> echo 'st=0' 'n=2'\nst=0 n=2\n",
		},
		// Controls.
		{
			"a plain command's word",
			`set -x; x=$( : $(echo a); : )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> x=+zsh:1> : a\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{"a for with no substitution", `set -x; x=$( for i in a; do :; done )`, "+zsh:1> x=+zsh:1> i=a\n+zsh:1> :\n+zsh:1> x='' \n"},
		{"untraced", `n=0; [[ a == $((n++)) ]]; echo $n`, "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
