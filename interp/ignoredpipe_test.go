// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

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

// A broken pipe is answered by the signal's disposition rather than by the
// errno the kernel handed back.
//
// The write is the same write every time: into a reader that has gone. What
// differs is what the script has arranged for SIGPIPE. Left alone it kills the
// writer where it stands and nothing after the write runs. Ignored or handled,
// the kernel has no fatal signal to raise and returns EPIPE to a writer that
// is still there — and a failed write is then the ordinary failure the wording
// and the status axis already describe, with the handler delivered the way
// every other handler is.

// brokenPipe is a stream whose reader has gone: every write fails the way the
// kernel fails one into a pipe nobody holds open.
//
// A real pipeline would do it too, at the cost of writing past the buffer to
// make the race a certainty. This says the same thing in one line and says it
// about the shell's own output, which is where a handler is still in force.
type brokenPipe struct{}

func (brokenPipe) Write(p []byte) (int, error) { return 0, syscall.EPIPE }

func pipeRun(t *testing.T, src string) (errOut string, status int) {
	t.Helper()
	// A trap set at the top level is the *process's* disposition, because
	// that is what a shell at the top level is — and one test process runs
	// every test in the package, so an arrangement left behind here would be
	// inherited by every child a later test starts. Handing it back is the
	// shell's own job now (#2446): a `signal.Reset` in a t.Cleanup stood here
	// and, measured, does not undo an Ignore at all.
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem, diag := failingWrites()
	var e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: brokenPipe{}, Stderr: &e, Semantics: &sem, Diagnostics: &diag})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return e.String(), st
}

func TestADefaultBrokenPipeEndsTheWriter(t *testing.T) {
	errOut, _ := pipeRun(t, `echo hi; echo after >&2`)
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing — the writer is dead and says so to no one", errOut)
	}
}

func TestAnIgnoredBrokenPipeLeavesTheWriterRunning(t *testing.T) {
	errOut, _ := pipeRun(t, `trap '' PIPE; echo hi; echo after >&2`)
	if !strings.Contains(errOut, "after") {
		t.Errorf("stderr = %q, want the command after the failed write to run", errOut)
	}
}

// With the signal disarmed the write is an ordinary failed write, so it is
// reported in the wording every other failed write uses rather than in one of
// its own, and it carries the same status.
func TestAnIgnoredBrokenPipeIsWordedLikeAnyFailedWrite(t *testing.T) {
	errOut, _ := pipeRun(t, `trap '' PIPE; echo hi; echo "st=$?" >&2`)
	if want := "echo: write error:"; !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want it to carry %q", errOut, want)
	}
	if !strings.Contains(errOut, "st=1") {
		t.Errorf("stderr = %q, want the failed write to fail the command", errOut)
	}
}

// A handler is not an ignore, and it is not the default action either: the
// signal is delivered rather than fatal, so the body runs — between commands,
// where every handler runs — and the failed write is still reported.
func TestAHandledBrokenPipeRunsTheHandlerAndCarriesOn(t *testing.T) {
	errOut, _ := pipeRun(t, `trap 'echo handled >&2' PIPE; echo hi; echo after >&2`)
	for _, want := range []string{"echo: write error:", "handled", "after"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut, want)
		}
	}
	if i, j := strings.Index(errOut, "handled"), strings.Index(errOut, "after"); i > j {
		t.Errorf("stderr = %q, want the handler to run before the next command", errOut)
	}
}

// The three arrangements over one shape, so the difference between dying and
// carrying on is the trap and nothing else.
func TestOnlyTheDefaultActionEndsTheWriter(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup string
		dies  bool
	}{
		{"nothing arranged", ``, true},
		{"ignored", `trap '' PIPE; `, false},
		{"handled", `trap ':' PIPE; `, false},
	} {
		errOut, _ := pipeRun(t, c.setup+`echo hi; echo after >&2`)
		if got := !strings.Contains(errOut, "after"); got != c.dies {
			t.Errorf("%s: stderr = %q, writer died = %v, want %v", c.name, errOut, got, c.dies)
		}
	}
}

// An element's own broken pipe is the element's, and it is never delivered to
// the shell that started it. The element's own handler runs — every shell in
// the panel runs it — and the outer one would be a signal arriving at a
// process that never had it, which no shell in the panel does.
func TestABrokenPipeInsideAnElementIsNotTheOuterShellsToHandle(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`trap 'echo outer >&2' PIPE; ` +
		`{ trap 'echo inner >&2' PIPE; echo "$v"; echo reached >&2; } | true; echo after >&2`
	errOut, _ := pipeRun(t, src)
	if strings.Contains(errOut, "outer") {
		t.Errorf("stderr = %q, want the outer handler left alone", errOut)
	}
	for _, want := range []string{"inner", "reached", "after"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut, want)
		}
	}
	if i, j := strings.Index(errOut, "inner"), strings.Index(errOut, "reached"); i > j {
		t.Errorf("stderr = %q, want the element's handler to run before its next command", errOut)
	}
}

// A handler an element sets for itself is the element's to run, and the shell
// around it has no part in it either way — it may have no PIPE trap at all and
// the element's still runs.
func TestAnElementRunsThePipeHandlerItSetForItself(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`{ trap 'echo child >&2' PIPE; echo "$v"; echo reached >&2; } | true; echo after >&2`
	errOut, _ := pipeRun(t, src)
	for _, want := range []string{"child", "reached", "after"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut, want)
		}
	}
}

// A handler runs between commands, and the element's last command has no
// command after it — so the body ending is the last chance to run one.
// Measured: `{ trap 'echo child >&2' PIPE; echo "$big"; } | true` prints child
// in dash, bash 5.3, bash 3.2, ksh93 and zsh.
func TestAnElementsPipeHandlerRunsWhenTheFailedWriteWasItsLastCommand(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`{ trap 'echo child >&2' PIPE; echo "$v"; } | true; echo after >&2`
	errOut, _ := pipeRun(t, src)
	if !strings.Contains(errOut, "child") {
		t.Errorf("stderr = %q, want the handler to run as the element ends", errOut)
	}
}

// And it runs while the element's redirections are still in force, which is
// what says the last chance is at the end of the element's list rather than
// after its body has been taken down. Measured with the element's standard
// error sent to a file: dash, bash 5.3, ksh93 and zsh all put the handler's
// output in the file and none of them lets it reach the shell's own stream.
func TestAnElementsPipeHandlerRunsInsideItsOwnRedirections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "elem.err")
	src := `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`{ trap 'echo child >&2' PIPE; echo "$v"; } 2>` + path + ` | true`
	errOut, _ := pipeRun(t, src)
	if strings.Contains(errOut, "child") {
		t.Errorf("stderr = %q, want the handler's output caught by the element's redirection", errOut)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(got), "child") {
		t.Errorf("redirected file = %q, want it to carry the handler's output", got)
	}
}

// The same last chance for a subshell written out, which is a different clone
// site from a pipeline element and would otherwise drop the arrival with the
// runner.
func TestASubshellRunsThePipeHandlerItSetForItself(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`( trap 'echo child >&2' PIPE; echo "$v" ); echo after >&2`
	errOut, _ := pipeRun(t, src)
	for _, want := range []string{"child", "after"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut, want)
		}
	}
}

// An ignore is not a handler on either side of the boundary: an element that
// ignores PIPE for itself carries on and runs nothing.
func TestAnElementThatIgnoresPipeForItselfRunsNoHandler(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`trap 'echo outer >&2' PIPE; ` +
		`{ trap '' PIPE; echo "$v"; echo reached >&2; } | true; echo after >&2`
	errOut, _ := pipeRun(t, src)
	if strings.Contains(errOut, "outer") {
		t.Errorf("stderr = %q, want no handler at all for an ignored signal", errOut)
	}
	if !strings.Contains(errOut, "reached") {
		t.Errorf("stderr = %q, want the element to carry on past the failed write", errOut)
	}
}

// A pipeline element is where the difference between an ignore and a handler
// shows: POSIX carries an ignore across the boundary and puts a handled
// condition back to its default, so the same trap decides opposite outcomes
// for a writer standing inside one.
func TestAPipelineElementInheritsTheIgnoreAndNotTheHandler(t *testing.T) {
	// 2^17 bytes against a 64K buffer, so the write cannot fit and the
	// reader has already gone.
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ` +
		`{ echo "$v"; echo reached >&2; } | true; echo after >&2`
	for _, c := range []struct {
		name    string
		setup   string
		carries bool
	}{
		{"nothing arranged", ``, false},
		{"an inherited ignore", `trap '' PIPE; `, true},
		{"an inherited handler", `trap ':' PIPE; `, false},
	} {
		errOut, _ := pipeRun(t, c.setup+src)
		if got := strings.Contains(errOut, "reached"); got != c.carries {
			t.Errorf("%s: stderr = %q, writer carried on = %v, want %v", c.name, errOut, got, c.carries)
		}
		if !strings.Contains(errOut, "after") {
			t.Errorf("%s: stderr = %q, want the shell after the pipeline to carry on", c.name, errOut)
		}
	}
}
