// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `suspend`, in the direction a script sees it — and the direction it parts
// from bash's builtin of the same name.
//
// Measured 2026-09-15 against zsh 5.9.2, `env -i` with a scratch HOME and no
// startup files, every stopping probe in a process group of its own and killed
// from outside. See dialect/zsh/suspend.go for the table and #2557 for the
// warning about probing this at all.
//
// There is deliberately **no corpus row**, for the reason that issue gives: a
// bare `suspend` really stops this shell, `make oracle` runs every row in
// every column, and a stopped process does not answer the harness's SIGTERM.
func TestSuspendRefusesTheWayZshDoes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		// One line and status 1 — no usage block, where bash writes one and
		// reports 2.
		{
			name: "a bad option", src: `suspend -q; echo "st=$?"`,
			want: "zsh:suspend:1: bad option: -q\nst=1\n",
		},
		{
			name: "a bad option after -f", src: `suspend -f -q; echo "st=$?"`,
			want: "zsh:suspend:1: bad option: -q\nst=1\n",
		},
		{
			name: "an operand", src: `suspend x; echo "st=$?"`,
			want: "zsh:suspend:1: too many arguments\nst=1\n",
		},
		{
			name: "an operand after --", src: `suspend -- x; echo "st=$?"`,
			want: "zsh:suspend:1: too many arguments\nst=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// **Job control decides nothing here**, which is the discriminating pair: this
// shell's builtin reaches the stop with no monitor running, where bash's
// refuses. A builtin that had borrowed bash's condition would answer the first
// row with `cannot suspend: no job control`.
//
// The login shell is the one thing this shell does refuse, and `-f` forces
// past it.
func TestSuspendAsksOnlyWhetherThisIsALoginShell(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		login           bool
	}{
		{
			name: "no monitor is no obstacle", src: `suspend; echo "st=$?"`,
			want: "zsh:suspend:1: this shell was not given a way to stop this process\nst=1\n",
		},
		{
			name: "a login shell is refused", login: true, src: `suspend; echo "st=$?"`,
			want: "zsh:suspend:1: can't suspend login shell\nst=1\n",
		},
		{
			name: "and -f forces past it", login: true, src: `suspend -f; echo "st=$?"`,
			want: "zsh:suspend:1: this shell was not given a way to stop this process\nst=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), LoginShell: tc.login,
			}, tc.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the stop itself. The hook returns rather than stopping anything, which
// is what makes the row runnable: a test that really stopped would stop the
// test binary and `go test` would wait for a SIGCONT nobody sends.
func TestSuspendCallsTheHookWithNoMonitorAtAll(t *testing.T) {
	var out strings.Builder
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: &out, Stderr: &out})
	calls := 0
	r.StopThisProcess = func() error { calls++; return nil }
	f := preset.Parse(t, `suspend; echo "st=$?"`)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 1 {
		t.Errorf("the hook was called %d times, want once", calls)
	}
	if got, want := out.String(), "st=0\n"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
}

// A login shell never reaches it, which is the other half of the same rule.
func TestALoginShellNeverReachesTheStop(t *testing.T) {
	var out strings.Builder
	r := preset.Runner(dialecttest.Base{
		Dir: t.TempDir(), Stdout: &out, Stderr: &out, LoginShell: true,
	})
	calls := 0
	r.StopThisProcess = func() error { calls++; return nil }
	f := preset.Parse(t, `suspend; echo "st=$?"`)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 0 {
		t.Errorf("the hook was called %d times for a login shell, want never", calls)
	}
	want := "zsh:suspend:1: can't suspend login shell\nst=1\n"
	if got := out.String(); got != want {
		t.Errorf("output %q, want %q", got, want)
	}
}
