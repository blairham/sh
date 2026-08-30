// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capture runs a script and returns what it wrote, so the dialect can be
// tested without building a binary.
func capture(t *testing.T, src string) (string, int) {
	t.Helper()
	old := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wr
	done := make(chan string, 1)
	go func() {
		var b [4096]byte
		n, _ := rd.Read(b[:])
		done <- string(b[:n])
	}()
	code := run(src)
	_ = wr.Close()
	os.Stdout = old
	return strings.TrimRight(<-done, "\n"), code
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
