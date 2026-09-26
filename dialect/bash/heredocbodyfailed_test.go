// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A here-document body that will not expand, on a command this shell runs
// itself, is **this shell's own failed expansion** — not the redirection's.
//
// So it costs what any other failed expansion costs here, which is the line:
// the command does not run, the rest of the line goes with it, and the next
// line reads 1. Measured 2026-09-26 on bash 5.3.20, a body of `$(( 1/0 ))`
// and `echo "after st=$?"` on the line after the delimiter (#4684).
func TestAFailedHeredocBodyGivesUpTheLineHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`: <<END`,
		`echo RAN <<END`,
		`f() { echo RAN; }; f <<END`,
		`{ echo RAN; } <<END`,
	} {
		out, st := runBash(t, dir, src+"\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("%s = %q (status %d), want the next line to run and read 1", src, out, st)
		}
	}
}

// The pair that says which of the two it is, and it is the whole argument for
// this column's answer. A **redirection that could not be opened** carries on
// with the rest of the line here and is caught by `||`; the failing body does
// neither, and an ordinary word holding the same expression does neither
// either. So the body follows the *expansion* and not the redirection.
func TestAFailedHeredocBodyIsNotAFailedRedirectionHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body, _ := runBash(t, dir, ": <<END; echo SAME\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	word, _ := runBash(t, dir, "echo $(( 1/0 )); echo SAME\necho \"after st=$?\"\n")
	open, _ := runBash(t, dir, ": < /nonexistent/f; echo SAME\necho \"after st=$?\"\n")
	if strings.Contains(body, "SAME") {
		t.Errorf("a failed body = %q, want the rest of the line given up", body)
	}
	if strings.Contains(word, "SAME") {
		t.Errorf("the same expression in a word = %q, want the rest of the line given up", word)
	}
	if !strings.Contains(open, "SAME") {
		t.Errorf("a redirection that would not open = %q, want the rest of the line run", open)
	}
	// And the same three through `||`, which is the sharper half: only the
	// redirection's failure is catchable.
	caught := func(src string) bool {
		t.Helper()
		out, _ := runBash(t, dir, src)
		return strings.Contains(out, "CAUGHT")
	}
	if caught(": <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n") {
		t.Error("a failed body was caught by ||, and this shell does not catch it")
	}
	if !caught(": < /nonexistent/f || echo CAUGHT\n") {
		t.Error("a redirection that would not open was not caught by ||: the probe cannot fire")
	}
}

// And the failure kind decides how far, exactly as it does in an ordinary
// word: `${q?word}` ends this shell wherever it is written, including in a
// body, on every command this shell runs itself.
//
// The *number* it ends at is the front end's — `bash -c` reports 127 for an
// unset parameter and this runner has no invocation to ask — so what is
// asserted here is that the shell stopped and did not stop at 0.
func TestAFailedHeredocBodyKeepsTheFailuresOwnReachHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`: <<END`,
		`echo RAN <<END`,
		`f() { echo RAN; }; f <<END`,
		`{ echo RAN; } <<END`,
	} {
		out, st := runBash(t, dir, src+"\n${q?bad}\nEND\necho after\n")
		if strings.Contains(out, "RAN") || strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
		if st == 0 {
			t.Errorf("%s: status = 0, want the failure's", src)
		}
	}
}

// The control the whole file rests on: a command this shell runs as a process
// of its own has its body expanded there, so the failure is the child's and
// the script carries on at 1 — and it is catchable, unlike every row above.
func TestAFailedHeredocBodyOnAProcessOfItsOwnIsContainedHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, "/bin/cat <<END || echo CAUGHT\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "CAUGHT") || !strings.Contains(out, "after st=0") || st != 0 {
		t.Errorf("= %q (status %d), want the failure caught and the script alive", out, st)
	}
}

// And the two controls that say the door is opened by a failure: a body that
// expands runs its command, and a quoted delimiter is not expanded at all.
func TestACleanOrQuotedHeredocBodyStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runBash(t, dir, ": <<END\nok\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a body that expands = %q (status %d), want after st=0", out, st)
	}
	if out, st := runBash(t, dir, ": <<'END'\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n"); out != "after st=0\n" || st != 0 {
		t.Errorf("a quoted delimiter = %q (status %d), want after st=0", out, st)
	}
}
