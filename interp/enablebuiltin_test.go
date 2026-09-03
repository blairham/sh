// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `enable` names a shell's own commands, and turning one off is real: the
// word stops resolving to a builtin and is looked up like any other.
func TestEnableTurnsABuiltinOffAndOnAgain(t *testing.T) {
	for _, c := range []struct {
		name, src, want, gone string
	}{
		{"a builtin is one to begin with", "type cd\n", "cd is a shell builtin", ""},
		{"turned off, it is not", "enable -n cd\ntype cd\n", "", "shell builtin"},
		{"and turning it on again brings it back", "enable -n cd\nenable cd\ntype cd\n", "cd is a shell builtin", ""},
		{"naming one that is not a builtin", "enable nosuchthing\n", "enable: nosuchthing: not a shell builtin", ""},
		{"and turning that one off", "enable -n nosuchthing\n", "enable: nosuchthing: not a shell builtin", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := enableRun(t, c.src, nil)
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
		})
	}
}

// A name that is not a builtin is a failure, whether it was being turned on
// or off.
func TestEnableReportsANameThatIsNotABuiltin(t *testing.T) {
	for _, src := range []string{"enable nosuchthing\n", "enable -n nosuchthing\n"} {
		out := enableRun(t, src+"echo \"st=$?\"\n", nil)
		if !strings.Contains(out, "st=1") {
			t.Errorf("%q said %q, want status 1", src, out)
		}
	}
}

// The listing is the command that would put each name back the way it is, so
// it can be read back in — and it is sorted, because a map is not and a
// listing that differed between two runs of the same shell would be the one
// thing a listing must not do.
func TestEnableListsTheNamesInOrder(t *testing.T) {
	out := enableRun(t, "enable\n", nil)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 10 {
		t.Fatalf("listed %d lines, want the whole table: %q", len(lines), out)
	}
	var last string
	for _, l := range lines {
		name, ok := strings.CutPrefix(l, "enable ")
		if !ok {
			t.Fatalf("line %q is not the command that would turn it on", l)
		}
		if name < last {
			t.Errorf("listing is not sorted: %q came after %q", name, last)
		}
		last = name
	}
	// A name turned off leaves the enabled listing for the other one, which
	// is written with the option that would turn it off again.
	if got := strings.TrimSpace(enableRun(t, "enable -n cd\nenable -n\n", nil)); got != "enable -n cd" {
		t.Errorf("disabled listing = %q, want %q", got, "enable -n cd")
	}
	// Two of them, because one cannot show an order — and this listing is
	// sorted where it is built rather than by the shared name table.
	want := "enable -n alias\nenable -n cd"
	if got := strings.TrimSpace(enableRun(t, "enable -n cd\nenable -n alias\nenable -n\n", nil)); got != want {
		t.Errorf("disabled listing = %q, want %q", got, want)
	}
	if strings.Contains(enableRun(t, "enable -n cd\nenable\n", nil), "enable cd\n") {
		t.Error("a name that was turned off is still in the enabled listing")
	}
}

// Turning one off and on again gets back whatever was registered, not the
// core's version of the name.
//
// This is the whole reason a name is set aside rather than unregistered: a
// dialect replaces builtins — `source` is `.` under another name, `declare`
// is `typeset` — and forgetting that would quietly restore something else.
func TestEnableRestoresTheRegisteredBuiltinAndNotTheCores(t *testing.T) {
	setup := func(r *Runner) {
		r.Register("cd", func(rr *Runner, _ context.Context, _ []string) int {
			_, _ = fmt.Fprintln(rr.Out(), "the replacement ran")
			return 0
		})
	}
	if out := enableRun(t, "cd\n", setup); !strings.Contains(out, "the replacement ran") {
		t.Fatalf("said %q, want the registered builtin to run to begin with", out)
	}
	if out := enableRun(t, "enable -n cd\nenable cd\ncd\n", setup); !strings.Contains(out, "the replacement ran") {
		t.Errorf("said %q, want the *registered* builtin back rather than the core's", out)
	}
}

func enableRun(t *testing.T, src string, setup func(*Runner)) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"}
	if setup != nil {
		setup(r)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
