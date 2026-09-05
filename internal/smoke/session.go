// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/pty"
)

// session is one shell, running on one pseudo-terminal.
//
// Everything the suite does goes through here, and the reason it is a type
// rather than a function is that a session can be *lost* — a check that wedges
// the shell must cost that check and not the nine after it. So a session can
// be started again, and the suite says how many times it had to be.
type session struct {
	dialect Dialect
	home    string
	bin     string
	path    string

	cmd     *exec.Cmd
	control *os.File
	screen  *Screen

	// prompt is the mark this session is synchronized on: the anchor both
	// prompts end with, or the dialect's own default where neither prompt
	// parameter took effect.
	prompt string
	// startup is everything drawn before the first line was typed, which is
	// where the prompt questions are answered.
	startup string
	// ready records that the shell has been *seen* to be at a prompt and
	// nothing has been typed since.
	//
	// The wait belongs on the transition and not on the state, which is the
	// half of #635 that is easy to get backwards. Waiting for a prompt is
	// waiting for one to be *drawn*, and a shell sitting at the prompt it
	// drew when it started will not draw another until it is given a line —
	// so a wait issued there waits out its whole budget and reports a
	// perfectly healthy shell as unresponsive. Anything typed clears this,
	// and the next wait is a real one.
	ready bool
	// exited records that the shell has gone, so a later check says so
	// rather than waiting out its budget.
	exited bool
}

// budget bounds a wait on a mark.
//
// It turns a hang into a finding, and it is deliberately not tuned to what the
// work costs: drawing a prompt costs a hundredth of a second, and nothing that
// happens on a loaded machine turns that into five. The measurement behind the
// number is in driver's terminal test — under `-race` on one core with eight
// spinning processes, 995 of 1000 prompt waits finished inside 0.11s and the
// five that did not sat at the deadline exactly, with the shell's output
// already on the screen. That is the shape of a lost keystroke, not of a slow
// machine, so a larger number would only make each failure cost more.
//
// It is smaller than that test's ten seconds for a reason particular to this
// suite: most rows are expected to fail today, and a sweep that spends ten
// seconds on each of them is one nobody runs twice.
const budget = 5 * time.Second

// startBudget covers a process starting and reading its startup files, which
// is more work than drawing a prompt.
const startBudget = 15 * time.Second

// start opens a terminal, puts a shell on it and waits until it is prompting.
func (s *session) start(ctx context.Context) error {
	control, terminal, err := pty.Open()
	if err != nil {
		return fmt.Errorf("opening a pseudo-terminal: %w", err)
	}
	// A terminal nobody sized is zero columns wide, and a line editor asking
	// how wide the terminal is would be told "do not know" — so the drawing
	// this suite watches would not be the drawing a person sees.
	if err := pty.SetSize(terminal, 24, 80); err != nil {
		_ = terminal.Close()
		_ = control.Close()
		return fmt.Errorf("sizing the terminal: %w", err)
	}

	cmd := exec.CommandContext(ctx, s.bin)
	// Invoked under the name of the shell it is, because that is what a
	// person's terminal does and because argv[0] is where a shell reads its
	// own name — a prompt that draws it should draw the right one.
	cmd.Args = []string{s.dialect.Name}
	cmd.Dir = s.home
	cmd.Env = environment(s.home, s.path, s.dialect)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// A session of its own with this terminal as its controlling one.
		// Without it ^Z is a byte nobody acts on: the kernel sends SIGTSTP
		// to the *controlling terminal's* foreground process group, so a
		// shell that has not taken the terminal cannot be suspended through
		// it and job control has nothing to hand back and forth.
		Setsid:  true,
		Setctty: true,
	}
	if err := cmd.Start(); err != nil {
		_ = terminal.Close()
		_ = control.Close()
		return fmt.Errorf("starting %s: %w", s.bin, err)
	}
	// The child holds the terminal end now. Held open here as well, the
	// control end would never see the end of the session.
	_ = terminal.Close()

	s.cmd, s.control, s.screen, s.exited = cmd, control, Watch(control), false

	// Which prompt arrived is the first finding, and it is also what every
	// later wait synchronizes on. The dialect's own default is in the list
	// so that a shell which honors no prompt parameter at all is still a
	// session this suite can drive — the alternative is one broken thing
	// reported as ten.
	mark, err := s.screen.AwaitAny(startBudget, promptAnchor, s.dialect.DefaultPrompt)
	s.startup = s.screen.Text()
	if err != nil {
		return fmt.Errorf("no prompt: %w", err)
	}
	s.prompt, s.ready = mark, true
	return nil
}

// stop ends the session, and takes the process group with it.
//
// The group and not the process: a suite that suspends things leaves stopped
// children behind, and a stopped `sleep` whose shell has been killed is a
// process nobody will ever reap. Setsid made the shell a group leader, so the
// negative pid is the whole session.
func (s *session) stop() {
	if s.cmd == nil {
		return
	}
	if p := s.cmd.Process; p != nil {
		// SIGCONT first: a stopped process does not act on SIGTERM until it
		// runs again, and SIGKILL to a group leaves its stopped members
		// stopped and untouched.
		_ = syscall.Kill(-p.Pid, syscall.SIGCONT)
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
	}
	_ = s.cmd.Wait()
	if s.control != nil {
		_ = s.control.Close()
	}
	s.cmd, s.control = nil, nil
}

// send types bytes at the shell.
//
// It is never called without a wait on a prompt in front of it — see Screen —
// and the callers below are written so there is no way to reach it otherwise.
func (s *session) send(text string) error {
	if s.control == nil {
		return errors.New("the session has ended")
	}
	// Whatever was typed, the shell is no longer known to be sitting at a
	// prompt with nothing in hand, so the next thing typed has to wait for
	// one to be drawn again.
	s.ready = false
	if _, err := s.control.WriteString(text); err != nil {
		return fmt.Errorf("typing %q: %w", Readable(text), err)
	}
	return nil
}

// atPrompt waits until the shell is reading, which is the only moment at which
// it is safe to type at it.
//
// A prompt already seen and not typed at since is one the shell is still
// sitting at; waiting for another would wait for a line to be given first.
func (s *session) atPrompt() error {
	if s.ready {
		return nil
	}
	if err := s.screen.Await(s.prompt, budget); err != nil {
		return err
	}
	s.ready = true
	return nil
}

// typeLine waits for the prompt and then types a line.
func (s *session) typeLine(line string) error {
	if err := s.atPrompt(); err != nil {
		return fmt.Errorf("waiting for a prompt before typing %q: %w", line, err)
	}
	return s.send(line + "\r")
}

// runLine types a line and waits for a mark in what it produced.
//
// The mark must not be text that is in the line, or the terminal echoing the
// keystrokes answers the wait and the check passes for a shell that ran
// nothing. That invariant is what a probe is for, so most callers use runProbe
// and this one takes the two apart only where the mark is the dialect's own
// wording rather than something the suite chose.
func (s *session) runLine(line, mark string) error {
	if err := s.typeLine(line); err != nil {
		return err
	}
	return s.screen.Await(mark, budget)
}

// runProbe types a probe's line and waits for its mark.
func (s *session) runProbe(p probe) error { return s.runLine(p.line, p.mark) }

// recover puts the session back at a prompt after a check, whatever the check
// did to it.
//
// The ordinary case is a check that finished and left the shell prompting, so
// that is asked first and costs nothing. Failing that, ^C abandons whatever is
// on the line and interrupts whatever is running, and then a bare line draws a
// prompt even where the ^C did nothing. If none of it works the session is
// lost and the caller starts another — which is the whole reason one check
// failing does not decide the nine after it.
func (s *session) recover() error {
	if s.exited {
		return errors.New("the session has ended")
	}
	if err := s.atPrompt(); err == nil {
		return nil
	}
	for _, keys := range []string{"\x03", "\r"} {
		if err := s.send(keys); err != nil {
			return err
		}
		if err := s.atPrompt(); err == nil {
			return nil
		}
	}
	return errors.New("the session stopped prompting")
}

// waitForExit reports how the shell ended, or that it did not.
func (s *session) waitForExit() (int, error) {
	type done struct {
		code int
		err  error
	}
	ch := make(chan done, 1)
	go func() {
		err := s.cmd.Wait()
		var ee *exec.ExitError
		switch {
		case err == nil:
			ch <- done{code: 0}
		case errors.As(err, &ee):
			ch <- done{code: ee.ExitCode(), err: nil}
		default:
			ch <- done{code: -1, err: err}
		}
	}()
	select {
	case d := <-ch:
		s.exited = true
		// The reaping is done, so stop must not try again.
		s.cmd = nil
		if s.control != nil {
			_ = s.control.Close()
			s.control = nil
		}
		return d.code, d.err
	case <-time.After(budget):
		return -1, fmt.Errorf("the shell was still running %v after being told to exit; last drawn: %s",
			budget, LastLines(s.screen.Text(), diagnosticLines))
	}
}

// drawn is what the session has on screen, cleaned up for a person to read.
func (s *session) drawn() string { return LastLines(s.screen.Text(), diagnosticLines) }

// startupDrawn is the same for what was on screen before anything was typed.
func (s *session) startupDrawn() string { return Readable(strings.TrimSpace(s.startup)) }
