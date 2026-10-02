// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestALocalOfEveryMatchHidesTheWholeTable is the first chunk of zsh's own
// E03posix (#5157): in a function, `typeset -h +g -m '*'` makes a local of
// every name a listing writes — the produced parameters and the tables among
// them — so that `unset -m '*'` leaves a bare `typeset` listing only what the
// function declared next. Every row measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`).
func TestALocalOfEveryMatchHidesTheWholeTable(t *testing.T) {
	const src = `fn() {
  typeset -h +g -m \*
  unset -m \*
  integer i=9
  float -H f=9
  declare -t scalar
  declare -H -a array
  typeset
  typeset +
}
fn
`
	want := "array local array\nfloat local f\ninteger local i=9\nlocal tagged scalar=''\n" +
		"array local array\nfloat local f\ninteger local i\nlocal tagged scalar\n"
	if out, _ := runZsh(t, t.TempDir(), src); out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
	for _, c := range []struct{ src, want string }{
		{`fn() { typeset -h +g -m "a*"; typeset -p argv; }; fn`, "typeset argv=''\n"},
		{`fn() { typeset -h +g -m pipestatus; typeset -p pipestatus; }; fn`, "typeset pipestatus=''\n"},
		{`fn() { typeset +g -m SECONDS; typeset -p SECONDS; }; fn`, "typeset -i10 SECONDS=0\n"},
		// Control: without `+g` a match is reached where it is.
		{`fn() { typeset -h -m pipestatus; typeset -p pipestatus; }; fn`, "typeset -g -a pipestatus=( 0 )\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src+"\n"); out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestReadonlyListsInTheStandardsFormUnderPosixBuiltins: `readonly -p` writes
// `readonly name=value`, and only for scalars, under POSIX_BUILTINS. Same
// shell, same day.
func TestReadonlyListsInTheStandardsFormUnderPosixBuiltins(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`setopt posixbuiltins typesettounset; readonly foo=bar novalue; readonly -p`, "readonly foo=bar\nreadonly novalue\n"},
		{`setopt posixbuiltins; readonly -i n=3; readonly -x e=3; readonly q='a b'; readonly -p`, "readonly e=3\nreadonly n=3\nreadonly q='a b'\n"},
		{`setopt posixbuiltins; readonly -a arr=(1 2); readonly -A as=(a 1); readonly -p`, ""},
		{`setopt posixbuiltins; f() { local -r loc=1; readonly -p }; f`, "readonly loc=1\n"},
		{`emulate sh; readonly foo=bar; readonly -p`, "readonly foo=bar\n"},
		// Control: the shell's own form without the option.
		{`readonly foo=bar; readonly -p`, "typeset -r foo=bar\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src+"\n"); out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestTypesetPrintTakesALayoutNumber is `typeset -p1`. Same shell, same day.
func TestTypesetPrintTakesALayoutNumber(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=("&" sand '""' "" plugh); typeset -p1 a`, "typeset -a a=(\n  '&'\n  sand\n  '\"\"'\n  ''\n  plugh\n)\n"},
		{`typeset -A h=(k v "a b" c); typeset -p1 h`, "typeset -A h=(\n  ['a b']=c\n  [k]=v\n)\n"},
		{`a=(1 2); typeset -p 1 a`, "typeset -a a=(\n  1\n  2\n)\n"},
		{`a=(1 2); typeset -ap1 a`, "typeset -a a=(\n  1\n  2\n)\n"},
		{`path=(/a /b); typeset -p1 path`, "typeset -aT PATH path=(\n  /a\n  /b\n)\n"},
		{`a=(); typeset -p1 a`, "typeset -a a=()\n"},
		{`typeset -A e; typeset -p1 e`, "typeset -A e=()\n"},
		{`s=x; typeset -p1 s`, "typeset s=x\n"},
		{`a=(1 2); typeset -p0 a`, "typeset -a a=( 1 2 )\n"},
		{`a=(1 2); typeset -p2 a; echo $?`, "zsh:typeset:1: bad argument to -p: 2\n1\n"},
		// The layout is the command's: the next listing is the plain one.
		{`a=(1 2); typeset -p1 a >/dev/null; typeset -p a`, "typeset -a a=( 1 2 )\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src+"\n"); out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
