// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// withFileSubst turns on the one grammar flag these tests need, so that they
// name the construct rather than a shell that happens to have it.
func withFileSubst(d *syntax.Dialect) { d.ProcessSubstitutionToFile = true }

// `=(cmd)` is the third spelling of process substitution and the only one
// whose path leads to a *file*: the command runs to completion, its output
// lands in that file, and the word becomes the path.
//
// Every row is measured against zsh 5.9.2, 2026-09-11.
func TestFileProcessSubstitutionRunsACommandIntoAFile(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the output is the file", `cat =(echo hi)`, "hi\n"},
		{"two in one command", `cat =(echo one) =(echo two)`, "one\ntwo\n"},
		{"nested", `cat =(cat =(echo deep))`, "deep\n"},
		// The discriminating probe against a fifo, and the one a corpus case
		// can carry: the path is random, so what is compared is what the
		// file *is*.
		{"a regular file and not a pipe", `[ -f =(echo hi) ] && echo regular`, "regular\n"},
		{"not a fifo", `[ -p =(echo hi) ] || echo notafifo`, "notafifo\n"},
		// `=()` is a file of zero bytes rather than an error, and so is
		// `=(:)`.
		{"an empty body", `[ -f =() ] && [ ! -s =() ] && echo empty`, "empty\n"},
		{"a body that does nothing", `[ ! -s =(:) ] && echo empty`, "empty\n"},
		// The reason the construct exists where a pipe will not do.
		{"compared with another", `diff =(echo a) =(echo a) && echo same`, "same\n"},
		// The status is the command's own and never the body's: measured,
		// `cat =(false)` and `cat =(exit 7)` are both 0 there.
		{"the body's status is discarded", `cat =(false); echo "st=$?"`, "st=0\n"},
		{"a body that exits nonzero", `cat =(exit 7); echo "st=$?"`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, withFileSubst, nil)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The file is finished before the path is handed over, so nothing waits for
// anybody and there is no buffer to fill.
//
// This is the property that separates the two spellings. `<(cmd)` is a pipe,
// so a body writing more than the pipe holds blocks until the reader drains
// it — which is fine for a reader, and fatal for anything that wants to seek.
// Measured on zsh 5.9.2: `wc -c < =(head -c 200000 /dev/zero)` answers
// 200000, well past any pipe buffer. A shell that ran the body on a goroutine
// and handed over a pipe would hang here rather than answer wrong, which is
// why the size is far over the buffer rather than just over it.
//
// **Counted to end of input rather than from one read.** `wc -c` reads until
// the stream ends, so a short read cannot pass by timing luck — which is how
// a real short read survived three sightings as "a macOS flake": the test
// that should have caught it asserted on a single read, and passed whenever
// the writer happened to finish first (#1907). Nothing here is on that road
// — the body is finished and the descriptor closed before the path is handed
// over, so there is no end-of-stream decision to get wrong — and the
// assertion is written this way so that it would still catch one.
func TestFileProcessSubstitutionOutrunsAPipeBuffer(t *testing.T) {
	out, st := runGrammar(t, `wc -c < =(yes hello | head -c 200000)`, withFileSubst, nil)
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	if got := strings.TrimSpace(out); got != "200000" {
		t.Errorf("bytes = %q, want %q", got, "200000")
	}
}

// The file lives exactly as long as the command that named it.
//
// Measured: `f==(echo hi); cat $f` is `No such file or directory` in zsh
// 5.9.2 — the same lifetime the pipes have, which is why the file is recorded
// in the same list and removed by the same removeProcSubs rather than getting
// a mechanism of its own.
func TestFileProcessSubstitutionIsRemovedWithItsCommand(t *testing.T) {
	out, st := runGrammar(t, `f==(echo hi); [ -e "$f" ] && echo there || echo gone`, withFileSubst, nil)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if out != "gone\n" {
		t.Errorf("out = %q, want %q", out, "gone\n")
	}
}

// What the file is, seen from outside the shell: a regular file, mode 0600,
// under the directory this shell made for its substitutions — and gone when
// the command that named it has finished.
//
// Through the event stream because that is the only moment the path is
// knowable from out here: the word expands to a name no script can predict,
// and by the time the command has run the file is gone. The open is recorded
// as an EventAccess for exactly the reason ownPipe gives, so asserting on it
// pins the recording as well as the mode.
func TestFileProcessSubstitutionMakesAPrivateRegularFile(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var modes []os.FileMode
	d := syntax.Core()
	withFileSubst(&d)
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Dialect: &d, Env: testPATH(),
		Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
		Events: SinkFunc(func(_ context.Context, e Event) {
			if e.Kind != EventAccess || e.Action.Kind != ActionOpen {
				return
			}
			// Taken here rather than after the run: the file exists only
			// between this event and the end of the command.
			fi, err := os.Lstat(e.Action.Path)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			paths = append(paths, e.Action.Path)
			modes = append(modes, fi.Mode())
		}),
	})
	f, err := syntax.Parse(`cat =(echo hi)`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 {
		t.Fatalf("recorded %d opens, want the substitution's one: %v", len(paths), paths)
	}
	// A regular file and nothing else. `mode.Type() == 0` is the whole of
	// that claim: a fifo, a symlink and a directory each set a bit here.
	if got := modes[0].Type(); got != 0 {
		t.Errorf("type bits = %v, want a regular file", got)
	}
	// 0600, measured: `ls -l =(echo hi)` is `-rw-------` in zsh 5.9.2, which
	// is also why naming one as a command is `permission denied` there.
	if got := modes[0].Perm(); got != 0o600 {
		t.Errorf("permissions = %#o, want %#o", got, 0o600)
	}
	if _, err := os.Lstat(paths[0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after the command, stat %s = %v, want it gone", paths[0], err)
	}
}

// A body that fails still produces a file, and the word still expands.
//
// Measured: `echo =(nosuchcmd-xyz)` prints the diagnostic, prints a path, and
// exits 0 in zsh 5.9.2. So the diagnostic is the body's and the status is the
// outer command's, and neither reaches the other.
func TestFileProcessSubstitutionKeepsTheWordWhenTheBodyFails(t *testing.T) {
	out, st := runGrammar(t, `[ -f =(nosuchcmd-xyz-1878) ] && echo regular`, withFileSubst, nil)
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if !strings.HasSuffix(out, "regular\n") {
		t.Errorf("out = %q, want it to end in %q", out, "regular\n")
	}
	if !strings.Contains(out, "nosuchcmd-xyz-1878") {
		t.Errorf("out = %q, want the body's own diagnostic in it", out)
	}
}
