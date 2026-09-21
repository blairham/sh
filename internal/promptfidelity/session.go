// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptfidelity

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/pty"
)

// A shell on a pseudo-terminal, for the two sources that need one.
//
// Small on purpose. It types lines and waits for the drawing to stop, which
// is the whole of what capturing a prompt needs — there is no mark to wait
// for, because a prompt is what a program draws when it has nothing left to
// do.

// ptySession is a program with a terminal on one end and this on the other.
type ptySession struct {
	cmd     *exec.Cmd
	control *os.File

	mu    sync.Mutex
	bytes []byte
	// drawn is bumped every time something arrives, so a wait can ask
	// whether anything has happened since it last looked rather than
	// sleeping for a fixed time.
	drawn int
	// jobs is every background process this session was asked to start,
	// by pid. See background, and close, which kills them by name.
	jobs []int
	done chan struct{}
}

// startPty launches a program on a terminal of the pinned width.
//
// The width is set on the terminal itself rather than through `COLUMNS`,
// because that is where a shell reads it from and a prompt with a right
// side is a different prompt at a different width. `COLUMNS` goes in the
// environment too, for the sources that read it instead.
func startPty(program string, argv []string, ctx Context, env []string) (*ptySession, error) {
	control, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	if ctx.Columns > 0 {
		// On the terminal end, which is the side the shell holds — see
		// internal/pty.SetSize, and the rows are the fixed 24 a comparison
		// of prompts has no use for beyond "more than the prompt".
		if err := pty.SetSize(tty, 24, ctx.Columns); err != nil {
			_ = control.Close()
			_ = tty.Close()
			return nil, err
		}
	}
	cmd := exec.Command(program, argv[1:]...)
	cmd.Args = argv
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.Dir = ctx.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	cmd.Env = append([]string{
		"TERM=xterm-256color",
		"LC_ALL=en_US.UTF-8",
		"PATH=" + os.Getenv("PATH"),
		"COLUMNS=" + strconv.Itoa(ctx.Columns),
	}, env...)
	if err := cmd.Start(); err != nil {
		_ = control.Close()
		_ = tty.Close()
		return nil, err
	}

	// The child holds the terminal end now, and this lets go of it.
	//
	// **Held open here as well, the control end's read could not finish even
	// after the shell it was driving was gone.** A read on the control end
	// ends when the *last* holder of the terminal end closes, so a harness
	// that keeps one for the life of the session has made itself into the
	// holder that keeps its own read alive. internal/smoke's session says
	// the same thing in the same place and for the same reason.
	_ = tty.Close()

	s := &ptySession{cmd: cmd, control: control, done: make(chan struct{})}
	go s.read()
	return s, nil
}

// read drains what the program draws.
func (s *ptySession) read() {
	defer close(s.done)
	buf := make([]byte, 8192)
	for {
		n, err := s.control.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.bytes = append(s.bytes, buf[:n]...)
			s.drawn++
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// line types one command and a return.
func (s *ptySession) line(text string) {
	_, _ = s.control.WriteString(text + "\r")
}

// background starts a job that will still be running when the prompt is
// drawn, and remembers its pid.
//
// **Remembered, because killing the shell's process group does not reach
// it.** Job control is exactly the arrangement that puts a background job
// in a process group of its own, so a kill at the shell's group leaves one
// process per job per render behind — measured, four orphans at ppid 1
// after a single run with one job. The pid is the only handle that works,
// and the shell is the only thing that knows it.
//
// The pid it prints is erased along with everything else before the screen
// is read, so nothing about this reaches the comparison.
//
// **The marks are printf's answer and never its format**, which is the rule
// await states and which this was the one caller breaking: with the mark
// spelled out in the format string, the terminal's echo of the typed line
// contained `job-0-is[%s]` and answered the wait by itself. `pidAfter` then
// read the last `job-0-is[` in the stream, which was the echo whenever the
// real output had not landed yet, and parsed `%s` as a pid — measured on CI
// as `the shell named no usable process`, twice on main in one window
// (#4021). Written with the number as an argument, the echo carries
// `job-%s-is[` and cannot be mistaken for the answer.
//
// The wait is on the *second* line rather than on the pid's, because a
// terminal is an ordered stream: `job-0-said` having been drawn is proof the
// whole of `job-0-is[<pid>]` was drawn before it. Waiting on the opening
// bracket would leave the field half-written, which is the same race one
// step further along.
func (s *ptySession) background(deadline time.Duration) error {
	n := strconv.Itoa(len(s.jobs))
	mark, said := "job-"+n+"-is", "job-"+n+"-said"
	s.line(jobLine(n))
	if err := s.await(said, deadline); err != nil {
		return err
	}
	pid, err := s.pidAfter(mark + "[")
	if err != nil {
		return err
	}
	s.jobs = append(s.jobs, pid)
	startedJobs = append(startedJobs, pid)
	return nil
}

// jobLine is the line background types to start one job and have the shell
// say what it started.
//
// **The job's own streams go to the null device, and that is the whole of
// #4026's second failure rather than tidiness.** A background job is in a
// process group of its own, so one this harness failed to learn the pid of
// survives close's kills — and a surviving holder of the terminal end keeps
// the control end's read blocked, because a read there ends only when the
// *last* holder lets go. Measured: an orphaned `sleep 600` did exactly
// that, and close waited out the sleep — 9m57s against the package's ten
// minute budget, which costs the whole leg its timeout rather than costing
// a rerun.
//
// Linux-only, and the reason is worth writing down because it is why this
// never showed here: macOS revokes the terminal end when the control end
// closes, so the same orphan's three streams read `(revoked)` and the read
// returns at once. Linux has no revoke and the read waits for the sleep.
//
// A job holding none of the terminal cannot cause it, however else a render
// goes wrong. What the prompt reads is the shell's job table, which a
// redirection does not touch, so the count this pins is the same count.
//
// A function, and not the line spelled out at its one call site, so the
// property can be asserted without a terminal — see session_test.go.
func jobLine(n string) string {
	return "sleep 600 </dev/null >/dev/null 2>&1 & " +
		"printf 'job-%s-is[%s]\\njob-%s-said\\n' " + n + " $! " + n
}

// jobPids is every background pid the shell said it started, read back off
// the screen.
//
// close needs this because background records a pid only when it managed to
// read one, and the render that fails is the render that started a job and
// did not.
//
// A pid counts only where the screen shows the **whole** handshake —
// `job-N-is[<pid>]` and then the `job-N-said` that follows it. Both, because
// what is found here is killed, and a kill is not a thing to aim with a
// pattern that a half-drawn field or another program's output could
// satisfy. The second mark is already there for the same ordering reason
// background waits on it for.
func jobPids(screen []byte) []int {
	var pids []int
	for n := 0; ; n++ {
		number := strconv.Itoa(n)
		mark := []byte("job-" + number + "-is[")
		i := bytes.Index(screen, mark)
		if i < 0 {
			return pids
		}
		rest := screen[i+len(mark):]
		end := bytes.IndexByte(rest, ']')
		if end < 0 {
			return pids
		}
		if !bytes.Contains(rest[end:], []byte("job-"+number+"-said")) {
			return pids
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(rest[:end])))
		if err != nil || pid <= 1 {
			return pids
		}
		pids = append(pids, pid)
	}
}

// startedJobs is every background process any session here has started, so
// a test can ask whether they are gone.
//
// A package variable and not a return value, because the question is about
// the *instrument* rather than about one session: an orphan left behind is
// a process nobody is holding a handle to, which is exactly why it has to
// be recorded somewhere a caller does not have to remember to look.
//
// It **accumulates** for the life of the process and is never trimmed, so a
// reader of it takes the tail from wherever it was before the render they
// are asking about. Trimming would be the wrong fix: the ledger is about
// everything this package has ever started.
var startedJobs []int

// pidAfter reads the number the shell printed after a mark.
func (s *ptySession) pidAfter(mark string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := bytes.LastIndex(s.bytes, []byte(mark))
	if i < 0 {
		return 0, errors.New("the shell did not say what it started")
	}
	rest := s.bytes[i+len(mark):]
	end := bytes.IndexByte(rest, ']')
	if end < 0 {
		return 0, errors.New("the shell did not finish saying what it started")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(rest[:end])))
	if err != nil || pid <= 1 {
		return 0, errors.New("the shell named no usable process")
	}
	return pid, nil
}

// settle waits until nothing has been drawn for settleFor, or the deadline
// passes.
//
// A quiet period rather than a mark, because a prompt has no end: it stops
// when the program has finished drawing it. A source still drawing after
// the deadline is a source whose row will differ, and differ visibly.
func (s *ptySession) settle(deadline time.Duration) error {
	end := time.Now().Add(deadline)
	last := -1
	quietSince := time.Now()
	for time.Now().Before(end) {
		s.mu.Lock()
		drawn, any := s.drawn, len(s.bytes) > 0
		s.mu.Unlock()
		if drawn != last {
			last, quietSince = drawn, time.Now()
		}
		if any && time.Since(quietSince) >= settleFor {
			return nil
		}
		select {
		case <-s.done:
			return errors.New("it ended before it drew a prompt")
		case <-time.After(5 * time.Millisecond):
		}
	}
	return errors.New("it never stopped drawing")
}

// await waits until a mark has been drawn.
//
// A mark and not a quiet period, for the one question quiet cannot answer:
// **is the program ready to be typed at?** A prompt program that is
// downloading a helper on its first run in a fresh home has quiet periods
// in the middle of the download, and the first run of this instrument
// settled inside one and compared a screen reading "fetching gitstatusd"
// against a prompt. Faithfully, and uselessly.
//
// The mark is never text that is typed. A terminal echoes keystrokes, so a
// wait on a mark the line contains is answered by the echo — the rule
// internal/smoke states — so the line is an expression and the mark is its
// answer.
func (s *ptySession) await(mark string, deadline time.Duration) error {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		s.mu.Lock()
		found := bytes.Contains(s.bytes, []byte(mark))
		s.mu.Unlock()
		if found {
			return nil
		}
		select {
		case <-s.done:
			return errors.New("it ended before it was ready")
		case <-time.After(5 * time.Millisecond):
		}
	}
	return errors.New("it never became ready")
}

// ready types an expression and waits for its answer, which is the proof
// that the program is at a prompt and running what it is given.
func (s *ptySession) ready(deadline time.Duration) error {
	s.line("echo fidelity-$((6 * 7))-ok")
	return s.await("fidelity-42-ok", deadline)
}

// screen is what a terminal of this width would be showing.
func (s *ptySession) screen(cols int) *cellgrid.Grid {
	s.mu.Lock()
	defer s.mu.Unlock()
	grid := cellgrid.New(cols)
	_, _ = grid.Write(s.bytes)
	return grid
}

// close ends the program and takes the terminal away.
//
// The kill is by process *group*, because a source starts background jobs
// to pin a job count and a kill at the top would leave them: a harness that
// left a `sleep` per row behind would be the thing this repository's own
// rules were most recently tightened about.
func (s *ptySession) close() {
	// The jobs first and by pid, because job control put each of them in a
	// process group of its own and the group kill below cannot reach them.
	//
	// Read off the **screen** as well as from what background recorded,
	// because background records a pid only when it managed to read one and
	// the render that fails is exactly the render that started a job and did
	// not. That orphan is a `sleep 600` at ppid 1 in a process group of its
	// own, which is the thing this repository's own rules were most recently
	// tightened about — and, before the redirections above, the thing that
	// held the terminal for ten minutes.
	s.mu.Lock()
	drawn := jobPids(s.bytes)
	s.mu.Unlock()
	for _, pid := range append(append([]int{}, s.jobs...), drawn...) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	if s.cmd.Process != nil {
		if pgid, err := syscall.Getpgid(s.cmd.Process.Pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = s.cmd.Process.Kill()
		}
	}
	_ = s.control.Close()
	_ = s.cmd.Wait()
	// Bounded, and that bound is the difference between a row that fails
	// and a package that times out.
	//
	// The read above ends when the last holder of the terminal end lets go,
	// and nothing takes that hold away from a process this did not manage to
	// kill — so an unbounded wait here is an unbounded wait on somebody
	// else's process. Measured as the whole of #4026's second failure: one
	// row spending the package's ten-minute budget, which costs the leg its
	// timeout rather than costing a rerun. Everything above is aimed at
	// making sure no such holder exists; this is what keeps the harness
	// honest if one ever does again.
	//
	// Abandoning the reader is safe and is not a leak. It appends under the
	// mutex to a session nothing reads again, and it ends of its own accord
	// the moment the terminal end is finally let go.
	select {
	case <-s.done:
	case <-time.After(closeWait):
	}
}

// closeWait bounds how long close waits for the reader to finish.
//
// It bounds a hang rather than measuring work: the read ends as soon as the
// last holder of the terminal is gone, so anything reaching this number is
// a holder that outlived the kills above — a finding, not a slow machine.
const closeWait = 10 * time.Second
