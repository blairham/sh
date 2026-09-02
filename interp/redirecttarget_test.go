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

// A redirection's target is expanded and then, in three of the four shells,
// neither split into fields nor matched as a pattern. bash expands it the way
// an argument is expanded and refuses anything that is not exactly one word.
//
// This did neither: it took bash's expansion and quietly kept the first field,
// which is the answer no shell gives. `> $e` with a space in it wrote to `a`,
// and `> $e` holding a pattern truncated whichever file happened to match —
// a file the script never named.
func TestARedirectionTargetIsNotSplitOrMatched(t *testing.T) {
	for _, tc := range []struct {
		name, src, wantFile string
	}{
		{"a space in it is part of the name", `e="a b"; echo hi > $e`, "a b"},
		{"quoting changes nothing here", `e="c d"; echo hi > "$e"`, "c d"},
		// The dangerous one. `x1` exists, and matching would have written
		// into it.
		{"a pattern is a name", `e="x*"; echo hi > $e`, "x*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			touch(t, dir, "x1")
			if _, st := run(t, tc.src, literalTargets(dir)); st != 0 {
				t.Fatalf("status %d", st)
			}
			if got := readFile(t, dir, tc.wantFile); got != "hi\n" {
				t.Errorf("%s = %q, want the redirection to have written it", tc.wantFile, got)
			}
			if got := readFile(t, dir, "x1"); got != "" {
				t.Errorf("x1 = %q, want a file the script never named left alone", got)
			}
		})
	}
}

// The other dialect refuses rather than choosing, and names the target as it
// was *written* — `$e`, not what `$e` came to, which by then is the only thing
// left unless the parser kept the text.
func TestARedirectionTargetThatIsNotOneWordIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"two words", `e="a b"; echo hi > $e`, "$e: ambiguous redirect"},
		{"no words at all", `e=; echo hi > $e`, "$e: ambiguous redirect"},
		{"an unset name", `echo hi > $unset`, "$unset: ambiguous redirect"},
		// Empty is as ambiguous as two: neither says where to write.
		{"braces", `echo hi > {a,b}`, "{a,b}: ambiguous redirect"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := run(t, tc.src, ordinaryTargets(dir))
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
			if st == 0 {
				t.Error("status 0, want the redirection refused")
			}
		})
	}
}

// And a target that is one word either way is not a question, so the core
// answers it rather than refusing: `> f` is the same in all four.
func TestARedirectionTargetIsAskedAboutOnlyWhenItMatters(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refuses   bool
	}{
		{"a plain name", `echo hi > f`, false},
		{"a quoted expansion", `e="a b"; echo hi > "$e"`, false},
		// A pattern matching nothing is itself under both readings.
		{"a pattern that matches nothing", `e="zz*"; echo hi > $e`, false},
		{"a pattern that matches", `e="x*"; echo hi > $e`, true},
		{"an expansion with a space", `e="a b"; echo hi > $e`, true},
		{"an expansion with nothing in it", `e=; echo hi > $e`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			touch(t, dir, "x1")
			out, _ := run(t, tc.src, func(r *Runner) {
				// Every axis a redirection target touches *except* the one
				// under test, so a refusal here is about this question and
				// not about splitting or matching in general.
				sem := CoreSemantics()
				sem.SplitParamExpansion = Yes
				sem.SplitCommandSubstitution = Yes
				sem.GlobExpansionResults = Yes
				sem.GlobNoMatchIsError = No
				sem.BraceExpansion = Yes
				sem.GlobNoMatchIsError = No
				r.Semantics, r.Dir = &sem, dir
			})
			if got := strings.Contains(out, "no dialect was chosen"); got != tc.refuses {
				t.Errorf("refused = %v, want %v (out %q)", got, tc.refuses, out)
			}
		})
	}
}

// A tilde still expands where nothing is split. Not splitting is not the same
// as not expanding, and leaving it out of that path made `[[ -f ~/x ]]` false
// in a home directory holding the file.
func TestATildeExpandsWhereNothingIsSplit(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "tf")
	out, _ := run(t, `[[ -f ~/tf ]] && echo yes || echo no`, func(r *Runner) {
		sem := CoreSemantics()
		r.Semantics, r.Dir = &sem, dir
		r.Vars = map[string]string{"HOME": dir}
	})
	if strings.TrimSpace(out) != "yes" {
		t.Errorf("out = %q, want the tilde expanded", out)
	}
}

// A tilde expands in a redirection target too, which is the same rule as
// inside `[[ ]]` reached by the other road.
func TestATildeExpandsInARedirectionTarget(t *testing.T) {
	dir := t.TempDir()
	if _, st := run(t, `echo hi > ~/tf`, func(r *Runner) {
		sem := CoreSemantics()
		sem.RedirectTargetIsAnOrdinaryWord = No
		r.Semantics, r.Dir = &sem, dir
		r.Vars = map[string]string{"HOME": dir}
	}); st != 0 {
		t.Fatalf("status %d", st)
	}
	if got := readFile(t, dir, "tf"); got != "hi\n" {
		t.Errorf("tf = %q, want the tilde expanded to the home directory", got)
	}
}

// One dialect says something shorter for a target that expanded to nothing —
// no reason attached, and "open" even where the redirection was creating.
func TestAnEmptyTargetCanHaveAWordingOfItsOwn(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"creating", `e=; echo hi > $e`, ": cannot open"},
		{"and opening", `e=; cat < $e`, ": cannot open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				sem.RedirectTargetIsAnOrdinaryWord = No
				sem.SplitParamExpansion = Yes
				dg := Diagnostics{EmptyRedirectTarget: "%[1]s: cannot open"}
				r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, t.TempDir()
			})
			// The whole line, so an added prefix or a bracketed reason
			// cannot slip past: the ordinary wordings both end in one.
			line := strings.TrimSpace(out)
			if !strings.HasSuffix(line, tc.want) || strings.Contains(line, "[") {
				t.Errorf("out = %q, want a line ending %q with no reason attached", out, tc.want)
			}
			if st == 0 {
				t.Error("status 0, want the redirection to have failed")
			}
		})
	}
}

// A name that is not there is not a relative one. Joining it to the working
// directory turns "no name" into *the directory*, which then opens — so
// `cd /tmp; cat < $unset` read the directory rather than failing, and only
// once the shell had been told where it was.
func TestAnEmptyTargetIsNotTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "in-there")
	out, st := run(t, `e=; cat < $e; echo "st=$?"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.SplitParamExpansion = Yes
		r.Semantics, r.Dir = &sem, dir
	})
	if strings.Contains(out, "in-there") {
		t.Errorf("out = %q, want the directory not opened", out)
	}
	if !strings.Contains(out, "st=1") && !strings.Contains(out, "st=2") {
		t.Errorf("out = %q, want the redirection to have failed", out)
	}
	if st != 0 {
		t.Logf("status %d", st)
	}
}

func literalTargets(dir string) func(*Runner) {
	return func(r *Runner) {
		sem := CoreSemantics()
		sem.SplitParamExpansion = Yes
		sem.SplitCommandSubstitution = Yes
		sem.GlobExpansionResults = Yes
		sem.GlobNoMatchIsError = No
		sem.RedirectTargetIsAnOrdinaryWord = No
		r.Semantics, r.Dir = &sem, dir
	}
}

func ordinaryTargets(dir string) func(*Runner) {
	return func(r *Runner) {
		sem := CoreSemantics()
		sem.SplitParamExpansion = Yes
		sem.SplitCommandSubstitution = Yes
		sem.GlobExpansionResults = Yes
		sem.GlobNoMatchIsError = No
		sem.RedirectTargetIsAnOrdinaryWord = Yes
		sem.BraceExpansion = Yes
		dg := Diagnostics{AmbiguousRedirect: "%[1]s: ambiguous redirect"}
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, dir
	}
}

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
