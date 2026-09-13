// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command tworowprobe drives a dialect binary through a pseudo-terminal with a
// two-row prompt and counts how many times the upper row reaches the screen.
//
// #2469 fixed the ladder of upper rows a two-row prompt left behind, and
// measured it at the repl layer. This asks the same question of the shipped
// binary, which is the thing a person actually runs.
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
	ready = "READY> "
)

func ps1(one bool) string {
	if one {
		return "PS1=" + ready
	}
	return "PS1=" + upper + "\n" + ready
}

func main() {
	bin := flag.String("bin", "", "the dialect binary to drive")
	name := flag.String("name", "bash", "argv[0] to invoke it under")
	one := flag.Bool("one", false, "use a one-row prompt instead of two")
	flag.Parse()
	if *bin == "" {
		fmt.Fprintln(os.Stderr, "tworowprobe: -bin is required")
		os.Exit(2)
	}

	home, err := os.MkdirTemp("", "tworow")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(home)

	control, terminal, err := pty.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := pty.SetSize(terminal, 24, 80); err != nil {
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
		ps1(*one),
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
		fmt.Fprintf(os.Stderr, "no two-row prompt arrived: %v\nscreen so far:\n%s\n", err, smoke.Readable(screen.Text()))
		os.Exit(1)
	}
	atPrompt := strings.Count(screen.Text(), upper)

	// One line of history to recall, then the interaction #2469 measured:
	// three Ups, a Down, and a keystroke.
	send := func(s string) { _, _ = control.WriteString(s); time.Sleep(150 * time.Millisecond) }
	send("echo one\r")
	_ = screen.Await("one", 5*time.Second)
	send("echo two\r")
	_ = screen.Await("two", 5*time.Second)

	if os.Getenv("PROBE_PASTE") != "" {
		send("\x1b[200~echo PASTED\x1b[201~")
		time.Sleep(600 * time.Millisecond)
		txt := screen.Text()
		if n := len(txt); n > 200 {
			txt = txt[n-200:]
		}
		fmt.Printf("after a bracketed paste, the line reads: %q\n", txt)
		send("Z")
		time.Sleep(400 * time.Millisecond)
		t2 := screen.Text()
		if n := len(t2); n > 120 {
			t2 = t2[n-120:]
		}
		fmt.Printf("then a plain Z keystroke:            %q\n", t2)
		send("\x03")
		send("exit\r")
		_ = cmd.Wait()
		return
	}
	before := strings.Count(screen.Text(), upper)
	send("\x1b[A")
	send("\x1b[A")
	send("\x1b[A")
	send("\x1b[B")
	send("x")
	time.Sleep(500 * time.Millisecond)
	after := strings.Count(screen.Text(), upper)

	send("\x03")
	send("exit\r")
	_ = cmd.Wait()

	fmt.Printf("upper row on screen: %d at the first prompt, %d before the recall, %d after three Ups + a Down + a keystroke\n",
		atPrompt, before, after)
	fmt.Printf("the recall interaction added %d copies of the upper row (zsh adds 0)\n", after-before)
	fmt.Printf("\nlast of the screen:\n%s\n", smoke.Readable(smoke.LastLines(screen.Text(), 6)))
	txt := screen.Text()
	if n := len(txt); n > 260 {
		txt = txt[n-260:]
	}
	fmt.Printf("\nraw tail: %q\n", txt)
}
