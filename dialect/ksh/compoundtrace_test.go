// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// kshTrace is kshOut with the trace on its own stream, so a row asserts what
// `set -x` wrote and not what the script printed.
func kshTrace(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		return "parse: " + err.Error(), -1
	}
	var out, errOut bytes.Buffer
	s, d := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errOut,
		Semantics: &s, Diagnostics: &d, Name: "ksh", Dialect: presetDialect(),
	}
	ksh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return errOut.String(), st
}

// A compound literal is traced as the assignments its body performs, one line
// each, rather than as the parentheses it was written with. Measured on
// ksh93u+ 2012-08-01, 2026-09-14, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME, over `-c`, a script file and standard input alike.
//
// It is the explanation of what #1959 recorded from the other end: `set -x;
// b=(); echo after` writes no line for the assignment here, where bash writes
// `b=()` and zsh writes `b=( )`. Not a rule about an empty array literal —
// `b=()` is an empty *compound* in this shell, so it performs no assignments
// and there is nothing to write. A reading that special-cased the empty
// literal would have written one line for `c=(a=1 b=2)` and passed the row it
// was measured from.
func TestACompoundLiteralIsTracedAsItsMembers(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; c=(a=1); echo after`, "+ c.a=1\n+ echo after\n"},
		// One line per member, in the order the body wrote them.
		{`set -x; c=(a=1 b=2); echo after`, "+ c.a=1\n+ c.b=2\n+ echo after\n"},
		// A nested body is its members too, under their own names.
		{`set -x; c=(a=1 b=(y=2)); echo after`, "+ c.a=1\n+ c.b.y=2\n+ echo after\n"},
		// The empty compound: no members, so no line — and the command after
		// it is what says the trace is still running.
		{`set -x; b=(); echo after`, "+ echo after\n"},
		// The append spelling of an empty body is the same nothing, over a
		// name already holding an array.
		{`set -x; b=(1); b+=(); echo after`, "+ b=( 1 )\n+ echo after\n"},
		// And the control: a literal of plain words is not a compound and is
		// traced as the literal it is, so the rule above is about the
		// construct rather than about the parentheses.
		{`set -x; b=(1 2); echo after`, "+ b=( 1 2 )\n+ echo after\n"},
	} {
		if out, st := kshTrace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}
