// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A here-document body holding a substitution that will not **parse**, on a
// command this shell runs itself, is the **redirection's** failure here: the
// command alone is given up, the rest of its line runs, and only a *special*
// builtin stops the shell — which is the POSIX rule this column keeps.
//
// Measured 2026-09-26 on ksh93u+ 2012-08-01 (/bin/ksh), `-c`, under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null, a body of
// `$(echo hi; for)` and `; echo SAME` on the redirection's own line (#4687).
func TestAHeredocBodyThatWillNotParseIsTheRedirectionsHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`: <<END`, true},
		{`echo RAN <<END`, false},
		{"f() { echo RAN; }\nf <<END", false},
		{`{ echo RAN; } <<END`, false},
	} {
		out, st := runKsh(t, dir, c.src+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", c.src, out)
		}
		if ended := !strings.Contains(out, "after"); ended != c.ends {
			t.Errorf("%s = %q, want the shell ending=%v", c.src, out, c.ends)
		}
		if c.ends && st != 3 {
			t.Errorf("%s: status = %d, want 3 — the refusal's own and not a failed "+
				"redirection's 1", c.src, st)
		}
		if !c.ends && !strings.Contains(out, "SAME") {
			t.Errorf("%s = %q, want the rest of the line run", c.src, out)
		}
	}
}

// The pair that says the shell was ended by the **special builtin** rule and
// not by the refusal: `command` takes a special builtin's specialness away,
// and with it the stop.
//
// Same body, same failure, same builtin, one word apart. bash does not move
// on this pair at all, which is what puts the two columns on opposite sides.
func TestCommandTakesTheStopOffAParseFailureHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bare, bareSt := runKsh(t, dir, ": <<END; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
	wrapped, _ := runKsh(t, dir, "command : <<END; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
	if strings.Contains(bare, "after") || bareSt != 3 {
		t.Errorf("a bare special builtin said %q at %d, want the shell ended at 3", bare, bareSt)
	}
	if !strings.Contains(wrapped, "SAME") || !strings.Contains(wrapped, "after") {
		t.Errorf("`command` said %q, want the rest of the line and the script carrying on", wrapped)
	}
}

// And the number it ends at is the **refusal's own**, not a failed
// redirection's — which is the one cell that separates a body that will not
// parse from a body that will not expand anywhere in the panel.
//
// Measured 2026-09-26: `: < /nonexistent/f` ends this shell at 1 and so does
// `: <<END` with `$(( 1/0 ))` in it, where `: <<END` with `$(echo hi; for)`
// ends it at 3.
func TestAParseFailureKeepsItsOwnStatusHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, open := runKsh(t, dir, ": < /nonexistent/f\necho \"after st=$?\"\n")
	_, expand := runKsh(t, dir, ": <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	_, parse := runKsh(t, dir, ": <<END\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
	if open != 1 || expand != 1 {
		t.Errorf("a failed open ended at %d and a failed expansion at %d, want 1 for both", open, expand)
	}
	if parse != 3 {
		t.Errorf("a refusal ended at %d, want the syntax status 3", parse)
	}
}
