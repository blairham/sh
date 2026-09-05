// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// capture runs a script through the binary's own shell value and returns what
// it wrote, so the dialect can be tested without building a binary.
//
// It takes the writers rather than redirecting os.Stdout, which is what the
// shared front end made possible: the pipe-and-goroutine version this replaced
// could deadlock on more than a pipe buffer of output.
func capture(t *testing.T, src string) (string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	sh := shell()
	sh.Stdout = &out
	sh.Stderr = &errs
	code := driver.Run(sh, src, "bash")
	if errs.Len() > 0 {
		t.Logf("stderr: %s", errs.String())
	}
	return strings.TrimRight(out.String(), "\n"), code
}

func TestPrimitivesDoWhatShellCannot(t *testing.T) {
	// `cd` has to change the runner's own directory, which no shell function
	// can say — the whole reason it is registered in Go rather than written
	// into the prelude.
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, code := capture(t, "cd "+dir+"; pwd")
	if code != 0 {
		t.Fatalf("status %d", code)
	}
	if got := filepath.Clean(out); got != real && got != filepath.Clean(dir) {
		t.Errorf("pwd = %q, want %q", got, real)
	}
}

func TestPreludeFunctionsBuildOnPrimitives(t *testing.T) {
	// pushd and popd are shell functions written on top of the registered cd,
	// which is the layering the extension story describes: Go for what shell
	// cannot express, shell for everything above it.
	dir := t.TempDir()
	out, code := capture(t, "cd "+dir+"; pushd /; pwd")
	if code != 0 {
		t.Fatalf("status %d", code)
	}
	// pushd prints the stack it just pushed — the measured behavior — so the
	// directory change is the last line, from pwd.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if got := filepath.Clean(lines[len(lines)-1]); got != "/" {
		t.Errorf("pushd did not change directory, got %q", out)
	}
	if !strings.HasPrefix(lines[0], "/ ") {
		t.Errorf("pushd did not print the stack, got %q", out)
	}
}

func TestTheDialectIsBash(t *testing.T) {
	// The axes, which are the whole of "which shell am I".
	if out, _ := capture(t, `echo $((0100))`); out != "64" {
		t.Errorf("a leading zero should be octal here, got %q", out)
	}
	if out, _ := capture(t, `x="a b"; printf "[%s]" $x`); out != "[a][b]" {
		t.Errorf("an unquoted expansion should split here, got %q", out)
	}
}

// TestItRunsAScriptFileAndNotOnlyDashC is the hole that made the conformance
// harness lie. This binary took only -c, so every case the corpus runs from a
// file failed with "bash: -c is required" — fourteen of them — and the number
// the harness published was a measurement of this file rather than of the core.
func TestItRunsAScriptFileAndNotOnlyDashC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.sh")
	if err := os.WriteFile(path, []byte("echo from-a-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errs bytes.Buffer
	sh := shell()
	sh.Stdout = &out
	sh.Stderr = &errs
	code := driver.MainArgs(sh, []string{"bash", path})
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if got := strings.TrimSpace(out.String()); got != "from-a-file" {
		t.Errorf("output = %q, want %q", got, "from-a-file")
	}
}

// TestAScriptIsNamedByItsPath is the other half of running a file: a shell
// names the *script* in a diagnostic, not itself. Without it the wording is
// right and the name in front of it is wrong, which the corpus checks for.
func TestAScriptIsNamedByItsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "case.sh")
	if err := os.WriteFile(path, []byte("set -u\necho \"$NOPE\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errs bytes.Buffer
	sh := shell()
	sh.Stdout = &out
	sh.Stderr = &errs
	if code := driver.MainArgs(sh, []string{"bash", path}); code == 0 {
		t.Fatal("an unset variable under set -u should fail")
	}
	if got := errs.String(); !strings.Contains(got, path) {
		t.Errorf("diagnostic %q does not name the script %q", got, path)
	}
}

// TestAPersonsRunCommandsFileIsRead is #807 measured against the binary rather
// than against the library, which is the distinction that matters here: the
// dialect's prelude is shell, so it moves shell state exactly the way a script
// does, and a startup rule that works with no prelude can still do nothing in
// the shell a person runs.
//
// The file is a realistic one — an alias, a function, an export and a prompt —
// because those are the four things that were silently missing, and each of
// them travels by a different mechanism.
func TestAPersonsRunCommandsFileIsRead(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".bashrc", strings.Join([]string{
		"alias ll='echo alias-ran'",
		"myfunc() { echo function-ran; }",
		"export FROMRC=yes",
		"PS1='rc> '",
	}, "\n")+"\n")

	out, errs, code := prompt(t, "ll\nmyfunc\necho \"FROMRC=$FROMRC\"\n", "bash", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	for _, want := range []string{"alias-ran", "function-ran", "FROMRC=yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want %q — the run-commands file did not take effect", out, want)
		}
	}
	// The prompt is written to the error stream, so a PS1 set by the file
	// shows up there rather than in the output.
	if !strings.Contains(errs, "rc> ") {
		t.Errorf("err = %q, want the prompt the file set", errs)
	}
}

// The person's file comes *after* the dialect's prelude, so what they wrote
// wins over what the shell shipped. The prelude is shell too, and a startup
// order that put it second would quietly undo every redefinition in a
// `~/.bashrc` — which is the shape that made a `$_` fix work in the core and
// do nothing in any binary.
func TestTheRunCommandsFileWinsOverThePrelude(t *testing.T) {
	home := scratchHome(t)
	// `pushd` is a prelude function, so redefining it is the sharpest test
	// there is of which of the two ran last.
	writeHomeFile(t, home, ".bashrc", "pushd() { echo mine-not-the-preludes; }\n")
	out, errs, code := prompt(t, "pushd /\n", "bash", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if !strings.Contains(out, "mine-not-the-preludes") {
		t.Errorf("out = %q, want the person's definition to have won", out)
	}
}

// A login shell reads the profile chain and not the run-commands file, which is
// this dialect's answer and the reason every tutorial tells a person to source
// one from the other by hand.
func TestALoginShellReadsTheProfileChain(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".bashrc", "echo read-rc\n")
	writeHomeFile(t, home, ".bash_profile", "echo read-profile\n")
	writeHomeFile(t, home, ".profile", "echo read-dot-profile\n")

	out, _, _ := prompt(t, "", "-bash", "-i")
	if !strings.Contains(out, "read-profile") || strings.Contains(out, "read-rc") {
		t.Errorf("out = %q, want the profile alone", out)
	}
	if strings.Contains(out, "read-dot-profile") {
		t.Errorf("out = %q, want only the first link of the chain", out)
	}
	// Asked for on the command line rather than inferred from argv[0], which
	// is the only way a person at a keyboard can say it.
	out, _, _ = prompt(t, "", "bash", "--login", "-i")
	if !strings.Contains(out, "read-profile") {
		t.Errorf("out = %q, want --login to have made it a login shell", out)
	}
}

// A file that breaks has to be escapable, or a person whose `~/.bashrc` fails
// every time the shell starts has no shell to repair it from.
func TestABrokenRunCommandsFileIsEscapable(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".bashrc", "if\n")

	if _, errs, code := prompt(t, "", "bash", "-i"); code == 0 || !strings.Contains(errs, ".bashrc") {
		t.Errorf("a broken file gave status %d and %q, want it reported", code, errs)
	}
	out, _, code := prompt(t, "echo alive\n", "bash", "--norc", "-i")
	if code != 0 || !strings.Contains(out, "alive") {
		t.Errorf("--norc gave status %d and %q, want a working session", code, out)
	}
	// And a file named in its place is read instead.
	writeHomeFile(t, home, "other", "echo from-the-named-file\n")
	out, _, _ = prompt(t, "", "bash", "--rcfile", filepath.Join(home, "other"), "-i")
	if !strings.Contains(out, "from-the-named-file") {
		t.Errorf("out = %q, want the named file", out)
	}
}

// scratchHome points HOME at a directory of this test's own. A test that read
// the developer's real home directory would be measuring their machine.
func scratchHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// The startup inputs a developer's own shell may have exported. t.Setenv
	// registers the restore; the unset that follows is what is wanted, and
	// there is no t.Unsetenv.
	for _, name := range []string{"ENV", "BASH_ENV", "ZDOTDIR"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
	return dir
}

func writeHomeFile(t *testing.T, home, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// prompt drives the binary's own shell value through an interactive session,
// with the typed lines on a pipe standing in for a person.
func prompt(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := shell()
	sh.Stdout, sh.Stderr = &o, &e
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(typed)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	sh.Stdin = r
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

// The mode can also be entered by option rather than by name, and the file
// follows it there.
//
// The two are asked separately because they are not the same moment: the name
// is read off argv[0] and turns the mode on *after* the startup files have
// run, while the option is applied before them. A front end that consulted
// only one of them reads the wrong file for the other, and this is the half a
// dialect without a `posix` option cannot reach.
//
// Measured 2026-09-05: `bash -o posix -i` reads `$ENV` and leaves `~/.bashrc`
// unread in bash 5.3.15, which is the panel member that counts — bash 3.2
// still reads `~/.bashrc` there, and disagrees with its later build about
// `--posix` in exactly the same way.
func TestTheModeCanBeEnteredByOptionToo(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".bashrc", "echo read-rc\n")
	writeHomeFile(t, home, "env.sh", "echo read-env\n")
	t.Setenv("ENV", filepath.Join(home, "env.sh"))

	out, _, _ := prompt(t, "", "bash", "-i")
	if !strings.Contains(out, "read-rc") || strings.Contains(out, "read-env") {
		t.Errorf("out = %q, want the shell's own file", out)
	}
	out, errs, code := prompt(t, "", "bash", "-o", "posix", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if !strings.Contains(out, "read-env") || strings.Contains(out, "read-rc") {
		t.Errorf("out = %q, want the standard's file in the mode", out)
	}
}
