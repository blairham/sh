// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `suspend`, in the direction a script sees it.
//
// Measured 2026-09-15 against bash 5.3.15, `env -i` with a scratch HOME and no
// startup files. Every probe that can stop was run in a process group of its
// own and killed from outside — see dialect/bash/suspend.go, and #2557 for
// what happens to a harness that forgets.
//
// There is deliberately **no corpus row**. `suspend` with no arguments stops
// zsh outright, `make oracle` runs every row in every column, and a stopped
// process does not answer the harness's SIGTERM: the row would hang the
// instrument rather than fail it. The measurements live here and in the
// implementation's comment instead.
func TestSuspendRefusesTheWayBashDoes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		// The common case, and the only one most scripts ever reach: `bash
		// -c` runs no monitor, so the builtin says so.
		{
			name: "no job control", src: `suspend; echo "st=$?"`,
			want: "bash: line 1: suspend: cannot suspend: no job control\nst=1\n", status: 0,
		},
		// An unknown letter is the complaint and then the usage line — and
		// the usage line carries no location, which is this shell's shape
		// for one.
		{
			name: "a bad option", src: `suspend -q; echo "st=$?"`,
			want: "bash: line 1: suspend: -q: invalid option\nsuspend: usage: suspend [-f]\nst=2\n",
		},
		// `-f` is read before the bad letter beside it and does not save it.
		{
			name: "a bad option after -f", src: `suspend -f -q; echo "st=$?"`,
			want: "bash: line 1: suspend: -q: invalid option\nsuspend: usage: suspend [-f]\nst=2\n",
		},
		// This builtin takes no operands, and the refusal is a usage error
		// rather than an ordinary one: 2, the same status the bad option
		// above gives. Re-measured on bash 5.3.15, 2026-09-16 — these two
		// rows said 1 until then, which is what the implementation's comment
		// had recorded and neither was right.
		{
			name: "an operand", src: `suspend x; echo "st=$?"`,
			want: "bash: line 1: suspend: too many arguments\nst=2\n",
		},
		{
			name: "an operand after --", src: `suspend -- x; echo "st=$?"`,
			want: "bash: line 1: suspend: too many arguments\nst=2\n",
		},
		{
			name: "two operands", src: `suspend a b; echo "st=$?"`,
			want: "bash: line 1: suspend: too many arguments\nst=2\n",
		},
		// `--` on its own ends the options and leaves the ordinary refusal.
		{
			name: "only --", src: `suspend --; echo "st=$?"`,
			want: "bash: line 1: suspend: cannot suspend: no job control\nst=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The monitor is what the refusal turns on, which is the row that tells a
// builtin reading the state from a builtin that always refuses: with `set -m`
// the job-control sentence is gone and what is left is the runner having no
// way to stop a process it does not own.
func TestSuspendAsksTheMonitorAndThenTheHook(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		want  string
		login bool
	}{
		{
			name: "the monitor clears the refusal",
			src:  `set -m; suspend; echo "st=$?"`,
			want: "bash: line 1: suspend: this shell was not given a way to stop this process\nst=1\n",
		},
		{
			// And `-f` clears it without the monitor, which is the whole of
			// what that letter is for.
			name: "-f clears it too",
			src:  `suspend -f; echo "st=$?"`,
			want: "bash: line 1: suspend: this shell was not given a way to stop this process\nst=1\n",
		},
		{
			// A login shell is refused even with the monitor running, and
			// bash answers it with the same sentence rather than one of its
			// own — measured: `bash -l -c 'suspend'`.
			name: "a login shell is refused with the monitor on", login: true,
			src:  `set -m; suspend; echo "st=$?"`,
			want: "bash: line 1: suspend: cannot suspend: no job control\nst=1\n",
		},
		{
			name: "and -f forces past a login shell", login: true,
			src:  `suspend -f; echo "st=$?"`,
			want: "bash: line 1: suspend: this shell was not given a way to stop this process\nst=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Terminal, because `set -m` is granted with one and declined
			// without: a runner that had none would be asserting the
			// monitor's refusal and calling it this builtin's.
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Terminal: true, LoginShell: tc.login,
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

// And the stop itself, which interp never makes: [interp.Runner.StopThisProcess]
// is nil in a library and filled in by driver, so what this asserts is that
// the builtin reaches it and reports what it reports.
//
// The hook here returns rather than stopping anything, which is what makes the
// row runnable at all — a test that really stopped would stop the test binary,
// and `go test` would wait for a SIGCONT nobody sends.
func TestSuspendCallsTheHookAndReportsAfterIt(t *testing.T) {
	t.Run("called once, and 0 after it returns", func(t *testing.T) {
		var out strings.Builder
		r := preset.Runner(dialecttest.Base{
			Dir: t.TempDir(), Stdout: &out, Stderr: &out, Terminal: true,
		})
		calls := 0
		r.StopThisProcess = func() error { calls++; return nil }
		f := preset.Parse(t, `set -m; suspend; echo "st=$?"`)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run: %v", err)
		}
		if calls != 1 {
			t.Errorf("the hook was called %d times, want once", calls)
		}
		if got, want := out.String(), "st=0\n"; got != want {
			t.Errorf("output %q, want %q", got, want)
		}
	})
	t.Run("a hook that fails is reported", func(t *testing.T) {
		var out strings.Builder
		r := preset.Runner(dialecttest.Base{
			Dir: t.TempDir(), Stdout: &out, Stderr: &out, Terminal: true,
		})
		r.StopThisProcess = func() error { return errors.New("operation not permitted") }
		f := preset.Parse(t, `set -m; suspend; echo "st=$?"`)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run: %v", err)
		}
		want := "bash: line 1: suspend: operation not permitted\nst=1\n"
		if got := out.String(); got != want {
			t.Errorf("output %q, want %q", got, want)
		}
	})
	t.Run("a refused suspend never reaches the hook", func(t *testing.T) {
		var out strings.Builder
		r := preset.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: &out, Stderr: &out})
		calls := 0
		r.StopThisProcess = func() error { calls++; return nil }
		f := preset.Parse(t, `suspend; echo "st=$?"`)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run: %v", err)
		}
		if calls != 0 {
			t.Errorf("the hook was called %d times with no monitor, want never", calls)
		}
		want := "bash: line 1: suspend: cannot suspend: no job control\nst=1\n"
		if got := out.String(); got != want {
			t.Errorf("output %q, want %q", got, want)
		}
	})
}
