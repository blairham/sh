// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The invariant the whole suite rests on: a mark is never text that is typed.
//
// A terminal echoes keystrokes, so a wait on a mark the line contains is
// answered by the echo — and the check then passes for a shell that drew the
// line back and ran nothing at all, which is exactly what a broken run loop
// looks like. Every line is written as an expression so the two differ, and
// this is what keeps the next one from being written the easy way.
func TestNoMarkIsInTheLineThatProducesIt(t *testing.T) {
	for _, p := range probes() {
		if strings.Contains(p.line, p.mark) {
			t.Errorf("the line %q contains its own mark %q, so the terminal's echo would answer the wait",
				p.line, p.mark)
		}
	}
}

// And no mark is in a *prompt* either, for the same reason one step removed: a
// prompt is drawn before every line, so a mark inside one would be answered by
// the shell prompting rather than by anything running.
func TestNoMarkIsInAPrompt(t *testing.T) {
	prompts := []string{
		promptAnchor, envPromptPrefix, rcPromptPrefix, continuationPrompt,
		Bash().DefaultPrompt, Zsh().DefaultPrompt,
	}
	for _, p := range probes() {
		for _, prompt := range prompts {
			if strings.Contains(prompt, p.mark) {
				t.Errorf("the prompt %q contains the mark %q", prompt, p.mark)
			}
		}
	}
}

// Both prompts end in the anchor, which is what lets a session synchronize
// whether or not the rc file was read.
//
// Without it the rc file failing would take every other row with it: there
// would be no recognizable prompt, every wait would time out, and one broken
// thing would be reported as ten.
func TestBothPromptsEndInTheAnchor(t *testing.T) {
	for _, d := range []Dialect{Bash(), Zsh()} {
		env := environmentValue(t, environment("/scratch", "/usr/bin", d), "PS1")
		if !strings.HasSuffix(env, promptAnchor) {
			t.Errorf("%s: the environment's PS1 %q does not end in the anchor %q", d.Name, env, promptAnchor)
		}
		if !strings.Contains(rcText(d), "PS1='"+rcPromptPrefix) {
			t.Errorf("%s: the rc file does not set a PS1 the suite can recognize:\n%s", d.Name, rcText(d))
		}
		if !strings.Contains(rcText(d), promptAnchor) {
			t.Errorf("%s: the rc file's PS1 does not end in the anchor %q", d.Name, promptAnchor)
		}
		// The escape has to be *in* the prompt, or the row that grades it
		// grades nothing.
		if !strings.Contains(env, d.CwdEscape) || !strings.Contains(rcText(d), d.CwdEscape) {
			t.Errorf("%s: %q is not in both prompts, so the escape row would have nothing to read",
				d.Name, d.CwdEscape)
		}
	}
}

// The rc file is written under the name the dialect should read, and under the
// scratch home rather than anywhere near a person's.
//
// The file name is the assertion and not a convenience. A suite that wrote to
// whichever file the shell happens to read today would pass forever and say
// nothing.
func TestTheScratchHomeIsWrittenWhereTheDialectWouldLook(t *testing.T) {
	root := t.TempDir()
	for _, d := range []Dialect{Bash(), Zsh()} {
		dir, err := home(root, d)
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		if !strings.HasPrefix(dir, root) {
			t.Fatalf("%s: the scratch home %q is not under %q", d.Name, dir, root)
		}
		rc := filepath.Join(dir, d.RCFile)
		b, err := os.ReadFile(rc)
		if err != nil {
			t.Fatalf("%s: no rc file at %s: %v", d.Name, rc, err)
		}
		// The four things a person's rc file has, which are the four rows
		// that depend on it being read.
		for _, want := range []string{"SMOKE_RC", "alias smokealias", "smokefunc()", "PS1="} {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s: %s has no %q in it", d.Name, d.RCFile, want)
			}
		}
		// And the things the session needs on disk.
		for _, name := range []string{completionTarget, tickerName} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("%s: %v", d.Name, err)
			}
		}
		// The prefix Tab is given must name exactly one file, or the row is
		// grading an ambiguous completion, which is a different feature.
		matches, err := filepath.Glob(filepath.Join(dir, completionPrefix+"*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Errorf("%s: %q matches %d files, want exactly one", d.Name, completionPrefix, len(matches))
		}
	}
}

// The session is handed a home of its own and never the real one.
//
// Stated as a test because it is the rule that would be broken silently: an
// environment assembled by listing what to include cannot leak, and one
// assembled by copying the caller's and removing things can.
func TestTheSessionsEnvironmentCarriesNothingOfTheCallers(t *testing.T) {
	t.Setenv("SOMETHING_PERSONAL", "should not travel")
	env := environment("/scratch/home", "/usr/bin:/bin", Bash())
	for _, kv := range env {
		if strings.HasPrefix(kv, "SOMETHING_PERSONAL=") {
			t.Errorf("the caller's environment reached the session: %q", kv)
		}
	}
	if got := environmentValue(t, env, "HOME"); got != "/scratch/home" {
		t.Errorf("HOME = %q, want the scratch home", got)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, kv := range env {
			if strings.Contains(kv, home) {
				t.Errorf("the real home is named in the session's environment: %q", kv)
			}
		}
	}
}

// Every row known to be missing names a row that exists.
//
// A typo in that map is invisible otherwise: the row it meant to excuse
// reports as a regression, and the row it names never appears — so the report
// gets noisier in one place and quieter in another and neither is obviously
// wrong.
func TestEveryKnownRowIsARealRow(t *testing.T) {
	real := map[string]bool{}
	for _, f := range Features() {
		if real[f] {
			t.Errorf("two rows are called %q, so the table cannot be read by name", f)
		}
		real[f] = true
	}
	for feature, issue := range known {
		if !real[feature] {
			t.Errorf("%s is recorded against %q, which is not a row this suite has", issue, feature)
		}
		if !strings.HasPrefix(issue, "#") {
			t.Errorf("%q is recorded against %q, which does not name an issue", feature, issue)
		}
	}
}

// A row says what it proves, because a table of thirteen names is a table
// nobody can act on.
func TestEveryRowSaysWhatItProves(t *testing.T) {
	for _, c := range checks() {
		if strings.TrimSpace(c.name) == "" || strings.TrimSpace(c.proves) == "" {
			t.Errorf("the row %q does not say what it proves", c.name)
		}
	}
}

// How a row is graded against what is known.
//
// The three cases are the whole point of the known list: a gap that is being
// worked on is not news, a gap that nobody knows about is, and a gap that has
// closed is the news this suite exists to deliver.
func TestHowARowIsGradedAgainstWhatIsKnown(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		r                          Result
		expected, fixed, regressed bool
	}{
		{"a working feature", Result{Outcome: Pass}, true, false, false},
		{"a gap nobody knows about", Result{Outcome: Fail}, false, false, true},
		{"a gap that could not even be reached", Result{Outcome: Blocked}, false, false, true},
		{"a known gap", Result{Outcome: Fail, Known: "#807"}, true, false, false},
		{"a known gap, closed", Result{Outcome: Pass, Known: "#807"}, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.Expected(); got != tc.expected {
				t.Errorf("Expected() = %v, want %v", got, tc.expected)
			}
			if got := tc.r.Fixed(); got != tc.fixed {
				t.Errorf("Fixed() = %v, want %v", got, tc.fixed)
			}
			if got := tc.r.Regressed(); got != tc.regressed {
				t.Errorf("Regressed() = %v, want %v", got, tc.regressed)
			}
		})
	}
}

// A blocked row is counted with the failures and never with the passes.
//
// It is the tempting mistake: "could not be reached" reads like "not this
// suite's problem", and counting it as a pass would report a shell whose rc
// file is never read as one where the prompt escapes are fine.
func TestBlockedIsNotAPass(t *testing.T) {
	rep := Report{Results: []Result{
		{Feature: "one", Outcome: Pass},
		{Feature: "two", Outcome: Fail, Known: "#807"},
		{Feature: "three", Outcome: Blocked},
		{Feature: "four", Outcome: Fail},
	}}
	if got := rep.Failures(); got != 3 {
		t.Errorf("Failures() = %d, want 3", got)
	}
	if got := rep.Unexpected(); got != 2 {
		t.Errorf("Unexpected() = %d, want the blocked row and the unexplained one", got)
	}
	if got := len(rep.Fixed()); got != 0 {
		t.Errorf("Fixed() = %d, want none", got)
	}
}

// A dialect's job wording is its own.
//
// bash lists a job as `Running` and zsh as `running`, and a suite that looked
// for one shell's word in the other's output would report a working `jobs` as
// broken — which it did, once, before this was data.
func TestTheJobWordingIsPerDialect(t *testing.T) {
	if Bash().JobRunning == Zsh().JobRunning || Bash().JobStopped == Zsh().JobStopped {
		t.Error("the two dialects were given the same job wording, which is not what they print")
	}
	for _, d := range []Dialect{Bash(), Zsh()} {
		if d.JobRunning == "" || d.JobStopped == "" || d.RCFile == "" || d.CwdEscape == "" {
			t.Errorf("%s is missing part of its description: %+v", d.Name, d)
		}
	}
}

func environmentValue(t *testing.T, env []string, name string) string {
	t.Helper()
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v
		}
	}
	t.Fatalf("%s is not in the session's environment", name)
	return ""
}
