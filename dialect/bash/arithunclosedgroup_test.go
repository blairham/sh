// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A parenthesised group that reads a value and then meets text it can use for
// neither an operator nor a close is worded here as the group it could not
// close, which is a sentence no other leftover gets: text an expression could
// not use is `arithmetic syntax error in expression`.
//
// Measured 2026-09-17 against bash 5.3.20, a script file, `env -i` with
// LC_ALL=C: the line reads (echo a) : missing `)' (error token is "a) "),
// where
// `$(( (1)x ))` is `(1)x : arithmetic syntax error in expression (error token
// is "x ")`. Ours wrote the parser's own prose for the first, `expected ) in
// arithmetic`, with nothing quoted back and no token named (#3071).
func TestAnUnclosedGroupIsWordedAsTheGroup(t *testing.T) {
	out, st := answersRun(t, `echo $(( (echo a) ))`)
	want := "(echo a) : missing `)' (error token is \"a) \")"
	if !strings.Contains(out, want) || st != 1 {
		t.Errorf("got %q at %d, want it to contain %q at 1", out, st, want)
	}
	out, st = answersRun(t, `echo $(( (1)x ))`)
	want = "(1)x : arithmetic syntax error in expression (error token is \"x \")"
	if !strings.Contains(out, want) || st != 1 {
		t.Errorf("got %q at %d, want it to contain %q at 1", out, st, want)
	}
}

// And it is an arithmetic failure rather than a parse one, so the command
// carrying it fails and the script runs on. Measured on the same run: the
// `((` reports at 1 and `after` is written.
func TestAnUnclosedGroupLeavesTheScriptRunning(t *testing.T) {
	out, st := answersRun(t, "(( (echo a) )); echo after=$?")
	if !strings.Contains(out, "after=1") || st != 0 {
		t.Errorf("got %q at %d, want it to contain %q at 0", out, st, "after=1")
	}
}
