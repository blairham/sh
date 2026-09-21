// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A command substitution whose body will not parse, written inside a subshell
// (#3274).
//
// Measured 2026-09-16 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> case.sh` with stdin from /dev/null, BusyBox v1.37.0 in the
// digest-pinned Alpine image internal/oracle reaches, under `--init`:
//
//	                     output                 status  continued
//	bash 5.3.20          start                  2       no
//	that binary as `sh`  start                  2       no
//	bash 3.2.57          start, inner, after=0  0       yes
//	zsh 5.9.2            start                  1       no
//	ksh93u+ 2012-08-01   start, after st=3      0       yes
//	dash 0.5.12          start                  2       no
//	BusyBox ash 1.37.0   start                  2       no
//
// Five end the script and ours ended only the subshell, in every dialect.
//
// **Whether the script continued is the assertion**, not the status: a shell
// that reported the failure, left 2 behind and carried on has the same status
// as one that stopped, and that is exactly what was wrong here. So standard
// output is read on its own — the `after` line is present or it is not — with
// the diagnostic kept on its own stream so the row is about the fatality and
// not about anybody's wording.
//
// The wording is a separate gap and is why three of these columns have no
// suite file of their own for this: bash, zsh and ksh93 name the `)` that
// closed the substitution where we name what the body alone ran out of, so a
// tier that compared both streams would be red on the sentence while the
// fatality was right. It is filed as #3296, `dash/` and `ash/` are byte-perfect
// and do have the file, and this row is what grades the other three.
func TestASubstitutionThatWillNotParseEndsTheScriptFromInsideASubshell(t *testing.T) {
	const src = "printf 'start\\n'\n" +
		"( v=$(echo hi; for); printf 'inner carried on\\n' )\n" +
		"printf 'after the subshell st=%s\\n' \"$?\"\n"
	for _, c := range []struct {
		preset string
		out    string
		status int
	}{
		{"bash", "start\n", 2},
		{"zsh", "start\n", 1},
		{"dash", "start\n", 2},
		{"ash", "start\n", 2},
		{"posix", "start\n", 2},
		// The one column that contains it, and it is contained rather than
		// missed: the subshell still dies, so `inner carried on` is absent
		// here too and the status the script reads is the syntax status.
		{"ksh", "start\nafter the subshell st=3\n", 0},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, errs, st := splitRun(t, presets[c.preset], src)
			if out != c.out || st != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, st, c.out, c.status)
			}
			if errs == "" {
				t.Errorf("nothing was reported; the failure has to be visible as well as fatal")
			}
			if strings.Contains(out, "inner carried on") {
				t.Errorf("the subshell ran on past the failure: %q", out)
			}
		})
	}
}

// The two controls, which say what this is not — and which every dialect
// already answered before #3274, so a fix that had simply made every parse
// failure fatal everywhere would pass the case above and fail these.
//
//   - The **same substitution at the top level**: fatal in every column and in
//     every dialect, each at its own syntax status. This is the one that says
//     SubstitutionParseErrorIsFatal is answered correctly and that the
//     fatality merely did not reach past the subshell.
//   - A **plain** parse error inside a subshell, `( for; do :; done )`: not a
//     runtime question at all. The *file* does not parse, in every dialect, so
//     nothing in it runs — which is fatal in every column and is why "a
//     subshell swallows a parse error" was never the explanation.
func TestTheTwoControlsThatSayWhatTheSubshellCaseIsNot(t *testing.T) {
	const top = "printf 'start\\n'\nv=$(echo hi; for)\nprintf 'carried on\\n'\n"
	for _, c := range []struct {
		preset string
		status int
	}{
		{"bash", 2}, {"zsh", 1}, {"ksh", 3}, {"dash", 2}, {"ash", 2}, {"posix", 2},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, _, st := splitRun(t, presets[c.preset], top)
			if out != "start\n" || st != c.status {
				t.Errorf("the top-level case wrote %q at %d, want %q at %d", out, st, "start\n", c.status)
			}
		})
	}
	const plain = "printf 'start\\n'\n( for; do :; done )\nprintf 'carried on\\n'\n"
	for name, p := range presets {
		if _, err := syntax.Parse(plain, p.Dialect()); err == nil {
			t.Errorf("%s read a plain parse error inside a subshell as a program", name)
		}
	}
}

// The fatality reaches out of every shell a body can be written in, not only
// out of the parentheses — which is what makes it one rule rather than a
// special case for `( … )`.
//
// Measured 2026-09-16 on bash 5.3.20, zsh 5.9.2 and dash 0.5.12: each of these
// writes `start` and nothing else and ends at that shell's syntax status,
// where ksh93u+ writes the line after and carries on in all three.
//
// The pipeline row is the one that needed the status carried with the stop.
// `cat` succeeds, so a script that stopped on the pipeline's own status would
// end at **0** — reported as dead and exiting like a success.
func TestTheFatalityReachesOutOfEveryShellABodyCanBeWrittenIn(t *testing.T) {
	for _, shape := range []struct{ name, src string }{
		{"a pipeline element", "printf 'start\\n'\nv=$(echo hi; for) | cat\nprintf 'after=%s\\n' \"$?\"\n"},
		{"a subshell inside a subshell", "printf 'start\\n'\n( ( v=$(echo hi; for) ) )\nprintf 'after=%s\\n' \"$?\"\n"},
		{"an enclosing command substitution", "printf 'start\\n'\nx=$( ( v=$(echo hi; for) ) )\nprintf 'after=%s\\n' \"$?\"\n"},
	} {
		t.Run(shape.name, func(t *testing.T) {
			for _, c := range []struct {
				preset string
				status int
			}{{"bash", 2}, {"zsh", 1}, {"dash", 2}, {"ash", 2}, {"posix", 2}} {
				out, _, st := splitRun(t, presets[c.preset], shape.src)
				if out != "start\n" || st != c.status {
					t.Errorf("%s wrote %q at %d, want %q at %d", c.preset, out, st, "start\n", c.status)
				}
			}
			// And contained in ksh93 however deep, which is the same one
			// answer rather than a boundary that happens to hold here.
			out, _, _ := splitRun(t, presets["ksh"], shape.src)
			if !strings.Contains(out, "after=") {
				t.Errorf("ksh wrote %q, want the script to carry on past it", out)
			}
		})
	}
}

// The EXIT trap still runs, and sees the status the script died of — which is
// the half a hold rather than a clear buys. Measured 2026-09-16 with `trap
// 'printf "bye st=%s\n" "$?"' EXIT` above the failing line: bash 5.3.20 writes
// `bye st=2`, zsh 5.9.2 `bye st=1`, dash `bye st=2`.
//
// Two shapes, because they take two different roads to the same handler: with
// a command after the subshell the stop is taken by that command and the trap
// runs afterwards with the box already empty; with none, the box is still full
// when the trap begins, and a clear there would have swallowed the handler.
func TestTheExitTrapRunsAfterAFatalSubstitutionAndSeesItsStatus(t *testing.T) {
	const withTail = "trap 'printf \"bye st=%s\\n\" \"$?\"' EXIT\nprintf 'start\\n'\n" +
		"( v=$(echo hi; for) )\nprintf 'after\\n'\n"
	const bare = "trap 'printf \"bye st=%s\\n\" \"$?\"' EXIT\nprintf 'start\\n'\n" +
		"( v=$(echo hi; for) )\n"
	for _, c := range []struct {
		preset string
		status int
	}{{"bash", 2}, {"zsh", 1}, {"dash", 2}, {"ash", 2}, {"posix", 2}} {
		t.Run(c.preset, func(t *testing.T) {
			for _, src := range []string{withTail, bare} {
				out, _, st := splitRun(t, presets[c.preset], src)
				want := "start\nbye st=" + strconv.Itoa(c.status) + "\n"
				if out != want || st != c.status {
					t.Errorf("wrote %q at %d, want %q at %d", out, st, want, c.status)
				}
			}
		})
	}
	// ksh93 contains the failure, so its trap sees the script's own end: 0
	// where a command followed the subshell and the syntax status where none
	// did. Both measured on ksh93u+ 2012-08-01.
	for _, c := range []struct {
		src, want string
	}{
		{withTail, "start\nafter\nbye st=0\n"},
		{bare, "start\nbye st=3\n"},
	} {
		if out, _, _ := splitRun(t, presets["ksh"], c.src); out != c.want {
			t.Errorf("ksh wrote %q, want %q", out, c.want)
		}
	}
}

// The core is asked only where the columns differ, which for this axis is
// *inside a subshell*: the top-level failure is unanimous and answered, and
// only the subshell case refuses.
//
// That is the row that keeps the axis off the common path. Asking it at every
// substitution failure would make a shell with no dialect refuse a question
// every reference agrees about.
func TestTheCoreIsAskedOnlyFromInsideASubshell(t *testing.T) {
	core := dialecttest.Preset{
		Name: "sh", Dialect: syntax.Core,
		Semantics: interp.CoreSemantics, Diagnostics: interp.CoreDiagnostics,
		Apply: noApply,
	}
	_, errs, st := splitRun(t, core, "printf 'start\\n'\nv=$(echo hi; for)\n")
	if strings.Contains(errs, "no dialect was chosen") || st == 0 {
		t.Errorf("the core refused the top-level case: %q at %d", errs, st)
	}
	_, errs, _ = splitRun(t, core, "printf 'start\\n'\n( v=$(echo hi; for) )\nprintf 'after\\n'\n")
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("the core answered the subshell case rather than refusing: %q", errs)
	}
}

// splitRun runs src on the preset's own shell with the two streams kept apart,
// and reports what each got and the status the run ended at.
//
// Apart rather than joined, which every case in this file turns on: what is
// being asserted is whether the *script* carried on, and a diagnostic mixed
// into standard output would make "stopped" and "stopped and complained"
// indistinguishable from "carried on with a complaint in the middle".
func splitRun(t *testing.T, p dialecttest.Preset, src string) (out, errs string, status int) {
	t.Helper()
	f := p.Parse(t, src)
	var o, e strings.Builder
	r := p.Runner(dialecttest.Base{Stdout: &o, Stderr: &e})
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return o.String(), e.String(), st
}

// The same fatality one substitution further in: a body refused **inside**
// another substitution's body, which is where it stopped reaching (#3355).
//
// Measured 2026-09-20 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> s.sh` with standard input on the null device, over
// `echo A$(echo B$(for)C)D` with a `printf` before it and an `after st=%s`
// after it:
//
//	                     output                  status
//	bash 5.3.20          start                   2
//	bash 3.2.57          start, ABCD, after=0    0
//	zsh 5.9.2            start                   1
//	ksh93u+ 2012-08-01   start, AD, after=0      0
//	dash 0.5.12          start                   2
//
// Three end the script, and the word holding the outer substitution is never
// expanded in any of them — `ABCD` is absent from every column but the one
// that does not stop at all. Here the outer body was abandoned and the
// *statement* went on, so `AD` was written and the script carried to its end
// at status 0: the stop was in the box every clone shares and nothing took
// it until the next command, which was a command the failure should have
// prevented. See Runner.pendingScriptStop.
//
// **ksh93 is the control and it is why the box is peeked rather than
// widened.** It contains the stop at the subshell, so nothing is recorded for
// anything to take, and its row is the one that still writes `AD` and carries
// on. A change that had made the give-up escape the body directly would have
// moved that column too.
func TestASubstitutionRefusedInsideAnotherBodyEndsTheScriptToo(t *testing.T) {
	const src = "printf 'start\\n'\n" +
		"echo A$(echo B$(for)C)D\n" +
		"printf 'after st=%s\\n' \"$?\"\n"
	for _, c := range []struct {
		preset string
		out    string
		status int
	}{
		{"bash", "start\n", 2},
		{"zsh", "start\n", 1},
		{"dash", "start\n", 2},
		{"ksh", "start\nAD\nafter st=0\n", 0},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, errs, st := splitRun(t, presets[c.preset], src)
			if out != c.out || st != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, st, c.out, c.status)
			}
			if errs == "" {
				t.Errorf("nothing was reported; the failure has to be visible as well as fatal")
			}
			if strings.Contains(out, "ABCD") {
				t.Errorf("the outer word was expanded past the failure: %q", out)
			}
		})
	}
}

// And the same stop when the statement that held the substitution is the
// script's **last** one, which is where nothing took it (#3355).
//
// The box is drained at the sequence point in Runner.stmt, and that point is
// the *next* command: a script whose last line holds the failure runs out
// before it is reached, and a prompt's own drain never comes for a script. So
// the failure was reported, the shell stopped, and the status it left was
// whatever the last command had made it.
//
// Measured 2026-09-20 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> s.sh` with standard input on the null device, over `printf 'one\n'`
// and then `v=$(echo hi; for) | :`:
//
//	                     output   status
//	zsh 5.9.2            one      1
//	bash 5.3.20          one      2
//	dash 0.5.12          one      2
//	bash 3.2.57          one      0
//	ksh93u+ 2012-08-01   one      0
//
// A **pipeline element** because that is a shell of its own whose stop has to
// travel in the box, and because the pipeline is waited for — so the number
// is the same every run. It is also the shape scriptStop.status was written
// for: the pipeline's own status is `:`'s, and reporting that instead of the
// failure's is exactly the 0 this leaves.
func TestASubstitutionRefusedInTheLastStatementStillSetsTheStatus(t *testing.T) {
	const src = "printf 'one\\n'\nv=$(echo hi; for) | :\n"
	for _, c := range []struct {
		preset string
		status int
	}{
		{"bash", 2},
		{"zsh", 1},
		{"dash", 2},
		{"ksh", 0},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, errs, st := splitRun(t, presets[c.preset], src)
			if out != "one\n" || st != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, st, "one\n", c.status)
			}
			if errs == "" {
				t.Errorf("nothing was reported; the failure has to be visible as well as counted")
			}
		})
	}
}
