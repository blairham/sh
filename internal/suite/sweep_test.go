// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

func gradeFile(t *testing.T, tests, name, ours, reference string) Result {
	t.Helper()
	s := Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests"}
	return grade(context.Background(), s, tests, name, ours, reference,
		bash.Dialect(), true, 2*time.Second)
}

// TestAKilledOracleAndAKilledDialectAreDifferentFindings is the distinction
// the issue asks for by name, and it is not presentation.
//
// A dialect binary killed by the timeout is a hang we published — the thing a
// user meets. The reference killed by the same timeout is a harness fault:
// the shell that wrote the file does not hang on it, so the kill says the
// machine was loaded or the bound was too tight. Rolling the two into one
// "timed out" number turns a busy laptop into a bug report and hides a real
// hang inside it.
func TestAKilledOracleAndAKilledDialectAreDifferentFindings(t *testing.T) {
	tests := testDir(t, map[string]string{"f.tests": "echo a\n"})
	bin := t.TempDir()
	hangs := fakeShell(t, bin, "hangs", "sleep 60")

	res := gradeFile(t, tests, "f.tests", hangs, "/bin/sh")
	if !res.DialectHung || res.OracleHung {
		t.Errorf("our binary hung and it was recorded as %+v", res)
	}
	if res.Scored {
		t.Error("a file nobody finished was scored; a truncated file is reported, never scored")
	}

	res = gradeFile(t, tests, "f.tests", "/bin/sh", hangs)
	if !res.OracleHung || res.DialectHung {
		t.Errorf("the oracle hung and it was recorded as %+v", res)
	}
	if res.Scored {
		t.Error("a file the oracle never finished was scored")
	}
}

func TestTwoRunsOfTheSameShellAreStrictlyIdentical(t *testing.T) {
	tests := testDir(t, map[string]string{"f.tests": "echo a\necho b >&2\nexit 2\n"})
	res := gradeFile(t, tests, "f.tests", "/bin/sh", "/bin/sh")
	if !res.Scored || !res.Strict {
		t.Fatalf("a shell graded against itself is not strict: %+v", res)
	}
	if res.Common != res.Longest || res.Longest == 0 {
		t.Errorf("line agreement over an identical run is %d/%d", res.Common, res.Longest)
	}
}

// TestAFileTheOracleDoesNotRepeatIsNotEvidence guards a third of what a sweep
// like this otherwise reports.
//
// A process id, a clock reading, a scheduling order: the reference disagrees
// with *itself* on these, so counting them against us would put a number on
// the machine rather than on the shell. Asked only where the two shells
// differed, which is the only place the answer changes anything.
func TestAFileTheOracleDoesNotRepeatIsNotEvidence(t *testing.T) {
	tests := testDir(t, map[string]string{"pid.tests": "echo $$\n"})
	res := gradeFile(t, tests, "pid.tests", "/bin/sh", "/bin/sh")
	if !res.Unstable {
		t.Fatalf("a file printing its own pid was treated as evidence: %+v", res)
	}
	if res.Scored {
		t.Error("an unstable file was scored")
	}
}

func TestARefusedFileCarriesOurOwnDiagnosticAndNothingOfTheFile(t *testing.T) {
	tests := testDir(t, map[string]string{
		"bad.tests": "echo 'unterminated\n",
		"ok.tests":  "echo fine\n",
	})
	bad := gradeFile(t, tests, "bad.tests", "/bin/sh", "/bin/sh")
	if bad.Parsed {
		t.Fatal("a file with an unterminated quote parsed")
	}
	if bad.Cause == "" {
		t.Fatal("a refused file has no cause")
	}
	// The cause is our lexer's words about an input, which is why it may be
	// printed. The file's own text is what may not, and there is nowhere in
	// a Result to put it.
	if strings.Contains(bad.Cause, "unterminated quote text") {
		t.Errorf("the cause carries the file: %q", bad.Cause)
	}
	if ok := gradeFile(t, tests, "ok.tests", "/bin/sh", "/bin/sh"); !ok.Parsed || ok.Cause != "" {
		t.Errorf("a readable file was recorded as refused: %+v", ok)
	}
}

// TestAResultCannotHoldAPathOrTheFileText is the CLEANROOM rule made
// structural.
//
// `make wild` keeps paths out of its report by remembering to; this keeps
// them out by having nowhere to put one. Every field here is a number, a
// boolean, or our own diagnostic with the per-file words removed. A field
// added later that could carry a line of somebody else's suite fails this.
func TestAResultCannotHoldAPathOrTheFileText(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(Result{}), reflect.TypeOf(Cause{})} {
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Type.Kind() != reflect.String {
				continue
			}
			if f.Name != "Cause" && f.Name != "Reason" {
				t.Errorf("%s.%s is a string; the only text this report may carry is "+
					"our own parser's diagnostic", typ.Name(), f.Name)
			}
		}
	}
}

func TestFilesAreTheSuitesOwnAndNotItsFixtures(t *testing.T) {
	tests := testDir(t, map[string]string{
		"b.tests": "", "a.tests": "", "a.sub": "", "a.right": "", "README": "",
	})
	names, err := Files(tests, ".tests")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.tests", "b.tests"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v — the rest of the directory is sourced, not run", names, want)
	}
}

// TestEachRunGetsTheSuiteFresh: a suite file writes, moves and deletes, and
// several only clean up if they got that far. Two runs sharing a directory
// would have the second reading the first one's leftovers, and the fetched
// tree would stop being what was unpacked.
func TestEachRunGetsTheSuiteFresh(t *testing.T) {
	tests := testDir(t, map[string]string{
		"w.tests": "test -f marker && echo LEFTOVERS\n: > marker\necho done\n",
	})
	s := Suite{ShellVar: "THIS_SH"}
	for range 2 {
		got := runIn(context.Background(), s, tests, "w.tests", "/bin/sh", 5*time.Second)
		if strings.Contains(got.Output, "LEFTOVERS") {
			t.Fatal("a run read what the run before it left behind")
		}
	}
	if _, err := os.Stat(filepath.Join(tests, "marker")); err == nil {
		t.Error("a run wrote into the fetched tree; it must write into a copy")
	}
}

func TestCausesAreRankedByFilesForfeited(t *testing.T) {
	got := rank(map[string]int{"rare": 1, "common": 9, "middling": 4, "also rare": 1})
	want := []Cause{
		{"common", 9}, {"middling", 4}, {"also rare", 1}, {"rare", 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v — ties break on the wording so two runs print the same report", got, want)
	}
}

func TestStatusPairsAreRankedAndStable(t *testing.T) {
	got := rankStatuses(map[[2]int]int{{2, 0}: 3, {0, 0}: 7, {1, 0}: 3})
	want := []StatusPair{{0, 0, 7}, {1, 0, 3}, {2, 0, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestThePanelIsAMultiDialectTable is the shape the issue asks for from day
// one: an instrument that could only ever grade bash would bend the substrate
// toward bash, which is what this project's premise forbids. A column that is
// not built is a row with a reason, never a silence.
func TestThePanelIsAMultiDialectTable(t *testing.T) {
	seen := map[string]bool{}
	ready := 0
	for _, s := range Panel {
		if s.Name == "" || s.Dialect == "" {
			t.Errorf("a panel row with no name: %+v", s)
			continue
		}
		if seen[s.Dialect] {
			t.Errorf("two rows claim dialect %q", s.Dialect)
		}
		seen[s.Dialect] = true
		if _, err := os.Stat(filepath.Join("..", "..", "cmd", s.Dialect)); err != nil {
			t.Errorf("%s names dialect %q and there is no cmd/%s to grade", s.Name, s.Dialect, s.Dialect)
		}
		if s.NotYet != "" {
			if len(strings.Fields(s.NotYet)) < 5 {
				t.Errorf("%s is not built and the reason is too short to act on: %q", s.Name, s.NotYet)
			}
			continue
		}
		ready++
		if !hex64.MatchString(s.SHA256) {
			t.Errorf("%s is a built column with no pinned digest", s.Name)
		}
		for name, value := range map[string]string{
			"URL": s.URL, "Root": s.Root, "TestDir": s.TestDir,
			"Ext": s.Ext, "ShellVar": s.ShellVar,
		} {
			if value == "" {
				t.Errorf("%s is a built column with no %s", s.Name, name)
			}
		}
		if len(s.Lookup) == 0 {
			t.Errorf("%s has no oracle to look for; the reference on the machine is the "+
				"oracle, never the suite's own expected output", s.Name)
		}
		if _, ok := s.Syntax(); !ok {
			t.Errorf("%s has no parser configuration", s.Name)
		}
	}
	if ready == 0 {
		t.Error("no column runs, so this instrument measures nothing")
	}
	if len(Built()) != ready {
		t.Errorf("Built() returned %d columns, want %d", len(Built()), ready)
	}
}

// TestTheExpectedOutputFilesAreNeverUnpacked. The oracle is the binary on the
// machine — partly because a shipped .right file was generated by a different
// build of a different version and disagrees with it, and mostly because the
// expected output is the most expression-like part of a suite. Excluding it
// from the extraction makes that a property of the tree rather than a promise
// about the code.
func TestTheExpectedOutputFilesAreNeverUnpacked(t *testing.T) {
	s, ok := Find("bash")
	if !ok {
		t.Fatal("no bash column")
	}
	if _, want := s.Wanted(s.Root + "/tests/case.right"); want {
		t.Error("an expected-output file would be unpacked")
	}
	if _, want := s.Wanted(s.Root + "/tests/case.tests"); !want {
		t.Error("the suite itself would not be unpacked")
	}
	if _, want := s.Wanted(s.Root + "/shell.c"); want {
		t.Error("the shell's own source would be unpacked")
	}
}

func TestSyntaxIsTheDialectTheBinaryIsBuiltFrom(t *testing.T) {
	s, _ := Find("bash")
	got, ok := s.Syntax()
	if !ok {
		t.Fatal("no dialect")
	}
	if !reflect.DeepEqual(got, bash.Dialect()) {
		t.Error("the parsed figure would be measured with a parser the binary does not have")
	}
	var zero syntax.Dialect
	if reflect.DeepEqual(got, zero) {
		t.Error("the dialect is the zero value")
	}
}
