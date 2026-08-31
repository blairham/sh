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
	if filepath.Clean(out) != "/" {
		t.Errorf("pushd did not change directory, got %q", out)
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
