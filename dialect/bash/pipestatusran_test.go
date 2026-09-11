// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A compound command writes no pipeline-status record of its own: what stands
// after one is whatever the last pipeline that actually **ran inside** it
// wrote, and a compound that ran nothing leaves the record from before it.
//
// Measured 2026-09-11 on bash 5.3.15 and 3.2.57 alike, each snippet written
// after `false | true` so that a replaced record shows as one element. We
// wrote the compound's own status for every one of these before (#2016),
// which is the plausible one-element answer nothing reports.
//
// It is a different mechanism from the neighbouring shell's rather than the
// other answer to one question: that one reads the body's *parse* and writes
// the compound's own status, and this one reads nothing and writes nothing.
// The rows below are the ones where the two differ.
func TestACompoundLeavesTheRecordToWhatRanInside(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The condition ran and wrote 1; the clause wrote nothing over it.
		{`if false; then :; fi`, "1"},
		{`if [[ a = b ]]; then :; fi`, "1"},
		{`while false; do :; done`, "1"},
		{`until true; do :; done`, "0"},
		// Nothing inside ran at all, so the record from before the compound
		// is what a script reads afterwards.
		{`case a in b) :;; esac`, "1 0"},
		{`case a in b) ;; esac`, "1 0"},
		{`for i in ; do :; done`, "1 0"},
		{`{ [[ a = a ]] & }`, "1 0"},
		// Two elements survive the braces, which is what says the record is
		// the inner *pipeline*'s rather than the inner last command's.
		{`{ [[ a = a ]] | [[ b = b ]]; }`, "0 0"},
		{`{ true | false; }`, "0 1"},
		// What did run wrote it, however deep, and a compound inside a
		// compound writes nothing either.
		{`{ :; }`, "0"},
		{`{ [[ a = a ]]; }`, "0"},
		{`{ x=1; }`, "0"},
		{`{ { :; } }`, "0"},
		{`{ if false; then :; fi; }`, "1"},
		{`for i in a; do false; done`, "1"},
		// A redirection on the compound does not change it, which is the
		// opposite of the rule the neighbouring axes follow.
		{`if false; then :; fi >/dev/null`, "1"},
		{`{ :; } >/dev/null`, "0"},
		// A subshell is not a compound for this purpose: it is a job and
		// reports its own status, one element, whatever it ran.
		{`( : )`, "0"},
		{`( false | true | false )`, "1"},
		// A `!` makes a pipeline of what follows it, and a pipeline writes.
		{`! { false; }`, "1"},
		// And a definition runs nothing, which both shells agree about and
		// no dialect is asked.
		{`f() { :; }`, "1 0"},
	} {
		out, st := runBash(t, t.TempDir(), "false | true; "+tc.src+`; echo "${PIPESTATUS[@]}"`)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("false | true; %s\n got %q (status %d)\nwant %q at 0", tc.src, got, st, tc.want)
		}
	}
}
