// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `zpty`, against docs/spec/pty.md.
//
// **Every pty here is read with a blocking read and nothing sleeps**, which is
// what makes these deterministic rather than timed: a blocking read waits for
// what it was asked for or for the command to end, and both of those are
// events rather than durations. A test that slept would be asserting on this
// machine's scheduler.
//
// Where a command has to be *doing nothing* for the question to mean anything
// — the echo pair — it waits on `zselect -t`, which touches no descriptor at
// all. `cat` and `read` are the two commands that cannot answer that question,
// for the reason the spec records: they put bytes back either way.

// The module loads and the builtin is there, which is the pair #3748 is about.
// Either one alone is the wrong answer: a table entry without the builtin
// walks a `zmodload zsh/zpty 2>/dev/null || return` past its own guard.
func TestZptyModuleAndBuiltinArriveTogether(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
print -r -- "load=$?"
zmodload -lF zsh/zpty
zpty -b P 'print -n x'
print -r -- "start=$?"
zpty -d P
print -r -- "delete=$?"`)
	want := "load=0\n+b:zpty\nstart=0\ndelete=0\n"
	if out != want || st != 0 {
		t.Errorf("the module and its builtin = %q (status %d), want %q", out, st, want)
	}
}

// It really is a terminal, and the command really is this shell: a function
// the caller defined runs under the pty and `[[ -t 1 ]]` is true in there.
//
// The two are one test because they are one claim — the command is the calling
// shell, on a terminal of its own — and a pty that answered the first without
// the second would be a pipe.
func TestZptyRunsTheCallingShellOnATerminal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
f() { [[ -t 1 ]] && print -n "f-on-a-terminal"; }
zpty P f
zpty -r P line '*terminal*'
print -r -- "st=$? line=$line"`)
	want := "st=0 line=f-on-a-terminal\n"
	if out != want || st != 0 {
		t.Errorf("the command under the pty = %q (status %d), want %q", out, st, want)
	}
}

// The words are joined with spaces and read as one command, so a pipeline is
// a legal command rather than a word this builtin would have quoted.
func TestZptyReadsItsOperandsAsOneCommand(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zpty P print -n abc
zpty -r P line '*abc*'
print -r -- "st=$? line=$line"`)
	want := "st=0 line=abc\n"
	if out != want || st != 0 {
		t.Errorf("the joined operands = %q (status %d), want %q", out, st, want)
	}
}

// A command that will not parse is reported and **the name is not
// registered**, which is the half a status alone cannot see: the next line
// asking after that name has to find none.
func TestZptyDoesNotRegisterANameWhoseCommandWillNotParse(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zpty BAD 'print <'
print -r -- "start=$?"
zpty -t BAD
print -r -- "test=$?"`)
	if !strings.Contains(out, "start=1\n") || !strings.Contains(out, "test=1\n") {
		t.Errorf("a command that will not parse = %q (status %d), want both statuses 1", out, st)
	}
	if !strings.Contains(out, "no such pty command: BAD") {
		t.Errorf("output = %q, want the name to have stayed unregistered", out)
	}
}

// Echo is **off** by default and `-e` turns it on, measured with a command
// that neither reads nor writes — so anything coming back is the terminal's
// own echo and nothing else.
//
// Without `-e` the blocking read has nothing to wait for and comes back when
// the command ends, at the status a finished command's empty read carries.
func TestZptyEchoesTheInputOnlyWhenAskedTo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zmodload zsh/zselect
zpty -e E 'zselect -t 30'
zpty -w E hello
zpty -r E seen
print -r -- "echo=$? seen=${seen%$'\r\n'}"
zpty D 'zselect -t 30'
zpty -w D hello
zpty -r D unseen
print -r -- "noecho=$? unseen=$unseen"`)
	want := "echo=0 seen=hello\nnoecho=2 unseen=\n"
	if out != want || st != 0 {
		t.Errorf("the echo pair = %q (status %d), want %q", out, st, want)
	}
}

// A pattern read stops **at** the match, and what came after it is still in
// the terminal for the next read.
//
// The second read is what makes this a test: a read that consumed everything
// and matched would pass the first assertion on its own.
func TestZptyAPatternReadStopsAtTheMatch(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zpty P print -n abcDONExyz
zpty -r P head '*DONE*'
print -r -- "head=$? [$head]"
zpty -r P rest
print -r -- "rest=$? [$rest]"`)
	want := "head=0 [abcDONE]\nrest=0 [xyz]\n"
	if out != want || st != 0 {
		t.Errorf("the pattern read = %q (status %d), want %q", out, st, want)
	}
}

// The three statuses of a read, and it is the **command** that decides between
// the two failures rather than the pattern: something read is 0, nothing from
// a command that has finished is 2, and nothing from one still running is 1.
func TestZptyAFailedReadSaysWhetherTheCommandHasFinished(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zmodload zsh/zselect
zpty P print -n hi
zpty -r P first
print -r -- "first=$? [$first]"
zpty -r P second
print -r -- "second=$?"
zpty -b R 'zselect -t 30'
zpty -r R nothing
print -r -- "running=$?"`)
	want := "first=0 [hi]\nsecond=2\nrunning=1\n"
	if out != want || st != 0 {
		t.Errorf("the read statuses = %q (status %d), want %q", out, st, want)
	}
}

// `-t` has three answers and not two: 0 while the command runs, 1 and silent
// once it has ended, and 1 with a diagnostic once the name is gone.
//
// The middle one is the row an implementation loses by forgetting that a
// command can end while its name stays.
func TestZptyTestTellsAnEndedCommandFromAnAbsentName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zmodload zsh/zselect
zpty -b R 'zselect -t 200'
zpty -t R
print -r -- "running=$?"
zpty P print -n x
zpty -r P v
zpty -t P
print -r -- "ended=$?"
zpty -t NOPE
print -r -- "absent=$?"
zpty -d`)
	want := "running=0\nended=1\nzsh:zpty:10: no such pty command: NOPE\nabsent=1\n"
	if out != want || st != 0 {
		t.Errorf("the three answers of -t = %q (status %d), want %q", out, st, want)
	}
}

// The listing is newest first, `-L` writes the call that would reproduce it
// with its flags, and a command that has ended lists as `(finished)`.
func TestZptyListsNewestFirstAndReproducibly(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zpty A print -n one
zpty -r A v
zpty -e B print -n two
zpty -r B w
zpty
zpty -L`)
	want := "(finished) B: 'print -n two'\n" +
		"(finished) A: 'print -n one'\n" +
		"zpty -e B 'print -n two'\n" +
		"zpty A 'print -n one'\n"
	if out != want || st != 0 {
		t.Errorf("the listings = %q (status %d), want %q", out, st, want)
	}
}

// A subshell sees the table, its delete reaches the command, and the parent
// keeps the **name** — so the parent's own delete answers 0 rather than
// complaining about a name it still has.
//
// Three rows, and each is a different half of the arrangement: the table is
// the subshell's own copy and the command in it is shared.
func TestZptyASubshellDeleteReachesTheCommandAndLeavesTheName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zmodload zsh/zselect
zpty -b P 'zselect -t 100'
( zpty -d P )
print -r -- "subshell=$?"
zpty -t P
print -r -- "parent-t=$?"
zpty -d P
print -r -- "parent-d=$?"`)
	want := "subshell=0\nparent-t=1\nparent-d=0\n"
	if out != want || st != 0 {
		t.Errorf("the subshell rows = %q (status %d), want %q", out, st, want)
	}
}

// `$REPLY` is the control end's descriptor, and it is a descriptor this shell
// can really use — which is the claim the manual makes and a number alone
// would not prove.
func TestZptyPublishesTheControlEndAsADescriptor(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zmodload zsh/system
zpty P print -n through-REPLY
sysread -i $REPLY got
print -r -- "read=$? got=$got"`)
	want := "read=0 got=through-REPLY\n"
	if out != want || st != 0 {
		t.Errorf("the published descriptor = %q (status %d), want %q", out, st, want)
	}
}

// The four refusals that share a sentence, and the two that do not.
func TestZptyRefusalsAreWordedAsMeasured(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zpty -Q
zpty S
zpty A print -n x
zpty A print -n y
zpty -w NOPE hello
zpty -r NOPE v`)
	wantWholeLines(t, out,
		"zsh:zpty:2: bad option: -Q",
		"zsh:zpty:3: missing command",
		"zsh:zpty:5: pty command name already used: A",
		"zsh:zpty:6: no such pty command: NOPE",
		"zsh:zpty:7: no such pty command: NOPE",
	)
}

// `-w` joins its strings with a space and adds a newline unless `-n`, which is
// what a command reading a line under the terminal sees.
func TestZptyWriteJoinsItsStringsAndEndsTheLine(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zpty
zpty P 'read -r line; print -n "got:$line"'
zpty -w P a b c
zpty -r P back '*got:*'
zpty -r P rest
print -r -- "st=$? [$rest]"`)
	if st != 0 || !strings.Contains(out, "[a b c]") {
		t.Errorf("the written line = %q (status %d), want the command to have read `a b c`", out, st)
	}
}
