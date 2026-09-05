// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runSplit keeps the two streams apart, which the shared helper deliberately
// does not: every question about `>&` is about which stream something went to.
func runSplit(t *testing.T, src string) (out, errOut string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := bash.Semantics()
	r := &Runner{Stdout: &o, Stderr: &e, Semantics: &sem}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return o.String(), e.String(), st
}

func TestDupToStderr(t *testing.T) {
	out, errOut, st := runSplit(t, `echo hi >&2`)
	if out != "" {
		t.Errorf("stdout should be empty, got %q", out)
	}
	if errOut != "hi\n" {
		t.Errorf("stderr = %q, want %q", errOut, "hi\n")
	}
	if st != 0 {
		t.Errorf("status = %d", st)
	}
}

func TestMergeStderrIntoStdout(t *testing.T) {
	out, errOut, _ := runSplit(t, `{ echo out; echo err >&2; } 2>&1`)
	if out != "out\nerr\n" {
		t.Errorf("stdout = %q, want both lines", out)
	}
	if errOut != "" {
		t.Errorf("stderr should be empty, got %q", errOut)
	}
}

// TestRedirectionOrderMatters is the classic, and it needs no special case:
// duplication copies the stream as it is at that moment, and the loop already
// runs left to right.
func TestRedirectionOrderMatters(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")

	_, errOut, _ := runSplit(t, `{ echo out; echo err >&2; } >`+f+` 2>&1`)
	if b, _ := os.ReadFile(f); string(b) != "out\nerr\n" {
		t.Errorf(">f 2>&1: file = %q, want both", b)
	}
	if errOut != "" {
		t.Errorf(">f 2>&1: stderr should be empty, got %q", errOut)
	}

	g := filepath.Join(dir, "g")
	out, errOut, _ := runSplit(t, `{ echo out; echo err >&2; } 2>&1 >`+g)
	if b, _ := os.ReadFile(g); string(b) != "out\n" {
		t.Errorf("2>&1 >f: file = %q, want only stdout", b)
	}
	// The err line goes where stdout pointed *at the time*, which is the
	// original stdout and not the file — that is the whole point of the
	// case, and it means asserting on stdout rather than on stderr.
	if out != "err\n" {
		t.Errorf("2>&1 >f: stdout = %q, want the err line", out)
	}
	if errOut != "" {
		t.Errorf("2>&1 >f: stderr = %q, want empty", errOut)
	}
}

func TestDupFromStdin(t *testing.T) {
	// `<&0` is a no-op that must not break reading.
	out, _, _ := runSplit(t, `echo x | { read v <&0; echo "[$v]"; }`)
	if !strings.Contains(out, "[x]") {
		t.Errorf("got %q", out)
	}
}

func TestAHighDescriptorDupIsKept(t *testing.T) {
	// `3>&1` used to be refused for want of a descriptor table; now the
	// table holds it and the command runs untouched.
	out, errOut, st := runSplit(t, `echo hi 3>&1`)
	if st != 0 || out != "hi\n" {
		t.Errorf("status %d, stdout %q, stderr %q", st, out, errOut)
	}
}

func TestDuplicatingFromANeverOpenedDescriptorIsRefused(t *testing.T) {
	_, errOut, st := runSplit(t, `echo hi >&9`)
	if st == 0 || !strings.Contains(errOut, "Bad file descriptor") {
		t.Errorf("status %d, stderr %q", st, errOut)
	}
}

func TestClosedDescriptorFailsTheWrite(t *testing.T) {
	// `>&-` closes. Writing then fails the way the kernel would.
	out, _, _ := runSplit(t, `echo hi >&-`)
	if out != "" {
		t.Errorf("a closed descriptor should not carry output, got %q", out)
	}
}
