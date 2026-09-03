// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A refused open is an open that did not happen, and the command must not run
// without it.
//
// It used to return quietly, so the command ran with the stream it was
// redirecting *away from*: `echo x > refused` wrote to the terminal and
// reported success. That is the shape of failure a gate exists to prevent —
// the write goes somewhere the script did not ask for, and nothing says so.
func TestARefusedOpenStopsTheCommand(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	for _, tc := range []struct{ name, src string }{
		{"writing", "echo new > " + victim},
		{"appending", "echo new >> " + victim},
		{"reading", "cat < " + victim},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(victim, []byte("precious\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var out, errs strings.Builder
			sem := PosixSemantics()
			r := &Runner{
				Semantics: &sem, Stdout: &out, Stderr: &errs,
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					if a.Kind == ActionOpen {
						return Deny
					}
					return Allow
				}),
			}
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			status, err := r.Run(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			// The command did not run: nothing of its output anywhere.
			if out.String() != "" {
				t.Errorf("wrote %q, want the command not to have run", out.String())
			}
			// It failed, and said why.
			if status == 0 {
				t.Errorf("status %d, want a failure", status)
			}
			if !strings.Contains(errs.String(), "refused") {
				t.Errorf("said %q, want the refusal reported", errs.String())
			}
			// And the file it was pointed at is untouched — the gate is
			// asked before the open, so a truncating redirect never
			// truncates.
			body, err := os.ReadFile(victim)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "precious\n" {
				t.Errorf("file holds %q, want it untouched", body)
			}
		})
	}
}

// A refused command is a command that failed, not a broken shell: the script
// carries on.
func TestARefusedCommandFailsAndTheScriptCarriesOn(t *testing.T) {
	var out, errs strings.Builder
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem, Stdout: &out, Stderr: &errs,
		Gate: GateFunc(func(_ context.Context, a Action) Decision { return Deny }),
	}
	f, err := syntax.Parse("/bin/echo one; echo mid=$?; :; echo end=$?", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "mid=126\nend=0\n" {
		t.Errorf("out = %q, want the refusal to fail and the rest to run", got)
	}
}
