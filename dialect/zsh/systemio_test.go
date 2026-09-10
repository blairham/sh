// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// `sysopen`, `sysread` and `syswrite`, measured against zsh 5.9.2 (2026-09-10)
// with `zsh -f` and no startup files.
//
// Every case here asserts **what was opened and what came out of it**, never
// the status alone. That is the trap this builtin sets: a `sysopen` that
// opened the wrong file, opened read-only where read-write was asked for, or
// put the descriptor on a number nobody can reach still answers 0, and a test
// reading `$?` would pass on all three.

// systemDeadline runs body and fails if it has not finished in time.
//
// Every case below can *hang* rather than answer wrongly — a fifo open with no
// peer, a `sysread` on a descriptor nothing will ever write to — and the two
// failures do not look alike from outside: a wrong answer is a red test in
// milliseconds, a hang is a package timeout ten minutes later naming whichever
// case the binary was sitting in. Descriptor and fifo behavior also differ
// between this machine and the Linux runner, which is the specific way this
// file could pass review here and stop CI there.
func systemDeadline(t *testing.T, what string, body func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		body()
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s did not finish: the shell is still running", what)
	}
}

// **The line a prompt theme's asynchronous worker starts with.** A process
// substitution opened for reading through a close-on-exec descriptor, read
// back, and the bytes compared — which is the whole of what the `|| return` on
// that line was guarding, and what a `sysopen` registered and hollow passes at
// the status and fails here.
func TestSysopenReadsAProcessSubstitutionThroughACloseOnExecDescriptor(t *testing.T) {
	systemDeadline(t, "sysopen on a process substitution", func() {
		out, st := runZsh(t, t.TempDir(), `sysopen -r -o cloexec -u fd <(print -n one; print -n two) || { print -r -- failed; return }
print -r -- "open=$? usable=$(( fd > 2 ))"
sysread -i $fd buf
print -r -- "read=$? buf=[$buf]"`)
		want := "open=0 usable=1\nread=0 buf=[onetwo]\n"
		if out != want || st != 0 {
			t.Errorf("sysopen on <(…) = %q (status %d), want %q", out, st, want)
		}
	})
}

// **`-o cloexec` keeps the descriptor from a child and a plain one does not.**
// The pair, because either half alone passes on a shell that gets the other
// direction wrong: a descriptor nothing inherits looks like a working cloexec
// and like a broken table at the same time.
//
// Both are opened on numbers the script names rather than allocated, so the
// child can be told which to write to without the case depending on where a
// shell's allocator starts.
func TestSysopenCloexecKeepsADescriptorFromAChild(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh to run as a child: %v", err)
	}
	dir := t.TempDir()
	systemDeadline(t, "sysopen -o cloexec", func() {
		out, st := runZsh(t, dir, `sysopen -w -o creat,trunc,cloexec -u 7 kept
sysopen -w -o creat,trunc -u 8 given
`+sh+` -c 'echo x >&7' 2>/dev/null
print -r -- "closed-in-child=$?"
`+sh+` -c 'echo y >&8' 2>/dev/null
print -r -- "open-in-child=$?"
exec 7>&- 8>&-`)
		want := "closed-in-child=1\nopen-in-child=0\n"
		if out != want || st != 0 {
			t.Errorf("cloexec pair = %q (status %d), want %q", out, st, want)
		}
	})
	// And the files say what the statuses said, which is the half a child that
	// failed for some unrelated reason cannot fake.
	if got := readBackForTest(t, filepath.Join(dir, "kept")); got != "" {
		t.Errorf("the close-on-exec file holds %q, want it empty", got)
	}
	if got := readBackForTest(t, filepath.Join(dir, "given")); got != "y\n" {
		t.Errorf("the inherited file holds %q, want %q", got, "y\n")
	}
}

// **The direction letters decide what the descriptor can do**, and reading the
// file back is what says so: a shell that opened everything read-write would
// pass every status check here.
func TestSysopenDirectionLettersDecideTheMode(t *testing.T) {
	dir := t.TempDir()
	systemDeadline(t, "sysopen direction letters", func() {
		out, st := runZsh(t, dir, `print -r -- abcdef > f
sysopen -u ro f
read -u $ro line
print -r -- "default-reads=$? [$line]"
sysopen -w -u wo f
read -u $wo line
print -r -- "write-only-reads=$?"
sysopen -a -u ap f
print -n -u $ap XY
exec {ap}>&-
sysopen -r -u back f
read -u $back line
print -r -- "appended=[$line]"`)
		want := "default-reads=0 [abcdef]\nwrite-only-reads=1\nappended=[abcdef]\n"
		if out != want || st != 0 {
			t.Errorf("direction letters = %q (status %d), want %q", out, st, want)
		}
	})
	// The appended bytes are past the newline the first line ended on, so the
	// file rather than the first `read` is what proves `-a` wrote at the end
	// instead of over the front.
	if got := readBackForTest(t, filepath.Join(dir, "f")); got != "abcdef\nXY" {
		t.Errorf("the appended file holds %q, want %q", got, "abcdef\nXY")
	}
}

// **`-w` does not truncate and `-o trunc` does.** The one thing a shell that
// reached for `>` gets wrong, and it is silent: the caller's file is empty and
// the status is 0 either way.
func TestSysopenWriteDoesNotTruncateWithoutTheFlag(t *testing.T) {
	dir := t.TempDir()
	systemDeadline(t, "sysopen truncation", func() {
		_, st := runZsh(t, dir, `print -r -- abcdef > plain
print -r -- abcdef > cut
sysopen -w -u a plain
exec {a}>&-
sysopen -w -o trunc -u b cut
exec {b}>&-`)
		if st != 0 {
			t.Fatalf("status = %d, want 0", st)
		}
	})
	if got := readBackForTest(t, filepath.Join(dir, "plain")); got != "abcdef\n" {
		t.Errorf("`sysopen -w` left %q, want the file untouched", got)
	}
	if got := readBackForTest(t, filepath.Join(dir, "cut")); got != "" {
		t.Errorf("`sysopen -w -o trunc` left %q, want it empty", got)
	}
}

// **`-o excl` creates and then refuses the file it created**, which is the
// pair that makes it a lock and the reason it carries O_CREAT with it — see
// sysopenFlag. A shell passing O_EXCL alone opens the existing file at status
// 0, so the guard a script wrote it as claims a lock somebody else holds.
func TestSysopenExclusiveCreatesAndRefusesWhatIsThere(t *testing.T) {
	systemDeadline(t, "sysopen -o excl", func() {
		out, st, errs := runZshSplit(t, t.TempDir(), `sysopen -w -o excl -u a fresh
print -r -- "fresh=$? made=$([ -f fresh ] && print yes)"
sysopen -w -o excl -u b fresh
print -r -- "again=$?"`)
		want := "fresh=0 made=yes\nagain=1\n"
		if out != want || st != 0 {
			t.Errorf("exclusive = %q (status %d), want %q", out, st, want)
		}
		if !strings.Contains(errs, "can't open file fresh: file exists") {
			t.Errorf("refusal = %q, want the system's own sentence in it", errs)
		}
	})
}

// **`-m` is the mode a file this command creates gets.** 0604 rather than a
// rounder number on purpose: no ordinary umask produces those bits, so a shell
// that ignored the letter and took the 0666 default cannot land on it by
// accident.
func TestSysopenModeAppliesToACreatedFile(t *testing.T) {
	systemDeadline(t, "sysopen -m", func() {
		out, st, errs := runZshSplit(t, t.TempDir(), `sysopen -w -o creat -m 604 -u a odd
zmodload zsh/stat
zstat -s +mode -- odd
sysopen -w -o creat -m zz -u b bad
print -r -- "invalid=$?"`)
		want := "-rw----r--\ninvalid=1\n"
		if out != want || st != 0 {
			t.Errorf("modes = %q (status %d), want %q", out, st, want)
		}
		if !strings.Contains(errs, "invalid mode zz") {
			t.Errorf("refusal = %q, want `invalid mode zz` in it", errs)
		}
	})
}

// **`-u` puts the number where the script said**, by a name, by an association
// key and by a number — the three shapes, each proved by reading the file back
// through what it left behind rather than by the status.
func TestSysopenPutsTheDescriptorWhereTheCallerAsked(t *testing.T) {
	systemDeadline(t, "sysopen -u forms", func() {
		out, st, errs := runZshSplit(t, t.TempDir(), `print -n hello > f
sysopen -r -u named f
sysread -i $named a
print -r -- "named=[$a]"
typeset -A h
sysopen -r -u 'h[k]' f
sysread -i $h[k] b
print -r -- "keyed=[$b]"
sysopen -r -u 7 f
sysread -i 7 c
print -r -- "numbered=[$c]"
sysopen -r f
print -r -- "unspecified=$?"`)
		want := "named=[hello]\nkeyed=[hello]\nnumbered=[hello]\nunspecified=1\n"
		if out != want || st != 0 {
			t.Errorf("-u forms = %q (status %d), want %q", out, st, want)
		}
		if !strings.Contains(errs, "file descriptor not specified") {
			t.Errorf("refusal = %q, want the one that names neither a letter nor an operand", errs)
		}
	})
}

// **`sysread` separates the timeout from the end of input.** They are two of
// its five statuses and a prompt theme's receive loop turns on the difference:
// it reads on, quietly, for a 4, and gives the worker up for anything else. A
// shell that collapsed them would stop that worker on its first idle turn.
//
// The quiet descriptor is a fifo this test makes and holds open itself, so
// nothing is ever going to arrive on it and the case still cannot block: the
// writing end exists, so the open returns, and `-t` bounds the wait.
func TestSysreadSeparatesTheTimeoutFromTheEndOfInput(t *testing.T) {
	dir := t.TempDir()
	quiet := filepath.Join(dir, "quiet")
	if err := syscall.Mkfifo(quiet, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	// Held open in *both* directions so that neither end of the case's own
	// open can wait for a peer, and closed when the case ends.
	hold, err := os.OpenFile(quiet, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("hold the fifo open: %v", err)
	}
	t.Cleanup(func() { _ = hold.Close() })
	systemDeadline(t, "sysread statuses", func() {
		out, st, _ := runZshSplit(t, dir, `sysopen -r -u q quiet
sysread -t 0 -i $q a
print -r -- "nothing-waiting=$?"
sysread -t 0.2 -i $q b
print -r -- "waited=$?"
sysopen -r -u e <(:)
sysread -t 5 -i $e c
print -r -- "end=$?"
sysread -i 99 d
print -r -- "no-such=$?"
print -n hi | { sysread -o 99 f }
print -r -- "copy-failed=$?"
sysread -s x g
print -r -- "usage=$?"`)
		want := "nothing-waiting=4\nwaited=4\nend=5\nno-such=2\ncopy-failed=3\nusage=1\n"
		if out != want || st != 0 {
			t.Errorf("sysread statuses = %q (status %d), want %q", out, st, want)
		}
	})
}

// **One read, not a line and not the whole stream.** `-s` bounds it, a short
// read is a success, and the second call finds what the first left — which is
// what separates this from `read`, and is the half a builtin written as "read
// everything" passes at the status.
func TestSysreadTakesOneReadAndLeavesTheRest(t *testing.T) {
	systemDeadline(t, "sysread bounds", func() {
		out, st := runZsh(t, t.TempDir(), `print -r -- one > f
print -r -- two >> f
sysopen -r -u fd f
sysread -s 3 -c n -i $fd a
print -r -- "first=[$a] n=$n st=$?"
sysread -i $fd b
print -r -- "rest=[$b]"`)
		want := "first=[one] n=3 st=0\nrest=[\ntwo\n]\n"
		if out != want || st != 0 {
			t.Errorf("sysread bounds = %q (status %d), want %q", out, st, want)
		}
	})
}

// **`-o` diverts rather than duplicates.** The one thing about this builtin
// that a reading of its manual gets backwards, and a shell that assigned as
// well leaves a caller's parameter holding data it has already passed on.
func TestSysreadWithAnOutputDescriptorLeavesTheParameterAlone(t *testing.T) {
	systemDeadline(t, "sysread -o", func() {
		out, st := runZsh(t, t.TempDir(),
			`print -n hello | { sysread -c n -o 1 buf; print -r -- "|st=$? n=$n buf=[$buf] REPLY=[$REPLY]" }`)
		want := "hello|st=0 n=5 buf=[] REPLY=[]\n"
		if out != want || st != 0 {
			t.Errorf("sysread -o = %q (status %d), want %q", out, st, want)
		}
	})
}

// **`syswrite` writes every byte and counts them**, and says nothing at all
// when the descriptor refuses — which is what lets `while syswrite $'\x05'; do
// …; done` be a loop that ends quietly when the far side goes.
func TestSyswriteWritesEveryByteAndReportsARefusalInTheStatusAlone(t *testing.T) {
	dir := t.TempDir()
	systemDeadline(t, "syswrite", func() {
		out, st, errs := runZshSplit(t, dir, `sysopen -w -o creat,trunc -u fd f
syswrite -c n -o $fd $'a\nb\n'
print -r -- "wrote=$? n=$n"
exec {fd}>&-
syswrite -o 99 gone
print -r -- "refused=$?"`)
		want := "wrote=0 n=4\nrefused=2\n"
		if out != want || st != 0 {
			t.Errorf("syswrite = %q (status %d), want %q", out, st, want)
		}
		if errs != "" {
			t.Errorf("stderr = %q, want a refusal that says nothing", errs)
		}
	})
	if got := readBackForTest(t, filepath.Join(dir, "f")); got != "a\nb\n" {
		t.Errorf("the written file holds %q, want %q", got, "a\nb\n")
	}
}

// **The three names this module does not register refuse by their own names**,
// which is the difference between a narrow module and a hollow one. See
// registerSystemIO: `zmodload zsh/system` is 0 because a missing builtin is
// loud at the line that calls it, and `zmodload -F` naming one is not.
func TestTheUnimplementedSystemBuiltinsRefuseByName(t *testing.T) {
	out, _, _ := runZshSplit(t, t.TempDir(), `zmodload zsh/system
print -r -- "plain=$?"
zmodload -F zsh/system b:sysopen b:sysread b:syswrite
print -r -- "have=$?"
zmodload -F zsh/system b:zsystem
print -r -- "zsystem=$?"
zmodload -F zsh/system b:sysseek
print -r -- "sysseek=$?"
zmodload -F zsh/system b:syserror
print -r -- "syserror=$?"`)
	want := "plain=0\nhave=0\nzsystem=1\nsysseek=1\nsyserror=1\n"
	if out != want {
		t.Errorf("feature answers = %q, want %q", out, want)
	}
}

// **A subscripted destination refuses rather than assigning the wrong shape.**
// zsh splices characters into the string a scalar already holds, this shell's
// assignment path turns the scalar into an array, and the one answer that must
// not be given is a `buf` that looks assigned. See sysreadName.
func TestSysreadRefusesADestinationItCannotAssign(t *testing.T) {
	systemDeadline(t, "sysread destinations", func() {
		out, st, errs := runZshSplit(t, t.TempDir(), `buf=xy
print -n hi | { sysread 'buf[$#buf+1]' }
print -r -- "st=$? buf=[$buf]"
print -n hi | { sysread 'bad name' }
print -r -- "name=$?"`)
		want := "st=1 buf=[xy]\nname=1\n"
		if out != want || st != 0 {
			t.Errorf("sysread destinations = %q (status %d), want %q", out, st, want)
		}
		if !strings.Contains(errs, "a subscripted parameter is not implemented yet") ||
			!strings.Contains(errs, "not an identifier: bad name") {
			t.Errorf("refusals = %q, want each destination named", errs)
		}
	})
}

// readBackForTest is a file a case wrote, read back as a string.
func readBackForTest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
