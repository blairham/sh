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
			if got := runWithRendezvous(t, tc.src, nil); got != "seen:kept\n" {
				t.Errorf("got %q, want %q", got, "seen:kept\n")
			}
		})
	}
}

// runWithRendezvous runs src in a directory holding `data`, a file with one
// line in it, already open on the descriptor `$a` — and `sync` and `sync2`,
// named pipes the script uses to hold one shell still while the other one
// closes, and to learn that a shell which outlived its caller has finished.
//
// The rendezvous is what makes a race into a question. Opening a named pipe
// for reading waits for a writer and opening one for writing waits for a
// reader, so the close in one shell is ordered before the read in the other
// however the goroutines are scheduled.
//
// tweak, when it is not nil, moves the axes a case depends on. The preset is
// the standard's, which leaves unanswered every axis this file's subject is
// reachable through.
func runWithRendezvous(t *testing.T, src string, tweak func(*Semantics)) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data"), []byte("kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sync", "sync2"} {
		if err := syscall.Mkfifo(filepath.Join(dir, name), 0o600); err != nil {
			t.Fatal(err)
		}
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
	if tweak != nil {
		tweak(&sem)
	}
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

// The same ownership one number lower: descriptor 0.
//
// A pipeline element's input is the pipe, and runPipeline closes it the
// moment the element finishes — so a substitution's body that reads the
// element's input and outlives the element was reading a descriptor that had
// already gone. It is the same bug as the table above and it was left behind
// by it, because the named three streams are fields on the Runner rather than
// entries in `fds` and the copy only walked the map (#2144).
//
// The shape is measured rather than argued: `printf "PIPE\n" | { exec 3<
// <(sleep 0.4; cat >out); }` writes `PIPE` in bash 5.3 and wrote
// `cat: stdin: Bad file descriptor` here. The sleep is what made that
// reproduce, and a sleep is a window rather than a question — so the case
// below parks the body on `sync` instead, which the shell cannot write until
// the pipeline it is waiting for has ended.
//
// Both answers to whether the last element runs in the current shell, because
// they are two code paths: one hands the element a clone whose descriptors
// this copies, and the other runs it on the shell itself where there is no
// clone at all and the substitution's own copy is the only one.
func TestASubstitutionsBodyKeepsTheInputItsPipelineElementCloses(t *testing.T) {
	// `exec 3<` parks the substitution's pipe so the body may write without
	// anybody reading, which is what lets the element finish first. The body
	// then waits on `sync`, reads the input the element no longer holds, and
	// reports through `sync2` that it is done.
	const src = `printf "PIPE\n" | { exec 3< <(read -r sig <sync
read -r v
echo "seen:$v" >out
echo done >sync2); }
echo go >sync
read -r ack <sync2
read -r line <out
echo "$line"`

	for _, last := range []struct {
		name   string
		answer Answer
	}{
		{"last element on a copy", No},
		{"last element on the shell itself", Yes},
	} {
		t.Run(last.name, func(t *testing.T) {
			got := runWithRendezvous(t, src, func(sem *Semantics) {
				// The body reads the input of the command its word stands
				// in, which is the answer under which the element's pipe is
				// what it holds — and so the only one this is reachable
				// under. See ProcessSubstitutionBodyReadsTheShellsInput.
				sem.ProcessSubstitutionBodyReadsTheShellsInput = No
				sem.LastPipelineElementInCurrentShell = last.answer
			})
			if want := "seen:PIPE\n"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
