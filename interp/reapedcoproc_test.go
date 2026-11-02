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

// The three values of Semantics.ReapedCoprocessEnds, named by the axis rather
// than by the shells that hold them, over one snippet that can tell all three
// apart: the coprocess ends, the shell is made to notice by a `wait`, and then
// both ends are asked after.
//
// `${CP[1]}` empty and the redirection ambiguous means the array went with the
// ends; `read` answering `hi` means the read end outlived the reaping.
//
// # Why a `wait` and not a subshell
//
// The row asks what the reaping takes back, so it needs a reaping, and `( : )`
// only delivers one if the coprocess got there first. Nothing orders the two:
// the body is a goroutine and the subshell costs microseconds, so at
// GOMAXPROCS=1 the subshell sometimes arrived with nothing to reap. The array
// was then still there and the "ends go" row read `n=2` and died on SIGPIPE
// like the "ends survive" one — 3 runs in 8000 on macOS, and on CI's ubuntu
// leg (#7299). Real bash loses the same race: `coproc CP { echo hi; sleep
// 0.2; }; ( : )` leaves `${#CP[@]}` at 2 in bash 5.3.20, and a `wait` in its
// place answers 0. That is TestASubshellDeliversTheReapNotice's #2661 again,
// which pins the subshell as a trigger on its own.
//
// A `wait` reaps the coprocess itself, so the notice is delivered once the
// body has ended and not merely once it is about to, and it leaves what the
// body wrote in the pipe — which is what the `read` row needs, and why the
// read-to-end-of-file handshake that test uses would not do here.
func TestReapedCoprocessEndsDecidesWhatIsTakenBack(t *testing.T) {
	const src = `coproc CP { echo hi; }
wait
echo "n=${#CP[@]}"
read -r a <&"${CP[0]}"
echo "read=$? a=[$a]"
echo x >&"${CP[1]}"
echo "write=$?"`
	for _, c := range []struct {
		disposal CoprocEndDisposal
		out      string
		status   int
	}{
		// Nothing taken back, so the write reaches a pipe with no reader and
		// the shell dies on SIGPIPE where it stands — `write=` never printed.
		// That death is shared ground and is the behavior this axis exists to
		// keep a script from *reaching*, rather than one to suppress.
		{CoprocEndsSurviveTheCoprocess, "n=2\nread=0 a=[hi]\n", 128 + 13},
		{CoprocWriteEndGoesWithTheCoprocess, "n=2\nread=0 a=[hi]\nwrite=1\n", 0},
		{CoprocEndsGoWithTheCoprocess, "n=0\nread=1 a=[]\nwrite=1\n", 0},
	} {
		got, st := runReapedCoproc(t, c.disposal, src)
		if got != c.out || st != c.status {
			t.Errorf("%v: got %q at %d, want %q at %d", c.disposal, got, st, c.out, c.status)
		}
	}
}

// The notice is delivered where the shell reaps a child and nowhere else, so
// builtins alone leave everything where it was — under every value of the
// axis, which is what says this is the trigger and not one of its answers.
func TestBuiltinsAloneDeliverNoReapNotice(t *testing.T) {
	const src = `coproc CP { echo hi; }
:
:
:
echo "n=${#CP[@]}"
read -r a <&"${CP[0]}"
echo "read=$? a=[$a]"`
	for _, d := range []CoprocEndDisposal{
		CoprocEndsSurviveTheCoprocess,
		CoprocWriteEndGoesWithTheCoprocess,
		CoprocEndsGoWithTheCoprocess,
	} {
		got, st := runReapedCoproc(t, d, src)
		if want := "n=2\nread=0 a=[hi]\n"; got != want || st != 0 {
			t.Errorf("%v: got %q at %d, want %q at 0", d, got, st, want)
		}
	}
}

// runReapedCoproc runs src with the coprocess keyword on, the array model
// answered, and the disposal under test.
//
// Standard error is discarded rather than joined: the wording of an ambiguous
// redirect is a dialect's and this asks only what was taken back.
func runReapedCoproc(t *testing.T, disposal CoprocEndDisposal, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.Coproc = true
	d.CoprocName = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.CoprocEndsInAnArray = Yes
	sem.ReapedCoprocessEnds = disposal
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: &strings.Builder{},
	})
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), st
}
