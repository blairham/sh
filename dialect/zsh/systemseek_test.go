// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"strings"
	"testing"
)

// splitLines is the lines of a captured stream, with the empty one a trailing
// newline leaves taken off — so a count of them is a count of complaints.
func splitLines(text string) []string {
	trimmed := strings.TrimSuffix(text, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// `sysseek` and `syserror`, measured against zsh 5.9.2 (2026-09-10) with
// `zsh -f`.
//
// Every `sysseek` case reads the file **through** the descriptor afterwards
// rather than reading the status: a seek that went nowhere answers 0, and so
// does one that went to the wrong place.

// **The seek moves the descriptor, and each of the three origins moves it from
// somewhere else.**
//
// `-w start` from the front, `-w current` from where the descriptor already
// is, `-w end` from the size — proved by what the next read returns rather
// than by `systell`, which is the same shell answering a question about its
// own bookkeeping.
func TestSysseekMovesTheDescriptorFromEachOrigin(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `print -n 'hello world' > f
sysopen -r -u 7 f
sysread -i 7 -s 5 a
sysseek -u 7 0
sysread -i 7 -s 5 back
print -r -- "first=[$a] rewound=[$back]"
sysseek -u 7 6
sysread -i 7 rest
print -r -- "from-start=[$rest]"
sysseek -u 7 0
sysread -i 7 -s 4 skip
sysseek -u 7 -w current 2
sysread -i 7 -s 3 onward
print -r -- "from-current=[$onward]"
sysseek -u 7 -w end -5
sysread -i 7 tail
print -r -- "from-end=[$tail]"`)
	want := "first=[hello] rewound=[hello]\nfrom-start=[world]\n" +
		"from-current=[wor]\nfrom-end=[world]\n"
	if out != want || st != 0 {
		t.Errorf("sysseek = %q (status %d), want %q", out, st, want)
	}
}

// **A seek the kernel refuses is a silent 2**, which is the same vocabulary
// `syswrite` uses for a descriptor that refuses and for the same reason: this
// is a call rather than a command line, and the status is where a call says
// what happened.
//
// The refusals that *do* speak are here beside it, because the pair is the
// point: `-w bogus` and `-u bogus` are usage errors and say so, and a position
// before the start of a file is not.
func TestWhatSysseekRefuses(t *testing.T) {
	dir := t.TempDir()
	out, st, errs := runZshSplit(t, dir, `print -n 'hello world' > f
sysopen -r -u 7 f
sysseek -u 7 -1
print -r -- "before-the-start=$?"
sysseek -u 99 0
print -r -- "nofd=$?"
sysseek
print -r -- "none=$?"
sysseek -u 7 0 1
print -r -- "two=$?"
sysseek -u 7 -w bogus 0
print -r -- "origin=$?"
sysseek -u bogus 0
print -r -- "notanumber=$?"
sysseek -x 0
print -r -- "letter=$?"`)
	want := "before-the-start=2\nnofd=2\nnone=1\ntwo=1\norigin=1\nnotanumber=1\nletter=1\n"
	if out != want || st != 0 {
		t.Errorf("refusals = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs,
		"zsh:sysseek:7: not enough arguments",
		"zsh:sysseek:9: too many arguments",
		"zsh:sysseek:11: unknown argument to -w: bogus",
		"zsh:sysseek:13: integer expected: bogus",
		"zsh:sysseek:15: bad option: -x",
	)
	// The two silent ones said nothing, which is the half a Contains check on
	// the sentences above cannot see.
	if got, want := len(splitLines(errs)), 5; got != want {
		t.Errorf("stderr = %q, want exactly %d complaints", errs, want)
	}
}

// **`syserror` is the platform's own sentence, in the platform's own
// capitalization** — which differs from every other diagnostic in this dialect
// and is measured rather than chosen: `syserror 2` is `No such file or
// directory` and this shell's `sysopen /no/such` is `no such file or
// directory`.
//
// A name and its number give the same sentence, which is what says the lookup
// went through `$errnos` rather than through a table of its own.
func TestSyserrorWritesThePlatformsSentence(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), `syserror 2
print -r -- "number=$?"
syserror ENOENT
print -r -- "name=$?"
syserror -e held EACCES
print -r -- "held=[$held] diverted=$?"
syserror -e prefixed -p "oops: " ENOENT
print -r -- "prefixed=[$prefixed]"
syserror bogus
print -r -- "unknown=$?"
syserror enoent
print -r -- "lowercase=$?"`)
	want := "number=0\nname=0\nheld=[Permission denied] diverted=0\n" +
		"prefixed=[oops: No such file or directory]\nunknown=2\nlowercase=2\n"
	if out != want || st != 0 {
		t.Errorf("syserror = %q (status %d), want %q", out, st, want)
	}
	// **`-e` diverts rather than copies**, so the two lines that named a
	// parameter wrote nothing here, and the two that did not wrote the same
	// sentence twice.
	if got, want := errs, "No such file or directory\nNo such file or directory\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// **The three numbers a platform's table cannot answer for**, each of which is
// this platform's wording rather than the runtime's: zero, a number past the
// end, and — on this machine — the one errno newer than the table Go's
// `syscall` package was generated from.
//
// `errno 0` is what a shell passing the runtime's answer through would print,
// and it is neither a sentence nor an admission that there is none.
func TestSyserrorAnswersForTheNumbersWithNoName(t *testing.T) {
	out, _, errs := runZshSplit(t, t.TempDir(), `syserror 0
syserror 9999
print -r -- "st=$?"`)
	if want := "st=0\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	for _, line := range splitLines(errs) {
		if line == "errno 0" || line == "errno 9999" {
			t.Errorf("stderr = %q, want the platform's wording rather than the runtime's", errs)
		}
	}
	if got := len(splitLines(errs)); got != 2 {
		t.Errorf("stderr = %q, want two sentences", errs)
	}
}

// **`syserror` with no operand refuses by name rather than inventing a zero.**
//
// In zsh the operand defaults to the C library's `errno` at that instant,
// which is not a behavior a shell can be held to — measured twice in one
// session it was `No such file or directory` and then `Interrupted system
// call`. This shell has no such variable, and the sentence for zero is
// `Undefined error: 0`, which *means nothing went wrong*: printing it at
// status 0 to a script asking what went wrong is the accepting-and-inert
// failure this module has produced twice.
//
// The part that can be answered honestly is: zsh's `ERRNO` is writable, so a
// script that has put a number there gets that number's sentence.
func TestSyserrorWithNoOperandRefusesUnlessErrnoWasSet(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), `syserror
print -r -- "bare=$?"
ERRNO=13
syserror
print -r -- "set=$?"`)
	want := "bare=1\nset=0\n"
	if out != want || st != 0 {
		t.Errorf("syserror with no operand = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs,
		"zsh:syserror:1: this shell keeps no errno of its own; name one, or set ERRNO",
		"Permission denied",
	)
}
