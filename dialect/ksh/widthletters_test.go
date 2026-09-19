// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// The three width letters, which this shell spells `-L[n]`, `-R[n]` and
// `-Z[n]` in its own usage line. Measured on ksh93u+ 2012-08-01, 2026-09-18,
// a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME and standard input on /dev/null (#2859).
//
// Every *value* in the first block is the same in the other column that has
// the letters, which is what makes it a rule rather than a dialect answer.
// The blocks after it are the three places the two columns part, and each has
// a field on the semantics vector standing behind it.
func TestTheWidthLettersPresentTheValue(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The rule: each letter names how many characters the value is
		// presented in, `-L` keeping the left and `-R` the right.
		{`typeset -L 5 b=ab; echo "[$b]"`, "[ab   ]\n"},
		{`typeset -R 5 c=ab; echo "[$c]"`, "[   ab]\n"},
		{`typeset -Z 4 d=7; echo "[$d]"`, "[0007]\n"},
		{`typeset -Z 4 w=abc; echo "[$w]"`, "[ abc]\n"},
		{`typeset -Z 5 i=-7; echo "[$i]"`, "[   -7]\n"},
		{`typeset -L 5 f=abcdefgh; echo "[$f]"`, "[abcde]\n"},
		{`typeset -R 3 g=abcdefgh; echo "[$g]"`, "[fgh]\n"},
		{`typeset -L 3 k="  x"; echo "[$k]"`, "[x  ]\n"},
		// The width a name was not given is learned from its first value and
		// becomes part of the declaration.
		{`typeset -L m=xy; typeset -p m`, "typeset -L 2 m=xy\n"},
		{`typeset -Z n=7; typeset -p n`, "typeset -Z 1 -R 1 n=7\n"},
		// The store is the *presentation* here, which is the split
		// CaseAttributeFoldsWhenRead already records for the case letters:
		// the listing writes the padded text back.
		{`typeset -L 5 b=ab; typeset -p b`, "typeset -L 5 b='ab   '\n"},
		{`typeset -R 5 c=ab; typeset -p c`, "typeset -R 5 c='   ab'\n"},
		// And the number is a word of its own, behind every other letter.
		{`typeset -u -L 5 v=ab; typeset -p v`, "typeset -u -L 5 v='AB   '\n"},
		{`typeset -xL 3 a=ab; typeset -p a`, "typeset -x -L 3 a='ab '\n"},
		{`typeset -rL 3 b=ab; typeset -p b`, "typeset -r -L 3 b='ab '\n"},
		{`typeset -tL 3 e=ab; typeset -p e`, "typeset -t -L 3 e='ab '\n"},
		// Written even where it is zero, which the float letter's number is
		// not: `typeset -L k` with no width and no value lists as `-L 0`.
		{`typeset -L k; typeset -p k`, "typeset -L 0 k\n"},
		{`typeset -R p; typeset -p p`, "typeset -R 0 p\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// `Z` is a **fill riding on a justification** here — `R` where the
// declaration names none — where the other column that has the letter reads
// it as a third justification of its own.
//
// The listing is the cheap half of the claim and the values are the rest:
// under the other reading `-LZ` is `L` alone, so `typeset -LZ 5 i=0012` is
// `0012 ` there and `12   ` here, the fill having no left-hand pad to lay
// down and taking the leading zeros off instead.
func TestTheZeroFillLetterRidesOnAJustification(t *testing.T) {
	if got := ksh.Semantics().DeclareZeroFillLetter; got != interp.DeclareZeroFillLetterRidesOnTheJustification {
		t.Errorf("DeclareZeroFillLetter = %v, want the fill riding on the justification", got)
	}
	for _, c := range []struct{ src, want string }{
		// The justification it takes where none is written, said back.
		{`typeset -Z 4 d=7; typeset -p d`, "typeset -Z 4 -R 4 d=0007\n"},
		{`typeset -Z 5 j=1.5; typeset -p j`, "typeset -Z 5 -R 5 j=001.5\n"},
		// And one written beside it, both letters kept.
		{`typeset -ZR 5 p=7; echo "[$p]"; typeset -p p`, "[00007]\ntypeset -Z 5 -R 5 p=00007\n"},
		{`typeset -ZL 5 q=7; echo "[$q]"; typeset -p q`, "[7    ]\ntypeset -Z 5 -L 5 q='7    '\n"},
		{`typeset -LZ 5 r=7; echo "[$r]"; typeset -p r`, "[7    ]\ntypeset -Z 5 -L 5 r='7    '\n"},
		{`typeset -RZ 5 s=7; echo "[$s]"; typeset -p s`, "[00007]\ntypeset -Z 5 -R 5 s=00007\n"},
		// The leading zeros the left-hand ride takes off, a value at a time.
		{`typeset -LZ 6 a=0012; echo "[$a]"`, "[12    ]\n"},
		{`typeset -LZ 6 a=00ab; echo "[$a]"`, "[ab    ]\n"},
		{`typeset -LZ 6 a=0.5; echo "[$a]"`, "[.5    ]\n"},
		{`typeset -LZ 6 a=000; echo "[$a]"`, "[      ]\n"},
		{`typeset -LZ 6 a=-07; echo "[$a]"`, "[-07   ]\n"},
		// And the ones the right-hand ride does not: the fill is laid down
		// over the value as it stands.
		{`typeset -RZ 5 a=00ab; echo "[$a]"`, "[000ab]\n"},
		{`typeset -RZ 6 a=0.5; echo "[$a]"`, "[0000.5]\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// Where a declaration writes both justifications the **last** wins, across
// words as well as inside one — the opposite of the rank this shell gives the
// numeric letters, and the opposite of the other column's answer here.
func TestTheLastJustificationWritten(t *testing.T) {
	if got := ksh.Semantics().WidthJustificationPrecedence; got != interp.WidthJustificationLastWrittenWins {
		t.Errorf("WidthJustificationPrecedence = %v, want the last letter written", got)
	}
	for _, c := range []struct{ src, want string }{
		{`typeset -LR 5 t=7; echo "[$t]"; typeset -p t`, "[    7]\ntypeset -R 5 t='    7'\n"},
		{`typeset -RL 5 u=7; echo "[$u]"; typeset -p u`, "[7    ]\ntypeset -L 5 u='7    '\n"},
		{`typeset -ZRL 5 c=7; typeset -p c`, "typeset -Z 5 -L 5 c='7    '\n"},
		{`typeset -LRZ 5 d=7; typeset -p d`, "typeset -Z 5 -R 5 d=00007\n"},
		// Across words, where both letters are really read and the later one
		// still wins. The one reading the order alone cannot reach.
		{`typeset -L -R 5 a=7; typeset -p a`, "typeset -R 5 a='    7'\n"},
		{`typeset -R -L 5 b=7; typeset -p b`, "typeset -L 5 b='7    '\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// A width letter cannot stand beside the integer one, and the refusal is
// typeset's whole usage block rather than a complaint about either letter.
// `typeset` is one of this shell's special builtins, so the script ends.
//
// `integer` is the same line under an alias — `integer='typeset -li'` — which
// is why the second half of the table needs no separate rule.
func TestAWidthLetterWillNotShareWithTheIntegerLetter(t *testing.T) {
	if got := ksh.Semantics().WidthLettersExcludeTheIntegerLetter; got != interp.Yes {
		t.Errorf("WidthLettersExcludeTheIntegerLetter = %v, want Yes", got)
	}
	for _, src := range []string{
		`typeset -iL 5 a=7`,
		`typeset -Li 5 a=7`,
		`typeset -iZ 5 a=7`,
		`typeset -iR 5 a=7`,
	} {
		out, st := kshOut(t, src+"\necho ran")
		if st != 2 || strings.Contains(out, "ran") ||
			!strings.Contains(out, "Usage: typeset [-bflmnprstuxACHS]") {
			t.Errorf("%s = %q at %d, want typeset's usage block at 2 and the script ended", src, out, st)
		}
	}
	// The second word is the same line under an alias — `integer='typeset
	// -li'` — so it meets the refusal through the aliases and not as a
	// builtin of its own.
	out, st := runKsh(t, t.TempDir(), "integer -L 5 a=ab\necho ran")
	if st != 2 || strings.Contains(out, "ran") ||
		!strings.Contains(out, "Usage: typeset [-bflmnprstuxACHS]") {
		t.Errorf("integer -L = %q at %d, want typeset's usage block at 2 and the script ended", out, st)
	}
	// The control, and it is what keeps this about the pair rather than
	// about the width letter: either letter alone is taken.
	for _, c := range []struct{ src, want string }{
		{`typeset -L 5 a=7; echo "[$a]"`, "[7    ]\n"},
		{`typeset -i a=7; echo "[$a]"`, "[7]\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// A second declaration reads the **text** the name is holding and not the
// presentation the first one laid over it. The store is the presentation in
// this column, so nothing else would take the pad back off.
//
// The last row is the control: a declaration naming what the name already has
// changes nothing, so the zeros this attribute itself wrote stay where they
// are.
func TestAWidthIsReplacedOverTheTextAndNotThePad(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -L 4 u=ab; typeset -R 4 u; typeset -p u`, "typeset -R 4 u='  ab'\n"},
		{`typeset -R 4 v=ab; typeset -L 4 v; typeset -p v`, "typeset -L 4 v='ab  '\n"},
		{`typeset -L 4 e=ab; typeset -L 6 e; typeset -p e`, "typeset -L 6 e='ab    '\n"},
		{`typeset -L 4 d=ab; typeset -Z 4 d; typeset -p d`, "typeset -Z 4 -R 4 d='  ab'\n"},
		{`typeset -L 4 e=12; typeset -Z 4 e; typeset -p e`, "typeset -Z 4 -R 4 e=0012\n"},
		{`typeset -R 4 f=12; typeset -Z 4 f; typeset -p f`, "typeset -Z 4 -R 4 f=0012\n"},
		{`typeset -Z 4 a=0012; typeset -L 4 a; typeset -p a`, "typeset -L 4 a='12  '\n"},
		{`typeset -Z 4 h=7; typeset -Z 6 h; typeset -p h`, "typeset -Z 6 -R 6 h=000007\n"},
		{`typeset -LZ 5 c=0012; typeset -R 5 c; typeset -p c`, "typeset -R 5 c='   12'\n"},
		{`typeset -L 4 t=0012; typeset -L 4 t; typeset -p t`, "typeset -L 4 t=0012\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// Taking the attribute **off** is the narrower half, and the asymmetry is
// measured: the zero fill comes back off and a blank pad does not. There is
// no raw text to fall back on in this column, so what is left is what the
// last presentation wrote — and zeros read as part of a value where blanks
// read as spacing.
func TestTakingAWidthOffKeepsTheBlanksAndDropsTheZeros(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -Z 4 d=7; typeset +Z d; echo "[$d]"; typeset -p d`, "[7]\nd=7\n"},
		{`typeset -R 5 b=ab; typeset +R b; echo "[$b]"; typeset -p b`, "[   ab]\nb='   ab'\n"},
		{`typeset -L 5 a=ab; typeset +L a; echo "[$a]"; typeset -p a`, "[ab   ]\na='ab   '\n"},
		{`typeset -L 3 b=abcd; typeset +L b; echo "[$b]"; typeset -p b`, "[abc]\nb=abc\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// An element written to an attributed name goes through the attribute here,
// which is CompoundElementsGoThroughTheAttribute's question and needed no new
// answer for the width letters: the same split the case letters make, the
// same way round. The other column pads a scalar and leaves an array's
// elements as they are.
func TestAnArraysElementsGoThroughTheWidth(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			`typeset -a n; typeset -L 3 n; n=(abcd efgh); typeset -p n`,
			"typeset -a -L 3 n=(abc efg)\n",
		},
		{
			`typeset -a n; typeset -R 3 n; n=(abcd efgh); n[0]=xyzw; typeset -p n`,
			"typeset -a -R 3 n=(yzw fgh)\n",
		},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The letters left this shell's missing list in the same change that gave
// them an attribute, under both words that read typeset's grammar.
func TestTheWidthLettersAreNoLongerMissing(t *testing.T) {
	d := ksh.Diagnostics()
	for _, name := range []string{"typeset", "integer"} {
		if got := d.UnimplementedOptionLetters[name]; strings.ContainsAny(got, "LRZ") {
			t.Errorf("UnimplementedOptionLetters[%s] = %q, which still claims a width letter is missing",
				name, got)
		}
	}
	s := ksh.Semantics()
	for _, c := range "LRZ" {
		if !strings.ContainsRune(s.DeclareOptions, c) {
			t.Errorf("DeclareOptions = %q, want the %c letter", s.DeclareOptions, c)
		}
		if !strings.ContainsRune(s.DeclareOptionsTakingANumber, c) {
			t.Errorf("DeclareOptionsTakingANumber = %q, want the %c letter",
				s.DeclareOptionsTakingANumber, c)
		}
	}
}

// `export` is not `typeset` here, which is the half worth measuring rather
// than assuming: the other column that has the letters takes them under both
// words. Measured 2026-09-18 — `export -L 5 a=ab` is `export: -L: unknown
// option` with export's own usage line at 2, and the script ends there.
func TestTheWidthLettersAreNotExportsLetters(t *testing.T) {
	for _, src := range []string{`export -L 5 a=ab`, `readonly -L 5 a=ab`} {
		word := strings.Fields(src)[0]
		out, st := kshOut(t, src+"\necho ran")
		if st != 2 || strings.Contains(out, "ran") ||
			!strings.Contains(out, word+": -L: unknown option") {
			t.Errorf("%s = %q at %d, want the letter refused as unknown at 2", src, out, st)
		}
	}
}
