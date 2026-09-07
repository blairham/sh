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
func TestIntegerReadsTypesetsLettersAndSpeaksAsTypeset(t *testing.T) {
	s, d := ksh.Semantics(), ksh.Diagnostics()
	if s.IntegerOptions != s.DeclareOptions {
		t.Errorf("IntegerOptions = %q and DeclareOptions = %q; this shell gives the "+
			"two names one letter set", s.IntegerOptions, s.DeclareOptions)
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
	out, st = runKsh(t, t.TempDir(), `integer -Z w=1`)
	if !strings.Contains(out, "typeset: -Z is not implemented yet") || st != 2 {
		t.Errorf("integer -Z = %q (status %d), want the letter named as missing", out, st)
	}
}

// An output base is refused by name rather than read and dropped. Measured:
// `integer -i 16 b=255` is `16#ff` here and `typeset -i2 c=5` is `2#101`, and
// this engine has nowhere to keep a base — so before this it reported 0, left
// `255` standing, and turned the `16` into a variable of its own.
func TestAnOutputBaseIsNamedAsMissingHere(t *testing.T) {
	if got := ksh.Semantics().IntegerAttributeTakesABase; got != interp.Yes {
		t.Errorf("IntegerAttributeTakesABase = %v, want Yes", got)
	}
	for _, src := range []string{
		`integer -i 16 b=255`,
		`typeset -i 16 b=255`,
		`typeset -i16 b=255`,
	} {
		out, st := runKsh(t, t.TempDir(), src)
		if !strings.Contains(out, "an output base is not implemented yet") || st != 2 {
			t.Errorf("%s = %q (status %d), want the base named as missing", src, out, st)
		}
	}
}

// No names is a listing of the integer variables here — `b=16#ff`, `g=7` —
// which this engine does not build, so it refuses by name. Falling through to
// the bare declaration listing would have answered with the whole variable
// table, which is a wrong answer rather than a missing one.
func TestIntegerWithNoNamesIsNamedAsMissingHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `integer nn=1
integer`)
	if !strings.Contains(out, "typeset: a listing is not implemented yet") || st != 2 {
		t.Errorf("bare integer = %q (status %d), want the listing named as missing", out, st)
	}
}

// The word bash and dash do not have stays theirs: nothing here leaks the
// registration into the other two, which is checked from their own packages —
// this is the positive half, that the word resolves to a builtin at all.
func TestIntegerIsABuiltinHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -v integer`)
	if !strings.Contains(out, "integer is a shell builtin") || st != 0 {
		t.Errorf("whence -v integer = %q (status %d), want it named a builtin", out, st)
	}
}
