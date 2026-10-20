// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A here-document body that will not expand, on a command this shell runs
// itself, is **this shell's own failed expansion** — not the redirection's.
//
// A failed expansion is fatal here, so the shell ends, whatever the command
// word was. Measured 2026-09-26 on zsh 5.9.2 under `-f`, a body of
// `$(( 1/0 ))` and `echo after` on the line after the delimiter (#4684).
func TestAFailedHeredocBodyEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`: <<END`,
		`echo RAN <<END`,
		`f() { echo RAN; }; f <<END`,
		`{ echo RAN; } <<END`,
	} {
		out, st := runZsh(t, dir, src+"\n$(( 1/0 ))\nEND\necho after\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", src, st)
		}
	}
}

// The pair that says which of the two it is, and it is the whole argument for
// this column's answer. A **redirection that could not be opened** is not
// fatal here — not even on a special builtin, where this shell is one of the
// two that decline the POSIX rule — so it carries on at 1 and `||` catches
// it. The failing body does neither.
func TestAFailedHeredocBodyIsNotAFailedRedirectionHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{`:`, `echo RAN`} {
		open, openSt := runZsh(t, dir, src+" < /nonexistent/f\necho \"after st=$?\"\n")
		if !strings.Contains(open, "after st=1") || openSt != 0 {
			t.Errorf("%s with a redirection that would not open = %q (status %d), "+
				"want the script alive at 1", src, open, openSt)
		}
		body, bodySt := runZsh(t, dir, src+" <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(body, "after") || bodySt != 1 {
			t.Errorf("%s with a failing body = %q (status %d), want the shell ended", src, body, bodySt)
		}
	}
	// And through `||`, which is the sharper half: only the redirection's
	// failure is catchable.
	if out, _ := runZsh(t, dir, ": < /nonexistent/f || echo CAUGHT\n"); !strings.Contains(out, "CAUGHT") {
		t.Error("a redirection that would not open was not caught by ||: the probe cannot fire")
	}
	if out, _ := runZsh(t, dir, ": <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a failing body's give-up to escape ||", out)
	}
}

// `${q?word}` in a body is the same answer, which is what says the reading is
// the expansion's and not the arithmetic's: the failure kind cannot move a
// column that is fatal for all of them.
func TestAParameterRefusalInAHeredocBodyEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{`: <<END`, `echo RAN <<END`, `{ echo RAN; } <<END`} {
		out, st := runZsh(t, dir, src+"\n${q?bad}\nEND\necho after\n")
		if strings.Contains(out, "RAN") || strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", src, st)
		}
	}
}

// The control the whole file rests on: a command this shell runs as a process
// of its own has its body expanded there, so the failure is the child's, the
// script carries on at 1, and `||` catches it.
func TestAFailedHeredocBodyOnAProcessOfItsOwnIsContainedHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runZsh(t, dir, "/bin/cat <<END || echo CAUGHT\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "CAUGHT") || !strings.Contains(out, "after st=0") || st != 0 {
		t.Errorf("= %q (status %d), want the failure caught and the script alive", out, st)
	}
}

// And the two controls that say the door is opened by a failure: a body that
// expands runs its command, and a quoted delimiter is not expanded at all.
func TestACleanOrQuotedHeredocBodyStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runZsh(t, dir, ": <<END\nok\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a body that expands = %q (status %d), want after st=0", out, st)
	}
	if out, st := runZsh(t, dir, ": <<'END'\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a quoted delimiter = %q (status %d), want after st=0", out, st)
	}
}
