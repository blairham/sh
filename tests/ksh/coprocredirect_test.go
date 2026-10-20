// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The coprocess body every row below starts: it reads one line, writes it
// back in brackets and ends.
//
// A body that ends on its own is what keeps these rows from *hanging* under a
// wrong answer rather than failing under one. `cat |&` never finishes, so a
// shell that let the write end stay open somewhere would leave the run
// waiting on a reap that cannot happen — a ten-minute test timeout instead of
// a message saying which line disagreed. The line it reads is written or not
// written by the very thing each row is about, so the body is also the
// assertion's other half: with the end moved away, the `print -p` that would
// have fed it fails and the body answers on end-of-file with an empty one.
const coprocEcho = "{ read l; print \"[$l]\"; } |&\n"

// `p` after `>&` or `<&` names the running coprocess's near end, so a script
// can park it on a number of its own and speak to the coprocess through that
// number instead of through the letters.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-16, from a script file under
// `env -i` with stdin from /dev/null.
func TestTheCoprocessIsADuplicationTarget(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the end this shell writes",
			coprocEcho + "exec 3>&p\nprint -u3 viathree\nexec 3>&-\nread -p got\necho \"got=$got\"\nwait\n",
			"got=[viathree]",
		},
		{
			"the end this shell reads",
			coprocEcho + "exec 4<&p\nprint -p sent\nread -u4 got\necho \"got=$got\"\nexec 4<&-\nwait\n",
			"got=[sent]",
		},
	} {
		out, st := runKshWithTools(t, tc.src)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q (status %d), want %q at 0", tc.name, out, st, tc.want)
		}
	}
}

// And the redirection **takes** the end rather than lending it: the letter
// that reached it finds no coprocess afterwards, one end at a time.
//
// Measured the same day. After `exec 3>&p` a `print -p` is
// `print: no query process [Bad file descriptor]` at 1, and after `exec 4<&p`
// a `read -p` is `read: no query process` — while the *other* end still
// answers in each case, which is what says the two move separately. zsh is
// the opposite answer and is pinned in its own dialect: there `print -p`
// still writes after an `exec 3>&p`.
func TestADuplicationTakesTheCoprocessEnd(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			"the write end, so the letter that wrote it finds none",
			coprocEcho + "exec 3>&p\nprint -p viap\necho \"st=$?\"\nexec 3>&-\nread -p out\necho \"out=$out\"\nwait\n",
		},
		{
			// The body is fed *before* the letter is asked, so that a shell
			// answering the other way reads a line rather than waiting on
			// one: a wrong answer here must fail, not deadlock.
			"the read end, so the letter that read it finds none",
			coprocEcho + "exec 4<&p\nprint -p sent\nread -p v\necho \"st=$?\"\nread -u4 out\necho \"out=$out\"\nexec 4<&-\nwait\n",
		},
	} {
		out, _ := runKshWithTools(t, tc.src)
		if !strings.Contains(out, "no query process") || !strings.Contains(out, "st=1") {
			t.Errorf("%s: got %q, want the letter refused at 1", tc.name, out)
		}
	}
}

// The end the redirection did **not** take is still the coprocess's, which is
// the half a rule about the facility as a whole would get wrong.
//
// Measured the same day: with the write end moved to a 3, `read -p` still
// answers what the body wrote; with the read end moved to a 4, `print -p`
// still reaches the body.
func TestTheOtherCoprocessEndSurvivesTheMove(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the read end, after the write end moved",
			coprocEcho + "exec 3>&p\nprint -u3 x\nexec 3>&-\nread -p out\necho \"out=$out\"\nwait\n",
			"out=[x]",
		},
		{
			"the write end, after the read end moved",
			coprocEcho + "exec 4<&p\nprint -p sent\nread -u4 out\necho \"out=$out\"\nexec 4<&-\nwait\n",
			"out=[sent]",
		},
	} {
		out, st := runKshWithTools(t, tc.src)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q (status %d), want %q at 0", tc.name, out, st, tc.want)
		}
	}
}

// Closing the number the end was moved to is what lets the coprocess read
// end-of-file, which is the whole reason a script moves it.
//
// Measured the same day: `cat |&; print -p a; read -p x; exec 3>&p; exec
// 3>&-; wait` answers `wait=0 x=[a]`. `cat` is the body here on purpose —
// nothing but the close can end it, so a shell that let the number go without
// ending the pipe has nothing for the `wait` to return from.
func TestClosingTheMovedEndEndsTheCoprocess(t *testing.T) {
	out, st := runKshWithTools(t,
		"cat |&\nprint -p a\nread -p x\nexec 3>&p\nexec 3>&-\nwait\necho \"wait=$? x=[$x]\"\n")
	if st != 0 || !strings.Contains(out, "wait=0 x=[a]") {
		t.Errorf("got %q (status %d), want wait=0 x=[a] at 0", out, st)
	}
}

// With no coprocess running the word is the duplication's own refusal and not
// an open that failed — and a file called `p` in the directory does not make
// it one.
//
// Measured the same day: `echo hi >&p` is `p: cannot open [Bad file
// descriptor]` at 1 with the script carrying on, and with `p` already a file
// the file is still empty afterwards, where bash 5.3.20 writes `hi` into it.
func TestTheCoprocessTargetWithNoCoprocessRunning(t *testing.T) {
	out, st := runKshWithTools(t, ": > p\necho hi >&p\necho \"st=$?\"\ncat p\necho end\n")
	if st != 0 || !strings.Contains(out, "p: cannot open [Bad file descriptor]") ||
		!strings.Contains(out, "st=1") || strings.Contains(out, "hi") {
		t.Errorf("got %q (status %d), want the refusal at 1 and no file written", out, st)
	}
}
