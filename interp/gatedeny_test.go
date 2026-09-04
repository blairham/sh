// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// A denied stat answers as a missing path does, quietly. The gate hiding a
// path and the kernel not having it are meant to be indistinguishable: to
// the construct asking, a path the policy hides does not exist.
func TestADeniedStatReadsAsAMissingPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "present")
	if err := os.WriteFile(file, []byte(":\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	denyStats := GateFunc(func(_ context.Context, a Action) Decision {
		if a.Kind == ActionStat {
			return Deny
		}
		return Allow
	})
	newRunner := func(out, errs *strings.Builder) *Runner {
		sem := PosixSemantics()
		return &Runner{Semantics: &sem, Dir: dir, Stdout: out, Stderr: errs, Gate: denyStats}
	}
	runSrc := func(t *testing.T, src string) (string, string, int) {
		t.Helper()
		var out, errs strings.Builder
		r := newRunner(&out, &errs)
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		status, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		return out.String(), errs.String(), status
	}

	t.Run("the test builtin goes false", func(t *testing.T) {
		out, errs, _ := runSrc(t, "test -f "+file+"; echo st=$?")
		if out != "st=1\n" {
			t.Errorf("out = %q, want the hidden file to test as absent", out)
		}
		if errs != "" {
			t.Errorf("stderr = %q, want the refusal to be quiet", errs)
		}
	})
	t.Run("a conditional file test goes false", func(t *testing.T) {
		out, errs, _ := runSrc(t, "[[ -e "+file+" ]]; echo st=$?")
		if out != "st=1\n" {
			t.Errorf("out = %q, want the hidden file to read as absent", out)
		}
		if errs != "" {
			t.Errorf("stderr = %q, want the refusal to be quiet", errs)
		}
	})
	t.Run("cd fails as it does for a missing directory", func(t *testing.T) {
		out, errs, status := runSrc(t, "cd "+dir)
		if status == 0 {
			t.Error("cd into a hidden directory reported success")
		}
		if !strings.Contains(errs, "No such file or directory") {
			t.Errorf("said %q, want the missing-directory sentence, not a refusal of its own", errs)
		}
		if out != "" {
			t.Errorf("out = %q, want nothing", out)
		}
	})
	t.Run("a PATH search finds nothing and runs nothing", func(t *testing.T) {
		// The ordering half of the claim: with every candidate's probe
		// denied, the walk resolves nothing — the command is not found, and
		// the exec gate is never shown a path the search turned up, because
		// the deny short-circuited the search before it turned one up.
		var out, errs strings.Builder
		var resolved []string
		sem := PosixSemantics()
		r := &Runner{
			Semantics: &sem, Dir: dir, Stdout: &out, Stderr: &errs,
			Gate: GateFunc(func(_ context.Context, a Action) Decision {
				if a.Kind == ActionExec && strings.Contains(a.Path, dir) {
					resolved = append(resolved, a.Path)
				}
				if a.Kind == ActionStat {
					return Deny
				}
				return Allow
			}),
		}
		f, err := syntax.Parse("PATH="+dir+"\npresent; echo st=$?", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "st=127\n" {
			t.Errorf("out = %q, want the command not found", got)
		}
		if len(resolved) != 0 {
			t.Errorf("the search resolved %v through probes the gate denied", resolved)
		}
	})
}

// A denied directory read yields no entries: the pattern misses, exactly as
// it does over a directory the process may not list.
func TestADeniedDirectoryReadMatchesNothing(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errs strings.Builder
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem, Dir: dir, Stdout: &out, Stderr: &errs,
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionReadDir {
				return Deny
			}
			return Allow
		}),
	}
	f, err := syntax.Parse("echo *", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "*\n" {
		t.Errorf("out = %q, want the unmatched pattern passed through", got)
	}
	if errs.String() != "" {
		t.Errorf("stderr = %q, want the refusal to be quiet", errs.String())
	}
}

// `.` denied its file reads is reported the way `.` refused its file by the
// kernel is: same diagnostic path, and above all the file never runs.
func TestADeniedSourceReadFailsLikeAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	sourced := filepath.Join(dir, "sourced.sh")
	if err := os.WriteFile(sourced, []byte("echo ran-anyway\n"), 0o600); err != nil {
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
	f, err := syntax.Parse(". "+sourced, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "ran-anyway") {
		t.Errorf("out = %q — the file ran without its open", out.String())
	}
	if status == 0 {
		t.Error("a file the gate withheld was reported as sourced")
	}
	if !strings.Contains(errs.String(), sourced) {
		t.Errorf("said %q, want the file named the way an unreadable one is", errs.String())
	}
}

// A denied process substitution aborts the command that named it, the way a
// refused redirect aborts its command: before the inner command exists.
func TestADeniedProcessSubstitutionAbortsTheCommand(t *testing.T) {
	var out, errs strings.Builder
	var mu sync.Mutex
	var execs []string
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem, Stdout: &out, Stderr: &errs,
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			mu.Lock()
			defer mu.Unlock()
			if a.Kind == ActionExec {
				execs = append(execs, filepath.Base(a.Path))
			}
			if a.Kind == ActionOpen {
				return Deny
			}
			return Allow
		}),
	}
	f, err := syntax.Parse("/bin/cat <(/bin/echo hi)", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	r.CleanUp()
	if out.String() != "" {
		t.Errorf("out = %q, want nothing — the substitution was refused", out.String())
	}
	if status == 0 {
		t.Error("a command whose substitution was refused reported success")
	}
	if !strings.Contains(errs.String(), "refused") {
		t.Errorf("said %q, want the refusal reported", errs.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(execs) != 0 {
		t.Errorf("execs asked about: %v, want none — neither the inner command nor the outer ran", execs)
	}
}

// A refusal delivered on a background job's goroutine is still a refusal:
// reported, and the job fails. This is the concurrency contract on Gate,
// exercised on the deny path — the race detector grades it.
func TestARefusalReachesABackgroundJob(t *testing.T) {
	var out, errs strings.Builder
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem, Stdout: &out, Stderr: &errs,
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionExec {
				return Deny
			}
			return Allow
		}),
	}
	f, err := syntax.Parse("/bin/echo hi & wait $!; echo st=$?", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "st=126\n" {
		t.Errorf("out = %q, want the job to have failed with the refusal's status and printed nothing", got)
	}
	if !strings.Contains(errs.String(), "refused") {
		t.Errorf("said %q, want the refusal reported from the job", errs.String())
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
