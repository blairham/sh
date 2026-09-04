// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// execSemantics answers the axes this file needs, so a test about one is not
// tripped by another going unanswered.
func execSemantics() Semantics {
	s := permissive()
	s.ExecFailureRunsExitTrap = Yes
	s.ExecTakesOptions = Yes
	return s
}

// execRun runs src and hands back both streams and the runner, so a test can
// look at what the runner ended up believing as well as what it printed.
func execRun(t *testing.T, dir, src string, setup func(*Runner)) (string, int, *Runner) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := execSemantics()
	dg := Diagnostics{}
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Env: testPATH(),
	}
	if setup != nil {
		setup(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1, r
	}
	return buf.String(), st, r
}

// TestReplaceProcessIsOptIn is the most important test in this file, and it is
// about the package being a library rather than about shell semantics.
//
// A Runner embedded in some other program must not be able to replace that
// program with whatever a script named. So the zero value does not, and a
// caller opts in — which means the default has to be checked, because the
// dangerous version is the one that looks like it works.
func TestReplaceProcessIsOptIn(t *testing.T) {
	if (&Runner{}).ReplaceProcess != nil {
		t.Fatal("a zero Runner must not be able to replace its process")
	}

	// Without the hook, `exec` still behaves: the command runs and the script
	// stops. That is what makes the safe default usable rather than a
	// degraded mode.
	out, st, _ := execRun(t, t.TempDir(), `exec echo replaced; echo NOT-REACHED`, nil)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("nothing after exec should run: %q", out)
	}
	if got := strings.TrimSpace(out); got != "replaced" {
		t.Errorf("output = %q, want replaced", got)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// TestASubshellNeverReplacesTheProcess is the bug this found, and it is the
// reason the opt-in is not the end of the story.
//
// A subshell in a real shell is a separate process, so `( exec echo hi ); echo
// after` prints both lines. Here a subshell is a cloned Runner in the *same*
// process, so calling execve in one would take the parent shell with it — and
// it did: the `echo after` vanished from all four dialect binaries.
func TestASubshellNeverReplacesTheProcess(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a subshell", `( exec echo inner ); echo after`},
		{"a pipeline element, which is a subshell by another name", `exec echo inner | cat; echo after`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			out, _, _ := execRun(t, t.TempDir(), tc.src, func(r *Runner) {
				r.ReplaceProcess = func(string, []string, []string) error {
					called = true
					return errors.New("must not be reached")
				}
			})
			if called {
				t.Error("execve in a shared-process subshell would replace the parent shell")
			}
			if !strings.Contains(out, "after") {
				t.Errorf("the parent shell should carry on: %q", out)
			}
			if !strings.Contains(out, "inner") {
				t.Errorf("the subshell's command should still run: %q", out)
			}
		})
	}
}

// TestReplaceProcessIsUsedWhenItIsSafe is the other half: outside a subshell,
// an opted-in caller does get the real thing.
func TestReplaceProcessIsUsedWhenItIsSafe(t *testing.T) {
	var gotPath string
	var gotArgv []string
	out, st, _ := execRun(t, t.TempDir(), `exec echo hi; echo NOT-REACHED`, func(r *Runner) {
		r.ReplaceProcess = func(path string, argv, _ []string) error {
			gotPath, gotArgv = path, argv
			// A real replacement does not return. Returning an error is how a
			// test says "the image could not be replaced", which is the only
			// case the caller can observe.
			return os.ErrPermission
		}
	})
	if gotPath == "" {
		t.Fatal("ReplaceProcess was not called outside a subshell")
	}
	if len(gotArgv) == 0 || gotArgv[0] != "echo" {
		t.Errorf("argv = %v, want it to start with the command", gotArgv)
	}
	// A failed replacement is a failed exec, and the script still stops.
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("a failed replacement still ends the script: %q", out)
	}
	if st != 126 {
		t.Errorf("status = %d, want 126 for a command that would not run", st)
	}
}

// TestASuccessfulExecRunsNoExitTrap is unanimous across the panel and is not a
// special case in a real shell — the trap died with the process the exec
// replaced. An implementation standing in a child has to say so explicitly, or
// it prints a handler nothing else prints.
func TestASuccessfulExecRunsNoExitTrap(t *testing.T) {
	out, _, _ := execRun(t, t.TempDir(), `trap "echo TRAP" EXIT; exec echo hi`, nil)
	if strings.Contains(out, "TRAP") {
		t.Errorf("no shell in the panel runs an EXIT trap after a successful exec: %q", out)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("the command should still have run: %q", out)
	}
}

// TestAFailedExecRunsTheExitTrapOnlyWhenTheDialectSaysSo is the axis, in both
// directions. The failure is the only case with a shell left to decide.
func TestAFailedExecRunsTheExitTrapOnlyWhenTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		runs Answer
		want bool
	}{
		{"the dialect runs it", Yes, true},
		{"the dialect drops it", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := execSemantics()
			sem.ExecFailureRunsExitTrap = tc.runs
			f, err := syntax.Parse(`trap "echo TRAP" EXIT; exec nosuchcmd-xyz`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			dg := Diagnostics{}
			r := &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
				Dir: t.TempDir(), Name: "testsh", Env: testPATH(),
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(buf.String(), "TRAP"); got != tc.want {
				t.Errorf("trap ran = %v, want %v (output %q)", got, tc.want, buf.String())
			}
		})
	}
}

// TestExecWithNoWordsKeepsItsRedirections is the other exec, which shares
// nothing with the first but a name.
func TestExecWithNoWordsKeepsItsRedirections(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")

	_, st, _ := execRun(t, dir, `exec > `+out+`; echo one; echo two`, nil)
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != "one\ntwo" {
		t.Errorf("file = %q, want both lines: the redirection has to outlive the exec", got)
	}
}

// TestExecWithNoWordsClearsAFailure is the same surprise as an empty eval: it
// reports success rather than preserving the status before it.
func TestExecWithNoWordsClearsAFailure(t *testing.T) {
	out, _, _ := execRun(t, t.TempDir(), `false; exec; echo st=$?`, nil)
	if got := strings.TrimSpace(out); got != "st=0" {
		t.Errorf("output = %q, want st=0", got)
	}
}

// TestARedirectionOnlyExecDoesNotEndTheScript separates the two forms: this
// one carries on, and a real EXIT trap still fires at the end.
func TestARedirectionOnlyExecDoesNotEndTheScript(t *testing.T) {
	dir := t.TempDir()
	out, _, _ := execRun(t, dir,
		`trap "echo TRAP" EXIT; exec 2>`+filepath.Join(dir, "e.txt")+`; echo after`, nil)
	if !strings.Contains(out, "after") {
		t.Errorf("the script should carry on: %q", out)
	}
	if !strings.Contains(out, "TRAP") {
		t.Errorf("the shell is still there, so its EXIT trap still fires: %q", out)
	}
}

// TestTheStatusSeparatesMissingFromUnrunnable is the distinction a single
// error type loses: 127 for a command that is not there, 126 for a file that
// is there and will not run. Both unanimous across the panel.
func TestTheStatusSeparatesMissingFromUnrunnable(t *testing.T) {
	dir := t.TempDir()
	unrunnable := filepath.Join(dir, "ne.sh")
	if err := os.WriteFile(unrunnable, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"a bare name that is not on PATH", `exec nosuchcmd-xyz`, 127},
		{"a path that does not exist", `exec ` + filepath.Join(dir, "absent"), 127},
		{"a file without the execute bit", `exec ` + unrunnable, 126},
		{"a directory", `exec ` + dir, 126},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st, _ := execRun(t, dir, tc.src, nil)
			if st != tc.want {
				t.Errorf("status = %d, want %d", st, tc.want)
			}
		})
	}
}

// TestGoErrorTextDoesNotReachADiagnostic covers a leak rather than a rule.
//
// Go wraps a lookup failure in exec.Error, whose text is Go's: `exec: "x":
// executable file not found in $PATH`. That reached four dialects' diagnostics
// before the error was unwrapped, where every real shell says "not found".
func TestGoErrorTextDoesNotReachADiagnostic(t *testing.T) {
	dir := t.TempDir()
	unrunnable := filepath.Join(dir, "ne.sh")
	if err := os.WriteFile(unrunnable, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, src, want string }{
		{"a name that is not on PATH", `exec nosuchcmd-xyz`, "not found"},
		{
			// The case that actually exercises the unwrapping: LookPath wraps
			// a permission error in exec.Error too, and only this path reaches
			// the error underneath. Checking the not-found case alone left the
			// unwrap untested — it returns before ever using it.
			"a file that is there and will not run",
			`exec ` + unrunnable, "Permission denied",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := execRun(t, dir, tc.src, nil)
			for _, leak := range []string{"executable file not found in $PATH", `exec: "`} {
				if strings.Contains(out, leak) {
					t.Errorf("output %q leaks Go's own error text (%q)", out, leak)
				}
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want a shell's wording (%q)", out, tc.want)
			}
		})
	}
}

// TestExecOptionsAreADialectQuestion pins that the answer comes before the
// lookup, because it decides which word the command is.
func TestExecOptionsAreADialectQuestion(t *testing.T) {
	// With options, `-a` is consumed and the command is what follows.
	sem := execSemantics()
	sem.ExecTakesOptions = Yes
	var argv []string
	f, err := syntax.Parse(`exec -a myname echo hi`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	dg := Diagnostics{}
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh", Env: testPATH(),
		ReplaceProcess: func(_ string, a, _ []string) error {
			argv = a
			return os.ErrPermission
		},
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if len(argv) == 0 || argv[0] != "myname" {
		t.Errorf("argv = %v, want -a to have replaced argv[0]", argv)
	}

	// Without options, the same `-a` is the name of a command and is reported
	// as not found rather than eaten.
	sem2 := execSemantics()
	sem2.ExecTakesOptions = No
	f2, err := syntax.Parse(`exec -a myname echo hi`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf2 bytes.Buffer
	dg2 := Diagnostics{}
	r2 := &Runner{
		Stdout: &buf2, Stderr: &buf2, Semantics: &sem2, Diagnostics: &dg2,
		Dir: t.TempDir(), Name: "testsh", Env: testPATH(),
	}
	st, err := r2.Run(context.Background(), f2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf2.String(), "-a") {
		t.Errorf("output = %q, want -a reported as the command", buf2.String())
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestADeniedExecLeavesTheShellRunning is the seam's rule applied here. A
// denied action is a command that failed, not a broken shell — and unlike
// every other exec failure this one must *not* end the script, because a gate
// that can end a script is a gate that can break one.
func TestADeniedExecLeavesTheShellRunning(t *testing.T) {
	out, _, _ := execRun(t, t.TempDir(), `exec echo hi; echo after`, func(r *Runner) {
		r.Gate = GateFunc(func(context.Context, Action) Decision { return Deny })
	})
	if strings.Contains(out, "hi") {
		t.Errorf("the command should not have run: %q", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("a denied exec must leave the shell running: %q", out)
	}
}

// TestExecEmitsEvents covers the audit half of the seam, which a sandbox and
// an agent protocol both read.
func TestExecEmitsEvents(t *testing.T) {
	var kinds []EventKind
	execRun(t, t.TempDir(), `exec echo hi`, func(r *Runner) {
		r.Events = SinkFunc(func(_ context.Context, e Event) {
			kinds = append(kinds, e.Kind)
		})
	})
	var sawStart, sawEnd bool
	for _, k := range kinds {
		switch k {
		case EventCommandStart:
			sawStart = true
		case EventCommandEnd:
			sawEnd = true
		}
	}
	if !sawStart || !sawEnd {
		t.Errorf("events = %v, want a start and an end", kinds)
	}
}

// TestExecIsASpecialBuiltin closes the third of the four gaps: the table said
// so before any of them existed.
func TestExecIsASpecialBuiltin(t *testing.T) {
	if !IsSpecialBuiltin("exec") {
		t.Error("exec should be a special builtin")
	}
	if _, ok := (&Runner{}).Builtin("exec"); !ok {
		t.Error("exec is called special and does not exist")
	}
}
