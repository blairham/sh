// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command viprobe types one key sequence into one shell through a
// pseudo-terminal and prints the line that came back.
//
// It exists because `docs/spec/editing.md` carries a table of vi-mode keys
// marked "not built", and a table of that shape is the kind of claim that goes
// stale silently: nothing in the test suite reads prose, and the keys it names
// are reachable only from a terminal. Re-measuring one of those rows by hand
// means starting a shell, turning vi mode on, typing a seed, typing the keys
// one at a time, and reading the accepted line — which is this program.
//
// Three things about it are load-bearing rather than style:
//
//   - **The prompt has two rows.** A one-row prompt is the wrong instrument
//     here: the editor's redraw arithmetic composes a prompt height with a
//     cursor column, and each half can be right while the composition is not.
//     The upper row is typed in as a `PS1` assignment rather than handed over
//     in the environment, because zsh does not read `PS1` from there.
//
//   - **Keys go in one at a time**, with a gap between them. `editing.md`
//     records why: written as one burst, bash joins a kill onto the kill before
//     it across an insert between them, because pending input is read by a path
//     that never sees the keystroke in between. A burst measures the harness.
//
//   - **The answer is read off the screen after the line is accepted**, not
//     from the editor. The seed is an `echo`, so the line the keys built is the
//     line that runs and its own output says what it was.
//
// Usage, one case per run:
//
//	go run ./internal/cmd/viprobe -bin build/probe-bash -name bash \
//	    -seed 'echo ab' -keys '\x1byyp'
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

const (
	upper = "UPPERROW"
	ready = "RDY> "
	// The marker a run ends on. It is typed as its own command after the
	// case's line has been accepted, so waiting for it proves the shell came
	// back to a prompt rather than that some redraw happened to contain it.
	done = "ZZDONEZZ"
)

// unescape reads Go's own escapes out of a flag, so a key sequence can be
// written `\x1byyp` on a command line without a shell eating the backslash.
func unescape(s string) (string, error) {
	return strconv.Unquote(`"` + strings.ReplaceAll(s, `"`, `\"`) + `"`)
}

func main() {
	bin := flag.String("bin", "", "the shell binary to drive")
	name := flag.String("name", "", "argv[0] to invoke it under (default: the binary's own name)")
	argv := flag.String("argv", "", "extra arguments, space separated — e.g. `--norc --noprofile -i` for real bash")
	seed := flag.String("seed", "echo true alpha beta gamma", "the line typed in insert mode before the keys")
	keys := flag.String("keys", "", "the key sequence, Go-escaped; usually starts with \\x1b")
	setup := flag.String("setup", "set -o vi", "commands run before the case, Go-escaped and one per \\n; empty to run none")
	gap := flag.Duration("gap", 40*time.Millisecond, "how long to wait between keystrokes")
	inputrc := flag.String("inputrc", "/dev/null", "the INPUTRC the shell is given; /dev/null means no readline settings at all")
	term := flag.String("term", "xterm", "the TERM the shell is told it is talking to")
	burst := flag.Bool("burst", false, "write the key sequence in one write, the way a terminal writes an escape sequence")
	idle := flag.Duration("idle", 0, "sit at the prompt this long after the setup and report what arrived with nobody typing")
	flag.Parse()
	if *bin == "" || *keys == "" {
		fmt.Fprintln(os.Stderr, "viprobe: -bin and -keys are required")
		os.Exit(2)
	}
	sequence, err := unescape(*keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "viprobe: -keys: %v\n", err)
		os.Exit(2)
	}
	prelude, err := unescape(*setup)
	if err != nil {
		fmt.Fprintf(os.Stderr, "viprobe: -setup: %v\n", err)
		os.Exit(2)
	}
	if err := run(*bin, *name, *argv, *seed, sequence, prelude, *term, *inputrc, *burst, *gap, *idle); err != nil {
		fmt.Fprintln(os.Stderr, "viprobe:", err)
		os.Exit(1)
	}
}

func run(bin, name, argv, seed, keys, setup, term, inputrc string, burst bool, gap, idle time.Duration) error {
	// The binary is resolved before the run, because the shell is started in
	// a scratch directory and a relative path would be read against that.
	bin, err := filepath.Abs(bin)
	if err != nil {
		return err
	}

	// A scratch home, so nothing here can reach the real history file or the
	// real startup files of the machine this runs on.
	home, err := os.MkdirTemp("", "viprobe")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(home) }()

	control, terminal, err := pty.Open()
	if err != nil {
		return err
	}
	if err := pty.SetSize(terminal, 24, 80); err != nil {
		return err
	}

	cmd := exec.Command(bin)
	args := []string{bin}
	if name != "" {
		args = []string{name}
	}
	if argv != "" {
		args = append(args, strings.Fields(argv)...)
	}
	cmd.Args = args
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"HISTFILE=" + home + "/history",
		"PATH=/usr/bin:/bin",
		"TERM=" + term,
		"LANG=en_US.UTF-8",
		"INPUTRC=" + inputrc,
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = terminal.Close()
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	screen := smoke.Watch(control)
	line := func(s string) { _, _ = control.WriteString(s + "\r"); time.Sleep(300 * time.Millisecond) }

	// The two-row prompt, typed rather than inherited. The continuation in the
	// middle of the quoted string is what puts the newline in.
	line(`PS1="` + upper)
	line(ready + `"`)
	if err := screen.Await(ready, 15*time.Second); err != nil {
		return fmt.Errorf("no two-row prompt arrived: %w\nscreen:\n%s", err, smoke.Readable(screen.Text()))
	}
	for _, before := range strings.Split(setup, "\n") {
		if before == "" {
			continue
		}
		line(before)
	}
	time.Sleep(200 * time.Millisecond)

	if idle > 0 {
		quiet := len(screen.Text())
		time.Sleep(idle)
		fmt.Printf("with nobody typing for %s:\n%s\n\n", idle, smoke.Readable(screen.Text()[quiet:]))
	}

	mark := len(screen.Text())
	for _, r := range seed {
		_, _ = control.WriteString(string(r))
		time.Sleep(gap)
	}
	time.Sleep(200 * time.Millisecond)
	if burst {
		// A terminal delivers `\e[A` in a single write, and the editor's
		// question is whether a byte is *there* rather than whether one
		// arrives within some number of milliseconds. Typing the bytes one at
		// a time therefore answers a different question from the one a person
		// pressing an arrow key asks, and answers it "bare Escape".
		_, _ = control.WriteString(keys)
		time.Sleep(gap)
	} else {
		for _, r := range keys {
			_, _ = control.WriteString(string(r))
			time.Sleep(gap)
		}
	}
	time.Sleep(400 * time.Millisecond)
	afterKeys := screen.Text()[mark:]

	_, _ = control.WriteString("\r")
	time.Sleep(600 * time.Millisecond)
	line("echo " + done)
	_ = screen.Await(done, 5*time.Second)
	whole := screen.Text()[mark:]

	fmt.Printf("bin       %s\nseed      %q\nkeys      %q\n\n", bin, seed, keys)
	fmt.Printf("before the line was accepted:\n%s\n\n", smoke.Readable(afterKeys))
	fmt.Printf("the whole case:\n%s\n", smoke.Readable(whole))
	// The raw bytes, because smoke.Readable replays the screen and a replay
	// eats every escape sequence it understands. A question about what the
	// shell *drew* — a color, a reverse-video region, a mode switch — cannot
	// be asked of the rendered text at all, and asking it there gets a
	// confident "no" from an instrument that could not have said yes.
	fmt.Printf("\nraw:\n%q\n", whole)
	return nil
}
