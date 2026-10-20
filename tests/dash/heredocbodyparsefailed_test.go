// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A here-document body holding a substitution that will not **parse** ends
// the shell here, on every command alike, at **2**.
//
// Measured 2026-09-26 on dash 0.5.12 (/bin/dash), `-c`, under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null, a body of
// `$(echo hi; for)` and `; echo SAME` on the redirection's own line (#4687).
func TestAHeredocBodyThatWillNotParseEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{
		`: <<END`,
		`echo RAN <<END`,
		"f() { echo RAN; }\nf <<END",
		`{ echo RAN; } <<END`,
		`cat <<END`,
	} {
		out, st := runDash(t, dir, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") || strings.Contains(out, "SAME") {
			t.Errorf("%s = %q, want nothing on that line run", cmd, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", cmd, out)
		}
		if st != 2 {
			t.Errorf("%s: status = %d, want 2", cmd, st)
		}
	}
}

// The control that says it is the refusal and not the here-document: a
// **quoted** delimiter leaves the same bytes literal and every shape runs at
// 0.
func TestAQuotedDelimiterCarriesTheSameBodyHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`: <<'END'`, `echo RAN <<'END'`, `cat <<'END'`} {
		out, st := runDash(t, dir, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "after st=0") || st != 0 {
			t.Errorf("%s = %q at %d, want the command run and the script carrying on", cmd, out, st)
		}
	}
}
