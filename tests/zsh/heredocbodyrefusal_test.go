// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// A substitution refused in a **here-document body** stands at the line after
// the document here, and carries a plain `parse error` under it rather than
// the quote of the word or an open context the body never held.
//
// Measured 2026-09-26 over script files under `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C` with stdin from /dev/null, against
// `/opt/homebrew/bin/zsh -f` 5.9.2 (`not a Go executable` by `go version
// -m`) and `cmd/zsh` built under its own name, each body holding
// `$(echo hi; for)`:
//
//	program                                       reference     before
//	cat <<END ⏎ <body> ⏎ END ⏎                     :4: twice     :2: then :3:
//	the same with `echo after` under it            :4: twice     :2: then :3:
//	echo p ⏎ cat <<END ⏎ <body> ⏎ END ⏎            :5: twice     :3: then :4:
//	cat <<END ⏎ x ⏎ <body> ⏎ END ⏎                 :5: twice     :3: then :4:
//	cat <<END ⏎ <body> ⏎ END  (no final newline)   :3: twice     :2: then :3:
//	cat <<END | cat ⏎ <body> ⏎ END ⏎ printf        :4: twice     :2: then :3:
//	{ cat; } <<END ⏎ <body> ⏎ END ⏎ echo a         :4: twice     :2: then :3:
//
// And the second message, where the three controls were already
// byte-identical and only the body was not:
//
//	program                            reference second line   before
//	cat <<END ⏎ $(echo hi; for) ⏎ END   parse error             unmatched "
//	echo $(echo hi; for)                parse error near `…'    identical
//	echo "$(echo hi; for)"              unmatched "             identical
//	cat <<<"$(echo hi; for)"            unmatched "             identical
//
// See interp/heredocbodyrefusallocation.go (#4714).
func TestARefusedHeredocBodyStandsAfterTheDelimiterHere(t *testing.T) {
	t.Parallel()
	if !zsh.Diagnostics().HeredocBodyRefusalIsLocatedAfterTheDelimiter {
		t.Error("the preset does not answer, so the refusal stands in the body")
	}
	if got := zsh.Diagnostics().HeredocBodyRefusalSentence; got != "parse error" {
		t.Errorf("the sentence is %q, want `parse error`", got)
	}
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"cat <<END\n$(echo hi; for)\nEND\n", ":4:"},
		{"cat <<END\n$(echo hi; for)\nEND\necho after\n", ":4:"},
		{"echo p\ncat <<END\n$(echo hi; for)\nEND\n", ":5:"},
		{"cat <<END\nx\n$(echo hi; for)\nEND\n", ":5:"},
		{"cat <<END | cat\n$(echo hi; for)\nEND\n", ":4:"},
		{"{ cat; } <<END\n$(echo hi; for)\nEND\necho a\n", ":4:"},
	} {
		out, _ := runZsh(t, dir, c.src)
		if n := strings.Count(out, c.want); n != 2 {
			t.Errorf("%q = %q with %d lines at %s, want both messages there", c.src, out, n, c.want)
		}
		if !strings.Contains(out, c.want+" parse error\n") {
			t.Errorf("%q = %q, want the plain sentence under the refusal", c.src, out)
		}
		if strings.Contains(out, "unmatched") {
			t.Errorf("%q = %q, want no open quote — the body holds none", c.src, out)
		}
	}
}

// The controls the body rows need are asserted where the harness can see
// them — interp/heredocbodyrefusallocation_test.go, which tells the runner
// what the program's text is and so can produce the second message a word,
// a quoted word and a here-string get. The rows above are the ones that need
// the preset, and what they show is that the dialect answers.
