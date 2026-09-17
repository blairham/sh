// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The word standing where a coprocess name belongs is expanded when the
// clause runs, so `coproc $v { … }` publishes the ends under whatever `$v`
// held. See Runner.coprocName.
func TestACoprocessNameIsExpandedWhenTheClauseRuns(t *testing.T) {
	out, st := runCoprocName(t,
		`v=UP
coproc $v { /bin/cat; }
echo hey >&"${UP[1]}"
read -r l <&"${UP[0]}"
echo "got=$l"
exec {UP[1]}>&-
wait "$UP_PID"`)
	if !strings.Contains(out, "got=hey") || st != 0 {
		t.Errorf("got %q at %d, want the ends published under UP", out, st)
	}
}

// And a word that is no name is **reported when the clause runs**, at 1, with
// the body never started and the script carrying on.
//
// The three parts are asserted together because each of them alone has a
// wrong answer that looks right: reporting without the status reads as a
// warning, the status without the report reads as a silent failure, and
// either without `reached-after` is what a parse-time refusal did — it ended
// the script and took every later line with it.
func TestACoprocessNameThatIsNoNameIsRefusedAtRunTime(t *testing.T) {
	for _, name := range []string{"@", "a-b", "1x", "a=b"} {
		out, st := runCoprocName(t,
			// The body writes to *stderr*: a coprocess's standard output is
			// the pipe the shell keeps, so a body echoing there could never
			// show up in this buffer and the assertion below would hold for
			// a shell that ran it.
			"coproc "+name+" { echo BODY-RAN >&2; }\necho \"st=$?\"\nwait\necho reached-after")
		if !strings.Contains(out, "`"+name+"': not a valid identifier") {
			t.Errorf("%q: got %q, want the identifier complaint", name, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%q: got %q, want status 1 for the clause", name, out)
		}
		if strings.Contains(out, "BODY-RAN") {
			t.Errorf("%q: got %q, want the body left unrun", name, out)
		}
		if !strings.Contains(out, "reached-after") || st != 0 {
			t.Errorf("%q: got %q at %d, want the script to carry on", name, out, st)
		}
	}
}

// A name that is only a name *after* expansion is taken, and one that stops
// being one after expansion is refused — which is what says the judgement is
// made on the value and not on the source text.
func TestACoprocessNameIsJudgedAfterExpansionAndNotBefore(t *testing.T) {
	out, _ := runCoprocName(t, `v="a b"
coproc $v { echo BODY-RAN >&2; }
echo "st=$?"
wait`)
	if !strings.Contains(out, "`a b': not a valid identifier") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the expanded text refused", out)
	}
	if strings.Contains(out, "BODY-RAN") {
		t.Errorf("got %q, want the body left unrun", out)
	}
}

// runCoprocName runs src with the coprocess word, its name, and the
// subscripted descriptor spelling that closing a feed needs, and hands back
// both streams so a refusal can be read beside what the script printed.
func runCoprocName(t *testing.T, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.Coproc = true
	d.CoprocName = true
	d.FdVariableSubscript = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := PosixSemantics()
	// POSIX has no coprocess, so it answers nothing about one; these cases
	// are about the name, which is the answer the array model carries.
	sem.CoprocEndsInAnArray = Yes
	var buf output
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &d,
		Stdout: &buf, Stderr: &buf, Env: testPATH(),
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	// Before the buffer is read: a clause that started a coprocess has a job
	// still writing into it, and a refused one may have started none.
	settle(r)
	return buf.String(), st
}
