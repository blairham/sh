// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// denyPath refuses every action aimed at one path.
func denyPath(path string) func(interp.Action) bool {
	return func(a interp.Action) bool { return a.Path == path }
}

// opened reports whether the stream recorded an access of this kind at path.
func opened(rec *recorder, kind interp.EventKind, path string) bool {
	return rec.seen(func(e interp.Event) bool {
		return e.Kind == kind && e.Action.Kind == interp.ActionOpen && e.Action.Path == path
	})
}

// The program a shell was pointed at is an access like any other.
//
// It was not, until the front end could hold a gate: the script was read
// through the os package, so a policy hiding a path could be handed to a
// binary that then read that very path and ran it. An audit trail had the
// same hole from the other side — every file the script opened, and not the
// script.
func TestTheScriptOperandPassesTheGate(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		path := writeScript(t, "echo ran\n")
		rec := &recorder{deny: denyPath(path)}
		sh := shell()
		sh.Gate, sh.Events = rec, rec

		out, errs, code := runArgs(t, sh, "testsh", path)

		if strings.Contains(out, "ran") {
			t.Errorf("out = %q, want a refused script not to have run", out)
		}
		// 126 rather than 127: the shell reached a path it may not read,
		// which is not the same as a path that is not there — saying "no such
		// file" would send someone looking for a typo.
		if code != 126 {
			t.Errorf("status = %d, want 126 for a script that would not open", code)
		}
		if !strings.Contains(errs, path) {
			t.Errorf("err = %q, want the script named", errs)
		}
		if !opened(rec, interp.EventDenied, path) {
			t.Error("no denial for the script reached the event stream")
		}
	})

	t.Run("allowed, and recorded", func(t *testing.T) {
		path := writeScript(t, "echo ran\n")
		rec := &recorder{}
		sh := shell()
		sh.Gate, sh.Events = rec, rec

		out, _, code := runArgs(t, sh, "testsh", path)

		if !strings.Contains(out, "ran") || code != 0 {
			t.Errorf("out = %q status = %d, want the script to have run", out, code)
		}
		if !opened(rec, interp.EventAccess, path) {
			t.Error("the script the shell read was not on the event stream")
		}
	})
}

// $ENV and ~/.profile are the same access, and the sharper of the two cases:
// the path comes from a shell variable, so a line typed at a prompt can aim
// it, and a policy that refuses every open a script makes must not be walked
// around by setting a variable and starting a session.
func TestTheStartupFilePassesTheGate(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "rc.sh")
	if err := os.WriteFile(rc, []byte("FROM_RC=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		deny   func(interp.Action) bool
		want   string
		denied bool
	}{
		{"refused", denyPath(rc), "rc=\n", true},
		{"allowed", nil, "rc=yes\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV", rc)
			rec := &recorder{deny: tc.deny}
			sh := shell()
			sh.Gate, sh.Events = rec, rec

			out, _, code := runPipedShell(t, sh, "echo rc=$FROM_RC\n", "testsh", "-i")

			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			// Refused or not, the session starts: a policy that hid the
			// startup file meant to keep the file out of the session, not to
			// keep the person out of a shell.
			if code != 0 {
				t.Errorf("status = %d, want the session to have started", code)
			}
			kind := interp.EventAccess
			if tc.denied {
				kind = interp.EventDenied
			}
			if !opened(rec, kind, rc) {
				t.Errorf("no %v for the startup file reached the event stream", kind)
			}
		})
	}
}

// The gate and the sink reach the prompt's own file, which is what makes
// HISTFILE part of the boundary rather than a way around it.
//
// Asserted through a whole invocation rather than on repl's types, because
// what is being checked is the wiring: a session gated for what a script does
// and ungated for what the prompt does looks exactly like a shell that works.
func TestTheHistoryFileIsInsideTheBoundary(t *testing.T) {
	// A home of its own before the history file is aimed, because naming
	// HISTFILE turns the block store back on and the store is under $HOME:
	// isolating one of the two leaves the other writing into a person's home.
	// The history file is then aimed away from it, which is the whole of what
	// this test is about.
	hist := filepath.Join(scratchHome(t), "history")
	if err := os.WriteFile(hist, []byte("echo earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", hist)

	rec := &recorder{deny: denyPath(hist)}
	sh := shell()
	sh.Gate, sh.Events = rec, rec

	if _, _, code := runPipedShell(t, sh, "echo typed\n", "testsh", "-i"); code != 0 {
		t.Errorf("status = %d, want the session to have run", code)
	}
	if !opened(rec, interp.EventDenied, hist) {
		t.Error("the history file was opened without asking the gate")
	}
}

// An ungated shell reads exactly what it read before, which is the default
// every binary that never asked for a policy is entitled to.
func TestTheFrontEndsReadsCostNothingWithoutAGate(t *testing.T) {
	path := writeScript(t, "echo ran\n")
	out, errs, code := runArgs(t, driver.Shell{Name: "testsh"}, "testsh", path)
	if out != "ran\n" || errs != "" || code != 0 {
		t.Errorf("got %q / %q / %d, want the script to have run cleanly", out, errs, code)
	}
}
