// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A here-document body that will not expand, on a command this shell runs
// itself, is the **redirection's** failure here.
//
// So the shell stops for a special builtin — the POSIX rule this column keeps
// — and the command alone is given up for everything else, at this shell's
// redirection status of 2. Measured 2026-09-26 on dash 0.5.12, a body of
// `$(( 1/0 ))` and `echo "after st=$?"` on the line after the delimiter
// (#4684).
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
		out, st := runDash(t, dir, c.src+"\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", c.src, out)
		}
		if ended := !strings.Contains(out, "after"); ended != c.ends {
			t.Errorf("%s = %q, want the shell ending=%v", c.src, out, c.ends)
		}
		if c.ends && st != 2 {
			t.Errorf("%s: status = %d, want 2", c.src, st)
		}
		if !c.ends && !strings.Contains(out, "after st=2") {
			t.Errorf("%s = %q, want the contained failure to leave 2", c.src, out)
		}
	}
}

// The pair that says which of the two it is: a **redirection that could not be
// opened** draws that grid row for row here, and a failed expansion in an
// ordinary word is fatal whatever it is written on — which is the reading the
// body's failure does not take.
func TestAFailedHeredocBodyMatchesAFailedOpenHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `echo RAN`} {
		open, openSt := runDash(t, dir, cmd+" < /nonexistent/f\necho \"after st=$?\"\n")
		body, bodySt := runDash(t, dir, cmd+" <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(open, "after") != strings.Contains(body, "after") || openSt != bodySt {
			t.Errorf("%s: a failed open said %q at %d and a failing body said %q at %d, "+
				"want the same reach", cmd, open, openSt, body, bodySt)
		}
	}
	// The control the comparison needs: the two commands really are graded
	// apart, so the agreement above is a grid rather than one answer twice.
	special, _ := runDash(t, dir, ": < /nonexistent/f\necho after\n")
	regular, _ := runDash(t, dir, "echo RAN < /nonexistent/f\necho after\n")
	if strings.Contains(special, "after") || !strings.Contains(regular, "after") {
		t.Errorf("a failed open said %q on a special builtin and %q on a regular one, "+
			"want only the special one fatal", special, regular)
	}
	if out, _ := runDash(t, dir, "echo $(( 1/0 ))\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want a failed expansion in a word to end this shell", out)
	}
}

// And the row the noun needs, which this column and BusyBox ash are the only
// two to have: a redirection's **target** whose expansion fails is *not*
// graded as a failed open here — it ends the shell on a regular builtin,
// where the same failure in a body carries on. So the answer belongs to the
// body and not to redirections at large.
func TestAFailedTargetIsNotAFailedHeredocBodyHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target, targetSt := runDash(t, dir, "echo RAN < $(( 1/0 ))\necho after\n")
	if strings.Contains(target, "after") || targetSt != 2 {
		t.Errorf("a failed target = %q (status %d), want the shell ended at 2", target, targetSt)
	}
	body, _ := runDash(t, dir, "echo RAN <<END\n$(( 1/0 ))\nEND\necho after\n")
	if !strings.Contains(body, "after") {
		t.Errorf("a failing body = %q, want the script alive — the row that separates the two", body)
	}
}

// The contained one is catchable and the fatal one is not.
func TestOnlyTheContainedHeredocFailureIsCatchableHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, _ := runDash(t, dir, "echo RAN <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a regular builtin's give-up caught by ||", out)
	}
	if out, _ := runDash(t, dir, ": <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a special builtin's give-up to escape ||", out)
	}
}

// The control the whole file rests on: a command this shell runs as a process
// of its own has its body expanded there, so the failure is the child's and
// the script carries on at 2.
func TestAFailedHeredocBodyOnAProcessOfItsOwnIsContainedHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runDash(t, dir, "/bin/cat <<END || echo CAUGHT\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "CAUGHT") || !strings.Contains(out, "after st=0") || st != 0 {
		t.Errorf("= %q (status %d), want the failure caught and the script alive", out, st)
	}
}

// And the two controls that say the door is opened by a failure: a body that
// expands runs its command, and a quoted delimiter is not expanded at all.
func TestACleanOrQuotedHeredocBodyStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runDash(t, dir, ": <<END\nok\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a body that expands = %q (status %d), want after st=0", out, st)
	}
	if out, st := runDash(t, dir, ": <<'END'\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a quoted delimiter = %q (status %d), want after st=0", out, st)
	}
}
