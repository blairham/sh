// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package gated_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/blairham/sh/gated"
)

// mainRun calls Main with buffers for both streams and returns what it wrote.
func mainRun(t *testing.T, argv []string, stdin io.Reader) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = gated.Main(context.Background(), argv, stdin, &out, &errb, "test")
	return code, out.String(), errb.String()
}

// TestMainBindsDollarZeroAndTheParameters is Main's half of
// TestArgsBindLikeShDashC: the words after the script reach $0, $1 and $2 and
// are not run. It is asserted through Main as well as through Shell.Run
// because the binding is two pieces of code, the front end slicing argv and
// the Shell handing the slices to the Runner, and dropping either one is a
// shell that still runs every script it is given.
func TestMainBindsDollarZeroAndTheParameters(t *testing.T) {
	code, stdout, stderr := mainRun(t,
		[]string{"-c", `echo "0=$0 1=$1 2=$2 n=$#"`, "probe", "alpha", "beta"}, nil)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, stderr)
	}
	if got, want := strings.TrimSpace(stdout), "0=probe 1=alpha 2=beta n=2"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Fatalf("an argument reached the gate: stderr %q", stderr)
	}

	code, stdout, _ = mainRun(t, []string{"-c", `echo "0=$0 n=$#"`}, nil)
	if code != 0 {
		t.Fatalf("with no args: status %d", code)
	}
	if got, want := strings.TrimSpace(stdout), "0=sh n=0"; got != want {
		t.Fatalf("with no args: stdout = %q, want %q", got, want)
	}
}

// TestMainReadsAScriptFromStdin is `kubectl exec -i pod -- /bin/sh < probe.sh`.
// The exit status is the script's, and $0 is the shell, as it is in real sh
// when the script arrives on standard input.
func TestMainReadsAScriptFromStdin(t *testing.T) {
	code, stdout, stderr := mainRun(t, nil, strings.NewReader("echo \"from stdin as $0\"\nexit 3\n"))
	if code != 3 {
		t.Fatalf("status %d, want the script's 3; stderr %q", code, stderr)
	}
	if got, want := strings.TrimSpace(stdout), "from stdin as sh"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestMainRefusesAnOversizedScriptOnStdin pins that the stdin route has the
// same cap as -c, rather than reading whatever arrives into memory.
func TestMainRefusesAnOversizedScriptOnStdin(t *testing.T) {
	big := strings.Repeat("#", 64<<10)
	code, stdout, stderr := mainRun(t, nil, strings.NewReader(big))
	if code != 1 {
		t.Fatalf("status %d, want 1", code)
	}
	if stdout != "" || !strings.Contains(stderr, "cap is") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestMainUsage(t *testing.T) {
	for name, argv := range map[string][]string{
		"bare -c":     {"-c"},
		"script file": {"probe.sh"},
		"interactive": {"-i"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := mainRun(t, argv, strings.NewReader("echo ran"))
			if code != 2 {
				t.Fatalf("status %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("something ran: stdout %q", stdout)
			}
			if !strings.HasPrefix(stderr, "usage: sh -c") {
				t.Fatalf("stderr %q", stderr)
			}
		})
	}
}

// TestMainSummarizesRefusalsOnStderr is the reason the summary exists: a
// refused action and an action that found nothing look the same on stdout.
func TestMainSummarizesRefusalsOnStderr(t *testing.T) {
	code, stdout, stderr := mainRun(t, []string{"-c", "/bin/echo pwned"}, nil)
	if strings.Contains(stdout, "pwned") {
		t.Fatalf("the binary RAN: stdout %q", stdout)
	}
	if code == 0 {
		t.Fatal("a refused command exited 0")
	}
	for _, want := range []string{"sh: the gate refused", "line 1: exec /bin/echo", "cannot exec"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr lacks %q:\n%s", want, stderr)
		}
	}

	// And it is absent when nothing was refused, so it cannot be mistaken
	// for boilerplate.
	_, _, stderr = mainRun(t, []string{"-c", "echo fine"}, nil)
	if strings.Contains(stderr, "refused") {
		t.Fatalf("a summary with nothing refused: %q", stderr)
	}
}

func TestMainReportsAParseFailure(t *testing.T) {
	code, _, stderr := mainRun(t, []string{"-c", "if then fi"}, nil)
	if code != 1 || !strings.Contains(stderr, "parse") {
		t.Fatalf("status %d, stderr %q", code, stderr)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("session dropped") }

// TestMainFailsWhenItCannotDeliver pins that a truncated answer never exits 0.
func TestMainFailsWhenItCannotDeliver(t *testing.T) {
	var errb bytes.Buffer
	if code := gated.Main(context.Background(), []string{"-c", "echo hi"}, nil, failWriter{}, &errb, ""); code != 1 {
		t.Fatalf("stdout write failure: status %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "write stdout") {
		t.Fatalf("stderr %q", errb.String())
	}

	var out bytes.Buffer
	if code := gated.Main(context.Background(), []string{"-c", "echo hi >&2"}, nil, &out, failWriter{}, ""); code != 1 {
		t.Fatalf("stderr write failure: status %d, want 1", code)
	}

	code, _, stderr := mainRun(t, nil, failReader{})
	if code != 1 || !strings.Contains(stderr, "read stdin") {
		t.Fatalf("stdin read failure: status %d, stderr %q", code, stderr)
	}
}
