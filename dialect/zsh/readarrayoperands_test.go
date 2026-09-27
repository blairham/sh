// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `read -A` takes exactly one name here: a second operand is a line this
// shell declines, with nothing read and nothing assigned. Measured 2026-09-26
// on zsh 5.9.2 (aarch64-apple-darwin25.4.0), run `-f` with `env -u FPATH` over
// a script file (#4622).
//
// The other shell with the letter fills the first name and clears the rest,
// so this rides an axis rather than the letter — see
// Semantics.ReadArrayTakesOneNameOnly.
func TestReadArrayTakesOneNameOnly(t *testing.T) {
	if got := zsh.Semantics().ReadArrayTakesOneNameOnly; got != interp.Yes {
		t.Errorf("ReadArrayTakesOneNameOnly = %v, want Yes", got)
	}
	out, st := runZsh(t, t.TempDir(), `b=keep; c=keep
read -A a b c <<<'1 2 3 4'
print "A st=$? a=(${(j:,:)a}) b=[$b] c=[$c]"
b2=keep
read -A a2 b2 <<<'1 2 3'
print "B st=$? a2=(${(j:,:)a2}) b2=[$b2]"
read -A a3 <<<'1 2 3'
print "C st=$? a3=(${(j:,:)a3})"`)
	// Row C is the control: one name is right here and in every column, so
	// what is refused is the *second* operand and nothing else. The `keep`
	// values are the other half — the refusal comes before the read, so the
	// operands are not cleared either.
	want := "zsh:read:2: only one array argument allowed\nA st=1 a=() b=[keep] c=[keep]\n" +
		"zsh:read:5: only one array argument allowed\nB st=1 a2=() b2=[keep]\n" +
		"C st=0 a3=(1,2,3)\n"
	if out != want || st != 0 {
		t.Errorf("read -A with a second operand = %q (status %d), want %q", out, st, want)
	}
}

// `read -k` and `read -q` fill the name `-A` took rather than `REPLY`, and
// the default array's name where the letter was written with no operand.
// Measured in the same run (#4635).
//
// Both halves of every row matter: the named parameter really is written, and
// `REPLY` — which these two used to write instead — is left alone. A script
// that seeded either one can tell, which is why both are seeded here.
func TestReadKeysAndQueryFillTheArrayLettersName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `REPLY=S; a=A0;       read -k2 -u0 -A a <<<'xy'; print "1 st=$? a=[$a] REPLY=[$REPLY]"
REPLY=S; reply=(R0); read -k2 -u0 -A   <<<'xy'; print "2 st=$? reply=[$reply] REPLY=[$REPLY]"
REPLY=S; a=A0;       read -q  -u0 -A a <<<'y';  print "3 st=$? a=[$a] REPLY=[$REPLY]"
REPLY=S; reply=(R0); read -q  -u0 -A   <<<'y';  print "4 st=$? reply=[$reply] REPLY=[$REPLY]"
REPLY=S;             read -k2 -u0      <<<'xy'; print "5 st=$? REPLY=[$REPLY]"`)
	// Row 5 is the control and it is what keeps this from being "these two
	// never write REPLY": with no `-A` the text still goes there.
	want := "1 st=0 a=[xy] REPLY=[S]\n" +
		"2 st=0 reply=[xy] REPLY=[S]\n" +
		"3 st=0 a=[y] REPLY=[S]\n" +
		"4 st=0 reply=[y] REPLY=[S]\n" +
		"5 st=0 REPLY=[xy]\n"
	if out != want || st != 0 {
		t.Errorf("read -k/-q with -A = %q (status %d), want %q", out, st, want)
	}
}

// And what lands there is a **scalar**, which is measured rather than assumed:
// the name is a scalar afterwards even where it was an array first, and so is
// `reply`, which is an array parameter before the read. So `-A` moves the name
// these two letters fill and does not make the text a list.
func TestReadKeysThroughTheArrayLetterStoresAScalar(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `ctrl=(xy); print "C t=${(t)ctrl} n=${#ctrl[@]}"
a=(p q); read -k2 -u0 -A a <<<'xy'
print "1 t=${(t)a} n=${#a[@]} join=[${(j:|:)a}]"
reply=(R0); read -k2 -u0 -A <<<'xy'
print "2 t=${(t)reply} n=${#reply[@]} join=[${(j:|:)reply}]"`)
	// The first row is the control that makes the count readable: a real
	// one-element array counts 1, so the 2 below is a scalar's length and
	// not two elements.
	want := "C t=array n=1\n" +
		"1 t=scalar n=2 join=[xy]\n" +
		"2 t=scalar n=2 join=[xy]\n"
	if out != want || st != 0 {
		t.Errorf("the stored kind = %q (status %d), want %q", out, st, want)
	}
}
