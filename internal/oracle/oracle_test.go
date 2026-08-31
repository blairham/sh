// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
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
