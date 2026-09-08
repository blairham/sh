// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The run-time pattern switches, named as options and never as a shell. What
// name a script uses to reach each one, and through which builtin, is a
// dialect's business and is asserted in dialect/bash.

func withOption(o MatchOption, on bool, more func(*Runner)) func(*Runner) {
	return func(r *Runner) {
		r.SetMatchOption(o, on)
		if more != nil {
			more(r)
		}
	}
}

func inDir(dir string) func(*Runner) {
	return func(r *Runner) { r.Dir = dir }
}

func TestUnmatchedPatternIsEmptyDeletesTheWord(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		on   bool
		src  string
		want string
	}{
		// The word is gone, not emptied: echo gets one operand, not two.
		{true, `echo zz*zz done`, "done"},
		{false, `echo zz*zz done`, "zz*zz done"},
		// A word that was never a pattern is not touched.
		{true, `echo plain`, "plain"},
		// And a quoted pattern was never a pattern.
		{true, `echo "zz*zz"`, "zz*zz"},
		// Matching is unaffected; only expansion empties.
		{true, `case zz in z*) echo hit;; esac`, "hit"},
	} {
		out, _ := run(t, tc.src, withOption(UnmatchedPatternIsEmpty, tc.on, inDir(dir)))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("on=%v %s = %q, want %q", tc.on, tc.src, got, tc.want)
		}
	}
	// Every word of the command can vanish, and that is not an error.
	out, st := run(t, `zz*zz; echo st=$?`, withOption(UnmatchedPatternIsEmpty, true, inDir(dir)))
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("a vanished command gave %q status %d, want st=0", out, st)
	}
}

func TestPatternsMatchHiddenLiftsTheLeadingPeriodRule(t *testing.T) {
	dir := fileDir(t, ".hid", "vis")
	for _, tc := range []struct {
		on   bool
		want string
	}{
		{false, "vis"},
		{true, ".hid vis"},
	} {
		out, _ := run(t, `echo *`, withOption(PatternsMatchHidden, tc.on, inDir(dir)))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("on=%v echo * = %q, want %q", tc.on, got, tc.want)
		}
	}
}

func TestGlobFoldsCaseReachesOnlyPathnameExpansion(t *testing.T) {
	dir := fileDir(t, "Apple")
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`echo a*`, "Apple"},
		{`echo [a]*`, "Apple"},
		{`echo A*`, "Apple"},
		// Matching keeps its case: the fold is expansion's alone.
		{`case A in a) echo hit;; *) echo exact;; esac`, "exact"},
		// So does parameter expansion.
		{`x=ABC; echo ${x#a}`, "ABC"},
	} {
		out, _ := run(t, tc.src, withOption(GlobFoldsCase, true, inDir(dir)))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
	// And without the option the pattern misses.
	out, _ := run(t, `echo a*`, inDir(dir))
	if got := strings.TrimSpace(out); got != "a*" {
		t.Errorf("off: echo a* = %q, want the literal pattern", got)
	}
}

func TestMatchFoldsCaseReachesOnlyCaseAndConditions(t *testing.T) {
	dir := fileDir(t, "Apple")
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`case A in a) echo hit;; *) echo exact;; esac`, "hit"},
		{`[[ ABC == a* ]] && echo hit || echo exact`, "hit"},
		{`[[ A != a ]] && echo differ || echo same`, "same"},
		// Not parameter expansion, which is measured: `ABC` keeps its A.
		{`x=ABC; echo ${x#a}`, "ABC"},
		// And not pathname expansion, which has a fold of its own.
		{`echo a*`, "a*"},
	} {
		out, _ := run(t, tc.src, withOption(MatchFoldsCase, true, inDir(dir)))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// treeDir is a directory holding d/e, with files f, d/f and d/e/f.
func treeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "e"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"f", "d/f", "d/e/f"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestStarStarCrossesDirectories(t *testing.T) {
	dir := treeDir(t)
	for _, tc := range []struct {
		on    bool
		alone bool
		src   string
		want  string
	}{
		// Zero levels and every deeper one, so the top-level f is a match.
		{true, false, `echo **/f`, "d/e/f d/f f"},
		{true, false, `echo d/**/f`, "d/e/f d/f"},
		// As the last component: the directory itself, slash-marked, then
		// everything beneath it — and that reading is the second option's,
		// which is why every row above leaves it off.
		{true, true, `echo d/**`, "d/ d/e d/e/f d/f"},
		{true, true, `echo **`, "d d/e d/e/f d/f f"},
		// Only exactly `**`: adjacent stars otherwise collapse to one.
		{true, false, `echo ***/f`, "d/f"},
		{true, false, `echo "**"/f`, "**/f"},
		// Off, `**` is `*` — one directory level, as everywhere else.
		{false, false, `echo **/f`, "d/f"},
		{false, true, `echo **/f`, "d/f"},
	} {
		opt := withOption(StarStarCrossesDirectories, tc.on,
			withOption(StarStarAloneCrossesDirectories, tc.alone, inDir(dir)))
		out, _ := run(t, tc.src, opt)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("on=%v alone=%v %s = %q, want %q", tc.on, tc.alone, tc.src, got, tc.want)
		}
	}
}

// TestStarStarAloneIsItsOwnQuestion is the split the panel measures: the
// slashed form crosses levels in bash, ksh93 and zsh alike, and the bare form
// crosses in the first two and is an ordinary pattern in the third. So the
// crossing being on says nothing about `**` with a slash *behind* it, and a
// `**/` written at the very end is the slashed form — the component after it
// is empty and written, which is what the index test in glob asks about and
// what a "nothing but empties follow" test would get wrong.
func TestStarStarAloneIsItsOwnQuestion(t *testing.T) {
	dir := treeDir(t)
	for _, tc := range []struct {
		alone bool
		src   string
		want  string
	}{
		{false, `echo **`, "d f"},
		{true, `echo **`, "d d/e d/e/f d/f f"},
		{false, `echo d/**`, "d/e d/f"},
		{true, `echo d/**`, "d/ d/e d/e/f d/f"},
		// Written with the slash, and level-crossing either way. The
		// matches carry the slash the pattern was written with, which is
		// what bash with the option and zsh without one both answer and
		// what these two rows asserted the other way round until #1350:
		// they were the shape that shows the loss most plainly, and they
		// forbade the right answer.
		{false, `echo **/`, "d/ d/e/"},
		{true, `echo **/`, "d/ d/e/"},
		{false, `echo **/f`, "d/e/f d/f f"},
		{true, `echo **/f`, "d/e/f d/f f"},
	} {
		opt := withOption(StarStarCrossesDirectories, true,
			withOption(StarStarAloneCrossesDirectories, tc.alone, inDir(dir)))
		out, _ := run(t, tc.src, opt)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("alone=%v %s = %q, want %q", tc.alone, tc.src, got, tc.want)
		}
	}
}

func TestStarStarSkipsHiddenUnlessAsked(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".h"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".h/x", "vis"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Bare `**`, so the walk needs the second option as well as the first.
	on := withOption(StarStarCrossesDirectories, true,
		withOption(StarStarAloneCrossesDirectories, true, inDir(dir)))
	if out, _ := run(t, `echo **`, on); strings.TrimSpace(out) != "vis" {
		t.Errorf("echo ** = %q, want the hidden tree skipped", out)
	}
	both := withOption(StarStarCrossesDirectories, true,
		withOption(StarStarAloneCrossesDirectories, true,
			withOption(PatternsMatchHidden, true, inDir(dir))))
	if out, _ := run(t, `echo **`, both); strings.TrimSpace(out) != ".h .h/x vis" {
		t.Errorf("echo ** with hidden = %q, want the hidden tree walked", out)
	}
}

func TestQuantifiedGroupsEverywhereIsARunTimeSwitch(t *testing.T) {
	// The pattern arrives through a variable because the outer parse is the
	// core's, which has no quantified groups; the switch is about what the
	// *matcher* reads, and eval below is about what nested parses read.
	src := `p="@(ab|cd)"; case ab in $p) echo hit;; *) echo no;; esac`
	for _, tc := range []struct {
		on   bool
		want string
	}{
		{true, "hit"},
		{false, "no"},
	} {
		out, _ := run(t, src, withOption(QuantifiedGroupsEverywhere, tc.on, nil))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("on=%v: got %q, want %q", tc.on, got, tc.want)
		}
	}
	// Nested input is parsed with the switched grammar: the same eval is a
	// group with the option on and a syntax error without it.
	evalSrc := `eval 'case ab in @(ab|cd)) echo hit;; esac'`
	out, _ := run(t, evalSrc, withOption(QuantifiedGroupsEverywhere, true, nil))
	if got := strings.TrimSpace(out); got != "hit" {
		t.Errorf("eval with the option on gave %q, want hit", got)
	}
	out, _ = run(t, evalSrc, nil)
	if strings.Contains(out, "hit") {
		t.Errorf("eval with the option off gave %q, want a refusal", out)
	}
}

// A subshell inherits the switches, because the clone copies the state. The
// other half — a subshell's change never escaping — needs a builtin to flip
// one mid-script, so it is asserted in the dialect that registers one.
func TestMatchOptionsReachASubshell(t *testing.T) {
	dir := t.TempDir()
	out, _ := run(t, `(echo zz*zz); echo zz*zz`,
		withOption(UnmatchedPatternIsEmpty, true, inDir(dir)))
	if got := strings.TrimSpace(out); got != "" {
		t.Errorf("the subshell lost the inherited option: %q", out)
	}
}
