// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `integer`, which this shell has and bash and dash do not. Measured
// 2026-09-06 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, over a script file.
//
// It is on the release bar because this shell's *own* `add-zsh-hook` opens
// with `integer del list help` on line 26 of its function file, so a startup
// file that installs a precmd hook cannot run without the word.

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

// An output base is refused by name rather than read and dropped. Measured:
// `integer -i 16 b=255` is `16#FF` here — uppercase, where ksh93 writes
// `16#ff` — and this engine has nowhere to keep a base, so before this it
// reported 0 and left `255` standing.
func TestAnOutputBaseIsNamedAsMissingHere(t *testing.T) {
	if got := zsh.Semantics().IntegerAttributeTakesABase; got != interp.Yes {
		t.Errorf("IntegerAttributeTakesABase = %v, want Yes", got)
	}
	for _, src := range []string{
		`integer -i 16 b=255`,
		`typeset -i 16 b=255`,
		`typeset -i16 b=255`,
	} {
		out, _ := runZsh(t, t.TempDir(), src)
		if !strings.Contains(out, "an output base is not implemented yet") {
			t.Errorf("%s = %q, want the base named as missing", src, out)
		}
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

// `integer` is a builtin here and its operands are a declaration's, so
// `integer n=5+2` is one word rather than a command with arguments.
func TestIntegerIsADeclaringBuiltinHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `whence -w integer
IFS=:; v='1:2'; integer n=$v; echo "n=[$n]"`)
	if !strings.Contains(out, "integer: builtin") {
		t.Errorf("whence -w integer = %q, want it named a builtin", out)
	}
	// The unquoted operand is not field-split, so `1:2` reaches the
	// assignment entire and the arithmetic complains about *it*. A word that
	// split would have assigned 1 and left 2 as a second operand.
	if !strings.Contains(out, "operator expected at `:2'") || st == 0 {
		t.Errorf("integer n=$v with IFS=: = %q (status %d), want the whole value in "+
			"one operand", out, st)
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
