// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A here-document body holding a substitution that will not **parse** ends
// the shell here, on every command alike, at **2** — the fatal status, and
// not the 1 this column reports for a failed redirection.
//
// That number is the second cell in the panel where a body that will not
// parse and a body that will not expand come apart: the same `: <<END` with
// `$(( 1/0 ))` in it ends this shell at 1, which is the redirection's.
//
// Measured 2026-09-26 on BusyBox v1.37.0 in the digest-pinned alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// `-c`, under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null,
// a body of `$(echo hi; for)` and `; echo SAME` on the redirection's own line
// (#4687).
func TestAHeredocBodyThatWillNotParseEndsTheShellHere(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{
		`: <<END`,
		`echo RAN <<END`,
		"f() { echo RAN; }\nf <<END",
		`{ echo RAN; } <<END`,
		`cat <<END`,
	} {
		out, st := run(t, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") || strings.Contains(out, "SAME") {
			t.Errorf("%s = %q, want nothing on that line run", cmd, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", cmd, out)
		}
		if st != 2 {
			t.Errorf("%s: status = %d, want the fatal 2 and not a failed redirection's 1", cmd, st)
		}
	}
}

// And the pair that number belongs to, in one place: a body that will not
// expand ends this shell at the redirection's 1 where a body that will not
// parse ends it at the refusal's 2.
func TestAParseFailureAndAnExpansionFailureAreNumberedApartHere(t *testing.T) {
	t.Parallel()
	_, expand := run(t, ": <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	_, parse := run(t, ": <<END\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
	if expand != 1 {
		t.Errorf("a failed expansion ended at %d, want the redirection's 1", expand)
	}
	if parse != 2 {
		t.Errorf("a refusal ended at %d, want the fatal 2", parse)
	}
}

// The control that says it is the refusal and not the here-document.
func TestAQuotedDelimiterCarriesTheSameBodyHere(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{`: <<'END'`, `echo RAN <<'END'`, `cat <<'END'`} {
		out, st := run(t, cmd+"; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n")
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "after st=0") || st != 0 {
			t.Errorf("%s = %q at %d, want the command run and the script carrying on", cmd, out, st)
		}
	}
}
