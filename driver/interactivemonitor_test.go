// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// monitorProbe writes what the shell it runs in says about the monitor, from
// the two readers that answer it: `$-` and the option listing.
//
// Whole lines from both, because a shell can disagree with itself here — real
// zsh 5.9.2 with a terminal puts `m` in `$-` and announces its jobs while its
// own `set -o` still lists `monitor off`, so a test reading one reader would
// call that shell right and this one wrong.
//
// It writes to a path of its own rather than to standard output, and that is
// not tidiness: one of the cases below makes standard *output* the terminal,
// and a probe that wrote there would have sent its answer to the terminal and
// left the test reading an empty file — which passes whatever the shell
// decided. Mutation caught exactly that: killing the decision outright left
// the case green.
const monitorProbe = `m=off
case $- in *m*) m=on;; esac
{ echo "m=$m"; set -o | grep '^monitor'; } > %s
`

// runInteractiveScript runs `-i script` with the streams the test gives it and
// the axis answered, returning what the script wrote.
func runInteractiveScript(t *testing.T, needsTerminal interp.Answer, in *os.File, out, errs *os.File) string {
	t.Helper()
	dir := t.TempDir()
	collected := filepath.Join(dir, "answer")
	path := filepath.Join(dir, "probe.sh")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(monitorProbe, collected)), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := shell()
	sh.Semantics.InteractiveMonitorNeedsATerminal = needsTerminal

	// Real files for the streams the test did not hand a terminal, so the
	// shell is asked the question a process would ask it: whether the
	// descriptor it holds is a terminal.
	sink, err := os.Create(filepath.Join(dir, "streams"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.Close() }()
	sh.Stdin = in
	sh.Stdout, sh.Stderr = sink, sink
	if out != nil {
		sh.Stdout = out
	}
	if errs != nil {
		sh.Stderr = errs
	}
	if code := driver.MainArgs(sh, []string{"testsh", "-i", path}); code != 0 {
		t.Fatalf("status %d, want 0", code)
	}
	b, err := os.ReadFile(collected)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An interactive shell with a terminal runs the monitor, whatever the dialect
// says about needing one.
//
// Unanimous, measured 2026-09-05 on `-i script.sh` through a pseudo-terminal
// with a scratch HOME: bash 5.3.15, dash, ksh93u+ 2012-08-01 and zsh 5.9.2 all
// put `m` in `$-`, and the first three list `monitor on`. So this half is a
// rule and not an axis, and a front end that leaves the monitor off is wrong
// in every dialect rather than in one.
func TestAnInteractiveShellWithATerminalRunsTheMonitor(t *testing.T) {
	for _, needs := range []interp.Answer{interp.Yes, interp.No} {
		_, tty := terminal(t)
		got := runInteractiveScript(t, needs, tty, nil, nil)
		if want := "m=on\nmonitor        on\n"; got != want {
			t.Errorf("with the axis %v: wrote %q, want %q", needs, got, want)
		}
	}
}

// A terminal on any one of the three standard streams is enough, which is
// measured rather than chosen: a pseudo-terminal on standard input alone, on
// standard output alone and on standard error alone each make bash 5.3.15,
// dash and zsh 5.9.2 report `monitor on` under `-i script.sh`. A *controlling*
// terminal with all three redirected elsewhere does not — all three report it
// off there — so the question is about the descriptors the front end holds.
func TestATerminalOnAnyOneOfTheThreeStreamsIsEnough(t *testing.T) {
	notATerminal := func(t *testing.T) *os.File { return openFile(t, os.DevNull) }
	for _, tc := range []struct {
		name  string
		which int
	}{
		{"standard input", 0},
		{"standard output", 1},
		{"standard error", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, tty := terminal(t)
			var in, out, errs *os.File
			switch tc.which {
			case 0:
				in, out, errs = tty, nil, nil
			case 1:
				in, out, errs = notATerminal(t), tty, nil
			case 2:
				in, out, errs = notATerminal(t), nil, tty
			}
			// The same assertion in all three, because the probe writes to
			// a path of its own: whichever stream is the terminal, the
			// answer lands in the file.
			got := runInteractiveScript(t, interp.Yes, in, out, errs)
			if want := "m=on\nmonitor        on\n"; got != want {
				t.Errorf("wrote %q, want %q — a terminal on this stream turns the monitor on", got, want)
			}
		})
	}
}

// With no terminal anywhere, the answer is the dialect's, and it is the whole
// of what the panel disagrees about here.
//
// Measured on the same invocation with every stream redirected: ksh93u+ still
// reports `monitor on` and `imBE`, and bash 5.3.15, dash and zsh 5.9.2 all
// leave it off and leave `m` out.
func TestWithNoTerminalTheMonitorIsTheDialectsAnswer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		needs interp.Answer
		want  string
	}{
		{"a dialect that needs one leaves it off", interp.Yes, "m=off\nmonitor        off\n"},
		{"and the one that does not runs it anyway", interp.No, "m=on\nmonitor        on\n"},
		{
			// An unanswered axis reads as the majority rather than refusing:
			// this is decided once at startup, before the program has run a
			// line, so "the shells disagree here" would land ahead of every
			// `-i script.sh` under a preset that has not chosen.
			"and an unanswered one is quiet and leaves it off",
			interp.Unspecified, "m=off\nmonitor        off\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runInteractiveScript(t, tc.needs, openFile(t, os.DevNull), nil, nil)
			if got != tc.want {
				t.Errorf("wrote %q, want %q", got, tc.want)
			}
		})
	}
}

// A script that is not interactive never runs the monitor, whichever way the
// axis is answered. Unanimous, and the reason the axis is named for an
// interactive shell: `sh script.sh` reports `monitor off` in all four.
func TestAScriptThatIsNotInteractiveNeverRunsTheMonitor(t *testing.T) {
	for _, needs := range []interp.Answer{interp.Yes, interp.No} {
		dir := t.TempDir()
		collected := filepath.Join(dir, "answer")
		path := filepath.Join(dir, "probe.sh")
		if err := os.WriteFile(path, []byte(fmt.Sprintf(monitorProbe, collected)), 0o600); err != nil {
			t.Fatal(err)
		}
		sh := shell()
		sh.Semantics.InteractiveMonitorNeedsATerminal = needs
		_, tty := terminal(t)
		sh.Stdin = tty
		if _, _, code := runArgs(t, sh, "testsh", path); code != 0 {
			t.Fatalf("status %d, want 0", code)
		}
		b, err := os.ReadFile(collected)
		if err != nil {
			t.Fatal(err)
		}
		if want := "m=off\nmonitor        off\n"; string(b) != want {
			t.Errorf("with the axis %v: wrote %q, want %q", needs, string(b), want)
		}
	}
}
