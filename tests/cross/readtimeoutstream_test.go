// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// What an expired `read -t` leaves in the stream, per dialect (#7021).
// Measured 2026-10-29 against bash 5.3.20, ksh93u+ 2012-08-01, zsh 5.9.2 and
// BusyBox ash 1.37.0 in the pinned alpine image, each fed through a pipe:
//
//	script: v=old; read -t 0.01 -r v; echo "st=$? [$v]"; read -r w; echo "next st=$? [$w]"
//
//	`a` waiting before the read, `bc` after it has returned:
//	  bash  st=142 [a]   next st=0 [bc]
//	  ksh   st=1 [old]   next st=0 [bc]
//	  ash   st=1 [old]   next st=0 [bc]
//	  zsh   st=0 [abc]   next st=1 []
//
//	nothing until the read has returned, then `xy`:
//	  bash  st=142 []    next st=0 [xy]
//	  ksh, ash, zsh      st=1 [old]   next st=0 [xy]
//
// **No shell leaves a byte that had arrived in the stream, and none takes one
// that arrives after the deadline.** What the first read does with a byte that
// was there is the two axes already recorded: bash keeps it, ksh93 and ash
// read it and drop it, zsh reads on to the end of the line. What arrives late
// is the next reader's in all four. This shell parked a read on the pipe when
// the deadline passed, and that read took the next byte to arrive and threw it
// away: `next [c]` and `next [y]` in every dialect.
//
// **And a deadline over the whole read stops it while bytes are still
// coming.** 262144 bytes with no newline, no pause, under `-t 0.01`:
//
//	bash  st=142 5574 of them     ksh, ash  st=1, v untouched     zsh  st=1, all of them
//
// That row is here because the fix for the first two waits for a byte to be
// there before reading it, and a wait that looked again after the deadline
// would have read the whole stream.

// firstLineWriter collects what a run writes and closes line once the first
// line of it is complete.
type firstLineWriter struct {
	mu   sync.Mutex
	b    strings.Builder
	once sync.Once
	line chan struct{}
}

func (w *firstLineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.b.Write(p)
	if strings.Contains(w.b.String(), "\n") {
		w.once.Do(func() { close(w.line) })
	}
	return n, err
}

func (w *firstLineWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// runOverAPipe runs src under the preset with a pipe for standard input that
// holds first before the run starts. feed is handed the write end and the
// writer, and decides what else arrives and when; it must close the write end
// when it is done.
func runOverAPipe(t *testing.T, p dialecttest.Preset, src, first string,
	feed func(w *os.File, out *firstLineWriter),
) string {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pw.WriteString(first); err != nil {
		t.Fatal(err)
	}
	out := &firstLineWriter{line: make(chan struct{})}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		feed(pw, out)
	}()
	r := p.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: out, Stderr: out})
	r.Stdin = pr
	f := p.ParseThrough(t, r, src)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// Closing the read end is what lets a feed still writing into a full pipe
	// return, once the shell has stopped reading it.
	_ = pr.Close()
	wg.Wait()
	return out.String()
}

// afterTheFirstLine writes rest once the shell has said the timed read is
// over, which no deadline can be inside of. Ten seconds is a backstop so a
// shell that never says it fails rather than hangs.
func afterTheFirstLine(rest string) func(*os.File, *firstLineWriter) {
	return func(w *os.File, out *firstLineWriter) {
		select {
		case <-out.line:
		case <-time.After(10 * time.Second):
		}
		_, _ = w.WriteString(rest)
		_ = w.Close()
	}
}

const readTwice = `v=old; read -t 0.01 -r v; echo "st=$? [$v]"; read -r w; echo "next st=$? [$w]"`

func TestAnExpiredReadLeavesTheStreamAsTheShellsDo(t *testing.T) {
	for _, row := range []struct {
		dialect      string
		arrived, not string
	}{
		{"bash", "st=142 [a]\nnext st=0 [bc]\n", "st=142 []\nnext st=0 [xy]\n"},
		{"ksh", "st=1 [old]\nnext st=0 [bc]\n", "st=1 [old]\nnext st=0 [xy]\n"},
		{"ash", "st=1 [old]\nnext st=0 [bc]\n", "st=1 [old]\nnext st=0 [xy]\n"},
		{"zsh", "st=0 [abc]\nnext st=1 []\n", "st=1 [old]\nnext st=0 [xy]\n"},
	} {
		t.Run(row.dialect, func(t *testing.T) {
			p := presets[row.dialect]
			feed := afterTheFirstLine("bc\n")
			if row.dialect == "zsh" {
				// zsh's deadline bounds only the wait for the first byte, so
				// with `a` already there it reads on and says nothing until
				// the line is whole. When the rest comes cannot change that
				// answer, so it comes fifty milliseconds in.
				feed = func(w *os.File, _ *firstLineWriter) {
					time.Sleep(50 * time.Millisecond)
					_, _ = w.WriteString("bc\n")
					_ = w.Close()
				}
			}
			if got := runOverAPipe(t, p, readTwice, "a", feed); got != row.arrived {
				t.Errorf("a byte waiting before the read: got %q, want %q", got, row.arrived)
			}
			if got := runOverAPipe(t, p, readTwice, "", afterTheFirstLine("xy\n")); got != row.not {
				t.Errorf("a byte arriving after the deadline: got %q, want %q", got, row.not)
			}
		})
	}
}

func TestAWholeReadDeadlineStopsAStreamThatNeverPauses(t *testing.T) {
	const size = 262144
	stream := func(w *os.File, _ *firstLineWriter) {
		_, _ = w.WriteString(strings.Repeat("a", size))
		_ = w.Close()
	}
	const src = `v=old; read -t 0.01 -r v; echo "st=$? ${#v}"`
	for _, row := range []struct {
		dialect, want string
	}{
		{"ksh", "st=1 3\n"},
		{"ash", "st=1 3\n"},
		{"zsh", "st=1 " + strconv.Itoa(size) + "\n"},
	} {
		if got := runOverAPipe(t, presets[row.dialect], src, "", stream); got != row.want {
			t.Errorf("%s: got %q, want %q", row.dialect, got, row.want)
		}
	}
	// bash keeps what it had when the deadline passed, which is some of the
	// stream and not all of it — how much is the machine's speed. Reading
	// all of it inside ten milliseconds would take a system call every
	// nineteen nanoseconds.
	got := runOverAPipe(t, presets["bash"], src, "", stream)
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(got, "st=142 "), "\n"))
	if err != nil || !strings.HasPrefix(got, "st=142 ") || n < 1 || n >= size {
		t.Errorf("bash: got %q, want 142 with part of the %d bytes", got, size)
	}
}
