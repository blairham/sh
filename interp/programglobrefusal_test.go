// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// programGlobSem refuses an unmatched pattern and answers the axis as given.
func programGlobSem(a Answer) Semantics {
	s := permissive()
	s.GlobNoMatchIsError = Yes
	s.UnmatchedPatternFailsOnlyTheProgram = a
	return s
}

// A pattern that matched nothing in a program's words fails the program and
// not the script, where the axis says so: the command does not run, is 1,
// and the next one does. Answered the other way, the refusal ends what any
// other does. See Semantics.UnmatchedPatternFailsOnlyTheProgram.
func TestAnUnmatchedPatternInAProgramsWordsFailsTheProgram(t *testing.T) {
	t.Parallel()
	const src = `/nonexistent/prog zz[x]zz; echo "after st=$?"`
	out, _ := run(t, src, withSem(programGlobSem(Yes)))
	if !strings.Contains(out, "zz[x]zz") || !strings.Contains(out, "after st=1") {
		t.Errorf("Yes: got %q, want the refusal and then after st=1", out)
	}
	if strings.Contains(out, "not found") || strings.Contains(out, "No such file") {
		t.Errorf("Yes: the program was looked for: %q", out)
	}
	out, _ = run(t, src, withSem(programGlobSem(No)))
	if strings.Contains(out, "after") {
		t.Errorf("No: the script carried on: %q", out)
	}
}

// The axis is asked only where a refusal lands in a program's words. A
// builtin's refusal, a pattern in the name, and a program whose words all
// matched ask nothing, so a dialect that has not answered is not refused on
// any of them.
func TestAnUnmatchedPatternAsksOnlyInAProgramsWords(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`echo zz[x]zz; echo after`,
		`zz[x]zz a; echo after`,
		`f() { :; }; f zz[x]zz; echo after`,
		`/nonexistent/prog a; echo after`,
	} {
		out, _ := run(t, src, withSem(programGlobSem(Unspecified)))
		if strings.Contains(out, "disagree") {
			t.Errorf("%q asked the axis: %q", src, out)
		}
	}
	out, _ := run(t, `/nonexistent/prog zz[x]zz; echo after`, withSem(programGlobSem(Unspecified)))
	if !strings.Contains(out, "disagree") {
		t.Errorf("a program's refusal did not ask: %q", out)
	}
}
