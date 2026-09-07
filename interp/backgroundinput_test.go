// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a job started with `&` reads for standard input (#1287).
//
// The probe reads one descriptor twice — once from the background job and then
// once from the script — so the two answers come out in opposite orders and
// neither can be mistaken for the other. Measured 2026-09-07,
// `<shell> -c '/bin/cat & wait; echo ---; /bin/cat' < f` writes `---` and then
// the file's line in dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57 and ksh93u+,
// and the file's line before the marker in zsh 5.9.2. POSIX XCU 2.9.3 says a
// background command's standard input "shall be assigned to an empty file or
// /dev/null" while job control is disabled, so the majority is the specified
// answer.
//
// The trailing foreground read is the control and it is what makes this a
// measurement rather than a spelling: it must produce the line under the
// majority answer, which proves the file and the descriptor were live and that
// the empty stream reached the job alone.
func TestABackgroundJobsStandardInputIsAnAxis(t *testing.T) {
	const src = `/bin/cat & wait "$!"; echo "---"; /bin/cat`
	for _, tc := range []struct {
		answer BackgroundJobInputPolicy
		want   string
	}{
		{BackgroundJobInputEmpty, "---\nDATA\n"},
		{BackgroundJobInputEmptyUnlessClosed, "---\nDATA\n"},
		{BackgroundJobInputIsTheShells, "DATA\n---\n"},
		// Unanswered is the POSIX answer rather than a refusal: `&` is far
		// too ordinary to refuse over an axis a job may never reach. Stated
		// here so a change of that default has to change a test.
		{BackgroundJobInputUnspecified, "---\nDATA\n"},
	} {
		out, errOut := runReadingAFile(t, src, "DATA\n", tc.answer)
		if out != tc.want {
			t.Errorf("%v: stdout = %q, want %q; stderr %q", tc.answer, out, tc.want, errOut)
		}
		if errOut != "" {
			t.Errorf("%v: stderr = %q, want nothing said", tc.answer, errOut)
		}
	}
}

// The loop the issue is about, which is where the empty input stops being a
// nicety: the job and the loop read the same descriptor, so a line the job
// consumes is a line the loop never sees.
//
// Three lines rather than one, because a single line cannot tell a loop that
// lost its input from a loop that ran once.
func TestABackgroundJobDoesNotEatTheLoopsInput(t *testing.T) {
	const src = `while read -r line; do /bin/cat & wait "$!"; echo "got $line"; done`
	out, errOut := runReadingAFile(t, src, "a\nb\nc\n", BackgroundJobInputEmpty)
	if want := "got a\ngot b\ngot c\n"; out != want {
		t.Errorf("stdout = %q, want %q; stderr %q", out, want, errOut)
	}
}

// And a shell with someone to tell substitutes nothing, whatever the axis
// says. POSIX XCU 2.9.3 conditions the empty input on job control being
// disabled, and the panel is measured to agree: on a pty,
// `bash -i -c '/bin/cat & sleep 0.3; jobs'` lists the job `Stopped` and zsh
// lists it `suspended (tty input)` — both handed it the terminal and let the
// kernel stop it with SIGTTIN, which an empty input can never produce.
//
// Asked under the answer that would otherwise replace the stream, so the
// assertion can only pass because job control turned it off.
func TestJobControlLeavesABackgroundJobsInputAlone(t *testing.T) {
	const src = `/bin/cat & wait "$!"; echo "---"; /bin/cat`
	out, errOut := runReadingAFileWith(t, src, "DATA\n", BackgroundJobInputEmpty,
		func(r *Runner) { r.JobControl = true })
	if want := "DATA\n---\n"; out != want {
		t.Errorf("stdout = %q, want %q; stderr %q", out, want, errOut)
	}
}

// runReadingAFile runs src with standard input on a file holding text, under
// one answer for a background job's own input.
//
// A real file and not a buffer, because the whole question is which of two
// readers gets the bytes of one descriptor, and an io.Reader the shell copies
// from is not that.
func runReadingAFile(t *testing.T, src, text string, answer BackgroundJobInputPolicy) (out, errOut string) {
	t.Helper()
	return runReadingAFileWith(t, src, text, answer, nil)
}

// runReadingAFileWith is runReadingAFile with a hand on the Runner, for the
// one question that is about the shell rather than about the axis.
func runReadingAFileWith(t *testing.T, src, text string, answer BackgroundJobInputPolicy, setup func(*Runner)) (out, errOut string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := testSemantics()
	sem.BackgroundJobInput = answer
	r := newTestRunner(t, &Runner{Stdin: in, Stdout: &o, Stderr: &e, Semantics: &sem})
	if setup != nil {
		setup(r)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	return o.String(), e.String()
}
