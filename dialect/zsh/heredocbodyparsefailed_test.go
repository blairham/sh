// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A here-document body holding a substitution that will not **parse** ends
// the shell here, on every command alike — which is
// Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell answering Yes
// and outranking whose failure it is.
//
// Measured 2026-09-26 on zsh 5.9.2 under `-f` (/opt/homebrew/bin/zsh), `-c`,
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null, a
// body of `$(echo hi; for)` and `; echo SAME` on the redirection's own line:
// every shape writes the refusal and exits **1** (#4687).
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
		out, st := runZsh(t, dir, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") || strings.Contains(out, "SAME") {
			t.Errorf("%s = %q, want nothing on that line run", cmd, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", cmd, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", cmd, st)
		}
	}
}

// The control that says the stop belongs to the refusal and not to having a
// here-document: a **quoted** delimiter leaves the body literal, so there is
// nothing to refuse and every shape runs at 0.
func TestAQuotedDelimiterCarriesTheSameBodyHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`: <<'END'`, `echo RAN <<'END'`, `cat <<'END'`} {
		out, st := runZsh(t, dir, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "after st=0") || st != 0 {
			t.Errorf("%s = %q at %d, want the command run and the script carrying on", cmd, out, st)
		}
	}
}
