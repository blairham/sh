// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `$sysparams[pid]` inside a process substitution, where the front end has
// given the shell a way to reconstruct a process boundary.
//
// The rest of system_test.go measures the answer where it has not: empty, in
// all four bodies a real shell would have forked. That is still the answer for
// three of them and for a Runner nobody handed a placeholder to, and it was
// the answer everywhere until the prompt's git backend turned out to be
// declining to start because of it (#2083).

// TestSysParamsPidIsTheBodysGroupWhenTheShellCanReconstructOne is the fix,
// stated the way the script that needs it reads the value: a real number,
// naming a process group that is not this process's.
//
// Both halves matter and the second is the one with a session riding on it.
// The plugin that reads this key spends it on `kill -- -$pgid`; answering with
// a number that is this shell's group is how a prompt theme killed the
// interactive shell it was drawing for (#2046), so "non-empty" is not the
// property to assert — "not ours" is.
func TestSysParamsPidIsTheBodysGroupWhenTheShellCanReconstructOne(t *testing.T) {
	got := strings.TrimSpace(runZshAnchored(t, `read -r line < <(print -r -- $sysparams[pid])
print -r -- $line`))
	pgid, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("the body read %q as its own process, want a number: %v", got, err)
	}
	if pgid == os.Getpid() {
		t.Fatalf("the body was given this shell's own pid %d", pgid)
	}
	self, err := syscall.Getpgid(0)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	if pgid == self {
		t.Fatalf("the body was given this shell's own process group %d; `kill -- -%d` is the shell", pgid, pgid)
	}
}

// And `$$` is still this shell everywhere, which is what makes the answer
// above a *difference* rather than a shell that has lost track of itself. That
// difference is the entire reason the key exists next to `$$`.
func TestTheShellsOwnPidIsStillItsOwnInsideAnAnchoredBody(t *testing.T) {
	got := strings.TrimSpace(runZshAnchored(t, `read -r line < <(print -r -- "$$ $sysparams[pid]")
print -r -- $line`))
	fields := strings.Fields(got)
	if len(fields) != 2 {
		t.Fatalf("the body printed %q, want two numbers", got)
	}
	if fields[0] != strconv.Itoa(os.Getpid()) {
		t.Errorf("$$ inside the body is %s, want this shell's %d", fields[0], os.Getpid())
	}
	if fields[0] == fields[1] {
		t.Errorf("both keys answered %s; the body's group must not be the shell's pid", fields[0])
	}
}

// And the other four bodies, which had nothing until #2114: a subshell, a
// command substitution, a pipeline element and a background job.
//
// Measured against zsh 5.9.2, one script and the same contexts. Real zsh
// answers a different pid in every one of them, because it forked; here each
// answers the group its body leads, which is the reading the value is spent
// under. The assertion is the one the session rides on and not "non-empty":
// the number must be neither this process nor this process's group, since the
// script's next line is `kill -- -$pgid`.
func TestEveryForkedBodyGetsAGroupOfItsOwn(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a subshell", `( print -r -- $sysparams[pid] )`},
		{"a command substitution", `print -r -- $(print -rn -- $sysparams[pid])`},
		{"a pipeline element", `{ print -r -- $sysparams[pid] } | { read -r l; print -r -- $l }`},
		{"a background job", `{ print -r -- $sysparams[pid] } &` + "\nwait"},
	} {
		t.Run(c.name, func(t *testing.T) {
			assertBodyGroup(t, runZshAnchored(t, c.src))
		})
	}
}

// The last element of a zsh pipeline is the exception, and it is not an
// oversight: that element runs on the shell itself, so the honest answer is
// the shell's own number.
//
// Measured, real zsh answers exactly that — `echo a | { read l; print -r --
// $sysparams[pid] }` prints the shell's pid where an element with something
// after it prints a
// fork's. It falls out of the mechanism rather than being special-cased:
// only the elements the pipeline *clones* are given an anchor, and the last
// one is not cloned.
func TestTheLastPipelineElementIsStillTheShell(t *testing.T) {
	got := strings.TrimSpace(runZshAnchored(t, "print -rn -- a | { read -r l; print -r -- $sysparams[pid] }"))
	if got != strconv.Itoa(os.Getpid()) {
		t.Errorf("the last element answered %q, want this shell's %d — it is the shell", got, os.Getpid())
	}
}

// Nothing is started for a body that never asks, which is what keeps this
// affordable: the anchor is a process, and a script full of subshells that
// never mention the key must not fork one per subshell.
//
// Asked as the count of children this process has left behind, which is zero
// either way once they are reaped — so it is asked the only way a test can:
// the parameter is unread, and the output is exactly what the body printed.
func TestABodyThatNeverAsksStartsNothing(t *testing.T) {
	const src = `( print -r -- one )
print -r -- $(print -rn -- two)
{ print -r -- three } | { read -r l; print -r -- $l }
{ print -r -- four } &
wait`
	if got := runZshAnchored(t, src); got != "one\ntwo\nthree\nfour\n" {
		t.Errorf("output = %q, want the four lines and nothing else", got)
	}
}

// A body nested inside another gets its own group, which is what a real shell
// does: it forks again.
func TestANestedBodyGetsItsOwnGroup(t *testing.T) {
	got := runZshAnchored(t, `( print -r -- $sysparams[pid]; ( print -r -- $sysparams[pid] ) )`)
	lines := strings.Fields(got)
	if len(lines) != 2 {
		t.Fatalf("output = %q, want two numbers", got)
	}
	if lines[0] == lines[1] {
		t.Errorf("both bodies answered %s; the inner one is a second fork in a real shell", lines[0])
	}
	assertBodyGroup(t, lines[0]+"\n")
	assertBodyGroup(t, lines[1]+"\n")
}

// assertBodyGroup is the whole of what a body's answer has to be: a number,
// and neither this process nor the group this process is in.
func assertBodyGroup(t *testing.T, out string) {
	t.Helper()
	pgid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("the body answered %q, want a number: %v", out, err)
	}
	if pgid == os.Getpid() {
		t.Fatalf("the body was given this shell's own pid %d", pgid)
	}
	self, err := syscall.Getpgid(0)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	if pgid == self {
		t.Fatalf("the body was given this shell's own process group %d; `kill -- -%d` is the shell", pgid, pgid)
	}
}

// runZshAnchored is runZshSplit with a placeholder program supplied, which is
// what a shell binary does for itself — see driver/procanchor.go. `cat` blocks
// on its standard input and exits at end of file, which is the whole contract.
func runZshAnchored(t *testing.T, src string) string {
	t.Helper()
	anchor, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("no placeholder program on this machine: %v", err)
	}
	f, perr := syntax.Parse(src, zsh.Dialect())
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	dir := t.TempDir()
	var out, errs bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
		Dialect: presetDialect(), ProcessAnchor: []string{anchor},
	}
	zsh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String() + errs.String()
}
