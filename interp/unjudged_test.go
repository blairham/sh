// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// stripTiming takes the report a `time` clause writes out of the output, so a
// case can assert on what its own probes printed. The report's numbers are a
// clock and cannot be written down, and the blank line it opens with is part
// of it.
var timingLine = regexp.MustCompile(`\b(real|user|sys)\b`)

func stripTiming(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || timingLine.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

// onlyProbe is the same for a diagnostic the shell wrote about a file it could
// not open: the path is a temporary directory's and the wording is the
// dialect's, and neither is what these cases are about.
func onlyProbe(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "survived") || strings.Contains(line, "st=") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

// Three failures a column declines to judge the way the rest of the panel
// does, and two of them share a mechanism: `set -e` and the ERR trap judge a
// statement once, and a construct that has already returned needs a mark
// rather than the inherited counter a *running* condition uses.

// judgeRun answers the axes a judging case needs and turns the ERR trap on.
func judgeRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.TimeKeyword = true
	}, func(r *Runner) {
		sem := permissive()
		sem.TrapHasErrCondition = Yes
		sem.ErrTrapRunsInsideFunctions = Yes
		sem.ErrTrapRunsInSubshells = No
		sem.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapFiresOnceForTheFailure
		set(&sem)
		r.Semantics = &sem
	})
}

// What a `time` clause does to the two judges. One column times its command in
// a context where nothing is judged at all — not the timed command, not the
// commands inside whatever it calls, and not the clause itself.
//
// The inside and the clause are two different halves and only the first is the
// inherited counter's: the counter is dropped when the clause returns and the
// statement is judged after that. A fix that raised the counter alone left
// `set -e; time false` stopping the script.
func TestWhetherATimedCommandIsJudged(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
	}{
		{"a simple command", `time false`},
		{"a pipeline", `time true | false`},
		{"a group whose first command failed", `time { false; }`},
		{"a function call", `f() { false; }; time f`},
		{"a loop", `time for i in 1; do false; done`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.src + `; printf survived`
			for _, w := range []struct {
				judged Answer
				want   string
			}{{Yes, ""}, {No, "survived"}} {
				got, _ := judgeRun(t, `set -e; `+src, func(s *Semantics) {
					s.TimedCommandIsJudged = w.judged
				})
				if got := stripTiming(got); got != w.want {
					t.Errorf("judged=%v under set -e: got %q, want %q", w.judged, got, w.want)
				}
			}
			for _, w := range []struct {
				judged Answer
				want   string
			}{{Yes, "Esurvived"}, {No, "survived"}} {
				got, _ := judgeRun(t, `trap 'printf E' ERR; `+src, func(s *Semantics) {
					s.TimedCommandIsJudged = w.judged
				})
				if got := stripTiming(got); got != w.want {
					t.Errorf("judged=%v under an ERR trap: got %q, want %q", w.judged, got, w.want)
				}
			}
		})
	}
}

// And the suspension ends with the clause, which is what makes it a context
// rather than an option: the status is untouched, a failure after the clause
// still stops the script, and a `||` still sees the failure the judges did not.
func TestTheTimedSuspensionEndsWithTheClause(t *testing.T) {
	unjudged := func(s *Semantics) { s.TimedCommandIsJudged = No }
	if got, _ := judgeRun(t, `set -e; time true; false; printf reached`, unjudged); stripTiming(got) != "" {
		t.Errorf("a failure after the clause should still stop the script, got %q", got)
	}
	if got, _ := judgeRun(t, `time false; printf 'st=%s' "$?"`, unjudged); stripTiming(got) != "st=1" {
		t.Errorf("the status should be the timed command's, got %q", got)
	}
	if got, _ := judgeRun(t, `set -e; time false || printf ran; printf ' after'`, unjudged); stripTiming(got) != "ran after" {
		t.Errorf("a `||` should still see the failure, got %q", got)
	}
}

// A redirection this shell could not open on a **compound** command, which one
// column leaves at status 1 without calling either judge. The same line on a
// *simple* command is a failure there as it is everywhere, which is the
// control that says the axis is about the compound and not about redirection.
func TestWhetherACompoundsRedirectionFailureIsJudged(t *testing.T) {
	bad := t.TempDir() + "/nodir/f"
	for _, c := range []struct {
		name     string
		src      string
		compound bool
	}{
		{"a group", `{ echo b; } > ` + bad, true},
		{"a loop", `for i in 1; do :; done > ` + bad, true},
		{"a while loop", `while false; do :; done > ` + bad, true},
		{"an if", `if :; then :; fi > ` + bad, true},
		{"a case", `case x in x) :;; esac > ` + bad, true},
		{"a subshell", `( echo x ) > ` + bad, true},
		{"an input redirection", `{ echo b; } < ` + bad, true},
		{"a simple command", `echo x > ` + bad, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, judged := range []Answer{Yes, No} {
				want := ""
				if judged == No && c.compound {
					want = "survived"
				}
				got, _ := judgeRun(t, `set -e; `+c.src+`; printf survived`, func(s *Semantics) {
					s.CompoundRedirectionFailureIsJudged = judged
				})
				if got := onlyProbe(got); got != want {
					t.Errorf("judged=%v: got %q, want %q", judged, got, want)
				}
			}
			// And the status is 1 either way, so it is the judging alone
			// that the axis moves.
			got, _ := judgeRun(t, c.src+`; printf 'st=%s' "$?"`, func(s *Semantics) {
				s.CompoundRedirectionFailureIsJudged = No
			})
			if onlyProbe(got) != "st=1" {
				t.Errorf("the status should be 1 whatever the axis says, got %q", got)
			}
		})
	}
}

// A subshell written as the last element of a pipeline, which one column
// judges *inside* the element's own process as well as judging the pipeline in
// the shell.
//
// `( exit 3 )` is the discriminating row: it has no failing command in it, so
// a second firing there is the subshell command itself and not its body.
func TestWhetherASubshellAsTheLastPipelineElementJudgesItself(t *testing.T) {
	for _, c := range []struct {
		name       string
		src        string
		once, both string
	}{
		{"a failing body", `true | ( false )`, "E .", "EE ."},
		{"no failing command in it at all", `true | ( exit 3 )`, "E .", "EE ."},
		{"a nested subshell", `true | ( ( false ) )`, "E .", "EE ."},
		{"the first element failing too", `false | ( false )`, "E .", "EE ."},
		{"an earlier element that is also a subshell", `true | ( false ) | ( false )`, "E .", "EE ."},
		// The three that fire once whatever the axis says, and each is a
		// different reason: a group is not a subshell command, an element
		// that succeeded is not judged at all, and a subshell that is not
		// the last element is not the one this is about.
		{"a group holding the subshell", `true | { ( false ); }`, "E .", "E ."},
		{"a subshell of its own outside a pipeline", `( false )`, "E .", "E ."},
		{"a subshell that is not the last element", `( false ) | true`, " .", " ."},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, w := range []struct {
				answer Answer
				want   string
			}{{No, c.once}, {Yes, c.both}} {
				got, _ := judgeRun(t, `trap 'printf E' ERR; `+c.src+`; printf ' .'`, func(s *Semantics) {
					s.ASubshellAsTheLastPipelineElementJudgesItself = w.answer
				})
				if got != w.want {
					t.Errorf("answer=%v: got %q, want %q", w.answer, got, w.want)
				}
			}
		})
	}
}

// And it is the *last* element alone, which needs a trap writing to the shared
// error stream to see: an element's standard output is the pipe to the element
// after it, so a firing in any element but the last writes into that pipe and
// is invisible to a case reading standard output.
//
// Measured with the same probe: `true | ( false ) | ( false )` under
// `trap 'printf E >&2' ERR` writes two E and not three in the column that
// answers yes, so an earlier subshell element is not judged where it ran.
func TestOnlyTheLastPipelineElementJudgesItselfAsASubshell(t *testing.T) {
	for _, w := range []struct {
		answer Answer
		want   int
	}{{No, 1}, {Yes, 2}} {
		got, _ := judgeRun(t, `trap 'printf E >&2' ERR; true | ( false ) | ( false ); printf ' .'`,
			func(s *Semantics) { s.ASubshellAsTheLastPipelineElementJudgesItself = w.answer })
		if n := strings.Count(got, "E"); n != w.want {
			t.Errorf("answer=%v: %d firings in %q, want %d", w.answer, n, got, w.want)
		}
	}
}
