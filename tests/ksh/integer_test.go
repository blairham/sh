// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `integer`, which this shell has and bash and dash do not. Measured
// 2026-09-06 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, ZDOTDIR and HISTFILE, over a script file.

// The declaration itself, and the reason the word exists: the attribute
// arrives without the letter, so a later assignment is an expression.
//
//	integer n=3            st=0 n=3
//	integer -r r=5         st=0 r=5
func TestIntegerDeclaresAnIntegerHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `integer n=3; echo "A st=$? n=$n"
n=5+2; echo "B n=$n"
integer -r r=5; echo "C st=$? r=$r"`)
	want := "A st=0 n=3\nB n=7\nC st=0 r=5\n"
	if out != want || st != 0 {
		t.Errorf("integer = %q (status %d), want %q", out, st, want)
	}
}

// The plus form removes nothing under this name, which is the axis. Measured:
// `integer n=5; integer +i n; n=3+4` is 7 here, where zsh answers `3+4`, and
// `integer -x e=1; integer +x e` leaves e in the environment where this
// shell's own `typeset +x` takes it out.
func TestAPlusFormOnIntegerRemovesNothingHere(t *testing.T) {
	if got := ksh.Semantics().IntegerPlusFormTakesAttributesOff; got != interp.No {
		t.Errorf("IntegerPlusFormTakesAttributesOff = %v, want No", got)
	}
	out, st := runKsh(t, t.TempDir(), `integer n=5; integer +i n; n=3+4; echo "A n=$n"`)
	if out != "A n=7\n" || st != 0 {
		t.Errorf("integer +i = %q (status %d), want A n=7 — the attribute the word "+
			"itself declared is out of the plus form's reach here", out, st)
	}
	// The export letter under the same rule, which is what makes it one
	// question about the *word* rather than one per letter. `export -p`
	// rather than `env`, because the runner has no PATH.
	out, st = runKsh(t, t.TempDir(), `integer -x e=1; integer +x e; export -p`)
	if !strings.Contains(out, "e=1") || st != 0 {
		t.Errorf("integer +x = %q (status %d), want e still exported", out, st)
	}
	// And the control: this shell's own `typeset +x` *does* unexport, so the
	// answer belongs to the second name and not to the letter.
	out, st = runKsh(t, t.TempDir(), `typeset -x q=1; typeset +x q; export -p`)
	if strings.Contains(out, "q=1") || st != 0 {
		t.Errorf("typeset +x = %q (status %d), want q unexported", out, st)
	}
}

// The letters are typeset's, and so are the ones that are missing: this shell
// hands `integer` the whole declaration grammar, so a letter it spells and
// this one does not is named as missing rather than as unknown — and named
// under `typeset`, which is what its own refusal says.
//
//	integer -Q w=1   →  typeset: -Q: unknown option  + typeset's usage
//
// With **one** exception, measured 2026-09-12 by asking ksh93u+ for every
// letter of typeset's grammar under the second name: `integer -f w=1` is
// refused, with the usage line alone, and nothing else is. A word that
// carries a type has no function to declare. So the two sets part by exactly
// that letter, and the *missing* sets stay identical — `-f` is not on either,
// because the function listing is built (#1494) and is simply not this word's.
func TestIntegerReadsTypesetsLettersAndSpeaksAsTypeset(t *testing.T) {
	s, d := ksh.Semantics(), ksh.Diagnostics()
	if want := strings.ReplaceAll(s.DeclareOptions, "f", ""); s.IntegerOptions != want {
		t.Errorf("IntegerOptions = %q and DeclareOptions = %q; the two names share "+
			"one letter set but for `f`", s.IntegerOptions, s.DeclareOptions)
	}
	if strings.Contains(s.IntegerOptions, "f") {
		t.Errorf("IntegerOptions = %q, want no `f`: this shell refuses `integer -f`",
			s.IntegerOptions)
	}
	if got := d.UnimplementedOptionLetters["integer"]; got != d.UnimplementedOptionLetters["typeset"] {
		t.Errorf("integer is missing %q and typeset %q; one set for both names",
			got, d.UnimplementedOptionLetters["typeset"])
	}
	if got := d.BuiltinComplaintName["integer"]; got != "typeset" {
		t.Errorf("BuiltinComplaintName[integer] = %q, want typeset", got)
	}
	out, st := runKsh(t, t.TempDir(), `integer -Q w=1`)
	if st != 2 {
		t.Errorf("status = %d, want 2 — a bad option to a declaration ends the script here", st)
	}
	for _, want := range []string{
		"typeset: -Q: unknown option",
		"Usage: typeset [-bflmnprstuxACHS]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want %q in it", out, want)
		}
	}
	// A letter this shell really has says so, under the same name.
	out, st = runKsh(t, t.TempDir(), `integer -S w=1`)
	if !strings.Contains(out, "typeset: -S is not implemented yet") || st != 2 {
		t.Errorf("integer -S = %q (status %d), want the letter named as missing", out, st)
	}
	// `-Z` used to be that example and is no longer missing: the width
	// letters are built (#2859), and under this word they are refused for
	// **conflicting with the type** rather than for being unknown — measured
	// 2026-09-18, `integer -L 5 a=ab` and `typeset -iZ 5 a=7` are both
	// typeset's whole usage block at 2 on ksh93u+, which is what `-H`
	// already does beside them.
	out, st = runKsh(t, t.TempDir(), `integer -Z 4 w=1`)
	if strings.Contains(out, "not implemented") ||
		!strings.Contains(out, "Usage: typeset [-bflmnprstuxACHS]") || st != 2 {
		t.Errorf("integer -Z 4 = %q (status %d), want typeset's usage block for the conflict", out, st)
	}
}

// An output base is read and the name renders in it, in all three spellings
// the base can be written in. Measured on 93u+: `16#ff` — lower case, where
// zsh writes `16#FF` — and the value the shell holds *is* those five
// characters, so a child is told them too.
func TestAnOutputBaseIsRead(t *testing.T) {
	if got := ksh.Semantics().IntegerAttributeTakesABase; got != interp.Yes {
		t.Errorf("IntegerAttributeTakesABase = %v, want Yes", got)
	}
	for _, src := range []string{
		`integer -i 16 b=255`,
		`typeset -i 16 b=255`,
		`typeset -i16 b=255`,
	} {
		out, st := runKsh(t, t.TempDir(), src+"\necho \"[$b]\"")
		if out != "[16#ff]\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", src, out, st, "[16#ff]\n")
		}
	}
}

// This shell counts an output base in lower case and carries on into upper,
// which is the whole of what its alphabet says: 61 is `Z`, 62 is `@` and 63
// is `_`, so base 64 is one it can spell and `typeset -i64 a=100` is `64#1A`.
// A base it cannot spell is taken in silence and rendered plain.
func TestTheOutputBaseAlphabetGoesPastThirtySix(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i36 b=100`, "[36#2s]\n"},
		{`typeset -i64 b=100`, "[64#1A]\n"},
		{`typeset -i64 b=63`, "[64#_]\n"},
		{`typeset -i1 b=5`, "[5]\n"},
		{`typeset -i10 b=255`, "[255]\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src+"\necho \"[$b]\"")
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// A negative value is the sixty-four-bit two's complement here, where zsh
// puts a sign in front of the magnitude.
func TestANegativeValueInAnOutputBaseIsTwosComplement(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -i16 h=-255\necho \"[$h]\"")
	want := "[16#ffffffffffffff01]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// The base is a property of the *name*: a later assignment renders in it, and
// a second declaration with a different base re-renders what the name already
// holds.
func TestTheOutputBaseBelongsToTheName(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `typeset -i8 c
c=64
echo "[$c]"
typeset -i i=5
typeset -i16 i
echo "[$i]"
typeset -i16 h=255
typeset -i8 h
echo "[$h]"`)
	want := "[8#100]\n[16#5]\n[8#377]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// This shell learns no base from the value it is given, which is where it
// parts company with zsh: `0x10` under `-i` is `16` here and `16#10` there.
func TestNoOutputBaseIsLearnedFromTheValue(t *testing.T) {
	if got := ksh.Semantics().IntegerBaseComesFromTheValueAssigned; got != interp.No {
		t.Errorf("IntegerBaseComesFromTheValueAssigned = %v, want No", got)
	}
	out, st := runKsh(t, t.TempDir(), "typeset -i b\nb=0x10\necho \"[$b]\"")
	if out != "[16]\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[16]\n")
	}
}

// How the base says itself back: a word of its own after the letter, and the
// value bare rather than quoted for the `#` in it.
func TestTheOutputBaseSaysItselfBack(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -i16 a=255\ntypeset -p a")
	want := "typeset -i 16 a=16#ff\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// No names is a listing of the integer variables here, and it works now that
// the word reaches `typeset -li` through the alias rather than a builtin of
// its own: the filtered listing was already built and the second name was what
// could not reach it. Measured 2026-09-18 on ksh93u+ 2012-08-01, a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`: `integer m=2` then a bare
// `integer` writes `m=2`, and `nn=1` with no attribute on it writes nothing at
// all (#3371).
func TestIntegerWithNoNamesListsTheIntegerNamesHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `integer nn=1
integer`)
	if out != "nn=1\n" || st != 0 {
		t.Errorf("bare integer = %q (status %d), want the integer names listed", out, st)
	}
	// And a name carrying no attribute is not in it, which is what says the
	// listing is filtered rather than the whole table.
	out, st = runKsh(t, t.TempDir(), `nn=1
integer`)
	if out != "" || st != 0 {
		t.Errorf("bare integer over a plain name = %q (status %d), want silence", out, st)
	}
}

// The word bash and dash do not have stays theirs, and here it is an **alias**
// rather than a builtin — `integer='typeset -li'`, which is what ksh93 ships.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null, each
// `unalias` and the word after it on lines of their own so the alias is gone
// before the next line is parsed:
//
//	'integer' a=1                  integer: not found, 127
//	cmd=integer; $cmd b=2          integer: not found, 127
//	unalias integer; integer a=1   integer: not found, 127
//	integer d=3                    st=0, d=3
//
// The literal spelling is the only one that works, which is exactly what an
// alias is: a quoted word, an expanded one and a word whose alias has been
// taken away are the three routes past one, and this shell has no command
// behind any of them (#3371).
func TestIntegerIsAnAliasHereAndNotABuiltin(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"the literal spelling declares", `integer d=3
print "d=[$d]"`, "d=[3]\n", 0},
		{"and it carries the lower-case letter the alias spells", `integer e=7+1
typeset -p e`, "typeset -l -i e=8\n", 0},
		{"a quoted spelling is not found", `'integer' a=1
print "a=[$a]"`, "ksh: integer: not found\na=[]\n", 0},
		{"an expanded one is not found", `cmd=integer
$cmd b=2
print "b=[$b]"`, "ksh: line 2: integer: not found\nb=[]\n", 0},
		// The third route past an alias — `unalias integer` and then the
		// word — is measured above and is not a row here: this helper parses
		// the whole snippet before it runs any of it, so the alias is still
		// in the table when the second line is read. It takes the
		// read-as-it-runs route a script file gets, which is `cmd/ksh`'s and
		// not a dialect test's.
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// Ten is what the letter names when nothing is written here, so a bare
// `typeset -i` — or `integer`, the same declaration under another word —
// takes the base off a name that had one, and a written ten records nothing
// at all: the listing has no base word. Measured on 93u+, where zsh keeps
// `16#FF` on every one of these.
func TestABareIntegerLetterTakesTheOutputBaseOff(t *testing.T) {
	if got := ksh.Semantics().IntegerBaseTenIsNoBase; got != interp.Yes {
		t.Errorf("IntegerBaseTenIsNoBase = %v, want Yes", got)
	}
	for _, tc := range []struct{ src, want string }{
		{"typeset -i16 a=255\ntypeset -i a\necho \"[$a]\"", "[255]\n"},
		{"typeset -i16 b=255\ninteger b\necho \"[$b]\"", "[255]\n"},
		{"typeset -i16 e=255\ntypeset -i10 e\necho \"[$e]\"", "[255]\n"},
		// The control: another letter is not this question, and the base
		// stands.
		{"typeset -i16 c=255\ntypeset -x c\necho \"[$c]\"", "[16#ff]\n"},
		{"typeset -i10 d=255\ntypeset -p d", "typeset -i d=255\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// Nothing is learned from the value here, so no listing carries a base it was
// not given: `c=0x10` under `-i` is `16` and lists as a bare `typeset -i`.
// The negative is the other half — the listing writes the text the name is
// holding, which is the bit pattern.
func TestNoLearnedBaseReachesTheListing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -i c; c=0x10; typeset -p c", "typeset -i c=16\n"},
		{"typeset -i b; b=10#5; typeset -p b", "typeset -i b=5\n"},
		{"typeset -i16 f=-255; typeset -p f", "typeset -i 16 f=16#ffffffffffffff01\n"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
