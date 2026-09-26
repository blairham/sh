// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A here-document body that will not expand, on a command this shell runs
// itself, is the **redirection's** failure here — and this is the column that
// shows it in the *status*, because a failed redirection reports 1 here where
// a fatal error exits 2.
//
// Measured 2026-09-26 on BusyBox v1.37.0 in the digest-pinned alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// a body of `$(( 1/0 ))` and `echo "after st=$?"` on the line after the
// delimiter: `: <<END` ends the shell at **1**, and `echo RAN <<END`, a
// function and a group each write the complaint and then `after st=1` (#4684).
func TestAFailedHeredocBodyIsTheRedirectionsHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`: <<END`, true},
		{`echo RAN <<END`, false},
		{`f() { echo RAN; }; f <<END`, false},
		{`{ echo RAN; } <<END`, false},
	} {
		out, st := run(t, c.src+"\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", c.src, out)
		}
		if ended := !strings.Contains(out, "after"); ended != c.ends {
			t.Errorf("%s = %q, want the shell ending=%v", c.src, out, c.ends)
		}
		if c.ends && st != 1 {
			t.Errorf("%s: status = %d, want 1 — the redirection's and not the fatal 2", c.src, st)
		}
		if !c.ends && !strings.Contains(out, "after st=1") {
			t.Errorf("%s = %q, want the contained failure to leave 1", c.src, out)
		}
	}
}

// And the number is the finding, so it is asserted against the one this shell
// gives a *fatal* error: the same expression in an ordinary word exits 2 here.
// A column where the two numbers agree could not tell which was taken.
func TestAFailedHeredocBodyTakesTheRedirectionsNumberHere(t *testing.T) {
	t.Parallel()
	if _, st := run(t, "echo $(( 1/0 ))\n"); st != 2 {
		t.Errorf("a failed expansion in a word: status = %d, want this shell's fatal 2", st)
	}
	if _, st := run(t, ": < /nonexistent/f\n"); st != 1 {
		t.Errorf("a failed open on a special builtin: status = %d, want this shell's redirection 1", st)
	}
	if _, st := run(t, ": <<END\n$(( 1/0 ))\nEND\n"); st != 1 {
		t.Errorf("a failing body on a special builtin: status = %d, want the redirection's 1", st)
	}
}

// The pair that says which of the two it is: a **redirection that could not be
// opened** draws that grid row for row here, and a failed expansion in an
// ordinary word is fatal whatever it is written on.
func TestAFailedHeredocBodyMatchesAFailedOpenHere(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{`:`, `echo RAN`} {
		open, openSt := run(t, cmd+" < /nonexistent/f\necho \"after st=$?\"\n")
		body, bodySt := run(t, cmd+" <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(open, "after") != strings.Contains(body, "after") || openSt != bodySt {
			t.Errorf("%s: a failed open said %q at %d and a failing body said %q at %d, "+
				"want the same reach", cmd, open, openSt, body, bodySt)
		}
	}
	special, _ := run(t, ": < /nonexistent/f\necho after\n")
	regular, _ := run(t, "echo RAN < /nonexistent/f\necho after\n")
	if strings.Contains(special, "after") || !strings.Contains(regular, "after") {
		t.Errorf("a failed open said %q on a special builtin and %q on a regular one, "+
			"want only the special one fatal", special, regular)
	}
	if out, _ := run(t, "echo $(( 1/0 ))\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want a failed expansion in a word to end this shell", out)
	}
}

// And the row the noun needs, which this column and dash are the only two to
// have: a redirection's **target** whose expansion fails ends the shell on a
// regular builtin, where the same failure in a body carries on.
func TestAFailedTargetIsNotAFailedHeredocBodyHere(t *testing.T) {
	t.Parallel()
	target, targetSt := run(t, "echo RAN < $(( 1/0 ))\necho after\n")
	if strings.Contains(target, "after") || targetSt != 2 {
		t.Errorf("a failed target = %q (status %d), want the shell ended at 2", target, targetSt)
	}
	body, _ := run(t, "echo RAN <<END\n$(( 1/0 ))\nEND\necho after\n")
	if !strings.Contains(body, "after") {
		t.Errorf("a failing body = %q, want the script alive — the row that separates the two", body)
	}
}

// The control the whole file rests on: a command this shell runs as a process
// of its own has its body expanded there, so the failure is the child's, the
// script carries on, and the status it leaves is the redirection's 1 here too.
func TestAFailedHeredocBodyOnAProcessOfItsOwnIsContainedHere(t *testing.T) {
	t.Parallel()
	out, st := run(t, "/bin/cat <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "after st=1") || st != 0 {
		t.Errorf("= %q (status %d), want the script alive at 1", out, st)
	}
}

// And the two controls that say the door is opened by a failure: a body that
// expands runs its command, and a quoted delimiter is not expanded at all.
func TestACleanOrQuotedHeredocBodyStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	if out, st := run(t, ": <<END\nok\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a body that expands = %q (status %d), want after st=0", out, st)
	}
	if out, st := run(t, ": <<'END'\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a quoted delimiter = %q (status %d), want after st=0", out, st)
	}
}
