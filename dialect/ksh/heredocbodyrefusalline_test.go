// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// This shell is the only one in the panel that carries a line **inside** the
// sentence as well as in the location, and the two are the same number. For a
// substitution refused in a here-document body this shell counted the second
// from the body, so the sentence disagreed with the prefix in front of it.
//
// Measured 2026-09-26 over a script file under `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C` with stdin from /dev/null, against `/bin/ksh`
// `Version AJM 93u+ 2012-08-01` (AT&T's own build, `not a Go executable` by
// `go version -m`) and `cmd/ksh` built under its own name, each program
// holding `$(echo hi; for)` as its here-document body:
//
//	command, and the line it is on   reference                    before
//	cat <<END        line 1          s.sh: line 1: … at line 1    at line 2
//	cat <<END        line 2          s.sh: line 2: … at line 2    at line 3
//	cat <<END        line 3          s.sh: line 3: … at line 3    at line 4
//	cat <<END | cat  line 1          s.sh: line 1: … at line 1    at line 2
//	{ cat; } <<END   line 1          s.sh: … at line 0            at line 2
//	{ cat; } <<END   line 2          s.sh: line 1: … at line 1    at line 3
//	{ cat; } <<END   line 3          s.sh: line 2: … at line 2    at line 4
//	: <<END          line 1          s.sh[1]: … at line 0         at line 2
//	: <<END          line 2          s.sh[2]: … at line 0         at line 3
//	: <<END          line 3          s.sh[3]: … at line 0         at line 4
//
// See interp/heredocbodyrefusalline.go for the two rules those rows carry
// (#4715).
func TestARefusedHeredocBodysSentenceNamesItsLocationHere(t *testing.T) {
	t.Parallel()
	if !ksh.Diagnostics().HeredocBodyRefusalNamesTheLineItIsLocatedAt {
		t.Error("the preset does not answer, so the sentence counts from the body")
	}
	// And the sentence carries a line at all, which is what makes the
	// question this dialect's: the other four put the line in the location
	// and have nothing here to disagree with it.
	if !ksh.Diagnostics().ParseFailureNamesItsOwnLine {
		t.Error("the wording does not name its own line, so there is no second number")
	}
	dir := t.TempDir()
	// Two lines of padding, so the location carries a line: this route
	// leaves line 1 unwritten, which is a different rule.
	out, _ := runKsh(t, dir, "echo p\necho p\ncat <<END\n$(echo hi; for)\nEND\n")
	if !strings.Contains(out, "at line 3:") {
		t.Errorf("= %q, want the sentence to name line 3 — the command's own", out)
	}
	if strings.Contains(out, "at line 4:") {
		t.Errorf("= %q, want the sentence not to count from the body's line 4", out)
	}
	// The control: the same refusal in an ordinary word, where the body's
	// line and the command's are the same line and nothing moves.
	word, _ := runKsh(t, dir, "echo p\necho p\necho $(echo hi; for)\n")
	if !strings.Contains(word, "at line 3:") {
		t.Errorf("a word = %q, want the sentence to name line 3", word)
	}
}
