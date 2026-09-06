// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/plugin"
	"github.com/blairham/sh/interp"
)

// A plugin's command is a builtin. Everything in this file goes through the
// interpreter rather than calling a stub, because being a builtin is the
// claim: found by name resolution, streams pointed wherever the runner points
// them, and a status that is the command's.

func TestAPluginCommandRunsAndWritesToTheShellsOutput(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "greet world"); code != 0 {
		t.Fatalf("status = %d, want 0", code)
	}
	if got := sh.out.String(); got != "hello world\n" {
		t.Errorf("out = %q, want what the plugin wrote", got)
	}
}

// The status the plugin answered is the command's status, which is what makes
// it usable in a script: `plugincmd || fallback` has to work.
func TestTheStatusIsThePluginsAnswer(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "shout loudly"); code != 3 {
		t.Errorf("status = %d, want the 3 the plugin answered", code)
	}
	if got := sh.errs.String(); !strings.Contains(got, "shouting about loudly") {
		t.Errorf("err = %q, want the plugin's error stream to reach the shell's", got)
	}
	if got := sh.out.String(); got != "" {
		t.Errorf("out = %q, want the error stream to have stayed off the output stream", got)
	}
}

// Under a redirection and inside a pipeline. This is the whole reason a
// builtin writes to r.Out() rather than to the process's own stdout, and a
// remoted builtin has to inherit the property rather than approximate it: the
// chunks come back over one connection and have to land on the streams of the
// call they belong to.
func TestAPluginCommandWorksUnderARedirectionAndInAPipeline(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "greet piped | tr a-z A-Z"); code != 0 {
		t.Fatalf("status = %d", code)
	}
	if got := sh.out.String(); got != "HELLO PIPED\n" {
		t.Errorf("out = %q, want the plugin's output to have gone through the pipe", got)
	}

	sh = newShell(t, h)
	dir := sh.r.Dir
	if code := sh.run(t, "greet redirected > out.txt; cat out.txt"); code != 0 {
		t.Fatalf("status = %d", code)
	}
	if got := sh.out.String(); got != "hello redirected\n" {
		t.Errorf("out = %q, want the redirection to have caught it", got)
	}
	_ = dir
}

// Two calls at once, which is the ordinary case rather than a stress test: a
// background job and each half of a pipeline run on their own goroutines. Each
// call's output has to reach its own streams, which is what the call id is
// for — a chunk that named no call would land on whichever stream was most
// recently asked for.
//
// The host does not serialize calls, and that is deliberate: a background job
// holding a plugin would otherwise block a foreground command into the same
// plugin. What follows is an obligation on the plugin, written down here
// because it is the sharpest edge in the protocol — a plugin that serves calls
// one at a time must not be put on both ends of one pipeline. The fixture
// this test uses is a shell script and is exactly that kind, so the two calls
// here are independent of each other. See the note in testdata/greet.
func TestTwoCallsAtOnceKeepTheirOwnStreams(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "greet one > a.txt & greet two > b.txt; wait; cat a.txt b.txt"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if got := sh.out.String(); got != "hello one\nhello two\n" {
		t.Errorf("out = %q, want each call's output in its own file", got)
	}
}

// The four host methods, which are the small part of the Runner a process that
// is not this one cannot do without.
func TestAPluginReadsAndWritesTheShellsOwnVariables(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "SRC=found; getset SRC; printf '%s\\n' \"$SEEN\""); code != 0 {
		t.Fatalf("status = %d", code)
	}
	// The value and whether the shell has it at all: an unset variable and an
	// empty one are different answers, and a shell spends a great deal of its
	// grammar on the difference.
	if got := sh.out.String(); got != "found/true\n" {
		t.Errorf("out = %q, want the plugin to have read the variable and written one back", got)
	}
}

func TestGetVarSaysWhenTheShellDoesNotHaveTheName(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "getset NOTHING_IS_SET_HERE; printf '%s\\n' \"$SEEN\""); code != 0 {
		t.Fatalf("status = %d", code)
	}
	if got := sh.out.String(); got != "/false\n" {
		t.Errorf("out = %q, want set to be false for a name the shell does not have", got)
	}
}

// The shell's working directory is the Runner's and not the process's, so a
// plugin resolving a relative path has no other way to ask. Proved by moving
// the shell somewhere the process is not.
func TestDirIsTheShellsDirectoryAndNotTheProcessesTest(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	sub := sh.r.Dir + "/inner"
	if code := sh.run(t, "mkdir inner && cd inner && where"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if got := strings.TrimSpace(sh.out.String()); got != sub {
		t.Errorf("where said %q, want the shell's own directory %q", got, sub)
	}
}

// Input, and it is a pull rather than a push: the plugin asks for bytes. A
// push would make the host's write block forever on a plugin that never
// reads, which is the lifetime rule's own forbidden shape.
func TestAPluginReadsTheCommandsInput(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "printf 'one\\ntwo\\n' | sink"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if got := sh.out.String(); got != "one\ntwo\n" {
		t.Errorf("out = %q, want everything on the command's input read back", got)
	}
}

// Input longer than one chunk, so the loop and the end-of-input flag both
// matter. A plugin that treated a short read as the end would truncate here.
func TestInputLongerThanOneChunkArrivesWhole(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	// The fixture asks for 64 bytes at a time, so this is many reads.
	if code := sh.run(t, "for i in 1 2 3 4 5 6 7 8 9 0; do printf 'abcdefghijklmnopqrstuvwxyz0123'; done | sink"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if got, want := len(sh.out.String()), 300; got != want {
		t.Errorf("read back %d bytes, want %d — a short read was taken for the end", got, want)
	}
}

// A plugin's complaint is located and named the dialect's way. Writing to the
// error stream is the other thing and carries neither, which is how a
// registered builtin ends up saying less than the one beside it.
func TestAPluginsDiagnosticIsWordedLikeTheShellsOwn(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "moan flimflam"); code != 1 {
		t.Fatalf("status = %d, want 1", code)
	}
	got := sh.errs.String()
	if !strings.Contains(got, "flimflam is not a thing") {
		t.Errorf("err = %q, want the plugin's text", got)
	}
	// Named as the shell, which is what Diagnosef adds and a bare write does
	// not.
	if !strings.Contains(got, "testsh") {
		t.Errorf("err = %q, want the shell's name on it", got)
	}
}

// A status that is not a status is a protocol error rather than a silent
// success. A plugin answering 256 has almost certainly handed back a wait
// status, and taking the low byte would report 0 for a command that failed.
func TestAStatusThatIsNotAStatusIsRefused(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "bigstatus"); code != 126 {
		t.Errorf("status = %d, want 126 — the command did not run", code)
	}
	if got := sh.errs.String(); !strings.Contains(got, "256") {
		t.Errorf("err = %q, want the answer it gave named", got)
	}
}

// A plugin command sits where a Go builtin sits in name resolution, which is
// the property that keeps a prelude winning outright over a plugin rather
// than by convention.
func TestAShellFunctionShadowsAPluginCommand(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "greet() { printf 'mine\\n'; }; greet world"); code != 0 {
		t.Fatalf("status = %d", code)
	}
	if got := sh.out.String(); got != "mine\n" {
		t.Errorf("out = %q, want the function to have won", got)
	}
}

// And a script cannot tell which of its builtins are plugins. That is
// deliberate in both directions: interp does not acquire a concept it has no
// use for, and a script has no business knowing.
func TestAScriptCannotTellAPluginCommandFromABuiltin(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	if kind, _ := sh.r.ResolveName("greet"); kind != interp.NameBuiltin {
		t.Errorf("ResolveName(greet) = %v, want a builtin", kind)
	}
	if !sh.r.KnownBuiltin("greet") {
		t.Error("KnownBuiltin(greet) is false, so `enable` could not see it")
	}
	names := sh.r.BuiltinNames()
	found := false
	for _, n := range names {
		if n == "greet" {
			found = true
		}
	}
	if !found {
		t.Error("BuiltinNames() does not list it, so a completer would not offer it")
	}
}

// Switching it off is `enable -n`'s business and works on a plugin's command
// exactly as on any other, because the name went in through Register.
func TestAPluginCommandCanBeSwitchedOff(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	sh := newShell(t, h)
	sh.r.SetBuiltinEnabled("greet", false)
	if kind, _ := sh.r.ResolveName("greet"); kind == interp.NameBuiltin {
		t.Error("it is still a builtin after being switched off")
	}
}
