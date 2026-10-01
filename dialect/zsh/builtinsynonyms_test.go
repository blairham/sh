// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The builtins zsh -f lists are builtins here, and `zsocket` is not one until
// its module is loaded** (#5265). Measured 2026-10-01 against zsh 5.9.2 with
// this script under `-c`, which writes these lines byte for byte: the table
// `whence` and `$builtins` read; `chdir` complaining as itself, at the line
// and inside a function; `history` and `r` complaining as `fc`; the buffer
// stack through `pushln` and `getln`; the one bit `ttyctl` keeps; `compcall`
// outside completion; `bye`; and `logout`, which refuses a shell that is not a
// login one and ends it anyway.
func TestTheBuiltinsZshListsAreHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `whence -w bye chdir history r getln pushln ttyctl echotc compcall logout noglob - zsocket
print -l ${(ko)builtins[(I)(bye|chdir|history|r|getln|pushln|ttyctl|echotc|compcall|logout|noglob|-|zsocket)]}
chdir /nonexistent-dir
f() {
  chdir /nonexistent-dir
}
f
history
r
pushln "a b" c; getln x; print -r -- "[$x]"; getln y; print -r -- "st=$? [$y]"
ttyctl; ttyctl -f; ttyctl; ttyctl -u; ttyctl
compcall; print -r -- "st=$?"
zmodload zsh/net/socket; whence -w zsocket
( bye 3 ); print -r -- "bye=$?"
logout; print -r -- unreached`)
	if want := "bye: builtin\nchdir: builtin\nhistory: builtin\nr: builtin\ngetln: builtin\npushln: builtin\nttyctl: builtin\nechotc: builtin\ncompcall: builtin\nlogout: builtin\nnoglob: builtin\n-: builtin\nzsocket: none\nbye\nchdir\ncompcall\nechotc\ngetln\nhistory\nlogout\nnoglob\npushln\nr\nttyctl\nzsh:chdir:3: no such file or directory: /nonexistent-dir\nf:chdir:1: no such file or directory: /nonexistent-dir\nzsh:fc:8: no such event: 1\nzsh:fc:9: current history line would recurse endlessly, aborted\n[a b c]\nst=1 []\ntty is not frozen\ntty is frozen\ntty is not frozen\nzsh:compcall:12: can only be called from completion function\nst=1\nzsocket: builtin\nbye=3\nzsh:logout:15: not login shell\n"; out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
}

// **`echotc` is `echoti` by termcap code**, and asks for exactly the
// parameters a string reads. Measured under `TERM=xterm` on zsh 5.9.2: `cm 3
// 4` is `\e[4;5H`, a bare or short `cm` is `not enough arguments`, `md 3` is
// `too many arguments`, `co` is 80, `am` is yes, `bw` no, an unknown code is
// `no such capability`, and with no terminal at all `echotc md` is silent at 1.
func TestEchotcReadsTheTermcapNames(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), xtermLikeTerminal(t)+`echotc md; print " $?"
echotc cm 3 4; print " $?"
echotc cm; print " $?"
echotc cm 1; print " $?"
echotc md 3; print " $?"
echotc xx; print " $?"
unset TERM; echotc md; print " $?"`)
	want := "\x1b[1m 0\n\x1b[4;5H 0\nzsh:echotc:3: not enough arguments\n 1\nzsh:echotc:4: not enough arguments\n 1\n" +
		"zsh:echotc:5: too many arguments\n 1\nzsh:echotc:6: no such capability: xx\n 1\n 1\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
