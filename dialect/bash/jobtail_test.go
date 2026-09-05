// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The job and lookup long tail, measured against bash 5.3 (2026-09-04):
// job specs by name, wait -n, disown, type's letters, ulimit -a's table and
// the directory stack.

func TestWaitJobSpecs(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 3; } &
wait %1; echo st=$?
wait %1; echo miss=$?`)
	if !strings.Contains(out, "st=3") {
		t.Errorf("got %q, want the job's status through %%1", out)
	}
	if !strings.Contains(out, "wait: %1: no such job") || !strings.Contains(out, "miss=127") {
		t.Errorf("got %q, want the missing spec refused at 127", out)
	}
}

func TestWaitAmbiguousName(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 3; } &
{ exit 4; } &
wait %{; echo st=$?`)
	if !strings.Contains(out, "wait: {: ambiguous job spec") || !strings.Contains(out, "st=127") {
		t.Errorf("got %q, want the ambiguous refusal at 127", out)
	}
}

func TestWaitN(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 3; } &
wait -n; echo st=$?
wait -n; echo empty=$?`)
	if !strings.Contains(out, "st=3") || !strings.Contains(out, "empty=127") {
		t.Errorf("got %q, want the first finisher's status and a silent 127", out)
	}
}

func TestDisownRemovesTheJob(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 0; } &
disown
jobs
echo st=$?
disown; echo none=$?`)
	if strings.Contains(out, "[1]") {
		t.Errorf("got %q, want the job gone from the listing", out)
	}
	if !strings.Contains(out, "disown: current: no such job") || !strings.Contains(out, "none=1") {
		t.Errorf("got %q, want the bare disown refused in bash's words", out)
	}
}

func TestKillMissingJob(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `kill %9; echo st=$?`)
	if !strings.Contains(out, "kill: %9: no such job") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the job complaint at 1", out)
	}
}

// type: -a lists everything, -p speaks only for files, -P always searches,
// -f leaves functions out.
func TestTypeLetters(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runBash(t, dir, `PATH=`+dir+`
tool() { :; }
type -a tool
type -p tool; echo p=$?
type -P echo; echo P=$?
type -f tool
type -p nosuchzz; echo miss=$?`)
	if !strings.Contains(out, "tool is a function\n") || !strings.Contains(out, "tool is "+exe+"\n") {
		t.Errorf("got %q, want -a listing the function and the file", out)
	}
	// -p on a function prints nothing at all; -P finds nothing for a
	// builtin name PATH does not hold, silently.
	if !strings.Contains(out, "p=0") || !strings.Contains(out, "P=1") {
		t.Errorf("got %q, want -p silent 0 and -P silent 1", out)
	}
	if !strings.Contains(out, "miss=1") {
		t.Errorf("got %q, want a silent 1 for -p on nothing", out)
	}
}

// -a with a function prints the body after the sentence, as plain type does.
func TestTypeAllPrintsTheBody(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `f() { echo hi; }
type -a f`)
	want := "f is a function\nf () \n{ \n    echo hi\n}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// ulimit -a renders bash's table — asserted whole, over fake limits, so the
// page is exact and nothing outside the test moves.
func TestUlimitListing(t *testing.T) {
	f, err := syntax.Parse("ulimit -a", bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "bash"}
	r.GetRlimit = func(res interp.Resource) (int64, int64, error) {
		switch res {
		case interp.ResourceFileSize:
			// 12345 of bash's 1024-byte blocks.
			return 12345 * 1024, 12345 * 1024, nil
		case interp.ResourceOpenFiles, interp.ResourceProcesses:
			return 256, 256, nil
		}
		return interp.RlimitInfinity, interp.RlimitInfinity, nil
	}
	r.SetRlimit = func(interp.Resource, int64, int64) error { return nil }
	bash.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	want := "core file size              (blocks, -c) unlimited\n" +
		"data seg size               (kbytes, -d) unlimited\n" +
		"file size                   (blocks, -f) 12345\n" +
		"max locked memory           (kbytes, -l) unlimited\n" +
		"max memory size             (kbytes, -m) unlimited\n" +
		"open files                          (-n) 256\n" +
		"pipe size                (512 bytes, -p) 1\n" +
		"stack size                  (kbytes, -s) unlimited\n" +
		"cpu time                   (seconds, -t) unlimited\n" +
		"max user processes                  (-u) 256\n" +
		"virtual memory              (kbytes, -v) unlimited\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// The directory stack: pushd prints the stack, a bare pushd swaps, popd
// pops and the empty stack refuses — the sentence bare, a shell function
// having no way to reach the engine's location prefix.
func TestDirectoryStack(t *testing.T) {
	home := t.TempDir()
	out, _ := runBash(t, home, bash.Prelude()+`
HOME=`+home+`
cd
pushd /tmp; echo st=$?
pushd; echo sw=$?
dirs
popd; echo p=$?
popd; echo p2=$?`)
	for _, want := range []string{
		"/tmp ~\nst=0\n",
		"~ /tmp\nsw=0\n",
		"~ /tmp\n",
		"/tmp\np=0\n",
		"popd: directory stack empty\np2=1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// The axes, pinned by name.
func TestJobAxes(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
	}{
		{"JobSpecsByName", s.JobSpecsByName},
		{"AmbiguousJobNameIsRefused", s.AmbiguousJobNameIsRefused},
		{"WaitReportsAMissingJob", s.WaitReportsAMissingJob},
		{"WaitNWaitsForTheNextJob", s.WaitNWaitsForTheNextJob},
		{"DisownRemovesTheJob", s.DisownRemovesTheJob},
	} {
		if tc.got != interp.Yes {
			t.Errorf("%s = %v, want yes", tc.axis, tc.got)
		}
	}
}
