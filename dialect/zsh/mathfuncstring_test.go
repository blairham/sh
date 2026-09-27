// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `functions -M -s` registers a math function whose argument arrives as one
// *string*, unevaluated, where the plain `-M` evaluates the arguments and
// hands over numbers. Measured 2026-09-26 on zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f` with `env -u FPATH` over a script
// file (#4443).
//
// The discriminator is what the implementation sees rather than what the
// expression is worth: a call written `sf(2+2)` passes `2+2` here and `4`
// without the letter, and an implementation reading `${#1}` can tell those
// apart where one reading `$1` as a number cannot.
func TestMathFunctionStringArgument(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `sf() { print -r -- "n=$# 1=[$1] 0=[$0]"; }
functions -M -s sf
: $(( sf(2+2) ))
: $(( sf(1,2) ))
: $(( sf() ))
nf() { print -r -- "n=$# 1=[$1] 2=[$2]"; }
functions -M nf
: $(( nf(2+2) ))`)
	// The last row is the control: the same call text through a registration
	// without the letter is one evaluated argument, so what `-s` changes is
	// the reading and not the call.
	want := "n=1 1=[2+2] 0=[sf]\n" +
		"n=1 1=[1,2] 0=[sf]\n" +
		"n=1 1=[] 0=[sf]\n" +
		"n=1 1=[4] 2=[]\n"
	if out != want || st != 0 {
		t.Errorf("functions -M -s = %q (status %d), want %q", out, st, want)
	}
}

// The value a string registration is worth is the same rule the ordinary one
// follows — the last arithmetic evaluated during the call, which is not
// `REPLY` — and with nothing evaluated at all it is whatever the shell last
// evaluated, which in a shell that has done no arithmetic is nought.
func TestMathFunctionStringValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `sf() { REPLY=${#1}; }
functions -M -s sf
print -r -- "A=[$(( sf(2+2) ))]"
sv() { REPLY=$((11)); }
functions -M -s sv
print -r -- "B=[$(( sv(2+2) ))]"`)
	// Row A is the one that matters: `REPLY=${#1}` is three, and the call is
	// worth nought — so `REPLY` is not what comes back, exactly as it is not
	// for a registration without the letter.
	want := "A=[0]\nB=[11]\n"
	if out != want || st != 0 {
		t.Errorf("the value of a string math function = %q (status %d), want %q", out, st, want)
	}
}

// The arity is not free: the whole argument text is one string, so the
// registration takes exactly one argument and anything else is refused with
// nothing registered.
func TestMathFunctionStringArity(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `functions -Ms a1 1;    print "A=$?"
functions -Ms a2 1 1;  print "B=$?"
functions -Ms a3 0;    print "C=$?"
functions -Ms a4 1 -1; print "D=$?"
functions -Ms a5 2;    print "E=$?"
functions -M`)
	want := "A=0\nB=0\n" +
		"zsh:functions:3: -Ms: must take a single string argument\nC=1\n" +
		"zsh:functions:4: -Ms: must take a single string argument\nD=1\n" +
		"zsh:functions:5: -Ms: must take a single string argument\nE=1\n" +
		"functions -Ms a2 1\nfunctions -Ms a1 1\n"
	if out != want || st != 0 {
		t.Errorf("the arity of a string registration = %q (status %d), want %q", out, st, want)
	}
}

// The listing writes a string registration back with the letters that made
// it, so the line still makes the registration it describes — and the arity
// it prints is the one the letter implies even where none was written.
func TestMathFunctionStringListing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `functions -Ms sf
functions -M nf
functions -Ms n4 1 1 other
functions -M
functions +Ms sf
functions -M`)
	want := "functions -Ms n4 1 1 other\nfunctions -M nf\nfunctions -Ms sf 1\n" +
		"functions -Ms n4 1 1 other\nfunctions -M nf\n"
	if out != want || st != 0 {
		t.Errorf("the listing = %q (status %d), want %q", out, st, want)
	}
}

// The two letters compose in either order and in either spelling, and `-s`
// on a line with no `-M` is a letter that decides nothing — the listing it
// writes is the one a bare `functions` writes.
func TestMathFunctionStringLetterSpellings(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `functions -sM c1
functions -s -M c2
functions -M
f() { :; }
functions -s f
functions -s nosuch; print "A=$?"`)
	want := "functions -Ms c2 1\nfunctions -Ms c1 1\n" +
		"f () {\n\t:\n}\nA=1\n"
	if out != want || st != 0 {
		t.Errorf("the spellings = %q (status %d), want %q", out, st, want)
	}
}

// And the letter is accepted rather than named as missing.
func TestMathFunctionStringLetterIsNotAlsoCalledMissing(t *testing.T) {
	if !strings.ContainsRune(zsh.Semantics().FunctionsOptions, 's') {
		t.Error("FunctionsOptions does not spell -s")
	}
	if got := zsh.Diagnostics().UnimplementedOptionLetters["functions"]; strings.ContainsRune(got, 's') {
		t.Errorf("UnimplementedOptionLetters[functions] = %q, which still claims -s is missing", got)
	}
}
