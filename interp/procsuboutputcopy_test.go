// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A writing substitution's body holds the output it was started with, as a
// process would, and writes into it as it goes.
//
// Every shell that has the construct forks the body, so its standard output is
// a descriptor of its own. A body started after `> >(a)` was applied therefore
// writes *through* `a`, and `a` reads until that body is done. Measured
// 2026-10-15, bash 5.3.20, 20 runs of 20 each:
//
//	{ echo hi; echo B >&4; } > >(sed s/^/A/) 4> >(sed s/^/b/)     Ahi AbB
//	{ echo A >&3; echo B >&4; } 3> >(sed s/^/x/) 4> >(sed s/^/y/)  yB xA
//
// Here the body shared the shell's `*os.File`, which the shell closed when the
// command's redirections were undone, and the bash dialect's body — whose
// bytes are held for the join — was delivered into that closed file: `Ahi`
// alone, with nothing said (#6526). The second row was `xA yB`, the bodies'
// held output delivered in list order rather than in the order they wrote.
//
// So the shell's output is a pipe here, not a buffer: only a descriptor can be
// copied, and an embedder's io.Writer keeps the capture and its join, which
// TestWhereAnUnwaitedForBodyIsJoined covers. The bodies are builtins so the
// rows need nothing on PATH, and they write only once their input has ended,
// which is what sed does into a pipe and what makes the second row an order
// rather than a race.
func TestAWritingSubstitutionsBodyHoldsTheOutputItWasStartedWith(t *testing.T) {
	gather := func(p string) string {
		return `>(v=; while read -r l; do v=$v$l; done; echo "` + p + `$v")`
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"a second body writes through the first",
			`{ echo hi; echo B >&4; } > >(while read -r l; do echo "A$l"; done) 4> ` + gather("b"),
			"Ahi\nAbB\n",
		},
		{
			"the first body reads until the second is done",
			`{ echo A >&3; echo B >&4; } 3> ` + gather("x") + ` 4> ` + gather("y"),
			"yB\nxA\n",
		},
	} {
		for _, a := range []struct {
			name   string
			answer Answer
		}{
			{"waiting at the command", Yes},
			{"not waiting", No},
		} {
			t.Run(tc.name+", "+a.name, func(t *testing.T) {
				f, err := syntax.Parse(tc.src, syntax.Core())
				if err != nil {
					t.Fatal(err)
				}
				pr, pw, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer pr.Close()
				read := make(chan string, 1)
				go func() {
					b, _ := io.ReadAll(pr)
					read <- string(b)
				}()
				var errs strings.Builder
				sem := testSemantics()
				sem.WritingSubstitutionIsWaitedForAtTheCommand = a.answer
				r := newTestRunner(t, &Runner{Stdout: pw, Stderr: &errs, Semantics: &sem, Env: testPATH()})
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, _ = r.Run(context.Background(), f)
				}()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Fatal("the shell did not return")
				}
				// The test's own end, so that end-of-file is the bodies'
				// copies going and nothing else.
				_ = pw.Close()
				select {
				case got := <-read:
					if got != tc.want {
						t.Errorf("got %q (stderr %q), want %q", got, errs.String(), tc.want)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("a body still held the shell's output after the shell returned")
				}
			})
		}
	}
}
