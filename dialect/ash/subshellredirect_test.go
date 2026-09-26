// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A redirection written on a `( … )` of its own leaves this shell's **fatal**
// number, where the identical redirection on anything else leaves its failed
// redirection's.
//
// This is the column worth measuring it on for the reason #4684 was worth
// measuring here: it numbers the two apart — a fatal error exits 2 and a
// failed redirection reports 1 — where bash, zsh and ksh93 report 1 for both
// and dash reports 2 for both.
//
// Measured 2026-09-26 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, `env -i PATH=/usr/bin:/bin HOME=<scratch> LC_ALL=C` over a
// script file with stdin from /dev/null and the status taken on the line
// after. A `GOOS=linux GOARCH=arm64` build of cmd/ash was copied in and
// confirmed by `go version -m`; the BusyBox binary is `not a Go executable`.
//
//	cat < nosuch                1   a command of its own
//	{ cat; } < nosuch           1   a group
//	while … done < nosuch       1   a loop
//	( cat < nosuch )            1   the redirection is the inner command's
//	( cat ) < nosuch            2   the subshell's own
//	( : ) < nosuch              2   and not about what is inside it
//	( cat ) > /nosuch/dir/f     2   a failed create, the same
//	( cat ) <<END $(( 1/0 ))    2   a body that will not expand
//	( cat ) <<END ${x?bad}      2   whatever the expansion failed on
//
// See interp.Diagnostics.SubshellRedirectFailureStatus (#4716).
func TestASubshellsOwnRedirectionLeavesTheFatalNumberHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src  string
		want int
	}{
		{"cat < /nonexistent/f", 1},
		{"{ cat; } < /nonexistent/f", 1},
		{"while read -r x; do :; done < /nonexistent/f", 1},
		{"( cat < /nonexistent/f )", 1},
		{"( cat ) < /nonexistent/f", 2},
		{"( : ) < /nonexistent/f", 2},
		{"( cat ) > /nonexistent/dir/f", 2},
	} {
		out, _ := run(t, c.src+"\necho \"after st=$?\"\n")
		if want := "after st=" + string(rune('0'+c.want)); !strings.Contains(out, want) {
			t.Errorf("%s = %q, want %q", c.src, out, want)
		}
	}
}

// A here-document body that will not expand is the same question, because it
// is the same failure: the redirection did not come out.
func TestASubshellsOwnFailingBodyLeavesTheFatalNumberHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src  string
		want string
	}{
		{"cat <<END", "after st=1"},
		{"{ cat; } <<END", "after st=1"},
		{"( cat <<END )", "after st=1"},
		{"( cat ) <<END", "after st=2"},
	} {
		out, _ := run(t, c.src+"\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", c.src, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s = %q, want %q", c.src, out, c.want)
		}
	}
	// And whatever the expansion failed on: the noun is the redirection and
	// not the kind of failure in it.
	out, _ := run(t, "( cat ) <<END\n${q?bad}\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(out, "after st=2") {
		t.Errorf("a `${q?bad}` body on a subshell = %q, want `after st=2`", out)
	}
}

// The number is only the number: the subshell does not run, and the script
// carries on, exactly as every other row does.
func TestASubshellsOwnRedirectionStillOnlyCostsTheCommandHere(t *testing.T) {
	t.Parallel()
	out, _ := run(t, "( echo RAN ) < /nonexistent/f\necho after\n")
	if strings.Contains(out, "RAN") || !strings.Contains(out, "after") {
		t.Errorf("= %q, want the subshell unrun and the script carried on", out)
	}
}
