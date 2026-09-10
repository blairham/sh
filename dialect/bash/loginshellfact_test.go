// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runBashLogin runs src in a shell that was started as a login shell, which
// runBash cannot do: the fact is interp.Runner.LoginShell, carried in by the
// front end, and `shopt login_shell` is the only thing in this dialect that
// reads it.
func runBashLogin(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	sem, dg := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &dg,
		Stdout: &out, Stderr: &out, Name: "testsh",
		Dialect:    presetDialect(),
		LoginShell: true,
	}
	bash.Apply(r)
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String(), st
}

// TestShoptLoginShellIsAFactAndNotAPreference pins `shopt login_shell`, which
// is the one name in this builtin that reports how the shell was *started*.
//
// Two things about it are measured on bash 5.3.15 and neither is what a name in
// shoptStates would do:
//
//   - it reads `on` under `-l` and `off` without it, so a static default is
//     wrong in one of the two cases whichever value it holds; and
//   - `shopt -s login_shell` and `shopt -u login_shell` both report **0** and
//     move nothing at all — in a login shell and outside one alike. So a write
//     is neither granted nor refused; it is ignored.
//
// The second is why this cannot be a shoptStates row even with the right
// default: that table refuses a write it cannot honour, out loud and at 1, and
// bash says nothing and answers 0. A generated shell snapshot writes the row it
// read back — `shopt -u login_shell` — ahead of every command, so a refusal
// there is an error on every one of them (#1709).
func TestShoptLoginShellIsAFactAndNotAPreference(t *testing.T) {
	t.Run("outside a login shell", func(t *testing.T) {
		for _, tc := range []struct {
			src    string
			want   string
			status int
		}{
			{`shopt login_shell`, "login_shell         \toff\n", 1},
			{`shopt -p login_shell`, "shopt -u login_shell\n", 1},
			{`shopt -q login_shell`, "", 1},
			// Both directions are taken and neither moves it.
			{`shopt -u login_shell; shopt -p login_shell`, "shopt -u login_shell\n", 1},
			{`shopt -s login_shell; shopt -p login_shell`, "shopt -u login_shell\n", 1},
		} {
			out, st := runBash(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q status %d, want %q status %d",
					tc.src, out, st, tc.want, tc.status)
			}
		}
		// The write itself is quiet and successful, which is the half a
		// refusal would get wrong.
		for _, src := range []string{`shopt -s login_shell`, `shopt -u login_shell`} {
			if out, st := runBash(t, t.TempDir(), src); st != 0 || out != "" {
				t.Errorf("%s = %q status %d, want a quiet success", src, out, st)
			}
		}
	})

	t.Run("in a login shell", func(t *testing.T) {
		for _, tc := range []struct {
			src    string
			want   string
			status int
		}{
			{`shopt login_shell`, "login_shell         \ton\n", 0},
			{`shopt -p login_shell`, "shopt -s login_shell\n", 0},
			{`shopt -q login_shell`, "", 0},
			// Asking for the opposite of the fact is still ignored, not
			// refused: the row that comes back is unchanged.
			{`shopt -u login_shell; shopt -p login_shell`, "shopt -s login_shell\n", 0},
		} {
			out, st := runBashLogin(t, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q status %d, want %q status %d",
					tc.src, out, st, tc.want, tc.status)
			}
		}
		if out, st := runBashLogin(t, `shopt -u login_shell`); st != 0 || out != "" {
			t.Errorf("shopt -u login_shell = %q status %d, want a quiet success", out, st)
		}
	})
}

// TestShoptLoginShellIsStillListed: moving the name out of shoptStates must not
// drop it from the listing every snapshot generator dumps.
func TestShoptLoginShellIsStillListed(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `shopt -p`)
	if st != 0 {
		t.Fatalf("shopt -p status %d, want 0 — the whole listing succeeds whatever the rows say, measured", st)
	}
	if want := "shopt -u login_shell\n"; !strings.Contains(out, want) {
		t.Errorf("shopt -p omitted %q", want)
	}
}
