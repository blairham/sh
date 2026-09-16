// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command histprobe drives a shell through a pty with a TWO-ROW prompt and
// types history-expansion lines at it, one at a time, waiting on a distinct
// done-marker after each. Scratch instrument for #3093.
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
