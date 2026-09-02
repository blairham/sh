// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// Deciding to prompt is part of reading the invocation, so it belongs to the
// shared front end rather than to one binary's main. It lived in one binary's
// main, which is how `sh` learned to prompt and `bash` answered
//
//	bash: unknown option "-i"
//
// — the same drift that once let `sh` run a script file and left the dialect
// binaries unable to. A test here is what makes it a property of the front
// end: every binary built on it has a prompt because this passes.
func TestTheFrontEndDecidesToPrompt(t *testing.T) {
	t.Run("-i prompts where standard input is not a terminal", func(t *testing.T) {
		out, errs, _ := runPiped(t, "echo prompted\n", "testsh", "-i")
		if !strings.Contains(out, "prompted\n") {
			t.Errorf("out = %q, want the typed line to have run", out)
		}
		// The prompt is on the error stream, so `sh -i < in > out` collects
		// the commands' output and nothing else.
		if !strings.Contains(errs, "$ ") {
			t.Errorf("err = %q, want a prompt on it", errs)
		}
		if strings.Contains(out, "$ ") {
			t.Errorf("out = %q, want no prompt on the output stream", out)
		}
	})

	t.Run("without -i the same input is a script", func(t *testing.T) {
		// Not a terminal and nothing named: `curl … | sh` arrives this way
		// and must not wait for a keystroke or print a prompt.
		out, errs, _ := runPiped(t, "echo scripted\n", "testsh")
		if !strings.Contains(out, "scripted\n") {
			t.Errorf("out = %q, want standard input read as a script", out)
		}
		if strings.Contains(errs, "$ ") {
			t.Errorf("err = %q, want no prompt for a script on standard input", errs)
		}
	})

	t.Run("an operand still wins", func(t *testing.T) {
		// `-i` is an option among options, not a mode: naming a script still
		// runs the script.
		path := writeScript(t, "echo from-the-script\n")
		out, _, _ := runPiped(t, "", "testsh", "-i", path)
		if !strings.Contains(out, "from-the-script\n") {
			t.Errorf("out = %q, want the named script to have run", out)
		}
	})

	t.Run("-c still wins", func(t *testing.T) {
		out, _, _ := runPiped(t, "", "testsh", "-i", "-c", "echo from-c")
		if !strings.Contains(out, "from-c\n") {
			t.Errorf("out = %q, want -c to have run", out)
		}
	})
}

// A prompt is the same shell as a script: the dialect's prelude is there, and
// so is what the session has already done to itself.
func TestAPromptIsTheSameShellAsAScript(t *testing.T) {
	sh := shell()
	sh.Prelude = "greet() { echo hello-from-the-prelude; }\n"
	out, _, _ := runPipedShell(t, sh, "greet\nx=5\necho \"x=$x\"\n", "testsh", "-i")
	if !strings.Contains(out, "hello-from-the-prelude") {
		t.Errorf("out = %q, want the dialect's prelude available at the prompt", out)
	}
	if !strings.Contains(out, "x=5") {
		t.Errorf("out = %q, want one line's state to reach the next", out)
	}
}

func runPiped(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	return runPipedShell(t, shell(), typed, argv...)
}

// runPipedShell invokes the front end with standard input that is not a
// terminal, which is what `-i` exists to override.
func runPipedShell(t *testing.T, sh driver.Shell, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = pipeWith(t, typed)
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

func pipeWith(t *testing.T, s string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(s)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	return r
}
