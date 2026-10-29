// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// A trapped signal that lands while a `read -t` still waits for its first
// byte ends the read as its deadline would: `T`, the timeout's status, the
// name untouched, and the line written afterwards the next read's (#7032). A
// `read` without `-t` reads on through the same signal, and so does a timed
// one with a byte already in. See
// interp.Semantics.ReadTimeoutWaitEndedByATrappedSignal.
//
// Every row is the script below run from -c and from a script file on zsh 5.9.2
// on 2026-10-29, with its output copied; the two routes agree on each. The
// job sends the signal a second in and writes `late` and `end` a second after
// that, so a read the signal did not end reads `late` long before its ten
// second deadline, and a shell that got that wrong prints `l=[late]` rather
// than hanging. The second is a margin for a loaded runner under the race
// detector: the read must already be waiting when the signal lands, or the
// signal is one handled before it began, which zsh reads on over.
//
// The rows run in parallel with each other and with nothing else in the
// package. Each signal is aimed at its own shell, which takes it without the
// kernel, but a test elsewhere that raises a real one would reach every
// Runner here with that signal trapped. An ignored signal is the one such
// route this test met (#7040), and is why no row here ignores one.
func TestATrappedSignalEndsATimedReadStillWaitingForItsFirstByte(t *testing.T) {
	const (
		ended  = "T\nst=1 l=[old]\nnext [late]\n"
		readOn = "T\nst=0 l=[late]\nnext [end]\n"
		inLine = "T\nst=0 l=[ablate]\nnext [end]\n"
		status = `echo "st=$?"`
		named  = `echo "st=$? l=[$l]"`
		fromFd = `read -r m <&3`
	)
	// late is the job: `sig` a second in, the line a second after.
	late := func(open, trap, sig, read, report, next string) string {
		return `mkfifo p; ` + open + `; l=old; ` + trap + `; ` +
			`(sleep 1; kill -` + sig + ` $$; sleep 1; printf 'late\nend\n' >p) & ` +
			read + `; ` + report + `; ` + next + `; echo "next [$m]"; wait`
	}
	const (
		readingOnly = `exec 4<>p 3<p`
		bothWays    = `exec 3<>p`
		trapINT     = `trap "echo T" INT`
	)
	rows := []struct{ name, src, want string }{
		{"nothing in", late(readingOnly, trapINT, "INT", `read -r -t 10 l <&3`, named, fromFd), ended},
		{"-u", late(readingOnly, trapINT, "INT", `read -r -t 10 -u 3 l`, named, fromFd), ended},
		{"both ways", late(bothWays, trapINT, "INT", `read -r -t 10 l <&3`, named, fromFd), ended},
		{"a fraction", late(readingOnly, trapINT, "INT", `read -r -t 9.5 l <&3`, named, fromFd), ended},
		{"-d", late(readingOnly, trapINT, "INT", `read -r -t 10 -d e l <&3`, named, fromFd), ended},
		// `-k` and `-q` print the status alone: what an expired one leaves in
		// the name is another matter (#7038). A read the signal did not end
		// takes `l` and leaves `ate`.
		{"-k", late(readingOnly, trapINT, "INT", `read -t 10 -k -u 3 l`, status, fromFd), "T\nst=1\nnext [late]\n"},
		{"-k 3", late(readingOnly, trapINT, "INT", `read -t 10 -k 3 -u 3 l`, status, fromFd), "T\nst=1\nnext [late]\n"},
		{"-q", late(readingOnly, trapINT, "INT", `read -t 10 -q -u 3 l`, status, fromFd), "T\nst=2\nnext [late]\n"},
		{"USR1", late(readingOnly, `trap "echo T" USR1`, "USR1", `read -r -t 10 l <&3`, named, fromFd), ended},
		{
			"a trap function",
			late(readingOnly, `TRAPUSR1() { echo T; }`, "USR1", `read -r -t 10 l <&3`, named, fromFd), ended,
		},
		{
			"standard input",
			`mkfifo p; exec 4<>p; l=old; trap "echo T" INT; ` +
				`(sleep 1; kill -INT $$; sleep 1; printf 'late\nend\n' >p) & ` +
				`{ read -r -t 10 l; echo "st=$? l=[$l]"; read -r m; echo "next [$m]"; } <p; wait`,
			ended,
		},
		{
			"a pipe",
			`l=old; trap "echo T" INT; (sleep 1; kill -INT $$) & { sleep 2; printf 'late\nend\n'; } | ` +
				`{ read -r -t 10 l; echo "st=$? l=[$l]"; read -r m; echo "next [$m]"; }; wait`,
			ended,
		},
		{
			"a coprocess",
			`mkfifo p; l=old; coproc { read x <p; printf 'late\nend\n'; }; trap "echo T" INT; ` +
				`(sleep 1; kill -INT $$; sleep 1; echo go >p) & ` +
				`read -r -t 10 -p l; echo "st=$? l=[$l]"; read -r -p m; echo "next [$m]"; wait`,
			ended,
		},
		// A second signal after the first has ended the read is the next
		// read's, and that one reads on.
		{
			"a second signal",
			`mkfifo p; exec 4<>p 3<p; l=old; trap "echo T" INT; trap "echo U" USR1; ` +
				`(sleep 1; kill -INT $$; sleep 0.5; kill -USR1 $$; sleep 1; printf 'late\nend\n' >p) & ` +
				`read -r -t 10 l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`,
			"T\nst=1 l=[old]\nU\nnext [late]\n",
		},
		// What does not end it. A read without `-t`; one with a byte in before
		// the signal, written before the read or during it, which reads on to
		// the end of its line with no deadline; and a signal handled before
		// the read began.
		{"untimed", late(readingOnly, trapINT, "INT", `read -r l <&3`, named, fromFd), readOn},
		{"a byte first", late(readingOnly+`; printf ab >p`, trapINT, "INT", `read -r -t 10 l <&3`, named, fromFd), inLine},
		{
			"a byte during",
			`mkfifo p; exec 4<>p 3<p; l=old; trap "echo T" INT; ` +
				`(sleep 0.5; printf ab >p; sleep 1; kill -INT $$; sleep 1; printf 'late\nend\n' >p) & ` +
				`read -r -t 10 l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`,
			inLine,
		},
		{
			"handled before it began",
			`mkfifo p; exec 4<>p 3<p; l=old; trap "echo T" INT; (sleep 1; printf 'late\nend\n' >p) & ` +
				`read -r -t 10 l <&3 2>"$(kill -INT $$)/dev/null"; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`,
			readOn,
		},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			if got := runReleasingAfter(t, r.src, nil); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}

// TestATrappedSignalEndsATimedReadOfAnInheritedPipe is the row above with the
// stream the shell's own standard input, a pipe its caller made — measured on
// zsh 5.9.2 as `{ sleep 2; printf 'late\nend\n'; } | zsh -c '…'`. Not
// parallel, for the reason above.
func TestATrappedSignalEndsATimedReadOfAnInheritedPipe(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(2 * time.Second)
		_, _ = pw.WriteString("late\nend\n")
		_ = pw.Close()
	}()
	got := runReleasingAfter(t, `l=old; trap "echo T" INT; (sleep 1; kill -INT $$) & `+
		`read -r -t 10 l; echo "st=$? l=[$l]"; read -r m; echo "next [$m]"; wait`, pr)
	_ = pr.Close()
	wg.Wait()
	if want := "T\nst=1 l=[old]\nnext [late]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// guardedOutput is a run's standard output and error, written from the jobs
// it starts as well as from the shell.
type guardedOutput struct {
	mu sync.Mutex
	b  strings.Builder
}

func (o *guardedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *guardedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}

// runReleasingAfter runs src in a directory of its own, with stdin for its
// standard input where it is not nil, and returns what it wrote. A run still
// going after twenty seconds is waiting on its fifo for a line a correct shell
// would already have read: the fifo is opened and handed one, and held open so
// a later `>p` finds a reader, which lets the run end, and the test fails with
// what it wrote rather than hanging the package.
func runReleasingAfter(t *testing.T, src string, stdin *os.File) string {
	t.Helper()
	dir := t.TempDir()
	var out guardedOutput
	r := preset.Runner(dialecttest.Base{
		Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Stdout: &out, Stderr: &out,
	})
	if stdin != nil {
		r.Stdin = stdin
	}
	f := preset.ParseThrough(t, r, src)
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), f)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err %v: %s", err, out.String())
		}
		return out.String()
	case <-time.After(20 * time.Second):
	}
	var held []*os.File
	for _, name := range []string{"p", "q"} {
		if w, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR, 0); err == nil {
			_, _ = w.WriteString("x\n")
			held = append(held, w)
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	for _, w := range held {
		_ = w.Close()
	}
	t.Fatalf("still running after twenty seconds, waiting on a read. Output: %q", out.String())
	return ""
}
