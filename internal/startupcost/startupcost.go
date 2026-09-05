// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package startupcost measures how long a shell takes to become useful.
//
// Two numbers, because a shell is started two ways and the costs are not the
// same. `-c` is what every subshell in every script pays, and it is over
// before anything is drawn. A prompt is what a person pays on every new
// terminal, and it is the only one an rc file is read on.
//
// It measures rather than asserts. There is no threshold here and no golden
// number, because a startup time is a fact about the machine that ran it as
// much as about the code — a figure committed to this tree would be stale
// within a day and would read as a target. What is committed is the *method*,
// so that the question can be asked again and asked the same way; the answer
// belongs in whatever recorded the run. docs/design/startup.md holds the run
// this was written for.
//
// The reference is the real shells, which is what the rest of this repository
// does everywhere else: a millisecond means nothing on its own, and "half of
// what bash costs on the same machine, in the same minute" means something.
// They are measured in the same loop rather than from memory, for the same
// reason the corpus records a live panel.
//
// Test infrastructure rather than product, so it is internal — the same place
// the oracle and the pseudo-terminal live, and for the same reason. Nothing
// that is a shell measures how long shells take to start.
package startupcost

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/pty"
)

// promptMark is what a shell is asked to draw as its prompt, and seeing it is
// the definition of "started" used here.
//
// A sentinel rather than the first byte to arrive, because the first byte is
// not the prompt: two of the reference shells write terminal setup — bracketed
// paste, a partial-line marker — before they draw anything, and a measurement
// that stopped at the first byte would credit them with the time it takes to
// send an escape sequence. It is unlikely enough to appear in that setup that
// finding it means the prompt.
const promptMark = "SHSTARTUPMARK"

// rcBody is the rc file the "with an rc" half reads.
//
// Small and dull on purpose. The point of the pair is the *difference* an rc
// makes to the same shell, so what it contains has to be something every shell
// in the comparison can read and nothing that is interesting in itself: the
// number would otherwise be a measurement of whatever clever thing the file
// did. Aliases, functions and exports are what an rc is mostly made of.
//
// It ends by drawing the sentinel, and that is not decoration. A shell reads
// more than the file it was pointed at: Ubuntu's bash reads /etc/bash.bashrc
// as well, macOS's zsh reads /etc/zshrc, and both of those set a prompt of
// their own — so a sentinel carried in the environment was silently replaced
// and the run waited for a mark that was never coming. The rc is read last, so
// a prompt set here survives whatever the machine's own files did, and finding
// it means both that the shell started and that this file ran.
const rcBody = rcContent + "\nPS1=" + promptMark + "\nPROMPT=" + promptMark + "\n"

const rcContent = `alias ll='ls -l'
alias la='ls -A'
alias l='ls -CF'
alias gs='git status'
alias gd='git diff'
alias ..='cd ..'
alias ...='cd ../..'
export EDITOR=vi
export PAGER=less
export LESS=-R
mkcd() { mkdir -p "$1" && cd "$1"; }
up() { cd ..; }
title() { printf '%s' "$1"; }
`

// Subject is one thing being measured: a shell, and how to ask it for each of
// the two numbers.
type Subject struct {
	// Name is what the result is reported under.
	Name string

	// Path is the binary. Empty means it was not found and the subject is
	// skipped, which is how a machine without one of the reference shells
	// still gets the rest of the answer.
	Path string

	// CommandArgs run a command string and exit. The string is appended.
	CommandArgs []string

	// PromptArgs start an interactive shell with no rc file read.
	PromptArgs []string

	// RCPromptArgs start an interactive shell that reads the file named by
	// the placeholder rcPlaceholder, which is replaced with a real path.
	// Empty means this shell has no rc route to measure here.
	RCPromptArgs []string

	// RCEnv is the environment a rc-reading start needs, with
	// rcPlaceholder replaced the same way. The three shells disagree about
	// how a file is named — a flag, a directory, a variable — which is
	// exactly why it is data rather than a branch.
	RCEnv []string

	// RCName is what the file has to be called in the directory it is
	// written to. Empty means the name does not matter because the shell is
	// pointed straight at it.
	RCName string

	// PromptTimeout is how long this subject is given to draw its prompt
	// before the measurement is abandoned. Zero means readTimeout, which is
	// what every real subject uses; it is a field only so that the test for
	// "a shell that never prompts fails rather than hangs" need not take ten
	// seconds to make its point.
	PromptTimeout time.Duration

	// Env is added to the environment of *every* start of this subject,
	// whether or not an rc file is involved.
	//
	// It is separate from RCEnv rather than folded into it, and the
	// difference is what makes one of the tests able to bite. A test that
	// silences the prompt's sentinel through RCEnv is silencing it through
	// the very thing a broken rc route would drop, so a RunPrompt that
	// ignored rcDir entirely still drew the mark and still passed.
	Env []string
}

// rcPlaceholder stands where a subject wants the rc file's path, or the
// directory holding it, before either exists.
const (
	rcPlaceholder  = "@RC@"
	dirPlaceholder = "@RCDIR@"
)

// Ours describes the binaries this repository builds. dir is where they were
// built.
//
// The two dialects are measured separately and not because their code differs:
// it does not, and that is the finding. A preset is a value, so a dialect
// costs whatever filling in a struct costs, and having both in the table is
// what makes that visible rather than assumed.
func Ours(dir string) []Subject {
	ours := func(name, bin, rc string) Subject {
		return Subject{
			Name:         name,
			Path:         filepath.Join(dir, bin),
			CommandArgs:  []string{"-c"},
			PromptArgs:   []string{"-i"},
			RCPromptArgs: []string{"-i"},
			// A home directory of its own with the file in it, which is how
			// a person's shell finds theirs and how the reference shells are
			// pointed at one here. The name is the dialect's — see
			// Semantics.InteractiveStartupFile — so this is the same route a
			// real start takes rather than a back door for measuring.
			RCEnv:  []string{"HOME=" + dirPlaceholder},
			RCName: rc,
		}
	}
	return []Subject{
		ours("ours-bash", "our-bash", ".bashrc"),
		ours("ours-zsh", "our-zsh", ".zshrc"),
	}
}

// References are the real shells, found on this machine.
//
// The candidate lists are the oracle panel's, because the shell being compared
// against has to be the shell the corpus was recorded from — a Homebrew bash 5
// and the bash 3.2 Apple ships are different programs with the same name, and
// crediting one with the other's startup would be a measurement of neither.
func References() []Subject {
	return []Subject{
		{
			Name: "bash",
			Path: firstPath("/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/bin/bash", "/usr/bin/bash"),
			// --norc is how bash is told to read nothing, and --rcfile is how
			// it is told to read one thing. Neither is a flag this project
			// has; they are how that binary is asked the question.
			CommandArgs:  []string{"-c"},
			PromptArgs:   []string{"--norc", "-i"},
			RCPromptArgs: []string{"--rcfile", rcPlaceholder, "-i"},
			RCName:       "bashrc",
		},
		{
			Name:        "zsh",
			Path:        firstPath("/opt/homebrew/bin/zsh", "/usr/local/bin/zsh", "/bin/zsh", "/usr/bin/zsh"),
			CommandArgs: []string{"-c"},
			PromptArgs:  []string{"-f", "-i"},
			// zsh is pointed at a *directory* rather than a file, and reads
			// the name it owns inside it. `-d` is NO_GLOBAL_RCS, and it is
			// what makes the pair a measurement of *this* rc: without it the
			// shell also reads /etc/zshrc, so the with-and-without difference
			// would be this file plus whatever the machine's administrator
			// put in that one. Measured on macOS, /etc/zshrc also sets a
			// prompt of its own, which silently replaced the sentinel and
			// left the read waiting for a mark that was never going to come.
			RCPromptArgs: []string{"-d", "-i"},
			RCEnv:        []string{"ZDOTDIR=" + dirPlaceholder},
			RCName:       ".zshrc",
		},
	}
}

func firstPath(candidates ...string) string {
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// WriteRC puts the rc file where a subject expects to find it and returns the
// path to it. The directory is the caller's to make and to remove.
func WriteRC(s Subject, dir string) (string, error) {
	name := s.RCName
	if name == "" {
		name = "rc"
	}
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, []byte(rcBody), 0o600)
}

// RunCommand starts the shell with a command string and waits for it to
// finish, returning how long the whole invocation took.
//
// The command is `:`, which every shell in the comparison has as a builtin
// that does nothing. What is being timed is everything around it — exec, the
// runtime coming up, the dialect being assembled, one line parsed and run —
// and a command that did work would add its own.
func RunCommand(s Subject) (time.Duration, error) {
	argv := append(append([]string{}, s.CommandArgs...), ":")
	cmd := exec.Command(s.Path, argv...)
	cmd.Env = bareEnv(s.Env)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	start := time.Now()
	err := cmd.Run()
	return time.Since(start), err
}

// RunPrompt starts the shell on a pseudo-terminal and returns how long it took
// to draw its first prompt.
//
// A terminal rather than a pipe, because a prompt is exactly the thing a shell
// declines to draw without one — the whole reason internal/pty exists. The
// clock starts before the fork, so the number includes exec and everything the
// process does before it can ask for a line: it is what a person waits for
// after pressing return on a new terminal, not what the shell spends once it
// is running.
//
// rcDir empty measures a start that reads nothing; otherwise the subject's rc
// route is used and the file is expected to be there already.
func RunPrompt(s Subject, rcDir string) (time.Duration, error) {
	args, env := s.PromptArgs, append([]string(nil), s.Env...)
	if rcDir != "" {
		if len(s.RCPromptArgs) == 0 {
			return 0, errors.New("startupcost: " + s.Name + " has no rc route")
		}
		rcPath := filepath.Join(rcDir, s.RCName)
		args = expand(s.RCPromptArgs, rcPath, rcDir)
		env = append(expand(s.RCEnv, rcPath, rcDir), s.Env...)
	}

	control, terminal, err := pty.Open()
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(s.Path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal
	cmd.Env = bareEnv(env)
	// Its own session with this terminal as the controlling one, which is what
	// makes the shell believe it has a person: a process that merely holds a
	// terminal descriptor is not in the foreground of it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		_ = terminal.Close()
		return 0, err
	}
	// The child has it now. Holding the other end open here would keep the
	// terminal from ever reporting end of input.
	_ = terminal.Close()
	defer stop(cmd, control)

	// A shell that never draws the prompt has to end the measurement rather
	// than stop it, and killing it is what does that: the read returns once
	// the last writer of the terminal is gone. A deadline on the descriptor
	// is not enough — the pseudo-terminal this opens is not one the runtime
	// polls, so SetReadDeadline never fired and a zsh whose prompt had been
	// replaced by /etc/zshrc hung the whole run instead of failing it.
	patience := s.PromptTimeout
	if patience == 0 {
		patience = readTimeout
	}
	timeout := time.AfterFunc(patience, func() { killSession(cmd) })
	defer timeout.Stop()

	var seen []byte
	buf := make([]byte, 4096)
	for {
		n, rerr := control.Read(buf)
		seen = append(seen, buf[:n]...)
		if bytes.Contains(seen, []byte(promptMark)) {
			return time.Since(start), nil
		}
		if rerr != nil {
			return 0, fmt.Errorf("%s drew no prompt: %w (saw %q)", s.Name, rerr, seen)
		}
	}
}

// readTimeout is how long a shell is given to draw its prompt before the
// measurement is abandoned. Generous, because it is not a threshold — nothing
// here asserts on how long a start took — and it only has to be shorter than
// the patience of whoever is waiting.
const readTimeout = 10 * time.Second

// waitTimeout is how long teardown waits for a killed shell to be reaped.
//
// It exists because a wait with no bound on it hung a CI run for the whole ten
// minutes `go test` allows, after the measurement had already finished. What a
// stuck teardown costs is a leaked process; what an unbounded one costs is the
// build, and the second is worse.
const waitTimeout = 5 * time.Second

// stop ends a measured shell and gives back what it was holding.
//
// The order is deliberate and each step is here because the previous one is not
// enough on its own.
//
// The terminal is closed first, so a shell blocked writing into a buffer nobody
// is draining is unblocked by the descriptor going away rather than left
// waiting for a reader that is never coming back.
//
// The signal goes to the whole session and not to the one process, because a
// shell is a thing that starts other things: `Setsid` made this child a session
// leader, so the negative pid reaches whatever it started as well. Killing only
// the leader leaves a grandchild holding the terminal open.
//
// And the wait is bounded, because none of the above is a proof. A process that
// cannot be reaped is a leak of one process; a wait that cannot return is a
// build that fails ten minutes later with a stack trace pointing at teardown,
// which is what happened before this existed.
func stop(cmd *exec.Cmd, control *os.File) {
	_ = control.Close()
	killSession(cmd)
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(waitTimeout):
	}
}

// killSession signals the process group the measured shell leads, falling back
// to the process itself where there is no group to name — which is what a
// failure to start looks like.
func killSession(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}

// bareEnv is the environment every measured shell is started in.
//
// Deliberately not the process's. A shell that inherited a developer's
// environment would be measured reading their files, and the number would move
// with what is in them — which is a real cost and a different question from
// this one. HOME points at nothing, so a shell that goes looking for a profile
// finds none.
func bareEnv(extra []string) []string {
	env := []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + os.TempDir() + "/startupcost-no-home",
		"TERM=dumb",
		// Both spellings, because the panel does not agree on which parameter
		// draws the prompt and neither name costs anything to set.
		"PS1=" + promptMark,
		"PROMPT=" + promptMark,
		// Empty rather than unset for the run that is meant to read nothing:
		// an empty $ENV names no file, which driver/startup.go answers as
		// "there is nothing to read".
		"ENV=",
	}
	return append(env, extra...)
}

func expand(in []string, rcPath, rcDir string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = replaceAll(s, rcPlaceholder, rcPath)
		out = append(out, replaceAll(s, dirPlaceholder, rcDir))
	}
	return out
}

func replaceAll(s, old, new string) string {
	for {
		i := bytes.Index([]byte(s), []byte(old))
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}
