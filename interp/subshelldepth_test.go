// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// How many subshell boundaries a construct puts between the shell and the
// command inside it.
//
// The parameter this answers is a dialect's, so the name below is the test's
// own and not any shell's: what is asserted here is the *construct*, which is
// the core's question. See [Runner.SubshellDepth].
func runDepth(t *testing.T, src string) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	// Where the last pipeline element runs is an axis, and the core answers
	// it with a refusal rather than a guess — so a pipeline row here has to
	// say which side it is asking about. Yes is the side the depth rows are
	// written against: the last element is this shell, and the rows below
	// assert that it therefore counts no boundary.
	sem := Semantics{LastPipelineElementInCurrentShell: Yes}
	r := newTestRunner(t, &Runner{Stdout: &out, Semantics: &sem})
	r.SetDynamic("DEPTH", func(rr *Runner) string { return strconv.Itoa(rr.SubshellDepth()) })
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return strings.TrimRight(out.String(), "\n")
}

// The whole grid, and it is a grid because three of its rows cannot be
// derived from the other four.
//
// A count is not a flag, so the rows that matter are the ones where two
// constructs nest: a copy of the runner is made for every one of them, and
// what the count has to get right is which of those copies is a *boundary*.
func TestASubshellDepthCountsTheBoundariesAndNotTheCopies(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"the shell itself", `echo $DEPTH`, "0"},
		{"parentheses", `( echo $DEPTH )`, "1"},
		{"parentheses inside parentheses", `( ( echo $DEPTH ) )`, "2"},
		{"a command substitution", `echo $( echo $DEPTH )`, "1"},
		// The three that run in this shell. A copy is not made for any of
		// them, and a rule that counted "something was entered" rather than
		// "the runner was copied" would put a 1 in each.
		{"a brace group", `{ echo $DEPTH ; }`, "0"},
		{"a function body", `f() { echo $DEPTH ; }; f`, "0"},
		{"an eval", `eval 'echo $DEPTH'`, "0"},
		// A pipeline: every stage but the last is a copy, and the last is
		// this shell. Both halves are asserted, since a rule copying every
		// stage passes the first and fails the second.
		{"a pipeline's first stage", `echo $DEPTH | { read -r v; echo "$v" ; }`, "1"},
		{"a pipeline's last stage", `true | echo $DEPTH`, "0"},
		// And the three rows the collapse is about — see
		// theForkIsTheParentheses. A fork whose whole command is a `( … )`
		// is those parentheses, and a fork whose command is anything else is
		// a body they would nest inside.
		{"parentheses as a background job", `( echo $DEPTH ) & wait`, "1"},
		{"parentheses with commands before the read", `( true; echo $DEPTH ) & wait`, "1"},
		{"parentheses inside a backgrounded brace group", `{ ( echo $DEPTH ) ; } & wait`, "2"},
		{"parentheses inside a backgrounded function", `f() { ( echo $DEPTH ) ; }; f & wait`, "2"},
		{"parentheses as a pipeline stage", `( echo $DEPTH ) | { read -r v; echo "$v" ; }`, "1"},
		{"parentheses inside a brace group as a pipeline stage", `{ ( echo $DEPTH ) ; } | { read -r v; echo "$v" ; }`, "2"},
		// The row that says the collapse is not a rule about every fork: a
		// command substitution forks and the parentheses inside it fork
		// again, so this is 2 where the two rows above it are 1.
		{"parentheses inside a command substitution", `echo $( ( echo $DEPTH ) )`, "2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runDepth(t, c.src); got != c.want {
				t.Errorf("%s: depth = %s, want %s", c.src, got, c.want)
			}
		})
	}
}
