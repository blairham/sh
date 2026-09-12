// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `integer`, which this shell has and bash and dash do not. Measured
// 2026-09-06 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, over a script file.
//
// It is on the release bar because `add-zsh-hook` declares integers before
// it does anything else — run it under a shell with no `integer` and it
// fails there — so a startup file that installs a precmd hook cannot run
// without the word.

// The declaration itself: the attribute arrives without the letter, so a
// later assignment is an expression.
//
//	integer n=3       st=0 n=3
//	integer -r r=5    st=0 r=5
func TestIntegerDeclaresAnIntegerHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `integer n=3; echo "A st=$? n=$n"
n=5+2; echo "B n=$n"
integer -r r=5; echo "C st=$? r=$r"
integer k; echo "D k=[$k]"`)
	// A valueless declaration is 0 here rather than empty, which is
	// DeclaredNameWithoutValueIsEmpty and this shell's own answer to it —
	// kept in the same test because it is what `integer del list help`
	// depends on.
	want := "A st=0 n=3\nB n=7\nC st=0 r=5\nD k=[0]\n"
	if out != want || st != 0 {
		t.Errorf("integer = %q (status %d), want %q", out, st, want)
	}
}

// The plus form means here what it means on `typeset`, which is the axis.
// Measured: `integer n=5; integer +i n; n=3+4` is `3+4` here where ksh93
// answers 7, and `typeset -p n` says a plain `typeset n=5` against ksh93's
// `typeset -l -i n=5`.
func TestAPlusFormOnIntegerRemovesTheAttributeHere(t *testing.T) {
	if got := zsh.Semantics().IntegerPlusFormTakesAttributesOff; got != interp.Yes {
		t.Errorf("IntegerPlusFormTakesAttributesOff = %v, want Yes", got)
	}
	out, st := runZsh(t, t.TempDir(), `integer n=5; integer +i n; n=3+4; echo "A n=$n"
integer m=5; typeset -p m`)
	want := "A n=3+4\ntypeset -i m=5\n"
	if out != want || st != 0 {
		t.Errorf("integer +i = %q (status %d), want %q", out, st, want)
	}
}

// The letter set is *narrower* than this shell's `typeset`, which is why it
// is its own field. Measured: `integer -a`, `-A`, `-f`, `-F`, `-T` and `-U`
// are bad options here where typeset takes every one of them — so a shell
// that reused DeclareOptions would accept `integer -A m`, an associative
// array in no shell that has the word.
func TestIntegerTakesFewerLettersThanTypesetHere(t *testing.T) {
	s := zsh.Semantics()
	if s.IntegerOptions == s.DeclareOptions {
		t.Fatalf("IntegerOptions and DeclareOptions are both %q; this shell narrows "+
			"the set under the second name", s.IntegerOptions)
	}
	for _, letter := range "aAfFTU" {
		if strings.ContainsRune(s.IntegerOptions, letter) {
			t.Errorf("IntegerOptions %q claims -%c, which is a bad option to this "+
				"shell's `integer`", s.IntegerOptions, letter)
		}
		if !strings.ContainsRune(s.DeclareOptions, letter) {
			t.Errorf("DeclareOptions %q does not claim -%c, so the narrowing this "+
				"test is about is not visible", s.DeclareOptions, letter)
		}
	}
	out, st := runZsh(t, t.TempDir(), `integer -A m; echo "st=$?"`)
	if !strings.Contains(out, "integer:1: bad option: -A") || st != 0 {
		t.Errorf("integer -A = %q (status %d), want it refused as a bad option under "+
			"its own name", out, st)
	}
	// A letter this shell's `integer` really has is named as missing instead,
	// and `-Z` is one it has and `typeset` here is also missing.
	out, _ = runZsh(t, t.TempDir(), `integer -Z m`)
	if !strings.Contains(out, "integer:1: -Z is not implemented yet") {
		t.Errorf("integer -Z = %q, want the letter named as missing", out)
	}
}

// An output base is read and the name renders in it, in all three spellings.
// `16#FF` here — upper case, where ksh93 writes `16#ff` — and the value the
// shell holds *is* those five characters.
func TestAnOutputBaseIsRead(t *testing.T) {
	if got := zsh.Semantics().IntegerAttributeTakesABase; got != interp.Yes {
		t.Errorf("IntegerAttributeTakesABase = %v, want Yes", got)
	}
	for _, src := range []string{
		`integer -i 16 b=255`,
		`typeset -i 16 b=255`,
		`typeset -i16 b=255`,
	} {
		out, _ := runZsh(t, t.TempDir(), src+"\necho \"[$b]\"")
		if out != "[16#FF]\n" {
			t.Errorf("%s = %q, want %q", src, out, "[16#FF]\n")
		}
	}
}

// This shell counts in upper case and stops at 36, so a base outside two to
// thirty-six is refused by name and the declaration is not made.
func TestAnOutOfRangeOutputBaseIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i64 a=100`, "invalid base (must be 2 to 36 inclusive): 64"},
		{`typeset -i1 a=5`, "invalid base (must be 2 to 36 inclusive): 1"},
		{`typeset -i0 a=5`, "invalid base (must be 2 to 36 inclusive): 0"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src+"\necho \"[$a]\"")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want it to name %q", tc.src, out, tc.want)
		}
		if !strings.Contains(out, "[]") {
			t.Errorf("%s = %q, want the name left with nothing", tc.src, out)
		}
	}
}

// The half of the feature that is not the letter: this shell learns a name's
// output base from the radix prefix of the value assigned to it, and it
// sticks — a later plain `5` under a name that learned 16 is `16#5`.
func TestAnOutputBaseIsLearnedFromTheValue(t *testing.T) {
	if got := zsh.Semantics().IntegerBaseComesFromTheValueAssigned; got != interp.Yes {
		t.Errorf("IntegerBaseComesFromTheValueAssigned = %v, want Yes", got)
	}
	for _, tc := range []struct{ src, want string }{
		{"typeset -i b\nb=0x10", "[16#10]\n"},
		{"typeset -i b\nb=0x10\nb=5", "[16#5]\n"},
		{"typeset -i b\nb=8#7\nb=99", "[8#143]\n"},
		{"b=0x10\ntypeset -i b\nb=7", "[16#7]\n"},
		// A leading zero is not a radix, and neither is a value that arrived
		// already evaluated — the expansion handed the assignment decimal.
		{"typeset -i b\nb=016", "[16]\n"},
		{"typeset -i b\nb=$((0x10))", "[16]\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src+"\necho \"[$b]\"")
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// A negative value puts the sign in front of the magnitude here, where ksh93
// prints the sixty-four-bit two's complement.
func TestANegativeValueInAnOutputBaseKeepsItsSign(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "typeset -i16 h=-255\necho \"[$h]\"")
	if out != "[-16#FF]\n" {
		t.Errorf("got %q, want %q", out, "[-16#FF]\n")
	}
}

// How the base says itself back: attached to the letter, and the value
// decoded to decimal — the one place this shell writes the number rather than
// the text it is holding.
func TestTheOutputBaseSaysItselfBack(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "typeset -i16 a=255\ntypeset -p a\nb=0x10\ntypeset -i b\ntypeset -p b")
	want := "typeset -i16 a=255\ntypeset -i16 b=16\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// No names is a listing of every integer parameter here, this shell's own
// specials included, which this engine does not build — so it refuses by
// name rather than falling through to the bare declaration listing and
// answering with the whole parameter table.
func TestIntegerWithNoNamesIsNamedAsMissingHere(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `integer nn=1
integer`)
	if !strings.Contains(out, "integer:2: a listing is not implemented yet") {
		t.Errorf("bare integer = %q, want the listing named as missing", out)
	}
}

// `integer` is a builtin here and its operands are a declaration's, so the
// value after `=` is not globbed.
//
// A field-splitting probe would not have shown this: this shell does not
// split an unquoted parameter at all, so `IFS=:; v=1:2; integer n=$v` keeps
// the whole value either way and a test written that way passes with the
// declaring word taken back out. The glob is what separates them — with the
// word declaring, `n=*` is the character, and without it the `*` matches the
// file next to it and the arithmetic reads a number.
func TestIntegerIsADeclaringBuiltinHere(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "7"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `whence -w integer`)
	if !strings.Contains(out, "integer: builtin") || st != 0 {
		t.Errorf("whence -w integer = %q (status %d), want it named a builtin", out, st)
	}
	out, st = runZsh(t, dir, `integer n=*; echo "n=[$n]"`)
	if !strings.Contains(out, "bad math expression: operand expected at `*'") || st != 1 {
		t.Errorf("integer n=* = %q (status %d), want the `*` to reach the arithmetic "+
			"entire", out, st)
	}
	// The other half, and the one that fails when the word stops declaring:
	// the whole of `n=*` is globbed there and the file named 7 is what it
	// matches, so the complaint is about a pattern and not about arithmetic.
	if strings.Contains(out, "no matches found") || strings.Contains(out, "n=[7]") {
		t.Errorf("integer n=* = %q; the operand was globbed, so the word is not "+
			"declaring", out)
	}
}

// The reason the word is on the release bar, end to end: the three lines
// this shell's own `add-zsh-hook` opens with.
func TestTheLinesAddZshHookOpensWith(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() {
  integer del list help
  echo "del=[$del] list=[$list] help=[$help]"
}
f`)
	want := "del=[0] list=[0] help=[0]\n"
	if out != want || st != 0 {
		t.Errorf("`integer del list help` = %q (status %d), want %q", out, st, want)
	}
}

// Ten is a base of its own here and not the letter's default, which is the
// row that parts company with ksh93 twice over: it is recorded and the
// listing says `-i10` back, and a later bare `typeset -i` leaves the base a
// name already has where it was.
func TestBaseTenIsABaseOfItsOwn(t *testing.T) {
	if got := zsh.Semantics().IntegerBaseTenIsNoBase; got != interp.No {
		t.Errorf("IntegerBaseTenIsNoBase = %v, want No", got)
	}
	for _, tc := range []struct{ src, want string }{
		{"typeset -i16 a=255\ntypeset -i a\necho \"[$a]\"", "[16#FF]\n"},
		{"typeset -i16 b=255\ninteger b\necho \"[$b]\"", "[16#FF]\n"},
		{"typeset -i16 c=255\ntypeset -x c\necho \"[$c]\"", "[16#FF]\n"},
		{"typeset -i10 d=255\ntypeset -p d", "typeset -i10 d=255\n"},
		// A base of ten still writes nothing into the value: it is the base
		// nothing is written in, which is the half both shells share.
		{"typeset -i10 e=255\necho \"[$e]\"", "[255]\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// A base learned from the value is recorded like one named on the letter, and
// the listing says it back — ten included, which is the row that says the
// learning path keeps a base this shell *takes* rather than only one it
// writes a value in. Measured: `b=10#5` lists as `typeset -i10 b=5` while
// `$b` is a plain `5`.
func TestALearnedOutputBaseSaysItselfBack(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -i c; c=0x10; typeset -p c", "typeset -i16 c=16\n"},
		{"typeset -i b; b=10#5; typeset -p b; echo \"[$b]\"", "typeset -i10 b=5\n[5]\n"},
		{"typeset -i d; d=8#7;  typeset -p d", "typeset -i8 d=7\n"},
		// A sign in front of the radix does not hide it.
		{"typeset -i e; e=-0x10; echo \"[$e]\"", "[-16#10]\n"},
		// And a negative under a base keeps its sign into the listing, where
		// this shell writes the number rather than the text it holds.
		{"typeset -i16 f=-255; typeset -p f", "typeset -i16 f=-255\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
