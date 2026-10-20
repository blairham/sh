// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `command` names an **external** command here, so its redirections are a
// process's and not this shell's.
//
// The axis is [interp.Semantics.CommandReachesABuiltin], which this column
// alone answers No — the shell has an option for the POSIX reading
// (`posixbuiltins`) and leaves it off. The consequence measured here is the
// one it had never been read for: where the word is a request for a program,
// the redirection target and the here-document body are expanded in that
// program's process, so a write in either does not come back and a failure in
// either costs the command rather than this shell.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh -f` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `not a Go executable` by `go version -m` —
// over a script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// the failing redirection on its own line with a second command beside it and
// `echo NEXT` on the line after (#4701).

// A write in the target of a `command`-prefixed redirection does not come
// back, where a bare builtin's does.
func TestACommandPrefixedTargetIsTheProgramsHere(t *testing.T) {
	t.Parallel()
	if zsh.Semantics().CommandReachesABuiltin != interp.No {
		t.Fatal("CommandReachesABuiltin is not No here, so `command` reaches a builtin and these rows are not this column's")
	}
	dir := t.TempDir()
	for _, c := range []struct {
		name, cmd string
		kept      bool
	}{
		{"a special builtin", `:`, true},
		{"a regular builtin", `read x`, true},
		{"behind the word", `command :`, false},
		{"a regular builtin behind it", `command read x`, false},
		{"a builtin with an argument behind it", `command echo RAN`, false},
		{"with -p", `command -p echo RAN`, false},
		{"a function behind it", `command f`, false},
		{"a program behind it", `command /bin/echo RAN`, false},
		// The reporting letters run nothing at all, in this column as in
		// every other, so they are this shell's under either answer.
		{"the reporting letter", `command -v :`, true},
	} {
		src := "f() { :; }\nunset u\n" + c.cmd + " > \"${u:=made}\" 2>/dev/null\nprintf 'u=%s' \"${u-unset}\"\n"
		out, _ := runZsh(t, dir, src)
		want := "u=unset"
		if c.kept {
			want = "u=made"
		}
		if !strings.HasSuffix(out, want) {
			t.Errorf("%s (%s): out = %q, want it to end in %s", c.name, c.cmd, out, want)
		}
	}
}

// And the same word at the other half of the redirection: a here-document
// body behind `command` is the program's too.
func TestACommandPrefixedHeredocBodyIsTheProgramsHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		cmd  string
		kept bool
	}{
		{`:`, true},
		{`echo`, true},
		{`command :`, false},
		{`command echo`, false},
	} {
		src := "unset u\n" + c.cmd + " <<END 2>/dev/null\n[${u:=made}]\nEND\nprintf 'u=%s' \"${u-unset}\"\n"
		out, _ := runZsh(t, dir, src)
		want := "u=unset"
		if c.kept {
			want = "u=made"
		}
		if !strings.HasSuffix(out, want) {
			t.Errorf("%s: out = %q, want it to end in %s", c.cmd, out, want)
		}
	}
}

// **Reach, status and `||` are three separate columns**, and a probe with the
// redirection alone on its line can tell none of them apart. A failing target
// on `command <builtin>` costs the command and nothing more: the rest of the
// line runs, 1 is left behind, `||` catches it and the script carries on —
// where the bare builtin ends this shell.
func TestAFailedCommandPrefixedTargetCostsTheCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`command :`, `command read x`, `command echo RAN`, `command -p echo RAN`} {
		out, st := runZsh(t, dir, cmd+" > $(( 1/0 )) ; echo \"SAME st=$?\"\necho NEXT\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", cmd, out)
		}
		if !strings.Contains(out, "SAME st=1") {
			t.Errorf("%s = %q, want the rest of the line running at 1", cmd, out)
		}
		if !strings.Contains(out, "NEXT") || st != 0 {
			t.Errorf("%s = %q (status %d), want the script carrying on", cmd, out, st)
		}
		caught, _ := runZsh(t, dir, cmd+" > $(( 1/0 )) || echo CAUGHT\necho NEXT\n")
		if !strings.Contains(caught, "CAUGHT") {
			t.Errorf("%s = %q, want || to catch it", cmd, caught)
		}
	}
}

// The pair that holds the *command* fixed and takes the word away: the same
// builtin, the same target, the same failure — and without `command` in front
// of it this shell ends, with no `SAME`, no `NEXT` and 1 as the script's own
// status. That is the row that says the word is doing the work.
func TestTheBareBuiltinStillEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `read x`, `echo RAN`, `f`, `{ echo RAN; }`} {
		out, st := runZsh(t, dir, "f() { echo RAN; }\n"+cmd+" > $(( 1/0 )) ; echo \"SAME st=$?\"\necho NEXT\n")
		if strings.Contains(out, "SAME") || strings.Contains(out, "NEXT") {
			t.Errorf("%s = %q, want the shell ended", cmd, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", cmd, st)
		}
	}
	// And the reporting letters with it, which is the control that says the
	// noun is a builtin behind the word rather than the word itself.
	for _, cmd := range []string{`command -v :`, `command -V :`} {
		out, st := runZsh(t, dir, cmd+" > $(( 1/0 )) ; echo \"SAME st=$?\"\necho NEXT\n")
		if strings.Contains(out, "NEXT") || st != 1 {
			t.Errorf("%s = %q (status %d), want the shell ended at 1", cmd, out, st)
		}
	}
}

// A failing here-document **body** behind the word answers the same way, and
// a bare builtin's still ends this shell.
func TestAFailedCommandPrefixedHeredocBodyCostsTheCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runZsh(t, dir, "command : <<END 2>/dev/null ; echo \"SAME st=$?\"\n$(( 1/0 ))\nEND\necho NEXT\n")
	if !strings.Contains(out, "SAME st=1") || !strings.Contains(out, "NEXT") || st != 0 {
		t.Errorf("behind the word = %q (status %d), want the line carrying on at 1", out, st)
	}
	bare, bareSt := runZsh(t, dir, ": <<END ; echo \"SAME st=$?\"\n$(( 1/0 ))\nEND\necho NEXT\n")
	if strings.Contains(bare, "NEXT") || bareSt != 1 {
		t.Errorf("bare = %q (status %d), want the shell ended at 1", bare, bareSt)
	}
}

// **Where the redirection is applied and what its failure costs are two
// questions**, and a failed *open* is what says so here: it costs the command
// on the bare builtin too, so the rows above did not move because a failure
// became survivable — they moved because the word moved the process.
func TestAFailedOpenCostsTheCommandWithOrWithoutTheWordHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `command :`, `command /bin/echo RAN`} {
		out, st := runZsh(t, dir, cmd+" > /nonexistent/d/f 2>/dev/null ; echo \"SAME st=$?\"\necho NEXT\n")
		if !strings.Contains(out, "SAME st=1") || !strings.Contains(out, "NEXT") || st != 0 {
			t.Errorf("%s = %q (status %d), want the line carrying on at 1", cmd, out, st)
		}
	}
}

// And the option that moves it, which is the decisive pair for the noun:
// `posixbuiltins` is the switch [interp.Semantics.CommandReachesABuiltin] is
// the axis for, and turning it on makes every row above the bare builtin's
// with the word `command` still written. Measured the same day on the same
// binary.
func TestPosixBuiltinsPutsTheWordBackInThisShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "setopt posixbuiltins\nunset u\ncommand : > \"${u:=made}\"\nprintf 'u=%s' \"${u-unset}\"\n")
	if !strings.HasSuffix(out, "u=made") {
		t.Errorf("out = %q, want the write kept once the option is on", out)
	}
	fail, st := runZsh(t, dir, "setopt posixbuiltins\ncommand : > $(( 1/0 )) ; echo \"SAME st=$?\"\necho NEXT\n")
	if strings.Contains(fail, "NEXT") || st != 1 {
		t.Errorf("out = %q (status %d), want the shell ended at 1 once the option is on", fail, st)
	}
}

// The positive row, so every null above is falsifiable: a `command`-prefixed
// target that expands is opened and the program it belongs to runs.
func TestACommandPrefixedTargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runZsh(t, dir, "command /bin/echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
