// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// pipelineSem answers LastPipelineElementInCurrentShell by name.
func pipelineSem(a Answer) Semantics {
	s := permissive()
	s.LastPipelineElementInCurrentShell = a
	return s
}

func TestLastPipelineElementRunsWhereTheDialectSays(t *testing.T) {
	// The axis had a value in every preset and nothing read it. Asserting
	// both sides is the point: taking the majority silently is what it did
	// before, and that looks identical to an implementation on one side.
	const src = `echo x | read v; echo "[$v]"`
	if got, _ := run(t, src, withSem(pipelineSem(Yes))); got != "[x]\n" {
		t.Errorf("current shell: got %q, want %q", got, "[x]\n")
	}
	if got, _ := run(t, src, withSem(pipelineSem(No))); got != "[]\n" {
		t.Errorf("subshell: got %q, want %q", got, "[]\n")
	}
}

// TestPipelineAxisIsAskedOnlyWhenItShows is what keeps the core usable. The
// answer cannot be observed through an external command, so an ordinary
// pipeline must not be refused for want of a dialect.
func TestPipelineAxisIsAskedOnlyWhenItShows(t *testing.T) {
	if got, st := run(t, `echo x | cat`, withSem(CoreSemantics())); got != "x\n" || st != 0 {
		t.Errorf("a plain pipeline should need no answer: got %q status %d", got, st)
	}
	// A builtin at the end can touch the shell, so the core must refuse.
	out, st := run(t, `echo x | read v`, withSem(CoreSemantics()))
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("an observable last element should be refused: got %q status %d", out, st)
	}
	// So can a brace group.
	if _, st := run(t, `echo x | { read v; }`, withSem(CoreSemantics())); st != 2 {
		t.Errorf("a group should be refused, status %d", st)
	}
	// A subshell is already one, so the axis changes nothing.
	if _, st := run(t, `echo x | (cat)`, withSem(CoreSemantics())); st != 0 {
		t.Errorf("a subshell needs no answer, status %d", st)
	}
}

// TestPipelineStillPipes guards the plumbing the axis rearranged: the last
// element reads the pipe whichever shell it runs in.
func TestPipelineStillPipes(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{{"subshell side", pipelineSem(No)}, {"current-shell side", pipelineSem(Yes)}} {
		if got, _ := run(t, `printf 'a\nb\n' | grep b`, withSem(tc.sem)); got != "b\n" {
			t.Errorf("%s: got %q, want %q", tc.name, got, "b\n")
		}
		if got, _ := run(t, `echo one | cat | cat`, withSem(tc.sem)); got != "one\n" {
			t.Errorf("%s: three elements: got %q", tc.name, got)
		}
	}
}

// pipefailSignalSem answers every axis a pipeline whose element a signal
// killed needs: whether pipefail exists, what base a signal death uses, and
// what a *substituted* one uses.
func pipefailSignalSem(base, substituted Answer) Semantics {
	s := permissive()
	s.PipefailOption = Yes
	s.LastPipelineElementInCurrentShell = No
	s.ReportsACommandKilledBySignal = No
	s.ReportsAnyKilledPipelineElement = No
	s.SignalDeathStatusIsTwoFiftySix = base
	s.PipefailSubstitutesTheBareSignal = substituted
	return s
}

// externalSignalPipeline is a real process killing itself, as the element the
// pipeline does not report. An external one rather than a builtin, because the
// base is what a *wait* decodes and only a real child is waited for.
const externalSignalPipeline = `/bin/sh -c 'kill -USR1 $$' | true`

// The status pipefail substitutes for an element a signal killed is its own
// question, and it is not the one that chooses 128 or 256.
//
// The two are crossed deliberately: the base answers wherever the shell
// reports a death, and this answers only where pipefail went looking for a
// status. Asserting all four corners is what stops one from being read as the
// other — the two real shapes are the diagonal, and the panel has no shell at
// either of the other corners.
func TestPipefailSubstitutesTheBareSignalIsItsOwnAxis(t *testing.T) {
	sig := int(syscall.SIGUSR1)
	for _, tc := range []struct {
		name              string
		base, substituted Answer
		want              int
	}{
		{"128 base, the death's own status", No, No, 128 + sig},
		{"128 base, the bare signal", No, Yes, sig},
		{"256 base, the death's own status", Yes, No, 256 + sig},
		{"256 base, the bare signal", Yes, Yes, sig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := pipefailSignalSem(tc.base, tc.substituted)
			got, _ := runWithPath(t, "set -o pipefail\n"+externalSignalPipeline+"\necho \"st=$?\"", sem)
			if want := "st=" + strconv.Itoa(tc.want); !strings.Contains(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// The same substitution for a *builtin* killed where it stands, which is the
// shape the corpus reaches: a write larger than a pipe into a pipe nobody
// reads. The status comes from inside this process rather than from a wait,
// and the axis has to reach it just the same.
func TestPipefailSubstitutesForABuiltinKilledWhereItStands(t *testing.T) {
	const src = `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done
{ echo "$v"; } | true
echo "st=$?"`
	for _, tc := range []struct {
		substituted Answer
		want        string
	}{
		{No, "st=" + strconv.Itoa(128+int(syscall.SIGPIPE))},
		{Yes, "st=" + strconv.Itoa(int(syscall.SIGPIPE))},
	} {
		sem := pipefailSignalSem(No, tc.substituted)
		got, _ := runWithPath(t, "set -o pipefail\n"+src, sem)
		if !strings.Contains(got, tc.want) {
			t.Errorf("substituted=%v: got %q, want %q", tc.substituted, got, tc.want)
		}
	}
}

// The substitution is only for an element pipefail went looking for. An
// element in the position the pipeline reports anyway keeps the status its
// death produced, whichever way the axis is answered — measured, and it is
// what makes this about the substitution rather than about pipelines.
func TestPipefailLeavesTheLastElementsOwnStatusAlone(t *testing.T) {
	sig := int(syscall.SIGUSR1)
	for _, substituted := range []Answer{Yes, No} {
		sem := pipefailSignalSem(Yes, substituted)
		got, _ := runWithPath(t, "set -o pipefail\ntrue | /bin/sh -c 'kill -USR1 $$'\necho \"st=$?\"", sem)
		if want := "st=" + strconv.Itoa(256+sig); !strings.Contains(got, want) {
			t.Errorf("substituted=%v: got %q, want %q", substituted, got, want)
		}
	}
}

// An ordinary non-zero exit is substituted unchanged either way. Without this
// the axis reads as a difference about pipefail in general.
func TestPipefailSubstitutesAnOrdinaryFailureUnchanged(t *testing.T) {
	for _, substituted := range []Answer{Yes, No} {
		sem := pipefailSignalSem(Yes, substituted)
		got, _ := runWithPath(t, "set -o pipefail\n(exit 42) | true\necho \"st=$?\"", sem)
		if !strings.Contains(got, "st=42") {
			t.Errorf("substituted=%v: got %q, want st=42", substituted, got)
		}
	}
}

// Asked only where a substitution actually happened and actually was a signal
// death, so a core with no dialect still runs every other pipeline.
func TestPipefailSignalAxisIsAskedOnlyWhenItShows(t *testing.T) {
	sem := pipefailSignalSem(No, Unspecified)

	for _, src := range []string{
		"set -o pipefail\n(exit 42) | true\necho \"st=$?\"",
		"set -o pipefail\ntrue | true\necho \"st=$?\"",
		// The death, but with no pipefail to substitute it.
		externalSignalPipeline + "\necho \"st=$?\"",
		// And a death in the position the pipeline reports anyway.
		"set -o pipefail\ntrue | /bin/sh -c 'kill -USR1 $$'\necho \"st=$?\"",
	} {
		if got, _ := runWithPath(t, src, sem); strings.Contains(got, "no dialect was chosen") {
			t.Errorf("%q was refused and should not have been: %q", src, got)
		}
	}
	got, _ := runWithPath(t, "set -o pipefail\n"+externalSignalPipeline+"\necho \"st=$?\"", sem)
	if !strings.Contains(got, "no dialect was chosen") {
		t.Errorf("a substituted signal death should be refused: got %q", got)
	}
	if !strings.Contains(got, "st=2") {
		t.Errorf("the refusal must be the status, not one of the two conventions: got %q", got)
	}
}

// runWithPath is run() with a PATH, which every case here needs: the element
// that dies is a real command.
func runWithPath(t *testing.T, src string, sem Semantics) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		r.Semantics = &sem
		r.Env = testPATH()
	})
}

// The signal has to survive the boundary a subshell puts round it: `( cmd )`
// as a pipeline element runs on a copy, and the status travels out of that
// copy without saying what produced it unless something carries the two
// together. Measured — the shell that substitutes the bare signal does so for
// a subshell element as readily as for a bare command.
func TestPipefailSubstitutesThroughASubshell(t *testing.T) {
	sig := int(syscall.SIGUSR1)
	for _, src := range []string{
		"set -o pipefail\n( /bin/sh -c 'kill -USR1 $$' ) | true\necho \"st=$?\"",
		"set -o pipefail\n( ( /bin/sh -c 'kill -USR1 $$' ) ) | true\necho \"st=$?\"",
	} {
		sem := pipefailSignalSem(Yes, Yes)
		got, _ := runWithPath(t, src, sem)
		if want := "st=" + strconv.Itoa(sig); !strings.Contains(got, want) {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}

// What killed the last command is not what ended the element. An element that
// went on to run something else reports that something else, so the signal
// must not be remembered past the command it belongs to.
func TestPipefailForgetsASignalTheElementRanPast(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The death, then an ordinary failure: the failure is the status and
		// nothing about it is a signal.
		{"set -o pipefail\n( /bin/sh -c 'kill -USR1 $$'; false ) | true\necho \"st=$?\"", "st=1"},
		// The death, then a success: the element did not fail at all.
		{"set -o pipefail\n( /bin/sh -c 'kill -USR1 $$'; true ) | (exit 7)\necho \"st=$?\"", "st=7"},
	} {
		sem := pipefailSignalSem(Yes, Yes)
		got, _ := runWithPath(t, tc.src, sem)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A shell with job control waits for its children itself, and that wait is
// told the signal directly rather than through an error. It is a second path
// to the same fact and far enough from the first to be worth its own test.
func TestPipefailSubstitutesFromAWatchedWait(t *testing.T) {
	sem := pipefailSignalSem(Yes, Yes)
	var out strings.Builder
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: &out, Stderr: &strings.Builder{}, Env: testPATH(),
		WaitForCommand: func(int) (Wait, error) {
			return Wait{Killed: true, Signal: syscall.SIGUSR1}, nil
		},
	})
	f, err := syntax.Parse("set -o pipefail\n/usr/bin/true | true\necho \"st=$?\"\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if want := "st=" + strconv.Itoa(int(syscall.SIGUSR1)); !strings.Contains(out.String(), want) {
		t.Errorf("got %q, want %q — the caller's own wait carries the signal too", out.String(), want)
	}
}

// The axis says where a shell stands; this says whether the session moved it.
//
// One shell in the panel has a name for the move — `shopt -s lastpipe` — and
// that name is its dialect's, so what the core owns is the switch and not the
// spelling. Both directions are asserted for the reason the axis test above
// asserts both: a switch that only ever reads on is indistinguishable from an
// implementation that ignores it.
func TestTheSessionCanKeepTheLastPipelineElement(t *testing.T) {
	const src = `echo x | read v; echo "[$v]"`
	keep := func(on bool) func(*Runner) {
		return func(r *Runner) {
			s := pipelineSem(No)
			r.Semantics = &s
			r.SetKeepsLastPipelineElement(on)
		}
	}
	if got, _ := run(t, src, keep(true)); got != "[x]\n" {
		t.Errorf("switch on over a subshell axis: got %q, want %q", got, "[x]\n")
	}
	if got, _ := run(t, src, keep(false)); got != "[]\n" {
		t.Errorf("switch off: got %q, want %q", got, "[]\n")
	}
	// And the getter reads back what was written, which is what the dialect's
	// query prints. The zero value is the subject here — a Runner nobody has
	// spoken to has to follow the axis — so it runs nothing and needs no
	// directory. testrunner:bare
	r := &Runner{}
	if r.KeepsLastPipelineElement() {
		t.Error("a Runner nobody has spoken to keeps the last element")
	}
	r.SetKeepsLastPipelineElement(true)
	if !r.KeepsLastPipelineElement() {
		t.Error("the switch did not read back on")
	}
}

// The monitor overrules the switch, and that is the half an implementation is
// most likely to skip. Measured in bash 5.3.15 on 2026-09-12: with the option
// set, `set -m` in front of the pipeline answers `[]` and `set -m; set +m`
// answers `[hi]`, so it is the monitor's state *at the pipeline* that decides.
func TestTheMonitorOverrulesTheKeptLastElement(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set +m; echo x | read v; echo "[$v]"`, "[x]\n"},
		{`set -m; echo x | read v; echo "[$v]"`, "[]\n"},
		// Turned on and off again: the option was never rewritten, so a
		// reading taken when the option was set would answer the first row.
		{`set -m; set +m; echo x | read v; echo "[$v]"`, "[x]\n"},
	} {
		got, _ := run(t, tc.src, func(r *Runner) {
			s := pipelineSem(No)
			s.MonitorNeedsATerminal = No
			r.Semantics = &s
			r.SetKeepsLastPipelineElement(true)
		})
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A shell the switch has moved must not report an unanswered axis, which is
// the shape that would make the core refuse a pipeline it was about to run in
// line. The axis is left Unspecified on purpose: the switch is the answer.
func TestTheKeptLastElementNeedsNoAxis(t *testing.T) {
	got, st := run(t, `echo x | read v; echo "[$v]"`, func(r *Runner) {
		s := CoreSemantics()
		r.Semantics = &s
		r.SetKeepsLastPipelineElement(true)
	})
	if got != "[x]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", got, st, "[x]\n")
	}
}

// The switch reaches the last element and no other. Every element but the last
// is a subshell in every shell in the panel, with or without the name, so this
// is what parts an option from a shell that stopped piping.
func TestTheKeptElementIsOnlyTheLastOne(t *testing.T) {
	got, _ := run(t, `echo x | read v | cat; echo "[$v]"`, func(r *Runner) {
		s := pipelineSem(No)
		r.Semantics = &s
		r.SetKeepsLastPipelineElement(true)
	})
	// `read` in the middle eats the line, so `cat` has nothing to write and
	// the whole of the output is the empty variable.
	if got != "[]\n" {
		t.Errorf("got %q, want %q", got, "[]\n")
	}
}

// A subshell carries its own copy, so `( … )` turning it on stays inside.
func TestTheKeptLastElementDoesNotEscapeASubshell(t *testing.T) {
	got, _ := run(t, `( echo x | read v; echo "in[$v]" ); echo "out[$v]"`, func(r *Runner) {
		s := pipelineSem(No)
		r.Semantics = &s
		r.SetKeepsLastPipelineElement(true)
	})
	if got != "in[x]\nout[]\n" {
		t.Errorf("got %q, want %q", got, "in[x]\nout[]\n")
	}
}
