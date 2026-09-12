// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A shell that runs *beside* another one holds its own descriptors, so a
// close in the shell that started it ends that shell's name for the file and
// nothing else.
//
// A real shell gets this from the fork and never decides it. Here the
// concurrent shell is a goroutine with a copy of the descriptor table, and
// the entries in that copy were the very same `*os.File` — so
// `exec {a}<&-` in the shell took the descriptor away from a substitution
// body, a background job or a pipeline element that was still using it
// (#2116).
//
// The visible cost was a command that never ran: a plugin manager's scheduler
// parks a descriptor per turn and drops it on the next, and the body starting
// `/bin/sleep` reported `fork/exec /bin/sleep: bad file descriptor` on about
// half of this machine's real startups. That spelling is a race and cannot be
// asserted; this one is the same ownership asked so that it always answers
// the same way — the close is made to land while the concurrent shell is
// parked on a rendezvous, before it touches the descriptor at all.
func TestAConcurrentShellKeepsADescriptorTheShellCloses(t *testing.T) {
	for _, tc := range []struct {
		name, src string
	}{
		{
			// The measured shape: a substitution's body outliving the
			// command that named it, with the shell dropping a parked
			// descriptor in between.
			"a process substitution's body",
			`exec {b}< <(read -r sig <sync; read -r v <&$a; echo "seen:$v")
exec {a}<&-
echo go >sync
read -r line <&$b
echo "$line"`,
		},
		{
			// The same question for a job, which outlives the shell that
			// started it by a longer way still.
			"a background job",
			`{ read -r sig <sync; read -r v <&$a; echo "seen:$v" >out; } &
exec {a}<&-
echo go >sync
wait
read -r line <out
echo "$line"`,
		},
		{
			// And for a pipeline element, where the shell closing the
			// descriptor is a *sibling* running at the same time rather than
			// the shell that made it.
			"a pipeline element",
			`{ read -r sig <sync; exec {a}<&-; } | { echo go >sync; read -r v <&$a; echo "seen:$v"; }`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithRendezvous(t, tc.src); got != "seen:kept\n" {
				t.Errorf("got %q, want %q", got, "seen:kept\n")
			}
		})
	}
}

// runWithRendezvous runs src in a directory holding `data`, a file with one
// line in it, already open on the descriptor `$a` — and `sync`, a named pipe
// the script uses to hold one shell still while the other one closes.
//
// The rendezvous is what makes a race into a question. Opening a named pipe
// for reading waits for a writer and opening one for writing waits for a
// reader, so the close in one shell is ordered before the read in the other
// however the goroutines are scheduled.
func runWithRendezvous(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data"), []byte("kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "sync"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The descriptor is opened by the script rather than handed in, so the
	// table entry is the one an `exec` makes and the close below is the one
	// that ended it.
	f, err := syntax.Parse("exec {a}<data\n"+src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	sem := PosixSemantics()
	sem.RedirectErrorOnSpecialBuiltinFatal = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: &strings.Builder{},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
