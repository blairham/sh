// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// `-r`, `-w` and `-x` ask whether this process may reach a path, which is not
// what the path's mode bits say.
//
// Reading the owner's three bits is wrong in **both** directions, and each
// direction needs a path of its own to show it: a root-owned directory at
// mode 755 has the owner's `w` set and an ordinary user still cannot write
// it, and a file at mode 400 has no `w` at all and an ACL can still grant one.
//
// Every `want` is the reference's own answer, measured 2026-09-28 against
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` says *not a Go executable* for it — and against
// `/opt/homebrew/bin/bash` and `/bin/dash`, which agree with it row for row.
// Script files under `env -i PATH=/usr/bin:/bin` with a scratch `HOME` and
// stdin at `/dev/null`, both shells reporting `id -u` of 501 (#5049).

// TestAPermissionTestAsksTheKernelNotTheModeBits is the forward direction:
// the mode bits grant and the process still cannot reach the path.
//
// Skipped for root, and that is the point of the skip rather than
// housekeeping: every path is reachable to root, so both readings agree on
// every row and the test would pass against the code it is here to refuse.
func TestAPermissionTestAsksTheKernelNotTheModeBits(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: both readings agree on every path, so nothing here can fail")
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		// The owner's `w` is set on all three and belongs to root.
		{`test -w /`, 1},
		{`test -w /etc`, 1},
		{`test -r /etc/master.passwd`, 1},
		// **The controls, and they are what say the mode is still read.** A
		// file this user owns answers correctly in both directions, so a
		// change that made every permission test false would fail here.
		{`test -w own600`, 0},
		{`test -w own400`, 1},
		{`test -r own400`, 0},
		{`test -x own700`, 0},
		{`test -r own000`, 1},
		{`test -w own000`, 1},
		{`test -x own000`, 1},
		// `/` is executable by everyone, so the two readings agree and this
		// row cannot fail — which is why it is a control and not evidence.
		{`test -x /`, 0},
		// And the path is still statted first: a name that is not there is
		// false for all three without asking anything else.
		{`test -r nosuchpath`, 1},
		{`test -w nosuchpath`, 1},
		{`test -x nosuchpath`, 1},
	} {
		t.Run(tc.src, func(t *testing.T) {
			writeModes(t, dir)
			if got := testStatus(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

// TestAGrantTheModeBitsDoNotCarryIsHonored is the **reverse** direction, and
// it is the row a fix keyed on ownership gets backwards.
//
// The file is this user's own, its mode has no `w` in it at all, and both the
// kernel and the reference call it writable because an access-control entry
// says so. Without this row, answering *no* for every path the user does not
// own would pass the forward test above.
//
// macOS only, because `chmod +a` is how this machine spells it; the check
// below writes to the file to prove the grant is real before grading
// anything, so a platform where the ACL silently did not take skips rather
// than asserting against a file that is simply read-only.
func TestAGrantTheModeBitsDoNotCarryIsHonored(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the ACL spelling here is macOS's")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: the mode bits are not consulted for root either")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "granted")
	if err := os.WriteFile(path, []byte("x\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	me, err := exec.Command("id", "-un").Output()
	if err != nil {
		t.Skip("no id(1) to name this user with")
	}
	who := string(me[:len(me)-1])
	if out, err := exec.Command("chmod", "+a", "user:"+who+" allow write,append", path).CombinedOutput(); err != nil {
		t.Skipf("no ACL support here: %v: %s", err, out)
	}
	// **Prove the grant is real before grading it.** An ACL that did not
	// take leaves a plain 0400 file, and a test that then asserted "writable"
	// would be asserting the bug. Opening it for append is the same question
	// the shell is about to be asked.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Skipf("the ACL did not take, so there is no grant to honor: %v", err)
	}
	_ = f.Close()

	if got, want := testStatus(t, dir, `test -w granted`), 0; got != want {
		t.Errorf("test -w on a file granted by ACL = %d, want %d — the mode bits say 400", got, want)
	}
	// And the control beside it: the same file's *read* is granted by the
	// mode bits, so this row agrees either way and says the operand is being
	// read at all.
	if got, want := testStatus(t, dir, `test -r granted`), 0; got != want {
		t.Errorf("test -r granted = %d, want %d", got, want)
	}
}

// TestAPermissionProbeIsOneEventLikeTheExistenceProbeIs is the other half of
// the decision that [Runner.accessible] does not ask the gate itself.
//
// `test -f` records one stat event, which TestAProbeIsAnEvent already pins.
// `test -w` asks the kernel a second question about the same path, and it
// must still be **one** act to a policy and one line in an audit: the
// existence half already went through the gate, and a path the gate hides
// never reaches the access call at all.
func TestAPermissionProbeIsOneEventLikeTheExistenceProbeIs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "own")
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"test -f " + path, "test -r " + path, "test -w " + path, "test -x " + path} {
		t.Run(src, func(t *testing.T) {
			var probes int
			for _, e := range collectEvents(t, src, nil) {
				if e.Kind == EventAccess && e.Action.Kind == ActionStat && e.Action.Path == path {
					probes++
				}
			}
			if probes != 1 {
				t.Errorf("saw %d stat events, want the one probe", probes)
			}
		})
	}
}

// And a path the gate hides answers *no* to all three, the way it answers no
// to `-e`: the refusal is quiet and indistinguishable from the file not being
// there, which is what ActionStat's deny semantics promise.
func TestAHiddenPathIsNotReadableWritableOrExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "own")
	if err := os.WriteFile(path, []byte("x\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	deny := GateFunc(func(_ context.Context, a Action) Decision {
		if a.Kind == ActionStat && a.Path == path {
			return Deny
		}
		return Allow
	})
	for _, tc := range []struct {
		src  string
		want int
	}{
		// Hidden, so every one of them is false — including the three that
		// would be true of the file as it stands on disk.
		{"test -e " + path, 1},
		{"test -r " + path, 1},
		{"test -w " + path, 1},
		{"test -x " + path, 1},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := gatedStatus(t, dir, tc.src, deny); got != tc.want {
				t.Errorf("%s behind a deny = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
	// The control: with the gate allowing, the same three are true — so the
	// rows above are the deny doing the work and not the file.
	allow := GateFunc(func(context.Context, Action) Decision { return Allow })
	for _, src := range []string{"test -e " + path, "test -r " + path, "test -w " + path, "test -x " + path} {
		if got := gatedStatus(t, dir, src, allow); got != 0 {
			t.Errorf("%s with the gate allowing = %d, want 0", src, got)
		}
	}
}

// gatedStatus runs one line under a gate and answers its status.
func gatedStatus(t *testing.T, dir, src string, gate Gate) int {
	t.Helper()
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Dir: dir,
		Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
		Gate: gate,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return int(r.ExitStatus())
}

// writeModes lays down the owned files the controls above name, fresh each
// time so one subtest cannot leave another's operand behind.
func writeModes(t *testing.T, dir string) {
	t.Helper()
	for name, mode := range map[string]os.FileMode{
		"own600": 0o600, "own400": 0o400, "own700": 0o700, "own000": 0o000,
	} {
		path := filepath.Join(dir, name)
		_ = os.Chmod(path, 0o600)
		if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}
