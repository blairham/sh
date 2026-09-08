// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A shell that ends leaves nothing in the temporary directory, whichever way
// it was invoked.
//
// This is #1284, and what made it a P1 rather than a tidiness note is that the
// leak was permanent and silent. A process substitution makes a directory
// under the shell's TMPDIR to hold its named pipes; the pipes go as soon as
// the command that named them ends, and nothing removed what was left. Nothing
// reports it either — the shell exits 0 and only a directory listing shows it.
// 10,585 accumulated in /tmp over one working session, and 4,188 more in the
// two days after those were swept by hand.
//
// The cause was a wiring gap and not a missing feature: Runner.CleanUp existed,
// was documented, and was tested by interp's own suite — and was called by
// nothing outside it. Every dialect binary leaked, because none of the front
// end's routes to a Runner had ever been given the line. So the assertion has
// to be about routes: fixing the one a test happens to take says nothing about
// the four it does not.
//
// # Why this counts directories rather than checking that CleanUp ran
//
// A test that asserts the call was made passes against a CleanUp that removes
// nothing, and against one called on a path the leak does not take — which is
// exactly the second half of what was wrong here. Counting what is in a
// scratch TMPDIR around a real invocation is the only assertion that can fail
// for the right reason.
//
// The whole-run guard in TestMain is the other half and the more valuable one,
// since it fails for a leak no test here thought to look for. This names the
// routes so a failure says which one.
func TestNoInvocationRouteLeavesItsPipeDirectoryBehind(t *testing.T) {
	// Measured before the fix, at this commit's parent: every row below left
	// exactly one directory per invocation, and a shell that used no
	// substitution left none — so the count is the construct's and not the
	// binary's.
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, sh driver.Shell, tmp string) (string, int)
	}{
		{"-c", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			out, _, code := runArgs(t, sh, "testsh", "-c", "cat <(echo hi)")
			return out, code
		}},
		{"a script file", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			out, _, code := runArgs(t, sh, "testsh", writeScript(t, "cat <(echo hi)\n"))
			return out, code
		}},
		{"a program on standard input", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			var o strings.Builder
			sh.Stdout = &o
			code := driver.RunStdin(sh, "cat <(echo hi)\n")
			return o.String(), code
		}},
		{"inside a function", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			out, _, code := runArgs(t, sh, "testsh", "-c", "f() { cat <(echo hi); }; f")
			return out, code
		}},
		{"inside a subshell", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			out, _, code := runArgs(t, sh, "testsh", "-c", "( cat <(echo hi) )")
			return out, code
		}},
		// The three ends that are not "the script ran out". Each was its own
		// road out of the shell before Finish took the job.
		{"after a command that failed", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			out, _, code := runArgs(t, sh, "testsh", "-c", "cat <(echo hi); false")
			return out, code
		}},
		{"after an explicit exit", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			out, _, code := runArgs(t, sh, "testsh", "-c", "cat <(echo hi); exit 3")
			return out, code
		}},
		{"from the EXIT trap, which runs after the script", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			// The one route that says *when* the cleanup happens rather than
			// whether: a trap body is more of the script and can run a
			// substitution of its own, so removing the directory before the
			// trap would break the construct instead of tidying after it.
			//
			// One axis answered, the way this package answers the ones it is
			// not about — the core refuses an unanswered one, and a test
			// about cleanup must not be deciding what a trap body does.
			sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes
			out, _, code := runArgs(t, sh, "testsh", "-c", `trap 'cat <(echo hi)' EXIT`)
			return out, code
		}},
		// A Session is the library shape of the same thing: one shell across
		// several inputs, ended by Close rather than by running out of script.
		{"a session that was closed", func(t *testing.T, sh driver.Shell, tmp string) (string, int) {
			var o strings.Builder
			sh.Stdout = &o
			s, code := driver.NewSession(sh)
			if s == nil {
				t.Fatalf("no session: status %d", code)
			}
			s.Run(context.Background(), "cat <(echo hi)")
			return o.String(), s.Close(context.Background())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			// The front end hands the Runner the process's environment, so
			// this is what aims the pipes somewhere countable. On a real
			// machine the answer is /tmp, which nothing ever empties — which
			// is what made the leak permanent.
			t.Setenv("TMPDIR", tmp)
			sh := shell()

			if n := len(pipeDirs(t, tmp)); n != 0 {
				t.Fatalf("%d directories before the run, want a clean scratch", n)
			}
			out, _ := tc.run(t, sh, tmp)
			if !strings.Contains(out, "hi") {
				t.Errorf("out = %q, want the substitution to have worked — a route "+
					"that leaves nothing behind because it did nothing proves nothing", out)
			}
			if left := pipeDirs(t, tmp); len(left) != 0 {
				t.Errorf("%d directories left after the shell ended: %v", len(left), left)
			}
		})
	}
}

// TestAShellThatNeverSubstitutesMakesNoDirectory, which is what keeps the
// count above meaningful: the directory is made when the first substitution
// needs one, so a run that leaves none has to be able to leave one.
func TestAShellThatNeverSubstitutesMakesNoDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	sh := shell()
	if out, _, code := runArgs(t, sh, "testsh", "-c", "echo plain"); out != "plain\n" || code != 0 {
		t.Fatalf("out = %q status %d", out, code)
	}
	if n := len(pipeDirs(t, tmp)); n != 0 {
		t.Errorf("%d directories from a shell that never substituted, want none", n)
	}
}

// TestManyInvocationsDoNotAccumulate is the shape the report had: not one
// directory but thousands, one per invocation, over a session's worth of
// them. One run leaving none and a hundred leaving a hundred is the failure
// that was actually shipped, and only the second says so.
func TestManyInvocationsDoNotAccumulate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	sh := shell()
	const runs = 100
	for range runs {
		if out, _, code := runArgs(t, sh, "testsh", "-c", "cat <(echo hi)"); out != "hi\n" || code != 0 {
			t.Fatalf("out = %q status %d", out, code)
		}
	}
	if left := pipeDirs(t, tmp); len(left) != 0 {
		t.Errorf("%d directories after %d invocations, want none — this was %d before the fix",
			len(left), runs, runs)
	}
}

// pipeDirs names the directories a shell made in dir to hold its pipes.
//
// By the prefix rather than by counting everything, because a test's TMPDIR is
// also where t.TempDir puts what the framework is going to remove itself.
func pipeDirs(t *testing.T, dir string) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(dir, "sh-procsub*"))
	if err != nil {
		t.Fatal(err)
	}
	return found
}
