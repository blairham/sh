// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/policy"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A policy that confines writes used to refuse process substitution itself.
//
// `<(cmd)` opens a FIFO under a directory the *interpreter* made, with a name
// the operating system chose; a script writes `<(cmd)` and can never write
// that path. So a rule about where the shell may write refused the mechanism
// while believing it refused an access — which is exactly what ActionOpen's
// scaffolding exemption exists to prevent, applied to everything around the
// pipe and not to the pipe.
//
// Measured before the change: this was 6 of 1426 corpus cases under the
// containment posture, and every case in `make conformance-gated` that
// differed in more than the wording of a diagnostic.
//
// Nothing here is a panel question. No real shell has a policy, so there is no
// outside behavior to match; what is measured against the world is that the
// construct still produces what it produced with no policy at all, which is
// the assertion each row below makes.

// containing is the corpus containment posture, parsed rather than
// hand-written: writes confined to one directory, everything else allowed.
//
// Through internal/policy on purpose. A selector or a pattern that does not
// mean what its author thought matches nothing, and a rule that matches
// nothing is indistinguishable from a rule being obeyed — so a hand-written
// Gate can pass a test the real policy language would fail.
func containing(t *testing.T, scratch string) *policy.Policy {
	t.Helper()
	p, err := policy.Parse(strings.NewReader(fmt.Sprintf(
		"version 1\ndefault allow\ndefault deny write\nallow write %s/**\n", scratch)))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	return p
}

// gated runs src under a policy and returns what the shell wrote.
func gated(t *testing.T, src string, g Gate, sink Sink) (out, errs string, status int) {
	t.Helper()
	var o, e output
	scratch := t.TempDir()
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem,
		Dir:       scratch,
		Stdout:    &o, Stderr: &e,
		Gate: g, Events: sink,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	status, err = r.Run(t.Context(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return o.String(), e.String(), status
}

// TestAProcessSubstitutionRunsUnderAPolicyThatConfinesWrites, in every shape
// the construct has: the shell's own end of the pipe, a redirect naming the
// pipe, and `.` reading one.
func TestAProcessSubstitutionRunsUnderAPolicyThatConfinesWrites(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an operand", `/bin/cat <(/bin/echo hi)`, "hi\n"},
		{"a redirect reading one", `read x < <(/bin/echo hi); echo "got:$x"`, "got:hi\n"},
		{"a redirect writing one", `/bin/echo out > >(/bin/cat); /bin/sleep 0.3`, "out\n"},
		{"two in one command", `/bin/cat <(/bin/echo one) <(/bin/echo two)`, "one\ntwo\n"},
		{"sourcing one", `. <(/bin/echo 'echo sourced')`, "sourced\n"},
		{"a subshell reading one", `( read x; echo "sub:$x" ) < <(/bin/echo z)`, "sub:z\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scratch := t.TempDir()
			out, errs, status := gated(t, tc.src, containing(t, scratch), nil)
			if out != tc.want || status != 0 {
				t.Errorf("out = %q status %d, want %q and 0\n\tstderr: %s",
					out, status, tc.want, errs)
			}
			if strings.Contains(errs, "refused") {
				t.Errorf("the policy refused something: %s", errs)
			}
		})
	}
}

// TestAProcessSubstitutionRunsUnderAPolicyThatConfinesReads is the other half,
// and it is the half the write-confining posture cannot reach.
//
// `.` opens its file for *reading*, and so does the shell's end of a `>(cmd)`,
// so a policy that only denies writes never touches either. A rule about where
// the shell may read is what puts both on the line — and, since the pipe lives
// under the temporary directory, a policy naming that directory is not a
// contrived one: it is what somebody writes when they mean "read only what I
// name".
func TestAProcessSubstitutionRunsUnderAPolicyThatConfinesReads(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"sourcing one", `. <(/bin/echo 'echo sourced')`, "sourced\n"},
		{"a redirect writing one", `/bin/echo out > >(/bin/cat); /bin/sleep 0.3`, "out\n"},
		{"an operand", `/bin/cat <(/bin/echo hi)`, "hi\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The shell's pipes go under TMPDIR, which newTestRunner points
			// at a directory of the test's own; denying reads beneath it is
			// what a "read only what I name" policy comes to here.
			tmp := t.TempDir()
			p, err := policy.Parse(strings.NewReader(fmt.Sprintf(
				"version 1\ndefault allow\ndeny read %s/**\n", tmp)))
			if err != nil {
				t.Fatalf("policy: %v", err)
			}
			var o, e output
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Dir:       t.TempDir(),
				Env:       []string{"TMPDIR=" + tmp},
				Stdout:    &o, Stderr: &e,
				Gate: p,
			})
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(t.Context(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := o.String(); got != tc.want {
				t.Errorf("out = %q, want %q\n\tstderr: %s", got, tc.want, e.String())
			}
			if strings.Contains(e.String(), "refused") {
				t.Errorf("the policy refused something: %s", e.String())
			}
		})
	}
}

// TestTheShellsOwnPipeIsRecordedEvenThoughItIsNotRefused.
//
// The exemption is from the *gate*, not from the record. An audit trail that
// stopped saying the shell had made a pipe would be paying for this fix with
// exactly the thing the event stream exists for, and the issue named that as
// the cost of the option it was weighing. It is not the cost of this one.
func TestTheShellsOwnPipeIsRecordedEvenThoughItIsNotRefused(t *testing.T) {
	var mu sync.Mutex
	var opens []Action
	sink := SinkFunc(func(_ context.Context, e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == EventAccess && e.Action.Kind == ActionOpen {
			opens = append(opens, e.Action)
		}
	})
	scratch := t.TempDir()
	// Both directions, because they are two branches with two records and a
	// mutation that drops one of them is invisible to a test of the other.
	// `<(cmd)` is the shell writing into the pipe; `>(cmd)` is the shell
	// reading out of it.
	src := "/bin/cat <(/bin/echo hi); /bin/echo out > >(/bin/cat); /bin/sleep 0.3"
	out, errs, status := gated(t, src, containing(t, scratch), sink)
	if out != "hi\nout\n" || status != 0 {
		t.Fatalf("out = %q status %d, stderr %s", out, status, errs)
	}
	mu.Lock()
	defer mu.Unlock()
	var got []string
	for _, a := range opens {
		if !strings.Contains(a.Path, "sh-procsub") {
			continue
		}
		kind := "read"
		if a.Write {
			kind = "write"
		}
		got = append(got, filepath.Base(a.Path)+" "+kind)
	}
	// Three, and each is a different open of the shell's own plumbing:
	// sub1 written, which is `<(cmd)` feeding the inner command's output in;
	// sub2 read, which is the shell taking `>(cmd)`'s input end; and sub2
	// written, which is the *redirect* opening the pipe it was pointed at.
	// The third is the one this fix had to reach as well as the first two.
	want := []string{"sub1 write", "sub2 read", "sub2 write"}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("the stream records %v, want %v", got, want)
	}
}

// TestThePipesNameIsExemptOnlyWhileItsCommandRuns.
//
// This is what keeps the recognition from being a hole a script can aim at.
// The set is the *command's*: a pipe is in it from the moment the word expands
// until the command that named it ends. A script that captures the path and
// comes back to it later is naming an ordinary path, and the gate is asked
// about it in the ordinary way.
func TestThePipesNameIsExemptOnlyWhileItsCommandRuns(t *testing.T) {
	scratch := t.TempDir()
	// The path is printed by one command and written by the next.
	src := `p=$(echo <(true)); echo "captured:$p" >&2; echo x > "$p"; echo "after:$?"`
	out, errs, _ := gated(t, src, containing(t, scratch), nil)
	if !strings.Contains(errs, "captured:") || !strings.Contains(errs, "sh-procsub") {
		t.Fatalf("the substitution did not expand to a pipe under it: %q", errs)
	}
	if !strings.Contains(errs, "refused") {
		t.Errorf("writing to the captured path was allowed.\n\tstdout %q\n\tstderr %q", out, errs)
	}

	// And the exemption is the pipe, not the directory it sits in. Here the
	// command *does* have a live substitution — so a recognition that matched
	// on the directory would wave through a write to a sibling path the
	// script captured earlier, which is a hole in the shape of a fix.
	sibling := `p=$(echo <(true)); /bin/cat <(true) > "$p"; echo "after:$?"`
	_, errs, _ = gated(t, sibling, containing(t, t.TempDir()), nil)
	if !strings.Contains(errs, "refused") {
		t.Errorf("a write to a neighbour of a live pipe was allowed: %q", errs)
	}
}

// TestWhatRunsInsideASubstitutionIsStillGated, which is the whole of the
// argument for the exemption: the thing worth refusing is not the pipe.
func TestWhatRunsInsideASubstitutionIsStillGated(t *testing.T) {
	t.Run("the inner command's exec", func(t *testing.T) {
		g := GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionExec && filepath.Base(a.Path) == "echo" {
				return Deny
			}
			return Allow
		})
		out, errs, _ := gated(t, `/bin/cat <(/bin/echo secret)`, g, nil)
		if strings.Contains(out, "secret") {
			t.Errorf("out = %q, want the refused command not to have run", out)
		}
		if !strings.Contains(errs, "refused") {
			t.Errorf("stderr = %q, want a refusal", errs)
		}
	})

	t.Run("a file the inner command reads", func(t *testing.T) {
		dir := t.TempDir()
		secret := filepath.Join(dir, "secret")
		if err := os.WriteFile(secret, []byte("TOPSECRET\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		p, err := policy.Parse(strings.NewReader(fmt.Sprintf(
			"version 1\ndefault allow\ndeny read %s/**\n", dir)))
		if err != nil {
			t.Fatalf("policy: %v", err)
		}
		out, _, _ := gated(t, `/bin/cat <(read x < `+secret+`; echo "$x")`, p, nil)
		if strings.Contains(out, "TOPSECRET") {
			t.Errorf("out = %q, want the denied read refused inside the substitution too", out)
		}
	})
}

// TestAnUnpoliciedProcessSubstitutionIsUnchanged, because the recognition must
// not have moved anything for a shell nobody gave a policy.
func TestAnUnpoliciedProcessSubstitutionIsUnchanged(t *testing.T) {
	out, errs, status := gated(t, `/bin/cat <(/bin/echo hi)`, nil, nil)
	if out != "hi\n" || status != 0 {
		t.Errorf("out = %q status %d stderr %q, want hi and 0", out, status, errs)
	}
}
