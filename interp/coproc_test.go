// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `coproc cat` runs cat in the background with a pipe on each named stream:
// [1] feeds it, [0] reads it back, and NAME_PID is the process — which is
// what closing the feed and waiting proves.
func TestCoprocTalksBothWaysAndCanBeWaitedFor(t *testing.T) {
	out := runCoproc(t,
		`coproc /bin/cat
echo hi >&"${COPROC[1]}"
read -r l <&"${COPROC[0]}"
echo "got=$l"
[ -n "$COPROC_PID" ] && echo pidset
exec {COPROC[1]}>&-
wait "$COPROC_PID"
echo "st=$?"`)
	if got, want := out, "got=hi\npidset\nst=0\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A compound command may carry a name of its own, and the array is called
// that instead.
func TestCoprocTakesAName(t *testing.T) {
	out := runCoproc(t,
		`coproc UP { /bin/cat; }
echo hey >&"${UP[1]}"
read -r l <&"${UP[0]}"
echo "got=$l"
exec {UP[1]}>&-
wait "$UP_PID"`)
	if got, want := out, "got=hey\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// runCoproc parses src with the keyword turned on and runs it with the posix
// answers.
//
// The subscripted descriptor name goes with it, because closing the feed is
// how a coprocess is told its input has ended and `exec {NAME[1]}>&-` is how
// that is written. These cases used to copy the element into a scalar first
// and close through that, which is the workaround for a shell without the
// flag rather than anything a script would say.
func runCoproc(t *testing.T, src string) string {
	t.Helper()
	d := syntax.Core()
	d.Coproc = true
	d.FdVariableSubscript = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	out := &strings.Builder{}
	r := &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: &strings.Builder{},
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
