// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/pty"
)

// The claim #720 was waiting for, asked of a real child process.
//
// interp hands a child `r.Stdout` directly: an *os.File is inherited as a
// descriptor and anything else makes os/exec build a pipe, so a captured child
// used to be on a pipe and everything downstream of `isatty` changed. The
// conduit's whole purpose is that the child is on a terminal anyway, and the
// only witness that can say so is a child asking the kernel.
//
// Two pseudo-terminals are involved and only one is this test's. The outer one
// is the session's — what a person would be looking at, and what this reads
// from to see what they saw. The inner one is the conduit's, made by the shell,
// and the test never touches it: if it did, it would be asserting on its own
// fixture rather than on what the shell built.
func TestACapturedChildIsStillOnATerminal(t *testing.T) {
	session := atACapturingPrompt(t)
	defer session.finish()

	// The mark must not be text the line contains. A terminal echoes what is
	// typed, so a wait on a word in the command is answered by the echo — and
	// the test then passes for a shell that drew back keystrokes and ran
	// nothing. This is the invariant internal/smoke states and the first
	// version of this test broke: it typed the words CHILD_ON_TTY and
	// CHILD_ON_PIPE and found both on the screen before the child ran.
	//
	// So the line is written as an expression and the answer is its value.
	session.typeLine(`/bin/sh -c 'test -t 1 && echo on-tty-$((6*7)) || echo on-pipe-$((6*7))'`)
	session.waitForOutput("on-tty-42", "the child reporting a terminal on its standard output")
	if strings.Contains(session.screen(), "on-pipe-42") {
		t.Error("the child was on a pipe, which is the whole thing the conduit exists to prevent")
	}
}

// And what it printed is still in the block, which is the other half: a conduit
// that gave the child its terminal back and recorded nothing would pass the
// test above.
func TestACapturedChildsOutputIsInTheBlock(t *testing.T) {
	session := atACapturingPrompt(t)

	session.typeLine(`/bin/sh -c 'echo one-$((6*7))'`)
	session.waitForOutput("one-42", "the first command's output on the terminal")
	session.typeLine(`/bin/sh -c 'echo two-$((6*7))'`)
	session.waitForOutput("two-42", "the second command's output on the terminal")
	session.finish()

	bodies := session.bodies(t)
	if len(bodies) != 2 {
		t.Fatalf("%d bodies, want 2: %q", len(bodies), bodies)
	}
	// Each block holds its own command's output and not the one before it,
	// which is what the drain buys. Without it the tail of the first arrives
	// while the second block is open and both records are wrong.
	if got, want := bodies[0], "one-42\n"; got != want {
		t.Errorf("the first block's body is %q, want %q", got, want)
	}
	if got, want := bodies[1], "two-42\n"; got != want {
		t.Errorf("the second block's body is %q, want %q", got, want)
	}
}

// The mark the drain waits on never reaches the terminal, and neither does a
// second copy of the output.
//
// The mark is written into the conduit and stripped by the pump. A pump that
// forwarded it would put a NUL and twenty-four hex digits on the screen after
// every command; one that dropped bytes looking for it would eat output. Both
// are silent failures on a screen nobody is reading during a test, so the
// screen is read.
func TestTheDrainMarkDoesNotReachTheTerminal(t *testing.T) {
	session := atACapturingPrompt(t)
	defer session.finish()

	// An expression again, so the count below is of the output alone: the echo
	// of this line contains `$((6*7))` and not `seen-42`.
	session.typeLine(`/bin/sh -c 'echo seen-$((6*7))'`)
	session.waitForOutput("seen-42", "the command's output")
	screen := session.screen()
	if n := strings.Count(screen, "seen-42"); n != 1 {
		// Once. Two would be the pump writing the same bytes twice, which is
		// what a carry that forwarded and then re-forwarded would do; none is
		// caught by the wait above.
		t.Errorf("the output appears %d times, want once: %q", n, screen)
	}
	if strings.ContainsRune(screen, 0) {
		t.Errorf("a NUL reached the terminal, so the drain mark was not stripped: %q", screen)
	}
}

// A command that prints a great deal is not lost or reordered by the conduit.
//
// The pump reads in 32 KiB bites and the mark can land across any of those
// boundaries, so a carry that was wrong would fail here and nowhere else. The
// body is the witness rather than the screen, because a terminal wraps.
func TestALotOfOutputSurvivesTheConduit(t *testing.T) {
	session := atACapturingPrompt(t)

	const lines = 4000
	// The tail is an expression and the wait is on its *value*, which is the
	// invariant the first test in this file states and this one had broken: a
	// terminal echoes what is typed, so a wait on a word the command line
	// contains is answered by the echo. It was, every time: the wait returned
	// while the line was still being drawn, which left even finish's wait for
	// the next prompt to be answered by one of the editor's own redraws — it
	// draws the prompt again on every keystroke — and the ^D then went in
	// before the command had started, which is the window finish exists to
	// stay out of. Measured on Linux under `-race` at one core: 15 runs in 17
	// failed on this, and none of the other five session tests in this file
	// failed at all.
	session.typeLine(`/bin/sh -c 'i=0; while [ $i -lt 4000 ]; do echo "line-$i"; i=$((i+1)); done; echo tail-$((6*7))'`)
	session.waitForOutput("tail-42", "the last line of a long run")
	session.finish()

	bodies := session.bodies(t)
	if len(bodies) != 1 {
		t.Fatalf("%d bodies, want 1", len(bodies))
	}
	got := strings.Split(strings.TrimSuffix(bodies[0], "\n"), "\n")
	if len(got) != lines+1 {
		t.Fatalf("%d lines in the body, want %d", len(got), lines+1)
	}
	for i := range lines {
		// Every line, in order. Checking only the ends would pass for a
		// conduit that dropped the middle, which is exactly what a bad carry
		// does.
		if want := "line-" + strconv.Itoa(i); got[i] != want {
			t.Fatalf("line %d is %q, want %q", i, got[i], want)
		}
	}
}

// capturingSession is a shell at a prompt on a real terminal, with the store
// on and output capture at its default.
type capturingSession struct {
	t        *testing.T
	control  *os.File
	tty      *os.File
	out      *syncBuffer
	dir      string
	finished bool
	done     chan error
}

// atACapturingPrompt starts one.
//
// The Runner's streams are the terminal itself, which is the configuration the
// conduit exists for and is what the other pty tests in this package
// deliberately do not have: they hand the Runner a buffer, so no conduit is
// built and the capture path is never entered.
func atACapturingPrompt(t *testing.T) *capturingSession {
	t.Helper()
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(tty, 40, 200); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	vars := map[string]string{
		"PS1":      "$ ",
		"HISTFILE": filepath.Join(dir, "hist"),
		// Named outright rather than left to HOME, because the suite's home
		// is a tripwire that must stay empty.
		"SH_BLOCKS_DIR": filepath.Join(dir, "blocks"),
	}
	r := newTestRunner(vars)
	// The terminal, not a buffer. os/exec inherits an *os.File as a
	// descriptor, so this is what makes the child's fd 1 a real thing.
	r.Stdout, r.Stderr = tty, tty

	out := &syncBuffer{}
	s := Shell{Runner: r, In: tty, Out: tty, Err: tty, Name: "sh"}
	sess := &capturingSession{t: t, control: control, tty: tty, out: out, dir: dir, done: make(chan error, 1)}

	// Everything the terminal shows, collected as it arrives: the pump writes
	// to the terminal from a goroutine of its own, so a reader that only ran
	// at the end would race the shell's exit.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := control.Read(buf)
			if n > 0 {
				_, _ = out.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		_, err := s.Run(t.Context())
		sess.done <- err
	}()
	sess.waitForOutput("$ ", "the first prompt")
	return sess
}

func (s *capturingSession) typeLine(line string) {
	s.t.Helper()
	if _, err := s.control.WriteString(line + "\r"); err != nil {
		s.t.Fatal(err)
	}
}

func (s *capturingSession) waitForOutput(want, what string) {
	s.t.Helper()
	waitFor(s.t, s.out, want, what)
}

func (s *capturingSession) screen() string { return s.out.String() }

// finish ends the session with a ^D, once the shell is reading again.
//
// The wait for the next prompt is the whole of this method and it is a
// synchronization rather than tidiness. A shell puts the terminal back in its
// own line discipline to run a command and takes it into raw mode again
// afterwards — see Shell.run — and the kernel drops what is queued but unread
// when the discipline changes. Every test here waits for its command's
// *output*, which arrives while that window is still open, so a ^D typed
// there is destroyed and the editor goes on waiting for a byte that will
// never come — the deadline below then reports the silence instead of the
// cause.
//
// Probed on Linux with a bare pseudo-terminal and no shell in it: a ^D that
// arrives while the terminal is raw is read as 0x04, and one that arrives in
// canonical mode and is read after the switch to raw comes back as **0x00** —
// the discipline keeps VEOF as a placeholder byte plus an end-of-line flag,
// and the mode change clears the flag. The editor has no meaning for that
// byte, so it keeps reading.
//
// The same probe types a whole line in that window and gets it back byte for
// byte, which is why this wait belongs where the ^D is typed and typeLine
// does not need one.
//
// That is #635, and #685 had already answered it by making the driver's
// helper do this wait. This is a second session helper, written without it,
// so it lost the answer along with the code.
//
// The prompt is the mark to wait on because it is drawn after raw mode is
// restored and immediately before the read, which is an order the two cannot
// swap. A *new* prompt: the buffer carries a cursor, and it has to, because
// the editor redraws the prompt on every keystroke and a search over
// everything drawn would be answered by one of those forever.
func (s *capturingSession) finish() {
	s.t.Helper()
	if s.finished {
		return
	}
	s.finished = true
	s.waitForOutput("$ ", "the prompt that says the shell is reading again")
	if _, err := s.control.WriteString("\x04"); err != nil {
		s.t.Fatal(err)
	}
	select {
	case err := <-s.done:
		if err != nil {
			s.t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		s.t.Fatal("the shell did not exit on ^D")
	}
	_ = s.tty.Close()
	_ = s.control.Close()
}

// bodies is what the store kept, one string per block that has a body, in the
// order the index records them.
func (s *capturingSession) bodies(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(s.dir, "blocks", "index.jsonl"))
	if err != nil {
		t.Fatalf("no block index: %v", err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		// `output` is the body's path, a plain string. The field name and
		// the shape are the format #495 froze, so a body is found the way any
		// consumer finds one.
		var rec struct {
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("index line %q: %v", line, err)
		}
		if rec.Output == "" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.dir, "blocks", rec.Output))
		if err != nil {
			t.Fatalf("body %q: %v", rec.Output, err)
		}
		out = append(out, string(body))
	}
	return out
}

// Which of the two ways a session gets, decided from what it was told and what
// it has.
//
// The table is the whole of #720's answer. The default keeps output where a
// pseudo-terminal can carry it and keeps *none* where one cannot — because the
// alternative there is the wrapper, and the wrapper is what costs a child its
// terminal. A session that wants that anyway still asks for it by name.
func TestWhichCaptureASessionGets(t *testing.T) {
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = tty.Close() }()

	for _, tc := range []struct {
		name         string
		output       string
		outputSet    bool
		streams      string // "terminal", "buffer", or "split"
		wantCapture  bool
		wantConduit  bool
		wantChildFd1 string
	}{
		{
			name: "the default at a terminal is a conduit", streams: "terminal",
			wantCapture: true, wantConduit: true, wantChildFd1: "the terminal",
		},
		{
			name: "the default without a terminal keeps nothing", streams: "buffer",
			wantCapture: false, wantChildFd1: "whatever it was",
		},
		{
			name:   "asked by name without a terminal, the wrapper",
			output: "1", outputSet: true, streams: "buffer",
			wantCapture: true, wantConduit: false, wantChildFd1: "a pipe, which is what was asked for",
		},
		{
			name:   "asked by name at a terminal, still a conduit",
			output: "1", outputSet: true, streams: "terminal",
			wantCapture: true, wantConduit: true, wantChildFd1: "the terminal",
		},
		{
			name:   "empty is off even at a terminal",
			output: "", outputSet: true, streams: "terminal",
			wantCapture: false, wantChildFd1: "the terminal",
		},
		{
			// Two different destinations merged into one and written back to
			// the first would send a command's diagnostics somewhere they
			// were not addressed.
			name: "a stream that is not a terminal takes no conduit", streams: "file",
			wantCapture: false, wantChildFd1: "whatever it was",
		},
		{
			// And the harder half: two streams that are *both* terminals and
			// are not the same one. IsTerminal says yes to each, so the only
			// thing that can refuse this is asking whether they are the same
			// file — which is why that question is asked with SameFile rather
			// than by comparing the two writers, and why this case exists
			// beside the one above rather than instead of it.
			name: "two different terminals take no conduit", streams: "two-terminals",
			wantCapture: false, wantChildFd1: "whatever it was",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{
				"HISTFILE":      filepath.Join(t.TempDir(), "hist"),
				"SH_BLOCKS_DIR": filepath.Join(t.TempDir(), "blocks"),
			}
			if tc.outputSet {
				vars["SH_BLOCKS_OUTPUT"] = tc.output
			}
			r := newTestRunner(vars)
			switch tc.streams {
			case "terminal":
				r.Stdout, r.Stderr = tty, tty
			case "buffer":
				r.Stdout, r.Stderr = &syncBuffer{}, &syncBuffer{}
			case "file":
				other, err := os.CreateTemp(t.TempDir(), "err")
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = other.Close() }()
				r.Stdout, r.Stderr = tty, other
			case "two-terminals":
				otherControl, otherTty, err := pty.Open()
				if err != nil {
					t.Skipf("no second pseudo-terminal: %v", err)
				}
				defer func() { _ = otherControl.Close() }()
				defer func() { _ = otherTty.Close() }()
				r.Stdout, r.Stderr = tty, otherTty
			}
			before := r.Stdout

			s := Shell{Runner: r}
			got := s.captureOutput()
			defer got.close()

			if (got != nil) != tc.wantCapture {
				t.Fatalf("capture = %v, want %v (%s)", got != nil, tc.wantCapture, tc.wantChildFd1)
			}
			if got == nil {
				if r.Stdout != before {
					t.Error("a session keeping nothing had its stream replaced anyway")
				}
				return
			}
			if (got.conduit != nil) != tc.wantConduit {
				t.Fatalf("conduit = %v, want %v", got.conduit != nil, tc.wantConduit)
			}
			// The claim that matters is about the *type* the Runner now
			// holds, because that is what os/exec reads: an *os.File is
			// inherited and anything else becomes a pipe. Asserting the
			// conduit exists would not say the Runner was given it.
			_, isFile := r.Stdout.(*os.File)
			if isFile != tc.wantConduit {
				t.Errorf("the Runner's stdout is a file = %v, want %v — the child gets %s",
					isFile, tc.wantConduit, tc.wantChildFd1)
			}
			if tc.wantConduit && r.Stdout == before {
				t.Error("the Runner still holds the terminal itself, so nothing is captured")
			}
		})
	}
}

// A session with nowhere to write a body keeps nothing, whatever it was told.
//
// Worth its own check now that the default is on: without it a session that
// turned the store off with an empty HISTFILE would still be paying for a
// pseudo-terminal and a copy of every byte, to fill a buffer nobody empties.
func TestNoStoreMeansNoCapture(t *testing.T) {
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = tty.Close() }()

	for _, tc := range []struct{ name, histfile, output string }{
		{name: "history off turns the store off", histfile: "", output: "1"},
		{name: "and the default agrees", histfile: "", output: ""},
	} {
		r := newTestRunner(map[string]string{
			"HISTFILE": tc.histfile, "SH_BLOCKS_OUTPUT": tc.output,
		})
		r.Stdout, r.Stderr = tty, tty
		s := Shell{Runner: r}
		if got := s.captureOutput(); got != nil {
			got.close()
			t.Errorf("%s: a session with no store built a capture", tc.name)
		}
	}
}

// The drain waits for the terminal, not for the command.
//
// The first version of this suite could not tell a conduit that drained from
// one that did not: 4000 lines went through fast enough that the pump was
// always finished before anything asked. A mutation run said so — removing the
// drain left every test green — so the wait is made visible instead of hoped
// for, by putting a sink behind the pump that cannot keep up.
//
// Deterministic, and that is the point. The sink is slow by construction, so a
// conduit that does not wait *cannot* have delivered everything, and the
// assertion below is about the bytes rather than about a duration.
func TestTheDrainWaitsForTheTerminalToCatchUp(t *testing.T) {
	sink := &slowSink{}
	conduit := newTestConduit(t, sink)

	// Written before the drain and not alongside it, which is the shape the
	// real thing has: the command has *exited* by the time takeOutput asks, so
	// everything it wrote is already in the queue. Writing from a goroutine
	// instead would race the mark ahead of the payload and the drain would
	// correctly return at once — which is how the first version of this test
	// failed against working code.
	//
	// Enough to outrun a sink that pauses on every write, and far more than
	// the terminal's own buffer, so the pump is still working when the last
	// write returns.
	const chunks, size = 64, 4096
	payload := bytes.Repeat([]byte("z"), size)
	for range chunks {
		if _, err := conduit.Stream().Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	conduit.drain()

	if got, want := sink.len(), chunks*size; got != want {
		t.Errorf("the terminal has %d bytes after the drain, want %d — the drain returned early", got, want)
	}
}

// A mark split across two reads is still found.
//
// The pump reads in 32 KiB bites and the mark can land across any boundary. A
// carry that was wrong would forward the first half to the terminal and then
// never match the second, so the drain would hang and a NUL would appear on
// screen — under a boundary landing inside a 26-byte token, which is to say
// rarely and unreproducibly. A mutation run had this survive, because nothing
// made the boundary land there on purpose.
//
// It lands there on purpose here, and the synchronization is on a mark in the
// stream rather than on quiet: the sink receiving the leading byte is what
// says the pump has already consumed the mark's first half.
func TestAMarkSplitAcrossTwoReadsIsStillFound(t *testing.T) {
	sink := &syncBuffer{}
	conduit := newTestConduit(t, sink)

	const split = 5
	if _, err := conduit.Stream().Write(append([]byte("lead"), conduit.mark[:split]...)); err != nil {
		t.Fatal(err)
	}
	// The pump has read that whole write, forwarded "lead" and is holding the
	// mark's first five bytes back. Waiting on the leading byte is the proof;
	// waiting on a pause would prove nothing.
	waitFor(t, sink, "lead", "the bytes before a partial mark")

	if _, err := conduit.Stream().Write(conduit.mark[split:]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conduit.reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the mark's two halves never came together, so a drain would hang here")
	}
	if got := sink.String(); got != "lead" {
		t.Errorf("the terminal saw %q, want %q — part of the mark was forwarded", got, "lead")
	}
}

// newTestConduit is a conduit writing into a sink of the caller's choosing,
// with no shell around it.
//
// The conduit's own terminal is what these two tests are about, so they build
// one directly rather than through a session: a session would put a prompt and
// an editor between the assertion and the thing it is asserting on.
func newTestConduit(t *testing.T, sink io.Writer) *ptyConduit {
	t.Helper()
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = tty.Close() })
	// The capture is the identity here: these are about the conduit's own
	// delivery, and a bounded copy in the way would make the assertion about
	// the bound instead.
	conduit, err := newPtyConduit(tty, sink, func(w io.Writer) io.Writer { return w })
	if err != nil {
		t.Skipf("no conduit: %v", err)
	}
	t.Cleanup(conduit.close)
	return conduit
}

// slowSink is a terminal that cannot keep up.
type slowSink struct {
	mu sync.Mutex
	n  int
}

func (s *slowSink) Write(p []byte) (int, error) {
	// Long enough that a pump with sixty-four writes to make is still making
	// them when a drain that did not wait would have returned, and short
	// enough that the test is not a pause.
	time.Sleep(time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n += len(p)
	return len(p), nil
}

func (s *slowSink) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// takeOutput drains before it takes, and this is the test that says the drain
// is *called* rather than that it works.
//
// The two are separate mutants and only one of them was caught: with the drain
// working perfectly, deleting the one line that calls it left every test green,
// because a session's own commands are small enough that the pump is always
// finished first. The cost of that would be a block holding the head of its
// output and the next block holding the tail — and a prompt drawn over output
// still arriving, which is the half a person sees.
//
// Same slow sink, so a conduit that is not drained *cannot* have delivered
// everything and the assertion is on the bytes.
func TestTakeOutputDrainsBeforeItTakes(t *testing.T) {
	sink := &slowSink{}
	capture := blocks.NewCapture(1 << 20)
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = tty.Close() }()
	conduit, err := newPtyConduit(tty, sink, capture.Stream)
	if err != nil {
		t.Skipf("no conduit: %v", err)
	}
	held := &outputCapture{cap: capture, conduit: conduit}
	defer held.close()

	const chunks, size = 64, 4096
	payload := bytes.Repeat([]byte("z"), size)
	for range chunks {
		if _, err := conduit.Stream().Write(payload); err != nil {
			t.Fatal(err)
		}
	}

	got := Shell{}.takeOutput(held)
	if want := int64(chunks * size); got.Bytes != want {
		t.Errorf("the block holds %d bytes, want %d — takeOutput did not drain first",
			got.Bytes, want)
	}
	if n := len(got.Text); n != chunks*size {
		t.Errorf("the body is %d bytes, want %d", n, chunks*size)
	}
}

// The inner terminal is the size of the outer one, asked by a child.
//
// Without this a full-screen program asks fd 1 how wide the terminal is and is
// told 0 by 0 — which is worse than a wrong answer, because a program that gets
// zero draws nothing and a program that gets eighty draws something. A fresh
// pseudo-terminal pair is zero by zero, so this is a thing the shell has to do
// and not a thing it gets.
//
// The session's terminal is 40 by 200, which is not any default: 24 by 80 would
// pass for a conduit that was never sized if anything anywhere filled that in.
func TestTheInnerTerminalHasTheOuterOnesSize(t *testing.T) {
	session := atACapturingPrompt(t)
	defer session.finish()

	// `stty size` prints rows then columns. The typed line does not contain
	// "40 200", so the wait is answered by the child and not by the echo.
	session.typeLine(`/bin/sh -c 'stty size <&1'`)
	session.waitForOutput("40 200", "the child reading the size of its own standard output")
}

// And it follows the outer one, which is the half a single sizing at startup
// would not do.
//
// Driven by a real SIGWINCH rather than by calling resize directly, because
// the signal is the wiring: a conduit that sized itself once and never
// registered for the signal passes every assertion that only calls resize.
//
// The signal is sent to this process on purpose. The kernel sends it to the
// foreground process group of a terminal that changed size, and a test process
// is not in this pseudo-terminal's session, so nothing would arrive on its own.
func TestTheInnerTerminalFollowsAResize(t *testing.T) {
	sink := &syncBuffer{}
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = tty.Close() }()
	if err := pty.SetSize(tty, 40, 200); err != nil {
		t.Fatal(err)
	}
	conduit, err := newPtyConduit(tty, sink, func(w io.Writer) io.Writer { return w })
	if err != nil {
		t.Skipf("no conduit: %v", err)
	}
	defer conduit.close()

	if rows, cols := terminalSize(conduit.Stream()); rows != 40 || cols != 200 {
		t.Fatalf("the inner terminal opened at %dx%d, want 40x200", rows, cols)
	}
	if err := pty.SetSize(tty, 12, 132); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	// The handler runs in a goroutine of its own, so this waits for the state
	// it is meant to reach rather than assuming it has.
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if rows, cols := terminalSize(conduit.Stream()); rows == 12 && cols == 132 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	rows, cols := terminalSize(conduit.Stream())
	t.Errorf("the inner terminal is %dx%d after a resize to 12x132", rows, cols)
}

// Closing the conduit lets what is in it reach the terminal first.
//
// The last block of a session is the case: a command finishes, the session
// ends, and whatever is still in the conduit is the tail of what the person
// asked for. A close that stopped the pump instead of waiting for it would
// drop exactly that, and drop it silently — the shell has already exited.
//
// Same slow sink, so a close that does not wait cannot have delivered
// everything.
func TestClosingWaitsForWhatIsStillInTheConduit(t *testing.T) {
	sink := &slowSink{}
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = tty.Close() }()
	conduit, err := newPtyConduit(tty, sink, func(w io.Writer) io.Writer { return w })
	if err != nil {
		t.Skipf("no conduit: %v", err)
	}

	const chunks, size = 64, 4096
	payload := bytes.Repeat([]byte("z"), size)
	for range chunks {
		if _, err := conduit.Stream().Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	conduit.close()

	if got, want := sink.len(), chunks*size; got != want {
		t.Errorf("the terminal has %d bytes after the close, want %d — the close did not wait", got, want)
	}
	// And twice is once: a session that closed a conduit on its way out and
	// then deferred a close as well must not wait for a pump that has gone.
	conduit.close()
}

// What is held back when the stream ends is written, not dropped.
//
// The carry is the longest tail of the stream that could still become a mark,
// and at end of stream it is one of two things: the start of a mark whose rest
// never came, which is the shell's own and worth nothing, or genuine output
// that happens to begin like one — a command printing a NUL as its last byte.
// The second is real and the first is not reachable in practice, since the
// shell closes the conduit only when it has stopped writing to it, so the carry
// is written. Losing a byte of somebody's output is the worse mistake.
func TestOutputEndingLikeAMarkIsNotSwallowed(t *testing.T) {
	sink := &syncBuffer{}
	conduit := newTestConduit(t, sink)

	// Ends with the mark's first byte, so it is held back as a possible mark
	// and only the end of the stream decides what it was.
	payload := append([]byte("tail"), conduit.mark[0])
	if _, err := conduit.Stream().Write(payload); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sink, "tail", "the bytes before the held-back one")
	conduit.close()

	if got, want := sink.String(), string(payload); got != want {
		t.Errorf("the terminal saw %q, want %q — the held-back byte was dropped", got, want)
	}
}

// The mark's shape, because the scan depends on it.
//
// Its only NUL bytes are the first and the last, which is what makes "could
// this tail still become a mark" have one answer instead of several: two
// lengths both matching would need a NUL somewhere in between. A mutation run
// found that taking the shortest match instead of the longest changes nothing
// here, and the reason is this shape rather than the search — so the shape is
// what gets asserted.
//
// The bracketing NULs are also why nothing a line discipline rewrites is in
// it: the middle is hex digits, which no discipline touches.
func TestTheMarkHasAShapeTheScanCanTrust(t *testing.T) {
	for range 8 {
		mark := newConduitMark()
		if len(mark) < 3 {
			t.Fatalf("mark %q is too short to bracket anything", mark)
		}
		if mark[0] != 0 || mark[len(mark)-1] != 0 {
			t.Errorf("mark %q is not bracketed by NULs", mark)
		}
		for i, c := range mark[1 : len(mark)-1] {
			if !strings.ContainsRune("0123456789abcdef", rune(c)) {
				t.Fatalf("mark %q has %q at %d, which is not a hex digit", mark, c, i+1)
			}
		}
		// And it is this session's own: a fixed mark would be stripped out of
		// a command that printed it.
		if other := newConduitMark(); bytes.Equal(mark, other) {
			t.Errorf("two marks came back the same: %q", mark)
		}
	}
}

// A terminal that will not say how big it is leaves the inner one as it was.
//
// Zero by zero is a real answer from the ioctl and it means "do not know" — the
// same reading terminalWidth already takes of it. Passing it on would tell a
// full-screen program already running that its terminal has no size, which is
// worse than the size it had a moment ago. So the last good answer stands.
//
// Reachable rather than theoretical: the outer terminal can go away while the
// conduit is still alive, and a SIGWINCH can arrive after it has.
func TestATerminalThatWillNotSaySizeLeavesTheInnerOneAlone(t *testing.T) {
	sink := &syncBuffer{}
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	if err := pty.SetSize(tty, 40, 200); err != nil {
		t.Fatal(err)
	}
	conduit, err := newPtyConduit(tty, sink, func(w io.Writer) io.Writer { return w })
	if err != nil {
		t.Skipf("no conduit: %v", err)
	}
	defer conduit.close()
	if rows, cols := terminalSize(conduit.Stream()); rows != 40 || cols != 200 {
		t.Fatalf("the inner terminal opened at %dx%d, want 40x200", rows, cols)
	}

	// The outer terminal goes away, so the ioctl fails and the size reads as
	// zero by zero.
	_ = tty.Close()
	if rows, cols := terminalSize(tty); rows != 0 || cols != 0 {
		t.Fatalf("a closed terminal reported %dx%d; this test needs one that will not say", rows, cols)
	}
	conduit.resize()

	if rows, cols := terminalSize(conduit.Stream()); rows != 40 || cols != 200 {
		t.Errorf("the inner terminal is %dx%d, want the 40x200 it had — a refusal was written through as a size",
			rows, cols)
	}
}

// A session that ends takes its conduit with it.
//
// The conduit runs two goroutines — the pump and the resize watcher — and
// holds two descriptors, all of them for as long as the session lasts. A
// session that did not close it would leak all four *per session*, which in a
// front end that starts a session per connection is a shell that grows until
// it runs out of descriptors. Nothing about the output would look wrong.
//
// Counted rather than inspected, because the conduit belongs to the session
// and there is no handle on it from out here. Several sessions are run so the
// leak is larger than the noise, and the count is waited on rather than read
// once: a goroutine that is on its way out has not gone yet.
func TestASessionThatEndsTakesItsConduitWithIt(t *testing.T) {
	settle := func() int {
		var last int
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			n := runtime.NumGoroutine()
			if n == last {
				return n
			}
			last = n
			time.Sleep(20 * time.Millisecond)
		}
		return last
	}
	before := settle()

	const sessions = 5
	for range sessions {
		session := atACapturingPrompt(t)
		session.typeLine(`/bin/sh -c 'echo alive-$((6*7))'`)
		session.waitForOutput("alive-42", "the command running under a conduit")
		session.finish()
	}

	// Two goroutines a session, so five sessions leak ten. A little slack for
	// the runtime's own, and far below what a leak would be.
	if after := settle(); after > before+2 {
		t.Errorf("%d goroutines after %d sessions, %d before — a conduit outlived its session",
			after, sessions, before)
	}
}
