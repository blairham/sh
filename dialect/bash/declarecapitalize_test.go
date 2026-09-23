// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `declare -c`, the third case attribute — which this shell has and does not
// advertise: its own usage line is `declare [-aAfFgiIlnrtux]`, with no `c` in
// it, and this dialect prints that line back byte for byte (#4160).
//
// The fold is **the first character upper and every other character lower**,
// which the letter's name does not say. Measured 2026-09-23 under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` from script files on GNU bash 5.3.15; `declare
// -p` is asserted beside the value because a listing is the only place the
// attribute itself shows.
//
// Rows three and four are the controls: the first *character* is uppercased
// rather than the first letter, so a value opening with a space or a digit keeps
// its opening and has the rest folded down anyway. `+c` removes it, and the fold
// reaches each element of an array.
func TestTheCapitalizeAttribute(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the whole value is folded, not only the front",
			`declare -c a="MIXED CASE"; printf '[%s]' "$a"; declare -p a`,
			"[Mixed case]declare -c a=\"Mixed case\"\n",
		},
		{
			"from any mixture",
			`declare -c b="mIxEd cAsE"; printf '[%s]' "$b"; declare -p b`,
			"[Mixed case]declare -c b=\"Mixed case\"\n",
		},
		{
			"a value opening with a space keeps its opening",
			`declare -c c="  leading spaces"; printf '[%s]' "$c"; declare -p c`,
			"[  leading spaces]declare -c c=\"  leading spaces\"\n",
		},
		{
			"and one opening with a digit",
			`declare -c d="9digit start"; printf '[%s]' "$d"; declare -p d`,
			"[9digit start]declare -c d=\"9digit start\"\n",
		},
		{
			"an empty value stays empty",
			`declare -c e=""; printf '[%s]' "$e"; declare -p e`,
			"[]declare -c e=\"\"\n",
		},
		{
			"a later assignment is folded too",
			`declare -c f; f="later ASSIGN"; printf '[%s]' "$f"; declare -p f`,
			"[Later assign]declare -c f=\"Later assign\"\n",
		},
		{
			// The whole value on every store, rather than once to what was
			// added: `x` plus ` MORE` is `X more` and not `X MORE`.
			"an append folds the whole value",
			`declare -c g="x"; g+=" MORE"; printf '[%s]' "$g"; declare -p g`,
			"[X more]declare -c g=\"X more\"\n",
		},
		{
			"+c removes it and leaves what is there",
			`declare -c k="one"; declare +c k; k="two THREE"; printf '[%s]' "$k"; declare -p k`,
			"[two THREE]declare -- k=\"two THREE\"\n",
		},
		{
			"each element of an array",
			`declare -c -a arr=(one TWO); printf '[%s]' "${arr[@]}"; declare -p arr`,
			"[One][Two]declare -ac arr=([0]=\"One\" [1]=\"Two\")\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// `-c` written with `-l` or `-u` on one declaration **cancels both**, which is
// what makes it a third member of the family
// interp.Semantics.TwoCaseLettersOnOneDeclarationCancel already answers rather
// than a fourth attribute standing beside them.
//
// Measured in the same run: no attribute is left and the value is untouched, in
// either order and with either partner.
func TestTheCapitalizeAttributeCancelsAgainstTheOtherTwo(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"c then u", `declare -cu h="mixed case"; printf '[%s]' "$h"; declare -p h`,
			"[mixed case]declare -- h=\"mixed case\"\n",
		},
		{
			"u then c", `declare -uc i="mixed case"; printf '[%s]' "$i"; declare -p i`,
			"[mixed case]declare -- i=\"mixed case\"\n",
		},
		{
			"c then l", `declare -cl j="MIXED CASE"; printf '[%s]' "$j"; declare -p j`,
			"[MIXED CASE]declare -- j=\"MIXED CASE\"\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// Where the letter sits in a listing: **last**, behind every other one
// including the two it cannot stand with. Measured in the same run, one letter
// at a time and then all of them.
func TestTheCapitalizeLetterIsListedLast(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`declare -c -x v=abc; declare -p v`, "declare -xc v=\"Abc\"\n"},
		{`declare -c -r v=abc; declare -p v`, "declare -rc v=\"Abc\"\n"},
		{`declare -c -t v=abc; declare -p v`, "declare -tc v=\"Abc\"\n"},
		{`declare -c -i v=5; declare -p v`, "declare -ic v=\"5\"\n"},
		{`declare -a -c -x v=(a); declare -p v`, "declare -axc v=([0]=\"A\")\n"},
		{`declare -A -c v; declare -p v`, "declare -Ac v\n"},
		{`declare -i -x -c -r -t v=9; declare -p v`, "declare -irtxc v=\"9\"\n"},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}
