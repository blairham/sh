// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// **^C at the prompt sets `$?` to 130** (#5867), whatever the line held and
// whatever the status was before.
//
// Measured 2026-10-04 against `/opt/homebrew/bin/bash` — GNU bash 5.3.20 —
// through a pseudo-terminal, `--norc --noprofile -i`, the command before the
// ^C failing with 1 (and with 127, and succeeding):
//
//	keys before ^C          then `$?`   PIPESTATUS
//	`abc`, half typed       130         130
//	nothing                 130         130
//	`echo "abc⏎` (at PS2)   130         130
//	`for i in 1; do⏎`       130         130
//
// This shell abandoned the line and left the previous status in place. The
// status before each row is `true | false`'s, so a shell that kept the status
// and a shell that reset it fail differently and both fail, and the record
// before is two elements where the one wanted is one.
//
// PIPESTATUS is the half that differs from zsh, which leaves its record alone:
// see interp.Semantics.PromptStatusWritesThePipelineRecord and the zsh twin in
// cmd/zsh/interruptstatuspty_test.go.
func TestControlCAtThePromptSetsTheStatus(t *testing.T) {
	control, screen := interruptSession(t, "")
	for _, tc := range []struct {
		name string
		// keys is typed before the ^C; a newline in it is a line the
		// prompt took and is waiting for the rest of.
		keys string
	}{
		{"a half-typed line", "abc"},
		{"an empty line", ""},
		{"a continuation prompt", "echo \"abc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			interruptAnswer(t, control, screen, "true | false", nil)
			interruptAt(t, control, screen, tc.keys)
			got := interruptAnswer(t, control, screen, `echo "st-$?-${PIPESTATUS[*]}"`, interruptStatusLine)
			if want := "130-130"; got != want {
				t.Errorf("after ^C on %s the next line read %q, want %q", tc.name, got, want)
			}
		})
	}
}

// A refused line goes through the same door, and the record with it: after
// `true | false` and a `)` the parser will not read, bash 5.3.20 reads `$?` as
// 2 and PIPESTATUS as 2, where this shell used to leave the pipeline's two
// elements behind. Measured beside the rows above.
func TestARefusedLineSetsThePipelineRecordToo(t *testing.T) {
	control, screen := interruptSession(t, "")
	interruptAnswer(t, control, screen, "true | false", nil)
	interruptAnswer(t, control, screen, ")", nil)
	got := interruptAnswer(t, control, screen, `echo "st-$?-${PIPESTATUS[*]}"`, interruptStatusLine)
	if want := "2-2"; got != want {
		t.Errorf("after a refused line the next line read %q, want %q", got, want)
	}
}

// And PROMPT_COMMAND runs after the ^C and is handed 130 — the route the
// status takes to a prompt theme. Measured the same way: with
// `PROMPT_COMMAND='pc=$?'`, a failing command, `abc` and ^C, `$pc` read 130.
func TestPromptCommandSeesTheStatusAControlCLeft(t *testing.T) {
	control, screen := interruptSession(t, `PROMPT_COMMAND='printf "pc[%s]\n" $?'`+"\n")
	interruptAnswer(t, control, screen, "false", nil)
	from := len(screen.Text())
	interruptAt(t, control, screen, "abc")
	if got := interruptDrawnSince(t, screen, from, interruptHookLine); got != "130" {
		t.Errorf("PROMPT_COMMAND after ^C saw $? = %s, want 130", got)
	}
}

// interruptRow and interruptMark are the two rows of the prompt the session
// synchronizes on. Two rows because the editor redraws the row being typed
// on: a wait on the last row alone could be answered by the redraw of a
// keystroke from before the ^C was read, and the upper row is drawn once per
// prompt. Neither is text anybody types.
const (
	interruptRow  = "BJROW"
	interruptMark = "bj> "
	interruptPS2  = "bj2> "
)

// interruptBudget turns a hang into a failure.
const interruptBudget = 20 * time.Second

// interruptStatusLine and interruptHookLine pick out what the typed line and
// the hook print. Neither can match the echo of what was typed: the echo
// carries `$?`, and these want digits where it stands.
var (
	interruptStatusLine = regexp.MustCompile(`st-([0-9]+-[0-9 ]*)\r?\n`)
	interruptHookLine   = regexp.MustCompile(`pc\[([0-9]+)\]`)
)

// interruptSession puts an interactive bash on a pseudo-terminal with a
// two-row prompt and a scratch home, rc appended to its startup file.
//
// A copy of the shape of cmd/zsh's jobNoticeSessionRC rather than a call to
// it: both are `package main` of a different binary, and this one is the
// first pseudo-terminal test this package has.
func interruptSession(t *testing.T, rc string) (*os.File, *smoke.Screen) {
	t.Helper()
	return interruptSessionWith(t, rc)
}

// interruptSessionWith is interruptSession with variables added to the
// session's environment, a later one replacing an earlier one of the same
// name — `TERM` included, which is `dumb` otherwise.
func interruptSessionWith(t *testing.T, rc string, env ...string) (*os.File, *smoke.Screen) {
	t.Helper()
	home := t.TempDir()
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	startup := "PS1='" + interruptRow + `\n` + interruptMark + "'\nPS2='" + interruptPS2 + "'\n" + rc
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(startup), 0o600); err != nil {
		t.Fatal(err)
	}

	sh := scratchShell(t)
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
	}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		sh.Env = slices.DeleteFunc(sh.Env, func(have string) bool { return strings.HasPrefix(have, name+"=") })
		sh.Env = append(sh.Env, kv)
	}
	screen := smoke.Watch(control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"bash", "-i"}) }()
	t.Cleanup(func() {
		// The shell first, so nothing is left reading a terminal this test
		// is about to close, and then both ends of it.
		_, _ = control.WriteString("exit\n")
		select {
		case <-done:
		case <-time.After(interruptBudget):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = control.Close()
	})
	awaitInterruptPrompt(t, screen, "the first prompt")
	return control, screen
}

// awaitInterruptPrompt waits for both rows of a fresh prompt.
func awaitInterruptPrompt(t *testing.T, screen *smoke.Screen, after string) {
	t.Helper()
	for _, mark := range []string{interruptRow, interruptMark} {
		if err := screen.Await(mark, interruptBudget); err != nil {
			t.Fatalf("no prompt after %s: %v", after, err)
		}
	}
}

// interruptAt types keys, waits for the continuation prompt if they left one,
// presses ^C, and waits for the fresh prompt the ^C leaves.
func interruptAt(t *testing.T, control *os.File, screen *smoke.Screen, keys string) {
	t.Helper()
	if _, err := control.WriteString(keys); err != nil {
		t.Fatalf("typing %q: %v", keys, err)
	}
	if len(keys) > 0 && keys[len(keys)-1] == '\n' {
		if err := screen.Await(interruptPS2, interruptBudget); err != nil {
			t.Fatalf("no continuation prompt after %q: %v", keys, err)
		}
	}
	if _, err := control.WriteString("\x03"); err != nil {
		t.Fatalf("typing ^C: %v", err)
	}
	awaitInterruptPrompt(t, screen, "^C")
}

// interruptAnswer types line, answers what rx captured from its output — or
// nothing, for a nil rx — and waits for the prompt after it.
func interruptAnswer(t *testing.T, control *os.File, screen *smoke.Screen, line string, rx *regexp.Regexp) string {
	t.Helper()
	from := len(screen.Text())
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	got := ""
	if rx != nil {
		got = interruptDrawnSince(t, screen, from, rx)
	}
	awaitInterruptPrompt(t, screen, strconv.Quote(line))
	return got
}

// interruptDrawnSince polls what was drawn after from for rx and answers its
// first group, so a wrong status fails at once rather than at a deadline.
func interruptDrawnSince(t *testing.T, screen *smoke.Screen, from int, rx *regexp.Regexp) string {
	t.Helper()
	deadline := time.Now().Add(interruptBudget)
	for {
		if m := rx.FindStringSubmatch(screen.Text()[from:]); m != nil {
			return m[1]
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("waited %v for %s; drawn since:\n%s",
				interruptBudget, rx, smoke.Readable(smoke.LastLines(screen.Text()[from:], 10)))
		}
		time.Sleep(2 * time.Millisecond)
	}
}
