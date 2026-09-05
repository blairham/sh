// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeRemovesWhatIsTrueOfTheMachine(t *testing.T) {
	// Diagnostics quote the shell's own path and the script's path, both of
	// which differ between a laptop and a CI runner. Leaving them in would
	// make the golden record drift for everyone but its author.
	sh := Found{Shell: Shell{Name: "bash"}, Path: "/opt/homebrew/bin/bash"}
	dir := "/var/folders/xy/oracle-123"

	got := normalize("/opt/homebrew/bin/bash: line 1: oops\n", sh, dir)
	if strings.Contains(got, "homebrew") {
		t.Errorf("shell path survived normalization: %q", got)
	}
	if !strings.Contains(got, "<shell>") {
		t.Errorf("want <shell> placeholder, got %q", got)
	}

	if got := normalize(dir+"/case.sh: bad\n", sh, dir); !strings.Contains(got, "<script>") {
		t.Errorf("script path not normalized: %q", got)
	}
	if got := normalize(dir+"/other: bad\n", sh, dir); !strings.Contains(got, "<tmp>") {
		t.Errorf("temp dir not normalized: %q", got)
	}
}

func TestNormalizeFlattensNewlinesAndTrailing(t *testing.T) {
	sh := Found{Shell: Shell{Name: "x"}, Path: "/bin/x"}
	got := normalize("a\nb\n", sh, "/tmp/d")
	if got != "a~b" {
		t.Errorf("normalize = %q, want %q", got, "a~b")
	}
}

func TestCompareOnlyComparesShellsPresentInBoth(t *testing.T) {
	// CI and a laptop have different panels. Failing on a shell that only one
	// side has would make the check useless exactly where it is most wanted.
	golden := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Output: "x"}, "ksh93": {Output: "y"}},
	}}
	now := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Output: "x"}}, // ksh93 absent here
	}}
	if d := now.Compare(golden); len(d) != 0 {
		t.Errorf("missing shell reported as drift: %v", d)
	}
}

func TestCompareDetectsChangedOutputAndStatus(t *testing.T) {
	golden := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Output: "x", Status: 0}},
		"c2": {"bash": {Output: "y", Status: 0}},
	}}
	now := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Output: "CHANGED", Status: 0}},
		// Same output, different status is still a difference.
		"c2": {"bash": {Output: "y", Status: 1}},
	}}
	d := now.Compare(golden)
	if len(d) != 2 {
		t.Fatalf("want 2 drifts, got %d: %v", len(d), d)
	}
	if d[0].CaseID != "c1" || d[1].CaseID != "c2" {
		t.Errorf("drifts not sorted by case: %v", d)
	}
}

func TestNewCasesAreNotDrift(t *testing.T) {
	golden := &Run{Results: map[string]map[string]Result{"old": {"bash": {}}}}
	now := &Run{Results: map[string]map[string]Result{
		"old": {"bash": {}},
		"new": {"bash": {Output: "z"}},
	}}
	if d := now.Compare(golden); len(d) != 0 {
		t.Errorf("a new case has nothing to drift from, got %v", d)
	}
	got := now.NewCases(golden)
	if len(got) != 1 || got[0] != "new" {
		t.Errorf("NewCases = %v, want [new]", got)
	}
}

func TestCorpusIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Corpus {
		switch {
		case c.ID == "":
			t.Error("a case has no ID; the ID is the golden-file key")
		case seen[c.ID]:
			t.Errorf("duplicate case ID %q — one would silently overwrite the other", c.ID)
		case c.Snippet == "":
			t.Errorf("%s: no snippet", c.ID)
		case c.Category == "":
			t.Errorf("%s: no category", c.ID)
		case c.Why == "":
			// A case nobody can evaluate when it changes is worse than no
			// case, because it will be "fixed" by updating the golden file.
			t.Errorf("%s: no Why; it could not be judged when it drifts", c.ID)
		}
		seen[c.ID] = true
		if err := c.validate(); err != nil {
			// A case whose invocation cannot be built records a harness error
			// in place of a measurement, which looks like a shell that
			// disagreed with everyone.
			t.Errorf("%s: %v", c.ID, err)
		}
	}
}

func TestArgsPlaceTheSnippetWhereTheCaseSaysAndKeepTheShellsFlagsFirst(t *testing.T) {
	// The binary under test is told which dialect to be by flags of its own
	// — `-dialect bash` — and those have to come before anything the case
	// spells out, because cmd/sh stops reading its own flags at the first
	// word that is not one.
	sh := Found{Shell: Shell{Name: "ours", Args: []string{"-dialect", "bash"}}, Path: "/bin/sh"}
	c := Case{ID: "t", Snippet: `echo hi`, Args: []string{"-o", "errexit", "-c", ArgSnippet, "name"}}

	cmd := command(t.Context(), sh, c, t.TempDir())
	want := []string{"/bin/sh", "-dialect", "bash", "-o", "errexit", "-c", "echo hi", "name"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("argv = %q, want %q", cmd.Args, want)
	}
}

func TestArgScriptWritesTheSnippetToTheFileTheNormalizerKnows(t *testing.T) {
	// The path has to be the one normalize() rewrites to <script>, or a case
	// whose diagnostic names the script would record the temp directory it
	// happened to run in and drift for everyone else.
	dir := t.TempDir()
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	c := Case{ID: "t", Snippet: `echo hi`, Args: []string{"-e", ArgScript, "a"}}

	cmd := command(t.Context(), sh, c, dir)
	want := []string{"/bin/sh", "-e", filepath.Join(dir, "case.sh"), "a"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("argv = %q, want %q", cmd.Args, want)
	}
	b, err := os.ReadFile(filepath.Join(dir, "case.sh"))
	if err != nil || string(b) != "echo hi\n" {
		t.Errorf("script file = %q, %v; want the snippet", b, err)
	}
	if got := normalize(filepath.Join(dir, "case.sh")+": bad\n", sh, dir); !strings.Contains(got, "<script>") {
		t.Errorf("the file ArgScript wrote does not normalize: %q", got)
	}
}

func TestArgsWithNoPlaceholderDoNotHandOverTheSnippet(t *testing.T) {
	// The shape an invocation that fails before it reads anything needs — a
	// script path that does not exist — and the shape Case.Stdin will need.
	// The snippet is still written down, as the thing that would have run.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	c := Case{ID: "t", Snippet: `echo the-snippet`, Args: []string{"-c", "echo the-argv"}}

	got := Exec(context.Background(), found[0], c)
	if got.Output != "the-argv" {
		t.Errorf("Output = %q, want %q: Args are the whole invocation", got.Output, "the-argv")
	}
}

func TestArgsAndScriptAreRefusedTogether(t *testing.T) {
	// Not a precedence rule: a case that quietly ran something other than
	// what it says would still be measured, and the measurement is the
	// product.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	for _, c := range []Case{
		{ID: "both", Snippet: "echo hi", Script: true, Args: []string{ArgScript}},
		{ID: "twice", Snippet: "echo hi", Args: []string{"-c", ArgSnippet, ArgScript}},
	} {
		got := Exec(context.Background(), found[0], c)
		if !strings.HasPrefix(got.Output, "harness error:") || got.Status != -1 {
			t.Errorf("%s: Exec = %q (status %d), want a harness error", c.ID, got.Output, got.Status)
		}
	}
}

func TestArgsRunTheSameInvocationOnBothSides(t *testing.T) {
	// The point of the field. A reference shell and the binary under test
	// have to be handed the same words, or a conformance run grades two
	// different invocations and reports the difference as a bug.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	ref := found[0]
	ours := Found{Shell: Shell{Name: "ours"}, Path: ref.Path}
	c := Case{ID: "t", Snippet: `echo "$0|$#"`, Args: []string{"-c", ArgSnippet, "name", "a"}}

	if a, b := Exec(context.Background(), ref, c), Exec(context.Background(), ours, c); a != b {
		t.Errorf("same case, different invocations: %q vs %q", a.Output, b.Output)
	}
}

func TestExecRunsASnippetAndCapturesStatus(t *testing.T) {
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	sh := found[0]

	got := Exec(context.Background(), sh, Case{ID: "t", Snippet: `printf hi; exit 3`})
	if got.Output != "hi" {
		t.Errorf("Output = %q, want %q", got.Output, "hi")
	}
	if got.Status != 3 {
		t.Errorf("Status = %d, want 3", got.Status)
	}
}

func TestExecDoesNotInheritTheDevelopersEnvironment(t *testing.T) {
	// IFS or HOME leaking in from whoever ran the harness would make the
	// record depend on their shell rather than on the shell under test.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	t.Setenv("IFS", ":")
	t.Setenv("ORACLE_LEAK_CANARY", "leaked")

	got := Exec(context.Background(), found[0], Case{ID: "t", Snippet: `echo "[${ORACLE_LEAK_CANARY-clean}]"`})
	if got.Output != "[clean]" {
		t.Errorf("environment leaked into the run: %q", got.Output)
	}
}

func TestMarkdownEndsWithExactlyOneNewline(t *testing.T) {
	// The generated file is committed, so end-of-file-fixer rewrites it if it
	// ends in a blank line — which fails the commit on every regeneration.
	r := &Run{
		Shells:  []ShellRecord{{Name: "bash", Version: "5"}},
		Results: map[string]map[string]Result{"c1": {"bash": {Output: "x"}}},
	}
	got := r.Markdown([]Case{{ID: "c1", Category: "cat", Snippet: "echo x", Why: "because"}})
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("want exactly one trailing newline, got %q", got[max(0, len(got)-12):])
	}
}

func TestNormalizeDoesNotEatShellNameInsideWords(t *testing.T) {
	// dash's basename is "sh", and replacing it anywhere turned
	// "can't shift that many" into "can't <shell>ift that many".
	sh := Found{Shell: Shell{Name: "bash-as-sh"}, Path: "/bin/sh"}
	got := normalize("sh: 1: shift: can't shift that many\n", sh, "/tmp/d")
	want := "<shell>: 1: shift: can't shift that many"
	if got != want {
		t.Errorf("normalize = %q, want %q", got, want)
	}
}

func TestResolveRejectsAPathThatIsNotTheShellItNames(t *testing.T) {
	// /bin/sh is bash on macOS and dash on Debian. A column labeled
	// bash-as-sh that actually ran dash is worse than a missing column,
	// because nothing about it looks wrong.
	var entry Shell
	for _, s := range Panel {
		if s.Name == "bash-as-sh" {
			entry = s
		}
	}
	if entry.MustReport == "" {
		t.Fatal("the bash-as-sh panel entry must assert what it expects to find")
	}

	found, missing := Resolve(context.Background())
	for _, f := range found {
		if f.MustReport != "" && !strings.Contains(strings.ToLower(f.Version), f.MustReport) {
			t.Errorf("%s resolved to %s reporting %q, which does not contain %q",
				f.Name, f.Path, f.Version, f.MustReport)
		}
	}
	_ = missing
}

func TestNormalizeUsesTheNameTheShellWasInvokedUnder(t *testing.T) {
	// bash invoked as sh reports "sh:" in diagnostics, not "bash:", so
	// normalizing only the binary's basename leaves the machine-specific
	// name in the record.
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	got := normalize("sh: -c: line 1: syntax error\n", sh, "/tmp/d")
	if !strings.HasPrefix(got, "<shell>:") {
		t.Errorf("argv0 name not normalized: %q", got)
	}
}

// TestSyntaxErrorCasesAreGraded is the point of grading them at all.
//
// They were skipped alongside the cases whose reference races, and the two
// exclusions are not alike: a racing reference cannot grade anything, but a
// rejection is as deterministic as an acceptance and its wording is exactly
// what the Diagnostics vector exists for. Skipping them left a whole vector
// ungraded and a report of 100% silent about it — nine real gaps, at the time
// this was written.
func TestSyntaxErrorCasesAreGraded(t *testing.T) {
	var syntaxErrors int
	for _, c := range Corpus {
		if c.SyntaxError {
			syntaxErrors++
			if !graded(c) {
				t.Errorf("%s is not graded, but a rejection is as deterministic as an acceptance", c.ID)
			}
		}
	}
	if syntaxErrors == 0 {
		t.Fatal("no SyntaxError cases in the corpus, so this proves nothing")
	}
	// The one thing that does disqualify a case still does.
	if graded(Case{ReferenceRaces: true}) {
		t.Error("a racing reference cannot grade anything and must stay excluded")
	}
}
