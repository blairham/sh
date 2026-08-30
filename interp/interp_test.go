// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

func run(t *testing.T, src string, setup func(*Runner)) (out string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	// Bash's answers unless a test says otherwise. A test asserting a
	// *behaviour* has to name a dialect, because the default is the strict
	// core and the core refuses anything the shells disagree about — which
	// is exactly what these tests are full of.
	bash := bash.Semantics()
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &bash}
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
	// External commands, deliberately: `true` and `echo` are builtins and do
	// not leave the process, so the gate does not see them — which is what
	// TestBuiltinsAreNotGated asserts from the other side.
	var seen []Action
	out, st := run(t, `/usr/bin/false; /bin/echo after`, func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			seen = append(seen, a)
			if strings.HasSuffix(a.Path, "false") {
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
	_, _ = run(t, `/usr/bin/true`, func(r *Runner) {
		r.Events = SinkFunc(func(_ context.Context, e Event) { kinds = append(kinds, e.Kind) })
	})
	if len(kinds) != 2 || kinds[0] != EventCommandStart || kinds[1] != EventCommandEnd {
		t.Errorf("events = %v, want a start then an end", kinds)
	}
}

func TestEveryConstructTheParserProducesCanRun(t *testing.T) {
	// This began as a list of what was refused — pipelines, subshells,
	// groups, `if`, then `[[ ]]` and `(( ))`, then background commands — and
	// it shrank to nothing. Inverted, it is worth more than it was: every
	// node the parser can produce must be executable, so a construct added to
	// the grammar without an interpreter for it fails here rather than
	// silently doing nothing at a prompt.
	for _, src := range []string{
		`:`,                              // simple command
		`: | :`,                          // pipeline
		`: && : || :`,                    // and-or
		`( : )`,                          // subshell
		`{ :; }`,                         // group
		`if :; then :; fi`,               // if
		`while false; do :; done`,        // while
		`until :; do :; done`,            // until
		`for i in a; do :; done`,         // for
		`case x in x) :;; esac`,          // case
		`f() { :; }; f`,                  // function
		`[[ -n x ]]`,                     // test clause
		`(( 1 ))`,                        // arithmetic command
		`: &`,                            // background
		`x=1`,                            // assignment
		`a=(1 2)`,                        // array assignment
		`: >/dev/null`,                   // redirection
		`echo "$(:)" "${x:-y}" "$((1))"`, // the three substitutions
	} {
		out, st := run(t, src, nil)
		if strings.Contains(out, "not implemented") {
			t.Errorf("%s: %s", src, strings.TrimSpace(out))
		}
		if st == -1 {
			t.Errorf("%s: refused as unsupported", src)
		}
	}
}

func TestPositionalParameters(t *testing.T) {
	// The rows docs/spec/grammar/word-splitting.md measured for `"$@"` and
	// `"$*"`, which are the only place quoting produces *more* than one field.
	tests := []struct{ name, src, want string }{
		{"at keeps one field per parameter", `set -- p q r; printf "[%s]" "$@"`, `[p][q][r]`},
		{"at keeps spaces within a parameter", `set -- "a b" c; printf "[%s]" "$@"`, `[a b][c]`},
		{"star joins into one field", `set -- p q r; printf "[%s]" "$*"`, `[p q r]`},
		{"star joins with IFS's first character", `set -- a b; IFS=:; printf "[%s]" "$*"`, `[a:b]`},
		{"count", `set -- p q r; echo $#`, "3\n"},
		{"a numbered parameter", `set -- p q r; echo $2`, "q\n"},
		{"past the end is empty", `set -- p; echo "[$5]"`, "[]\n"},
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

func TestAtIsZeroFieldsAndStarIsOne(t *testing.T) {
	// Why `set -- "$@"` is safe on an empty list and `set -- "$*"` is not.
	if got, _ := run(t, `set --; set -- "$@"; echo $#`, nil); got != "0\n" {
		t.Errorf(`"$@" with no parameters gave %q, want 0`, got)
	}
	if got, _ := run(t, `set --; set -- "$*"; echo $#`, nil); got != "1\n" {
		t.Errorf(`"$*" with no parameters gave %q, want 1 empty field`, got)
	}
}

func TestBuiltinsRunInThisShell(t *testing.T) {
	// The reason they are builtins: a child process could not do this.
	if got, _ := run(t, `set -- a b; shift; printf "[%s]" "$@"`, nil); got != "[b]" {
		t.Errorf("shift gave %q", got)
	}
	if got, _ := run(t, `x=1; unset x; printf "[%s]" "${x-gone}"`, nil); got != "[gone]" {
		t.Errorf("unset gave %q", got)
	}
	if _, st := run(t, `:`, nil); st != 0 {
		t.Errorf(": exited %d", st)
	}
}

func TestBuiltinsAreNotGated(t *testing.T) {
	// The gate covers what leaves the process. A builtin does not, so gating
	// it would be reporting an action that never happened.
	var seen int
	_, _ = run(t, `set -- a; shift; :`, func(r *Runner) {
		r.Gate = GateFunc(func(context.Context, Action) Decision { seen++; return Allow })
	})
	if seen != 0 {
		t.Errorf("the gate saw %d actions for builtins alone, want 0", seen)
	}
}

func TestAssignmentPrefixIsAnAxisWithTwoSides(t *testing.T) {
	// POSIX keeps an assignment prefixed to a special builtin; dash and
	// ksh93 comply and bash and zsh do not. An axis is only pinned by
	// testing both of its positions — asserting one is asserting a default.
	src := `set -- a b; x=1 shift; printf "[%s]" "$x"`

	posix := PosixSemantics()
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &posix }); got != "[1]" {
		t.Errorf("under posix the assignment should persist, got %q", got)
	}
	bash := bash.Semantics()
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &bash }); got != "[]" {
		t.Errorf("under bash it should not, got %q", got)
	}
}

func TestOnlyExportedVariablesReachTheEnvironment(t *testing.T) {
	out, _ := run(t, `x=private; export y=shared; env`, nil)
	if strings.Contains(out, "x=private") {
		t.Error("an unexported shell variable reached the environment")
	}
	if !strings.Contains(out, "y=shared") {
		t.Error("an exported variable did not reach the environment")
	}
}
