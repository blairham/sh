// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A here-document body holding a substitution that will not **parse**, on a
// command this shell runs itself, is **this shell's own failed expansion**
// here: the *line* is given up and the next one reads 1.
//
// Measured 2026-09-26 on bash 5.3.20 (/opt/homebrew/bin/bash), `-c`, under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null, a body of
// `$(echo hi; for)` and `; echo SAME` on the **redirection's own line** —
// which is what tells the line apart from the command, since `; echo SAME`
// written after the *delimiter* is a separate line and both readings print
// it. `: <<END`, `read x <<END`, a function and a group each write the
// refusal, no `SAME`, and then `after st=1` (#4687).
func TestAHeredocBodyThatWillNotParseGivesUpTheLineHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{
		`: <<END`,
		`echo RAN <<END`,
		`f() { echo RAN; }
f <<END`,
		`{ echo RAN; } <<END`,
	} {
		out, st := runBash(t, dir, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", cmd, out)
		}
		if strings.Contains(out, "SAME") {
			t.Errorf("%s = %q, want the rest of the line given up too", cmd, out)
		}
		if !strings.Contains(out, "after st=1") {
			t.Errorf("%s = %q, want the script carrying on at 1", cmd, out)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0 — the script ran to its end", cmd, st)
		}
	}
}

// The pair that says it is **not** the redirection's failure here: a special
// builtin is graded exactly as a regular one is, and stripping a special
// builtin's specialness with `command` moves nothing.
//
// That is the pair ksh93 parts on — there `: <<END` ends the shell and
// `command : <<END` carries on — and it is what makes
// Semantics.RedirectErrorOnSpecialBuiltinFatal the wrong reading for this
// column. Measured 2026-09-26.
func TestASpecialBuiltinIsNoDifferentForAParseFailureHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var got []string
	for _, cmd := range []string{`: <<END`, `command : <<END`, `echo RAN <<END`} {
		out, _ := runBash(t, dir, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "SAME") || !strings.Contains(out, "after st=1") {
			t.Errorf("%s = %q, want the line given up and the script alive at 1", cmd, out)
		}
		got = append(got, out[strings.LastIndex(out, "after"):])
	}
	for _, g := range got[1:] {
		if g != got[0] {
			t.Errorf("%q against %q: the command word must not move this column", g, got[0])
		}
	}
}

// And the external control, which is the other construct's and must not move:
// a command this shell runs as a process of its own loses **only the
// command**, so the rest of its line runs.
func TestAnExternalCommandsParseFailureLosesOnlyTheCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, "cat <<END; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "SAME") {
		t.Errorf("= %q, want the rest of the line run", out)
	}
	if !strings.Contains(out, "after st=0") || st != 0 {
		t.Errorf("= %q at %d, want the script carrying on", out, st)
	}
	// The control the row needs: the refusal really happened, so this is a
	// command being given up rather than a body that parsed.
	if !strings.Contains(out, "syntax") && !strings.Contains(out, "unexpected") {
		t.Errorf("= %q, want the refusal reported", out)
	}
}
