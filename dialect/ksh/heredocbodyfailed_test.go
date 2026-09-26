// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A here-document body that will not expand, on a command this shell runs
// itself, is the **redirection's** failure here.
//
// So it draws the grid a file that will not open draws: the shell stops for a
// special builtin, which is the POSIX rule this column keeps, and the command
// alone is given up for everything else. Measured 2026-09-26 on ksh93u+
// 2012-08-01, a body of `$(( 1/0 ))` and `echo "after st=$?"` on the line
// after the delimiter (#4684).
func TestAFailedHeredocBodyIsTheRedirectionsHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`: <<END`, true},
		{`echo RAN <<END`, false},
		{`f() { echo RAN; }; f <<END`, false},
		{`{ echo RAN; } <<END`, false},
	} {
		out, st := runKsh(t, dir, c.src+"\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", c.src, out)
		}
		if ended := !strings.Contains(out, "after"); ended != c.ends {
			t.Errorf("%s = %q, want the shell ending=%v", c.src, out, c.ends)
		}
		if c.ends && st != 1 {
			t.Errorf("%s: status = %d, want 1", c.src, st)
		}
		if !c.ends && !strings.Contains(out, "after st=1") {
			t.Errorf("%s = %q, want the contained failure to leave 1", c.src, out)
		}
	}
}

// The pair that says which of the two it is: a **redirection that could not be
// opened** draws that grid row for row here, so the body's failure is graded
// as one rather than as a failed expansion — which is fatal in this column
// whatever it is written on.
func TestAFailedHeredocBodyMatchesAFailedOpenHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `echo RAN`} {
		open, openSt := runKsh(t, dir, cmd+" < /nonexistent/f\necho \"after st=$?\"\n")
		body, bodySt := runKsh(t, dir, cmd+" <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(open, "after") != strings.Contains(body, "after") || openSt != bodySt {
			t.Errorf("%s: a failed open said %q at %d and a failing body said %q at %d, "+
				"want the same reach", cmd, open, openSt, body, bodySt)
		}
	}
	// The control the comparison needs: the two commands really are graded
	// apart, so the agreement above is a grid rather than one answer twice.
	special, _ := runKsh(t, dir, ": < /nonexistent/f\necho after\n")
	regular, _ := runKsh(t, dir, "echo RAN < /nonexistent/f\necho after\n")
	if strings.Contains(special, "after") || !strings.Contains(regular, "after") {
		t.Errorf("a failed open said %q on a special builtin and %q on a regular one, "+
			"want only the special one fatal", special, regular)
	}
	// And a failed expansion in an ordinary word is fatal here, which is the
	// reading the body's failure does **not** take.
	if out, _ := runKsh(t, dir, "echo $(( 1/0 ))\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want a failed expansion in a word to end this shell", out)
	}
}

// The contained one is catchable and the fatal one is not.
func TestOnlyTheContainedHeredocFailureIsCatchableHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, "echo RAN <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a regular builtin's give-up caught by ||", out)
	}
	if out, _ := runKsh(t, dir, ": <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a special builtin's give-up to escape ||", out)
	}
}

// `${q?word}` in a body splits the same way, which is what says the grading is
// the redirection's and not the parameter's: the same parameter in an ordinary
// word ends this shell wherever it is written.
func TestAParameterRefusalInAHeredocBodySplitsTheSameWayHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`: <<END`, true},
		{`echo RAN <<END`, false},
		{`{ echo RAN; } <<END`, false},
	} {
		out, _ := runKsh(t, dir, c.src+"\n${q?bad}\nEND\necho \"after st=$?\"\n")
		if ended := !strings.Contains(out, "after"); ended != c.ends {
			t.Errorf("%s = %q, want the shell ending=%v", c.src, out, c.ends)
		}
	}
	if out, _ := runKsh(t, dir, "echo ${q?bad}\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want the same parameter in a word to end this shell", out)
	}
}

// The control the whole file rests on: a command this shell runs as a process
// of its own has its body expanded there, so the failure is the child's and
// the script carries on at 1.
func TestAFailedHeredocBodyOnAProcessOfItsOwnIsContainedHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runKsh(t, dir, "/bin/cat <<END || echo CAUGHT\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "CAUGHT") || !strings.Contains(out, "after st=0") || st != 0 {
		t.Errorf("= %q (status %d), want the failure caught and the script alive", out, st)
	}
}

// And the two controls that say the door is opened by a failure: a body that
// expands runs its command, and a quoted delimiter is not expanded at all.
func TestACleanOrQuotedHeredocBodyStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runKsh(t, dir, ": <<END\nok\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a body that expands = %q (status %d), want after st=0", out, st)
	}
	if out, st := runKsh(t, dir, ": <<'END'\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a quoted delimiter = %q (status %d), want after st=0", out, st)
	}
}
