// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// redirRunAt runs src with Dir set, and hands back the error as well as the
// output: a refusal here is an error from Run rather than a diagnostic.
func redirRunAt(t *testing.T, dir, src string) (string, int, error) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Dir: dir, Name: "testsh", Env: testPATH()})
	st, rerr := r.Run(context.Background(), f)
	return buf.String(), st, rerr
}

// The original bug: the descriptor was dropped and the redirection applied to
// stdout, so `exec 3>out` opened the file, said nothing, exited 0 — and sent
// every later write into it. Then it was refused outright. Now the table
// holds it, and the two halves that matter are that stdout is untouched and
// that `>&3` finds the file.
func TestAHighDescriptorOpensIntoTheTable(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	got, _, err := redirRunAt(t, dir, "exec 3>"+out+"\necho hi\necho aside >&3\nexec 3>&-\necho done")
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	// The point of the original bug: stdout must not have gone into the file.
	if !strings.Contains(got, "hi") || !strings.Contains(got, "done") {
		t.Errorf("stdout = %q, want the script's own output on it", got)
	}
	b, rerr := os.ReadFile(out)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(b) != "aside\n" {
		t.Errorf("the file holds %q, want only what was aimed at fd 3", b)
	}
}

// The saved-stream idiom every configure script uses: save stdout, redirect
// through the saved copy, close it.
func TestASavedStreamIsFoundAgain(t *testing.T) {
	dir := t.TempDir()
	got, st, err := redirRunAt(t, dir, `exec 6>&1
echo through >&6
{ echo grouped; } >&6
exec 6>&-
echo "st=$?"`)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	for _, want := range []string{"through", "grouped", "st=0"} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout = %q (status %d), missing %q", got, st, want)
		}
	}
}

// A dup that is a prefix to one command is that command's alone.
func TestAPerCommandDupIsTakenBackAfterward(t *testing.T) {
	got, _, err := redirRunAt(t, t.TempDir(), `true 6>&1; echo hi >&6; echo "st=$?"`)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !strings.Contains(got, "Bad file descriptor") || !strings.Contains(got, "st=1") {
		t.Errorf("output = %q, want the descriptor gone once the command ended", got)
	}
}

// A subshell's table is its own: what it saves the parent never sees.
func TestASubshellsDescriptorsStayInTheSubshell(t *testing.T) {
	got, _, err := redirRunAt(t, t.TempDir(), `(exec 7>&1); echo hi >&7; echo "st=$?"`)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !strings.Contains(got, "Bad file descriptor") || !strings.Contains(got, "st=1") {
		t.Errorf("output = %q, want the parent unaware of the subshell's descriptor", got)
	}
}

// Reading through a saved input descriptor, which is the `exec 5<&0` half.
func TestASavedInputStreamReadsAgain(t *testing.T) {
	dir := t.TempDir()
	got, _, err := redirRunAt(t, dir, `exec 5<&0; echo "st=$?"`)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !strings.Contains(got, "st=0") {
		t.Errorf("output = %q, want the save to succeed", got)
	}
}

// Every descriptor this shell does have still works, which is what says the
// refusal is aimed at the ones it does not.
func TestTheThreeNamedStreamsStillRedirect(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{`echo a > f; cat f`, "a"},
		{`echo a > f; echo b >> f; cat f`, "a\nb"},
		{`echo a 1> f; cat f`, "a"},
		// Not these two through `cat`: the helper gives Stdout and Stderr
		// one buffer, so what the command wrote to the terminal is mixed in
		// with what it wrote to the file. They are checked below by reading
		// the file, which is the only way to see where a stream went.
		{`echo a > f; cat < f`, "a"},
		{`echo a &> f; cat f`, "a"},
	} {
		got, _, err := redirRunAt(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if strings.TrimSpace(got) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(got), c.want)
		}
	}
	// Which stream reached the file, read from the file itself.
	for _, c := range []struct{ name, src, want string }{
		{"stderr alone", `{ echo out; echo err >&2; } 2> g`, "err\n"},
		{"both", `{ echo out; echo err >&2; } > g 2>&1`, "out\nerr\n"},
		{"stdout alone", `{ echo out; echo err >&2; } > g`, "out\n"},
	} {
		if _, _, err := redirRunAt(t, dir, c.src); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, "g"))
		if rerr != nil {
			t.Errorf("%s: %v", c.name, rerr)
			continue
		}
		if string(b) != c.want {
			t.Errorf("%s: file holds %q, want %q", c.name, b, c.want)
		}
	}
}

// `>&N` for a descriptor that was never opened already answered honestly, and
// still does — the two paths report the same impossibility differently, which
// is worth pinning so neither drifts into the other's silence.
func TestDuplicatingAnUnopenedDescriptorStillFails(t *testing.T) {
	dir := t.TempDir()
	got, st, err := redirRunAt(t, dir, `echo hi >&5; echo "st=$?"`)
	if err != nil {
		t.Fatalf("aborted where it used to report: %v", err)
	}
	if !strings.Contains(got, "Bad file descriptor") || !strings.Contains(got, "st=1") {
		t.Errorf("said %q status %d, want a reported failure", got, st)
	}
}
