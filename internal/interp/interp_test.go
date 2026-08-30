// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/syntax"
)

func run(t *testing.T, src string, setup func(*Runner)) (out string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := &Runner{Stdout: &buf, Stderr: &buf}
	if setup != nil {
		setup(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

func TestFieldSplittingMatchesTheSpec(t *testing.T) {
	// The rows here are the ones docs/spec/grammar/word-splitting.md measured,
	// including the asymmetry a symmetric implementation gets wrong: a
	// trailing separator is absorbed and a leading one is not.
	tests := []struct{ name, src, want string }{
		{"whitespace runs collapse", `x="a  b   c"; printf "[%s]" $x`, `[a][b][c]`},
		{"edges are stripped", `x="  a  b  "; printf "[%s]" $x`, `[a][b]`},
		{"quoted is one field", `x="a b"; printf "[%s]" "$x"`, `[a b]`},
		{"adjacent separators make one empty field", `IFS=:; x="a::b"; printf "[%s]" $x`, `[a][][b]`},
		{"leading separator makes an empty field", `IFS=:; x=":a"; printf "[%s]" $x`, `[][a]`},
		{"trailing separator does not", `IFS=:; x="a:"; printf "[%s]" $x`, `[a]`},
		{"whitespace around a separator is one delimiter", `IFS=" :"; x="a : b"; printf "[%s]" $x`, `[a][b]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := run(t, tc.src, nil)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestColonExtendsTheTest(t *testing.T) {
	// The one rule behind all four conditionals.
	tests := []struct{ src, want string }{
		// `unset` is a builtin this slice does not have; a fresh runner has
		// no variables anyway, which is the state the case is about.
		{`printf "[%s]" "${u:-D}"`, `[D]`},
		{`e=; printf "[%s]" "${e:-D}"`, `[D]`},
		{`e=; printf "[%s]" "${e-D}"`, `[]`},
		{`s=S; printf "[%s]" "${s:-D}"`, `[S]`},
		{`e=; printf "[%s]" "${e:+A}"`, `[]`},
		{`e=; printf "[%s]" "${e+A}"`, `[A]`},
	}
	for _, tc := range tests {
		got, _ := run(t, tc.src, nil)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestAssignHasASideEffectThatOutlives(t *testing.T) {
	got, _ := run(t, `printf "[%s]" "${u:=V}"; printf "[%s]" "$u"`, nil)
	if got != `[V][V]` {
		t.Errorf("got %q, want [V][V]", got)
	}
}

func TestExitStatusAndAndOr(t *testing.T) {
	tests := []struct {
		src    string
		want   string
		status int
	}{
		{`true`, ``, 0},
		{`false`, ``, 1},
		{`! true`, ``, 1},
		{`! false`, ``, 0},
		{`true && echo yes`, "yes\n", 0},
		{`false && echo yes`, ``, 1},
		{`false || echo no`, "no\n", 0},
		// The tree already encodes one left-associative level; nothing in the
		// interpreter re-decides it.
		{`true || echo A && echo B`, "B\n", 0},
	}
	for _, tc := range tests {
		got, st := run(t, tc.src, nil)
		if got != tc.want || st != tc.status {
			t.Errorf("%s: got %q status %d, want %q status %d", tc.src, got, st, tc.want, tc.status)
		}
	}
}

func TestCommandNotFoundIs127(t *testing.T) {
	out, st := run(t, `definitely-not-a-command-xyz`, nil)
	if st != 127 {
		t.Errorf("status = %d, want 127 — the status every panel shell uses", st)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("output %q should say what happened", out)
	}
}

func TestGateRefusesAndTheShellCarriesOn(t *testing.T) {
	// A denied command is a command that failed, not a broken shell, so the
	// status is set and execution continues.
	var seen []Action
	out, st := run(t, `true; echo after`, func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			seen = append(seen, a)
			if strings.HasSuffix(a.Path, "true") {
				return Deny
			}
			return Allow
		})
	})
	if len(seen) != 2 {
		t.Fatalf("the gate saw %d actions, want 2", len(seen))
	}
	if !strings.Contains(out, "refused") {
		t.Errorf("a refusal must be reported, got %q", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("the shell must carry on after a refusal, got %q", out)
	}
	if st != 0 {
		t.Errorf("status = %d; the second command succeeded", st)
	}
}

func TestGateSeesRedirectionsToo(t *testing.T) {
	// A sandbox that only gates execution has not gated the thing that writes
	// to the filesystem.
	var opens int
	_, _ = run(t, `echo hi >`+t.TempDir()+`/f`, func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionOpen {
				opens++
				if !a.Write {
					t.Error("a > redirection should be reported as a write")
				}
			}
			return Allow
		})
	})
	if opens != 1 {
		t.Errorf("the gate saw %d opens, want 1", opens)
	}
}

func TestEventsDescribeWhatHappened(t *testing.T) {
	var kinds []EventKind
	_, _ = run(t, `true`, func(r *Runner) {
		r.Events = SinkFunc(func(_ context.Context, e Event) { kinds = append(kinds, e.Kind) })
	})
	if len(kinds) != 2 || kinds[0] != EventCommandStart || kinds[1] != EventCommandEnd {
		t.Errorf("events = %v, want a start then an end", kinds)
	}
}

func TestUnsupportedIsRefusedNotSkipped(t *testing.T) {
	// A shell that quietly does nothing is worse than one that says it cannot.
	for _, src := range []string{`a | b`, `(a)`, `{ a; }`, `if true; then a; fi`} {
		out, st := run(t, src, nil)
		if st != -1 || !strings.Contains(out, "not implemented yet") {
			t.Errorf("%s: got %q status %d, want an explicit refusal", src, out, st)
		}
	}
}
