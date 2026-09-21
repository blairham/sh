// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a child inherits, where that is not the shell's own input.
//
// A shell hands its standard input to every command it runs, and os/exec
// reads a non-file one on the child's behalf whether or not the child ever
// reads it — a pipe and a copying goroutine, so `/bin/echo hi` causes one
// Read. That is what stops a caller whose input is a *question* from simply
// filling the field in: the question would be put once per external command
// rather than once per `read`. Runner.ChildStdin is the split (#934).
//
// The rule these grade is one sentence: the substitution reaches the shell's
// own input and nothing a script put there.

// tally is a stream that says nothing and counts who asked.
type tally struct {
	reads atomic.Int64
	rest  string
}

func (c *tally) Read(p []byte) (int, error) {
	c.reads.Add(1)
	if c.rest == "" {
		return 0, io.EOF
	}
	n := copy(p, c.rest)
	c.rest = c.rest[n:]
	return n, nil
}

// childStdinRun runs src with a shell input that counts its readers and a
// caller-supplied stream for the children.
func childStdinRun(t *testing.T, src string, in io.Reader, child io.Reader, set func(*Semantics)) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := testSemantics()
	if set != nil {
		set(&sem)
	}
	r := newTestRunner(t, &Runner{
		Stdin: in, ChildStdin: child, Stdout: &o, Stderr: &e, Semantics: &sem,
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return o.String(), e.String(), st
}

// TestAChildInheritsTheShellsInputWhenNobodySaysOtherwise: the default, and
// the control that makes every case below a measurement of the field rather
// than of a shell that hands children nothing. It is also the fact the field
// exists because of — a child that reads nothing still causes a Read.
func TestAChildInheritsTheShellsInputWhenNobodySaysOtherwise(t *testing.T) {
	in := &tally{}
	out, errs, st := childStdinRun(t, `/bin/echo hi`, in, nil, nil)
	if out != "hi\n" || errs != "" || st != 0 {
		t.Fatalf("stdout %q stderr %q status %d", out, errs, st)
	}
	if in.reads.Load() == 0 {
		t.Error("a child caused no Read of the shell's input: os/exec has stopped " +
			"copying an untouched stdin, and Runner.ChildStdin's reason has expired")
	}
}

// TestAChildKeepsTheCallersStreamWhileTheShellReadsItsOwn: the split itself.
// The shell reads a person and the child reads nothing, from one command
// each, in one run — which is the arrangement an ACP session is.
func TestAChildKeepsTheCallersStreamWhileTheShellReadsItsOwn(t *testing.T) {
	in := &tally{rest: "a line\n"}
	out, errs, st := childStdinRun(t,
		`/bin/echo first; read -r line; echo "got=[$line]"`, in, noInput{}, nil)
	if want := "first\ngot=[a line]\n"; out != want || errs != "" || st != 0 {
		t.Fatalf("stdout %q stderr %q status %d, want %q", out, errs, st, want)
	}
	// The shell read it, which is the half a count can state: the line came
	// from the stream under test and not from somewhere else. What the
	// child did *not* do is the case below, where nothing reads at all and
	// the count is exactly zero.
	if in.reads.Load() == 0 {
		t.Error("the shell's own `read` never reached the shell's input")
	}
}

// TestAnExternalCommandDoesNotTouchTheShellsInput: the row the field is for.
// Two commands that read nothing, and the shell's own stream is never asked.
func TestAnExternalCommandDoesNotTouchTheShellsInput(t *testing.T) {
	in := &tally{}
	out, errs, st := childStdinRun(t, `/bin/echo one; /bin/echo two`, in, noInput{}, nil)
	if want := "one\ntwo\n"; out != want || errs != "" || st != 0 {
		t.Fatalf("stdout %q stderr %q status %d, want %q", out, errs, st, want)
	}
	if n := in.reads.Load(); n != 0 {
		t.Errorf("the shell's input was read %d times, want none: no command read anything", n)
	}
}

// TestAStreamTheScriptNamedReachesAChildUnchanged: the boundary. A
// redirection and a pipe are streams the script asked for by name, so a
// child gets each of them — a substitution that replaced whatever a child
// was about to inherit would leave `cat < f` printing nothing, at status 0.
func TestAStreamTheScriptNamedReachesAChildUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("from the file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := &tally{}
	src := `/bin/cat < ` + path + `; /bin/echo piped | /bin/cat`
	out, errs, st := childStdinRun(t, src, in, noInput{}, nil)
	if want := "from the file\npiped\n"; out != want || errs != "" || st != 0 {
		t.Fatalf("stdout %q stderr %q status %d, want %q", out, errs, st, want)
	}
	if n := in.reads.Load(); n != 0 {
		t.Errorf("the shell's input was read %d times, want none", n)
	}
}

// TestABackgroundJobIsSubstitutedThroughItsInputGuard: the wrapper the rule
// has to see through. A job started with `&` is handed the shell's input in
// the dialects that give it one at all, guarded so that two readers of one
// caller's stream do not race — and a guarded stream is a different value,
// so an identity rule that stopped at the outermost layer would hand the
// child the shell's own reader after all.
//
// Asked under the answer that hands the job the descriptor, because the
// others substitute an empty stream before this rule is reached and could
// not tell the two apart (#1287).
func TestABackgroundJobIsSubstitutedThroughItsInputGuard(t *testing.T) {
	in := &tally{}
	out, errs, st := childStdinRun(t, `/bin/echo bg & wait "$!"`, in, noInput{}, func(s *Semantics) {
		s.BackgroundJobInput = BackgroundJobInputIsTheShells
	})
	if !strings.Contains(out, "bg") || st != 0 {
		t.Fatalf("stdout %q stderr %q status %d, want the job to have run", out, errs, st)
	}
	if n := in.reads.Load(); n != 0 {
		t.Errorf("the shell's input was read %d times by a background job, want none", n)
	}
}

// TestAReplacementStandInIsSubstitutedToo: the second call site of the same
// rule, which is exactly where a fix gets forgotten — `exec cmd` in a Runner
// with no replacement hook stands in with an ordinary child and hands over
// the same three streams.
func TestAReplacementStandInIsSubstitutedToo(t *testing.T) {
	in := &tally{}
	out, _, _ := childStdinRun(t, `exec /bin/echo replaced`, in, noInput{}, nil)
	if !strings.Contains(out, "replaced") {
		t.Fatalf("stdout = %q, want the stand-in to have run", out)
	}
	if n := in.reads.Load(); n != 0 {
		t.Errorf("the shell's input was read %d times by a replacement, want none", n)
	}
}

// noInput is a stream with nothing in it, standing in for the null device a
// caller would really hand its children.
type noInput struct{}

func (noInput) Read([]byte) (int, error) { return 0, io.EOF }
