// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An empty operand to a file test.
//
// Measured 2026-09-07 under `env -i` with HOME, ZDOTDIR and HISTFILE in a
// scratch directory: bash 5.3, bash 3.2, dash, ksh93 and zsh 5.9.2 all
// answer false — status 1 — for every file operator given an empty operand,
// in `[ ]`, `test` and `[[ ]]` alike. Unanimous, so core.
//
// It was true for six of them here: `atDir("")` was `filepath.Join(r.Dir,
// "")`, which is `r.Dir`, so an empty operand stat'd the *working
// directory*. A directory exists, is a directory, is readable, writable,
// executable and has a non-zero size — `-e -d -s -r -w -x` all said yes —
// and is not a regular file, so `-f` was right by accident. That accident is
// why the bug lasted: the guard most scripts write is `[ -f "$x" ]`, and the
// one that matters, `[ -d "$dir" ]`, is the one that lied. A real plugin
// manager chose its cache directory with `[[ -d $XDG_CACHE_HOME ]]`, took
// the branch, and tried to build its layout at `/`.

// emptyFileOperators is the operand family, all of it — not just the six
// that were wrong. `-t` is excluded because it asks about a descriptor
// rather than a path and its empty answer is an axis of its own
// (TerminalTestRequiresANumber); `-N`, `-O` and `-G` are excluded because
// they are not operators here yet at all.
var emptyFileOperators = []string{
	"-e", "-f", "-d", "-s", "-r", "-w", "-x",
	"-b", "-c", "-p", "-S", "-L", "-h", "-g", "-u", "-k",
}

// emptyOperandForms are the four ways a script arrives at an empty operand.
// A caller needs all four rejected, and no shell in the panel distinguishes
// them: expansion has already made them one empty word by the time the test
// sees it. Asserted anyway, because "unset" and "set but empty" are
// different states of the variable table and only the operand is the same.
var emptyOperandForms = []struct{ name, prelude, word string }{
	{"unset", "unset NOPE\n", `"$NOPE"`},
	{"set and empty", "NOPE=\n", `"$NOPE"`},
	{"substituted empty", "NOPE=$(false)\n", `"$NOPE"`},
	{"a literal empty word", "", `""`},
}

// TestEmptyFileOperandIsFalseInEveryConstruct asserts the exact boolean and
// the exact status for each operator in each of the three spellings.
//
// The boolean is printed rather than inferred, because the bug's answer was
// a plausible one at status 0: any assertion that only checks for the
// absence of an error passes against it.
//
// r.Dir is a directory that certainly exists and is certainly readable,
// writable and executable, which is exactly the state that made the wrong
// answers wrong. A test run from a directory that did not exist would pass
// without the fix.
func TestEmptyFileOperandIsFalseInEveryConstruct(t *testing.T) {
	dir := t.TempDir()
	for _, form := range emptyOperandForms {
		for _, op := range emptyFileOperators {
			for _, spelling := range []string{
				"[ " + op + " WORD ]",
				"test " + op + " WORD",
				"[[ " + op + " WORD ]]",
			} {
				cond := strings.Replace(spelling, "WORD", form.word, 1)
				src := form.prelude + "if " + cond + "; then echo T; else echo F; fi"
				out, st := fileCondRun(t, dir, src, nil)
				if out != "F\n" || st != 0 {
					t.Errorf("%s, %s: printed %q at status %d, want %q at 0",
						cond, form.name, out, st, "F\n")
				}
				// And the construct's own status, which is what `&&`,
				// `if` and `set -e` all actually read.
				bare, st := fileCondRun(t, dir, form.prelude+cond, nil)
				if st != 1 || bare != "" {
					t.Errorf("%s, %s: status %d %q, want 1 and silence",
						cond, form.name, st, bare)
				}
			}
		}
	}
}

// TestEmptyFileOperandAsksTheFilesystemNothing — an empty operand is not a
// path, so it is not a probe: the gate is never asked about it. It is also
// one fewer stat, and the syscall the fix removes was the one returning the
// wrong answer.
//
// The same script with a real operand *is* asked about, which is what makes
// the negative mean something.
func TestEmptyFileOperandAsksTheFilesystemNothing(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	if err := os.WriteFile(present, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stats := func(t *testing.T, src string) []string {
		t.Helper()
		var mu sync.Mutex
		var seen []string
		sem := CoreSemantics()
		_, _ = run(t, src, func(r *Runner) {
			r.Semantics = &sem
			r.Dir = dir
			r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
				if a.Kind == ActionStat {
					mu.Lock()
					seen = append(seen, a.Path)
					mu.Unlock()
				}
				return Allow
			})
		})
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
	for _, src := range []string{
		`[ -d "" ]`, `test -e ""`, `[[ -L "" ]]`,
		`[ "" -ef "" ]`, `[ "" -nt "" ]`,
	} {
		if got := stats(t, src); len(got) != 0 {
			t.Errorf("%s stat'd %v, want nothing asked of the gate", src, got)
		}
	}
	// The control: a named operand does reach the gate, and reaches it as
	// the runner-relative path rather than a process-relative one.
	if got := stats(t, `[ -d present ]`); len(got) != 1 || got[0] != present {
		t.Errorf("[ -d present ] stat'd %v, want exactly [%s]", got, present)
	}
}

// TestEmptyOperandIsNotTheWorkingDirectory pins the root cause rather than
// the symptom: the six operators a directory answers yes to are the ones
// that were wrong, and `.` is the spelling that still means the working
// directory and still says yes. Both halves have to hold — a fix that made
// `[ -d . ]` false too would pass a test that only checked the empty case.
func TestEmptyOperandIsNotTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, op := range []string{"-e", "-d", "-s", "-r", "-w", "-x"} {
		if st := fileCondStatus(t, dir, `[ `+op+` "" ]`); st != 1 {
			t.Errorf(`[ %s "" ] = %d, want 1`, op, st)
		}
		if st := fileCondStatus(t, dir, `[ `+op+` . ]`); st != 0 {
			t.Errorf("[ %s . ] = %d, want 0 — the working directory still answers", op, st)
		}
	}
	// And `-f`'s answer is unchanged: it was already right, and moving it
	// to make the family uniform would be a regression wearing a
	// consistency argument.
	if st := fileCondStatus(t, dir, `[ -f "" ]`); st != 1 {
		t.Errorf(`[ -f "" ] = %d, want 1`, st)
	}
	if err := os.WriteFile(filepath.Join(dir, "reg"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if st := fileCondStatus(t, dir, `[ -f reg ]`); st != 0 {
		t.Errorf("[ -f reg ] = %d, want 0", st)
	}
	if st := fileCondStatus(t, dir, `[ -f . ]`); st != 1 {
		t.Errorf("[ -f . ] = %d, want 1 — a directory is not a regular file", st)
	}
}

// TestEmptyOperandInTheFileComparisons — `-nt`, `-ot` and `-ef` took their
// two operands the same way, so `[ "" -ef "" ]` compared the working
// directory with itself and answered **true** in every dialect. Every shell
// in the panel says false.
//
// An empty side is a file that is not there, which is the answer that puts
// it on the existing MissingFileIsOlder axis rather than on a new one:
// measured, `[ f -nt "" ]` is true in bash 5.3, bash 3.2 and ksh93 and
// false in dash and zsh — the same split, to the same values, as
// `[ f -nt nosuchfile ]`.
func TestEmptyOperandInTheFileComparisons(t *testing.T) {
	dir := newerOlderDir(t)
	// Unanimous, so asked of no dialect.
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`[ "" -ef "" ]`, 1},
		{`[[ "" -ef "" ]]`, 1},
		{`[ new -ef "" ]`, 1},
		{`[ "" -ef new ]`, 1},
		{`[ "" -nt "" ]`, 1},
		{`[ "" -ot "" ]`, 1},
		{`[ "" -nt new ]`, 1},
		{`[ new -ot "" ]`, 1},
	} {
		out, st := fileCondRun(t, dir, tc.src, nil)
		if st != tc.want || out != "" {
			t.Errorf("%s = %d %q, want %d without a question", tc.src, st, out, tc.want)
		}
	}
	// The two that land on the axis, and land on it with the same values a
	// missing name does.
	for _, src := range []string{
		`[ new -nt "" ]`, `[ "" -ot new ]`,
		`[[ new -nt "" ]]`, `[[ "" -ot new ]]`,
	} {
		for answer, want := range map[Answer]int{Yes: 0, No: 1} {
			out, st := fileCondRun(t, dir, src, func(s *Semantics) {
				s.MissingFileIsOlder = answer
			})
			if st != want || out != "" {
				t.Errorf("%s with %v = %d %q, want %d and silence", src, answer, st, out, want)
			}
		}
		out, st := fileCondRun(t, dir, src, nil)
		if st != 2 || !strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s unanswered = %d %q, want the refusal at 2", src, st, out)
		}
	}
}

// TestAtDirLeavesAnEmptyPathEmpty — the resolution itself, through the
// callers that are not file tests. `.` given an empty operand must not find
// the working directory readable, and must not source it.
func TestAtDirLeavesAnEmptyPathEmpty(t *testing.T) {
	dir := t.TempDir()
	out, st := fileCondRun(t, dir, `. ""`, nil)
	if st == 0 {
		t.Errorf(`. "" = %d %q, want a failure — an empty operand names no file`, st, out)
	}
	// And a real relative operand still resolves against the runner's
	// directory, not the process's.
	if err := os.WriteFile(filepath.Join(dir, "inc.sh"), []byte("echo sourced\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st = fileCondRun(t, dir, `. ./inc.sh`, nil)
	if st != 0 || out != "sourced\n" {
		t.Errorf(". ./inc.sh = %d %q, want 0 and \"sourced\\n\"", st, out)
	}
}

// TestAnEmptyRedirectTargetIsNotTheWorkingDirectory — the same resolution,
// reached from the other side.
//
// redirect.go carried its own copy of this rule, written when `cat < $unset`
// was found reading the working directory; the copy is gone and the
// redirection resolves through atDir like everything else, so there is one
// place that decides what an empty path means.
//
// The wording is what makes the two readings distinguishable, and it is the
// panel's: measured, bash 5.3 says `: No such file or directory` for both
// directions. Joining the empty name onto the directory instead opens the
// *directory*, and the complaint becomes `Is a directory` — from the
// redirection on a write, and from the command itself on a read, which is
// how it hid.
func TestAnEmptyRedirectTargetIsNotTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in-there"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{`cat < ""`, `echo hi > ""`} {
		out, st := fileCondRun(t, dir, src+`; echo "st=$?"`, nil)
		if !strings.Contains(out, "No such file or directory") {
			t.Errorf("%s said %q, want it could not open the name", src, out)
		}
		if strings.Contains(out, "Is a directory") || strings.Contains(out, "in-there") {
			t.Errorf("%s said %q — the empty name resolved to the working directory", src, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s left status %q (run status %d), want st=1", src, out, st)
		}
	}
	// The control: a real relative target still resolves against the
	// runner's directory rather than the process's.
	if out, _ := fileCondRun(t, dir, `echo written > out.txt; cat out.txt`, nil); out != "written\n" {
		t.Errorf("a named target = %q, want it written and read back in the runner's directory", out)
	}
}
