// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"strings"
	"testing"
	"time"
)

// `zsh/zselect`, measured against zsh 5.9.2 (2026-09-10) with `zsh -f`.
//
// **The failure this builtin invites is a wait that does not wait.** Every
// case below that reads only a status would pass on a `zselect` that returned
// immediately and always — `-t 0` is 1 and so is a timeout — so the two that
// matter read the *clock*, and one of them waits on a byte a second process
// writes.

// **The line the prompt theme's worker guards on.**
//
// `internal/worker.zsh:196` writes `! { zselect -t0 || (( $? != 1 )) } ||
// return`, which is a probe: it asks for a wait of no time on nothing at all
// and insists the answer is exactly 1. So a `zselect` answering 0 there fails
// the guard, and so does one answering 1 with a diagnostic — the theme's
// worker stops either way, which is what it did while the module was refused.
//
// The two lines above it are here as well, because the whole of #1768 is that
// they are three consecutive lines and the first two are `|| return`.
func TestTheProbeAPromptWorkerGuardsOnPasses(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), `zmodload zsh/zselect || { print -r -- "module=failed"; return }
print -r -- "module=$?"
! { zselect -t0 || (( $? != 1 )) } || { print -r -- "probe=failed"; return }
print -r -- "probe=ok"`)
	want := "module=0\nprobe=ok\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the worker's two guards = %q (status %d, stderr %q), want %q", out, st, errs, want)
	}
}

// **`-t` is in hundredths of a second**, which is the unit nothing else in
// this shell uses and the one a reader is most likely to get wrong: the
// theme's `zselect -t 1000` is a ten-second wait, and a shell reading the
// number as milliseconds would turn its heartbeat loop into a spin.
//
// The clock is the assertion. A `zselect` that returned at once answers the
// same 1 this expects.
func TestZselectTimesOutInHundredthsOfASecond(t *testing.T) {
	dir := t.TempDir()
	systemDeadline(t, "zselect -t", func() {
		started := time.Now()
		out, st := runZsh(t, dir, "zselect -t 30\nprint -r -- \"st=$?\"")
		waited := time.Since(started)
		if want := "st=1\n"; out != want || st != 0 {
			t.Errorf("zselect -t 30 = %q (status %d), want %q", out, st, want)
		}
		// Thirty hundredths is 300ms. Two thirds of it, because the clock
		// here includes the shell's own start.
		if waited < 200*time.Millisecond {
			t.Errorf("zselect -t 30 took %v, want at least 200ms — it did not wait", waited)
		}
		// And it is not seconds, which is the other way to read the number:
		// thirty of those would still be running.
		if waited > 3*time.Second {
			t.Errorf("zselect -t 30 took %v, want well under 3s — the unit is not seconds", waited)
		}
	})
	systemDeadline(t, "zselect -t 0", func() {
		started := time.Now()
		out, st := runZsh(t, dir, "zselect -t 0\nprint -r -- \"st=$?\"")
		if want := "st=1\n"; out != want || st != 0 {
			t.Errorf("zselect -t 0 = %q (status %d), want %q", out, st, want)
		}
		if waited := time.Since(started); waited > 200*time.Millisecond {
			t.Errorf("zselect -t 0 took %v, want no wait at all", waited)
		}
	})
}

// **A `zselect` with no timeout blocks until a descriptor is ready**, and the
// descriptor is fed by a second process so that nothing in this shell can
// decide when it becomes ready.
//
// Three assertions and each kills a different mistake: the status says the
// wait ended in a descriptor rather than in nothing, `$reply` says *which*
// descriptor and says it in the shell's own numbering rather than the
// kernel's, and the clock says the wait happened at all.
func TestZselectBlocksUntilADescriptorIsReadable(t *testing.T) {
	dir := helperScripts(t)
	const delay = 700 * time.Millisecond
	systemDeadline(t, "zselect on a descriptor", func() {
		started := time.Now()
		out, st := runZsh(t, dir, `sysopen -r -u fd <(delaywrite unused 700)
zselect -r $fd
print -r -- "st=$? reply=($reply) named=$(( $reply[2] == fd )) shape=$#reply"`)
		waited := time.Since(started)
		if !strings.HasPrefix(out, "st=0 reply=(-r ") || !strings.Contains(out, "named=1 shape=2") {
			t.Errorf("zselect -r on a slow writer = %q (status %d), want st=0 with the descriptor named", out, st)
		}
		if waited < delay*2/3 {
			t.Errorf("zselect -r took %v, want at least %v — it did not block", waited, delay*2/3)
		}
	})
}

// **The answer's shape**, which is what a caller reads rather than the status:
// an array of a set's letter followed by every descriptor ready in it, the
// sets in the order `r`, `w`, `e`, and the descriptors inside each ascending.
//
// `-a` puts it in a named array and `-A` in an association keyed by the
// descriptor, whose value is the letters it was ready in — the one shape here
// where a descriptor appears once rather than once per set. All measured.
func TestZselectAnswersInReplyAndInTheNamedParameters(t *testing.T) {
	dir := t.TempDir()
	// Two descriptors on a regular file, which is ready for reading and for
	// writing at every instant — so what is being read here is the *shape* of
	// the answer with the waiting taken out of it. The numbers are named
	// rather than allocated so the expected text does not depend on where
	// this shell's allocator starts.
	//
	// A file rather than this shell's own standard output, which is where the
	// case was written first: in a test the three named streams are memory
	// buffers rather than descriptors, so `zselect -w 1` there asks about
	// something the kernel has never heard of and is right to answer nothing.
	open := "sysopen -w -o creat,trunc -u 7 a\nsysopen -w -o creat,trunc -u 8 b\n"
	out, st := runZsh(t, dir, open+`zselect -w 7 -w 8 -t 0
print -r -- "reply=($reply)"
zselect -a mine -w 8 -t 0
print -r -- "mine=($mine)"
zselect -A keyed -w 8 -t 0
print -r -- "keyed=(${(kv)keyed})"
zselect -w 8 7 -t 0
print -r -- "bare=($reply)"
zselect -r 7 -w 7 -A both -t 0
print -r -- "both=(${(kv)both})"`)
	// A regular file is ready in **both** directions whatever it was opened
	// for — the kernel answers about the file rather than about the open —
	// which is measured in zsh too and is what makes the last line the case
	// for a descriptor appearing once with two letters.
	want := "reply=(-w 7 8)\nmine=(-w 8)\nkeyed=(8 w)\nbare=(-w 7 8)\nboth=(7 rw)\n"
	if out != want || st != 0 {
		t.Errorf("the answer = %q (status %d), want %q", out, st, want)
	}
}

// **Nothing is assigned when nothing was ready.**
//
// Measured — `reply=(x y); zselect -r 1 -t 0` leaves `reply` as `(x y)` — and
// it is not tidiness: a shell that emptied the array would be telling a caller
// holding a stale answer that it now has a fresh one, at the status that says
// there was nothing to have.
func TestZselectLeavesTheAnswerAloneWhenNothingIsReady(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `reply=(old answer)
mine=(other answer)
zselect -r 0 -t 0
print -r -- "st=$? reply=($reply)"
zselect -a mine -r 0 -t 0
print -r -- "st=$? mine=($mine)"`)
	want := "st=1 reply=(old answer)\nst=1 mine=(other answer)\n"
	if out != want || st != 0 {
		t.Errorf("a wait that found nothing = %q (status %d), want %q", out, st, want)
	}
}

// **What `zselect` refuses, in its own words.** Every line measured against
// zsh 5.9.2 one at a time.
//
// An option letter this builtin has not got is not `bad option` — it is
// `expecting file descriptor`, because a word starting with a dash that is not
// one of the six letters is read as one that should have been a descriptor.
// That is also what a stray operand gets, and it is why the last two lines are
// in the same case as the first.
func TestWhatZselectRefuses(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), `zselect -q 3 -t 0
print -r -- "letter=$?"
zselect -t
print -r -- "novalue=$?"
zselect -t bogus
print -r -- "notanumber=$?"
zselect -a "bad name" -t 0
print -r -- "badarray=$?"
zselect -t 0 extra
print -r -- "operand=$?"
zselect -r 0x -t 0
print -r -- "garbage=$?"`)
	want := "letter=1\nnovalue=1\nnotanumber=1\nbadarray=1\noperand=1\ngarbage=1\n"
	if out != want || st != 0 {
		t.Errorf("refusals = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs,
		"zsh:zselect:1: expecting file descriptor: q",
		"zsh:zselect:3: argument expected after -t",
		"zsh:zselect:5: number expected after -t",
		"zsh:zselect:7: invalid array name: bad name",
		"zsh:zselect:9: expecting file descriptor: extra",
		"zsh:zselect:11: garbage after file descriptor: x",
	)
}

// **The module loads and answers for its one feature by name.**
//
// `zmodload zsh/zselect` was `not implemented yet` before #1768 — the module
// was not in the feature table at all — and the theme's `|| return` on that
// line is the first of the two guards the worker's body stops at.
func TestTheZselectModuleLoadsAndNamesItsOneBuiltin(t *testing.T) {
	out, st, _ := runZshSplit(t, t.TempDir(), `zmodload zsh/zselect
print -r -- "plain=$?"
zmodload -F zsh/zselect b:zselect
print -r -- "named=$?"
zmodload -lF zsh/zselect
zmodload -F zsh/zselect b:nosuchthing
print -r -- "invented=$?"`)
	want := "plain=0\nnamed=0\n+b:zselect\ninvented=1\n"
	if out != want || st != 0 {
		t.Errorf("the module = %q (status %d), want %q", out, st, want)
	}
}
