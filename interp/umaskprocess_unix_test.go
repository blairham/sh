// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The mask has to reach the kernel somewhere, and these are the two places it
// does: the mode a file is created with, and the mask a child inherits.
//
// Everything else about the mask can be checked with a hook that only
// remembers a number, and none of it would have caught the mode being wrong —
// a shell that keeps a perfect ledger of a mask it never applies passes every
// test in umaskscope_test.go. So the hook here is the real system call, which
// is what a binary that is a shell installs.
//
// Not parallel, and the process's own mask is put back afterwards: this is
// the one test in the package that really moves it.
func realUmaskRunner(t *testing.T, start int, dir string, out *bytes.Buffer) *Runner {
	t.Helper()
	was := syscall.Umask(start)
	t.Cleanup(func() { syscall.Umask(was) })
	sem := permissive()
	sem.UmaskPrintsFourDigits = Yes
	sem.UmaskSetWithSPrints = No
	sem.CommandNotFoundStatusIsNotFound = No
	r := newTestRunner(t, &Runner{
		Stdout: out, Stderr: out,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh", Env: []string{"PATH=" + dir},
	})
	r.SetUmask = func(mask int) (int, error) { return syscall.Umask(mask), nil }
	return r
}

// runReal parses and runs src through a shell whose mask really is the
// process's to move.
func runReal(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
}

// The mode a redirection creates a file with is the mask's whole point, and a
// body's mask reaches the files that body creates and no others.
//
// Every shell on the oracle panel answers the same script the same way, which
// is why this is here rather than under a dialect: `a` and `c` are
// `-rw-------` and `b` is `-rw-rw-r--`. Before #2949 the background job's mask
// reached the shell as well, so `c` came out `-rw-rw-r--` too.
func TestTheMaskDecidesTheModeOfEachFileTheShellCreates(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	r := realUmaskRunner(t, 0o022, dir, &out)
	runReal(t, r, `umask 077; :>a; { umask 002; :>b; } & wait; :>c; umask`)
	for _, want := range []struct {
		name string
		mode os.FileMode
	}{{"a", 0o600}, {"b", 0o664}, {"c", 0o600}} {
		info, err := os.Stat(filepath.Join(dir, want.name))
		if err != nil {
			t.Fatalf("%s: %v (the shell said %q)", want.name, err, out.String())
		}
		if got := info.Mode().Perm(); got != want.mode {
			t.Errorf("%s is %#o, want %#o", want.name, got, want.mode)
		}
	}
	if !strings.Contains(out.String(), "0077") {
		t.Errorf("the shell answered %q, want the 077 it kept", out.String())
	}
}

// A child does not read the mask, it inherits one — copied out of the parent
// at the fork — so the shell's own mask has to be on the process for exactly
// as long as a fork takes and no longer.
//
// The child is asked rather than the hook, because the hook cannot tell a
// mask that was on the process at the right moment from one that was on it at
// the wrong one.
func TestAChildInheritsTheMaskTheShellHolds(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "report-mask", "umask")
	var out bytes.Buffer
	r := realUmaskRunner(t, 0o022, dir, &out)
	runReal(t, r, `umask 077; report-mask; umask 002; report-mask; { umask 007; report-mask; } & wait; report-mask`)
	got := strings.Fields(out.String())
	want := []string{"0077", "0002", "0007", "0002"}
	if len(got) != len(want) {
		t.Fatalf("the children said %q, want %q", got, want)
	}
	for i := range want {
		if strings.TrimLeft(got[i], "0") != strings.TrimLeft(want[i], "0") {
			t.Errorf("child %d said %q, want %q", i, got[i], want[i])
		}
	}
}
