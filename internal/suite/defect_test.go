// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
)

// gradeOurs is gradeFile for a native column: the same grader, the same
// files, and Ours set. The only difference the flag makes is the two rules
// #2297 is about, which is what these tests are here to hold still.
func gradeOurs(t *testing.T, tests, name, ours, reference string) Result {
	t.Helper()
	s := Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests", Ours: true}
	res, _ := grade(context.Background(), s, tests, name, ours, reference,
		StaticRead{Dial: bash.Dialect()}, true, Doc{}, Options{Timeout: 2 * time.Second})
	return res
}

// TestOurOwnSuiteIsAskedToRepeatEvenWhereTheTwoShellsAgreed is the half of
// #2297 that nothing else in this report can see.
//
// A fetched column asks the reference for a second run only where the two
// shells differed, because that is the only place the answer changes
// anything. Under that rule a case whose answer moves between runs passes
// whenever our binary happens to land on the answer the reference gave this
// time — and no other number says so, because every figure the column prints
// is about agreement and the two runs agreed.
//
// A pid will not stage that, which is worth saying because it is the obvious
// thing to reach for and it tests the opposite: two runs of the same binary
// are two processes, so their pids differ, the column sees a disagreement and
// the old rule already asks. The shape that hides is the one where the two
// runs *match* and the reference would still have said something else. So the
// reference here counts its own runs and prints the count, and our binary
// prints the first count. They agree; the reference does not agree with
// itself.
func TestOurOwnSuiteIsAskedToRepeatEvenWhereTheTwoShellsAgreed(t *testing.T) {
	tests := testDir(t, map[string]string{"drift.tests": "exec_the_shell\n"})
	bin := t.TempDir()
	counter := filepath.Join(t.TempDir(), "n")
	// Every run of this one answers one higher than the last, and nothing it
	// writes leaves a trace in the run directory the harness lays out — which
	// is exactly the shape a case with a clock or an external file has.
	reference := fakeShell(t, bin, "counts", `n=$(cat "$SUITE_COUNTER" 2>/dev/null || echo 0)
n=$((n + 1))
printf '%s\n' "$n" > "$SUITE_COUNTER"
printf '%s\n' "$n"`)
	ours := fakeShell(t, bin, "says-one", "printf '%s\\n' 1")

	run := func(s Suite) Result {
		t.Helper()
		zero(t, counter)
		res, _ := grade(context.Background(), s, tests, "drift.tests", ours, reference,
			StaticRead{Dial: bash.Dialect()}, true, Doc{},
			Options{Timeout: 2 * time.Second, Extra: []string{"SUITE_COUNTER=" + counter}})
		return res
	}

	// The old rule, unchanged: the two shells agreed, so nothing asked, and
	// the case passes on a coincidence. This is not a bug in the fetched
	// column — it is the right answer for a suite nobody here may edit.
	fetched := run(Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests"})
	if !fetched.Strict {
		t.Fatalf("the two shells were supposed to agree on the first run: %+v", fetched)
	}

	// The same file, the same two binaries, in a suite of ours.
	mine := run(Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests", Ours: true})
	if !mine.Unstable {
		t.Fatalf("a native case the reference answers two ways passed on a coincidence: %+v", mine)
	}
	if mine.Scored || mine.Strict {
		t.Errorf("a native case that does not repeat was scored: %+v", mine)
	}
}

// zero resets the counter the reference above keeps, so each grading starts
// from the same place and the two halves of the test are comparable.
func zero(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAStableNativeCaseIsStillStrict is the other side of the rule: the extra
// run may not cost a passing case its pass.
func TestAStableNativeCaseIsStillStrict(t *testing.T) {
	tests := testDir(t, map[string]string{"f.tests": "echo a\necho b >&2\nexit 2\n"})
	res := gradeOurs(t, tests, "f.tests", "/bin/sh", "/bin/sh")
	if !res.Scored || !res.Strict {
		t.Fatalf("a deterministic native case is not strict: %+v", res)
	}
	if res.Unstable {
		t.Error("a deterministic native case was called a defect")
	}
}

// TestAnUnstableCaseIsADefectOnlyInOurOwnSuite is the rule read off the
// report rather than off a field.
//
// The same measurement means opposite things in the two kinds of column.
// Nobody here may edit a fetched suite, so a file its own shell will not
// repeat is evidence about neither shell and dropping it from the scored set
// is the only honest move available. Ours ships no expected output, so a file
// the reference answers two ways has no expectation for anything to be graded
// against — and it is ours, so it is a bug in the case.
func TestAnUnstableCaseIsADefectOnlyInOurOwnSuite(t *testing.T) {
	unstable := Result{Unstable: true}
	cases := []NamedResult{{Name: "pid.tests", Result: unstable}}

	ours := Report{Suite: Suite{Ours: true}, Cases: cases, Unstable: 1}
	if got := ours.CaseDefects(); len(got) != 1 || got[0] != "pid.tests" {
		t.Errorf("a native column did not name its bad case: %v", got)
	}

	// A fetched column cannot name one, and it is Suite.attribute that makes
	// it so: a fetched case reaches Cases with an empty name, so there is
	// nothing here to report even if the rule were asked the wrong way.
	fetched := Report{Suite: Suite{Ours: false}, Cases: []NamedResult{{Result: unstable}}, Unstable: 1}
	if got := fetched.CaseDefects(); len(got) != 0 {
		t.Errorf("a fetched column reported a case defect: %v", got)
	}
}

// TestMustRepeatIsOursAndOnlyOurs keeps the two columns' rules from drifting
// into one. An extra run per file of bash's 83 buys nothing and costs a
// minute; an extra run per file of ours is the whole guarantee.
func TestMustRepeatIsOursAndOnlyOurs(t *testing.T) {
	if !(Suite{Ours: true}).mustRepeat() {
		t.Error("a native column does not re-run its own cases")
	}
	if (Suite{}).mustRepeat() {
		t.Error("a fetched column re-runs every file")
	}
	for _, s := range Ours {
		if !s.mustRepeat() {
			t.Errorf("the %s column does not re-run its own cases", s.Name)
		}
	}
}

// TestAnUnstableCaseSaysWhatMoved is the half of the report that turns a
// defect into a work list.
//
// #2291's assessment named `jobs.tests` as non-deterministic off a CI log,
// and nothing in the tree could say which of that file's two hundred lines
// had moved — the flake shows on about two runs in fourteen, on a runner
// nobody can log into, and 64 runs of the same file under contention on the
// machine this was written on would not reproduce it. A name alone sends a
// reader to read the whole file; the run that catches it is the only thing
// that can say where.
//
// The carve-out is [Suite.attribute]'s: our own files are committed and meant
// to be opened, and a fetched suite is another project's expression, so this
// says nothing at all for a fetched column even though the same measurement
// was made.
func TestAnUnstableCaseSaysWhatMoved(t *testing.T) {
	tests := testDir(t, map[string]string{"drift.tests": "exec_the_shell\n"})
	bin := t.TempDir()
	counter := filepath.Join(t.TempDir(), "n")
	// Two lines that stand still and one that does not, so the report has
	// something to leave out as well as something to name.
	reference := fakeShell(t, bin, "counts", `n=$(cat "$SUITE_COUNTER" 2>/dev/null || echo 0)
n=$((n + 1))
printf '%s\n' "$n" > "$SUITE_COUNTER"
printf 'steady one\n'
printf 'run %s\n' "$n"
printf 'steady two\n'`)
	ours := fakeShell(t, bin, "says-one", `printf 'steady one\nrun 1\nsteady two\n'`)

	run := func(s Suite) (Result, string) {
		t.Helper()
		zero(t, counter)
		return grade(context.Background(), s, tests, "drift.tests", ours, reference,
			StaticRead{Dial: bash.Dialect()}, true, Doc{},
			Options{Timeout: 2 * time.Second, Extra: []string{"SUITE_COUNTER=" + counter}})
	}

	mine, moved := run(Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests", Ours: true})
	if !mine.Unstable {
		t.Fatalf("the case was supposed to be unstable: %+v", mine)
	}
	// The line that moved, both ways round, and neither of the two that did
	// not — a report that named every line would be the file again.
	for _, want := range []string{"first run only: run 1", "second run only: run 2"} {
		if !strings.Contains(moved, want) {
			t.Errorf("what moved does not carry %q: %q", want, moved)
		}
	}
	if strings.Contains(moved, "steady") {
		t.Errorf("a line that did not move was reported: %q", moved)
	}

	// And the same measurement says nothing for a fetched column, where
	// quoting the file is what the whole no-path rule forbids.
	_, fetched := run(Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests"})
	if fetched != "" {
		t.Errorf("a fetched column quoted its suite: %q", fetched)
	}
}

// TestCaseDefectReportsIsOursAlone reads the same rule off the report, which
// is where a person meets it.
func TestCaseDefectReportsIsOursAlone(t *testing.T) {
	cases := []NamedResult{{
		Name:   "jobs.tests",
		Result: Result{Unstable: true},
		Moved:  "first run only: a running child answers -> 1",
	}}
	ours := Report{Suite: Suite{Ours: true}, Cases: cases, Unstable: 1}
	got := ours.CaseDefectReports()
	if len(got) != 1 || !strings.HasPrefix(got[0], "jobs.tests — first run only:") {
		t.Errorf("a native column did not say what moved: %v", got)
	}

	fetched := Report{
		Suite:    Suite{Ours: false},
		Cases:    []NamedResult{{Result: Result{Unstable: true}}},
		Unstable: 1,
	}
	if got := fetched.CaseDefectReports(); len(got) != 0 {
		t.Errorf("a fetched column reported a case defect: %v", got)
	}
}

// TestATierCheckNamesTheReferenceThatMovedAndTheLine is #2291's reading turned
// into a guard: CI said four files were "not deterministic" in the tier
// checks and could not say which reference had moved or on what line, so the
// only route to the row was to reproduce a flake that shows under load on a
// machine nobody can log into.
//
// The reference that counts its own runs is the drifting one; the two others
// repeat themselves. The report must name the counter, and carry both of its
// answers.
func TestATierCheckNamesTheReferenceThatMovedAndTheLine(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "f.tests"), []byte("exec_the_shell\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	counter := filepath.Join(t.TempDir(), "n")
	zero(t, counter)
	refs := []Reference{
		{Name: "mine", Path: fakeShell(t, bin, "mine", "printf 'mine\\n'")},
		{Name: "steady", Path: fakeShell(t, bin, "steady", "printf 'steady\\n'")},
		{Name: "counts", Path: fakeShell(t, bin, "counts", `n=$(cat '`+counter+`')
n=$((n + 1))
printf '%s\n' "$n" > '`+counter+`'
printf 'run %s\n' "$n"`)},
	}
	col := Suite{Name: "mine", Dialect: "d", Dirs: []string{"d"}, Ours: true}
	own, err := OnlyHere(context.Background(), root, col, refs, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(own.Unstable) != 1 || own.Unstable[0] != "f.tests" {
		t.Fatalf("the drifting file was not reported unstable: %+v", own)
	}
	why := own.Moved["f.tests"]
	for _, want := range []string{"counts:", "run 1", "run 2"} {
		if !strings.Contains(why, want) {
			t.Errorf("the report %q does not say %q", why, want)
		}
	}
	if strings.Contains(why, "steady") || strings.Contains(why, "mine:") {
		t.Errorf("the report %q names a reference that repeated itself", why)
	}
}
