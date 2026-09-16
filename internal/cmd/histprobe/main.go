// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command histprobe drives a shell through a pseudo-terminal with a TWO-ROW
// prompt and types lines at it one at a time, waiting on a distinct
// done-marker after each.
//
// It exists because history expansion cannot be measured any other way. It is
// **interactive by default and off in a script**, so a `-c` probe answers
// "absent" for a feature that is merely switched off, and the corpus harness
// runs every case as a command string — a row there would have recorded the
// same "nothing happened" in all six columns and looked like agreement. The
// panel table in docs/spec/history.md was produced by this program.
//
// A two-row prompt rather than a one-line PS1, for the reason tworowprobe
// gives: components can each match the shell they imitate while the
// composition of them does not.
//
// The marker after every line is what makes the run readable. Raw-mode input
// is lost silently and a pty read blocks, so the loop waits on text that
// could only have come from the line before it rather than on a timer, and it
// never clears the screen between waits. `-rc 'HISTCONTROL=ignorespace'` with
// a marker that begins with a space keeps the markers out of the history the
// designators index — without it, `!$` names the marker and the transcript
// measures the instrument.
//
//	histprobe -bin /opt/homebrew/bin/bash -name bash //	  -rc HISTCONTROL=ignorespace -lines 'echo one two three|echo !!'
//
// Written for #3093.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

const (
	upper = "UPPERROW"
	ready = "RDY> "
)

func main() {
	bin := flag.String("bin", "", "shell binary")
	name := flag.String("name", "bash", "argv[0]")
	rc := flag.String("rc", "", "extra rc lines (\\n separated)")
	lines := flag.String("lines", "", "lines to type, | separated")
	zsh := flag.Bool("zsh", false, "write a .zshrc instead of taking PS1 from the environment")
	flag.Parse()
	if *bin == "" || *lines == "" {
		fmt.Fprintln(os.Stderr, "histprobe: -bin and -lines required")
		os.Exit(2)
	}

	home, err := os.MkdirTemp("", "histprobe")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(home) }()

	rcText := "PS1='" + upper + "\n" + ready + "'\n"
	if *rc != "" {
		rcText += strings.ReplaceAll(*rc, "\\n", "\n") + "\n"
	}
	rcName := ".bashrc"
	if *zsh {
		rcName = ".zshrc"
	}
	if err := os.WriteFile(home+"/"+rcName, []byte(rcText), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	control, terminal, err := pty.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cmd := exec.Command(*bin)
	cmd.Args = []string{*name}
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=xterm-256color",
		"ENV=" + home + "/" + rcName,
		"PS1=" + upper + "\n" + ready,
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = terminal.Close()

	screen := smoke.Watch(control)
	if err := screen.Await(ready, 15*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "no prompt: %v\nscreen:\n%s\n", err, smoke.Readable(screen.Text()))
		os.Exit(1)
	}

	send := func(s string) { _, _ = control.WriteString(s) }
	for i, ln := range strings.Split(*lines, "|") {
		mark := fmt.Sprintf("DONE%d", i)
		send(ln + "\r")
		time.Sleep(250 * time.Millisecond)
		send(" echo " + mark + "X\r")
		if err := screen.Await(mark+"X\r\n", 6*time.Second); err != nil {
			// A shell that is stuck (an open quote, a pending read) never
			// reaches the marker; say so rather than hanging the next line.
			fmt.Printf("!! no marker after %q: %v\n", ln, err)
			break
		}
	}
	send("\x03")
	send("exit\r")
	_ = cmd.Wait()

	fmt.Printf("%s\n", smoke.Readable(screen.Text()))
}
