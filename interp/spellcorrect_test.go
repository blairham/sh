// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `cd` may correct a misspelled operand, where the shell has asked for that.
//
// The capability one shell in the panel calls `cdspell`, asserted by where the
// shell ends up and by what it printed — not by the switch, which a table
// storing anything at all would satisfy.
//
// Every expectation here is bash 5.3.15 through a pseudo-terminal on
// 2026-09-08, because the name is interactive-only there: the same operand
// under `-c` is refused by both shells, which is the last case below.
func TestCdCorrectsOneEditPerComponent(t *testing.T) {
	for _, c := range []struct {
		name        string
		on          bool
		interactive bool
		operand     string
		wantDir     string
		wantOut     string
	}{
		{"a transposition", true, true, "alpah", "alpha", "alpha\n"},
		{"a dropped letter", true, true, "alph", "alpha", "alpha\n"},
		{"an extra letter", true, true, "alphaa", "alpha", "alpha\n"},
		{"a wrong letter", true, true, "alpma", "alpha", "alpha\n"},
		// Any component, and the printed form is the operand's own shape.
		{"a leading component", true, true, "alpah/beta", "alpha/beta", "alpha/beta\n"},
		{"a trailing component", true, true, "alpha/beat", "alpha/beta", "alpha/beta\n"},
		{"two components at once", true, true, "alpah/beat", "alpha/beta", "alpha/beta\n"},
		// Two edits is not one. bash refuses `doucmnets` against `documents`,
		// which is nine characters — long enough that a length-scaled budget
		// would have allowed it, so the threshold is flat.
		{"two edits", true, true, "alhpaa", "", ""},
		// Two *substitutions*, which is the same length as the name and so
		// takes a different branch from the one above: `alhpaa` is longer
		// than `alpha` and is refused by the length check before any
		// character is compared, where this one has to be counted.
		{"two wrong letters", true, true, "almma", "", ""},
		// An adjacent swap that is not the only difference. The two middle
		// letters of `alpha` really are transposed here, so a check that
		// stopped at the swap would take it — and the character after it is
		// wrong as well, which makes the whole name two edits away.
		{"a swap and a wrong letter", true, true, "alhpx", "", ""},
		{"nothing like it", true, true, "zzzz", "", ""},
		// Off, and in a script, the correction never happens.
		{"the option off", false, true, "alpah", "", ""},
		{"in a script", true, false, "alpah", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "alpha", "beta"), 0o755); err != nil {
				t.Fatal(err)
			}
			out, errOut := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errOut, Interactive: c.interactive,
			})
			r.SetCorrectsCdSpelling(c.on)
			f, err := syntax.Parse("cd "+c.operand+"\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			want := dir
			if c.wantDir != "" {
				want = filepath.Join(dir, filepath.FromSlash(c.wantDir))
			}
			if r.Dir != want {
				t.Errorf("Dir = %q, want %q", r.Dir, want)
			}
			// The corrected operand goes to stdout, which is what tells it
			// apart from a diagnostic — measured, `cd alpah 1>/dev/null`
			// hides it and `2>/dev/null` does not.
			if out.String() != c.wantOut {
				t.Errorf("stdout = %q, want %q", out.String(), c.wantOut)
			}
			// And a refusal names the operand as it was written, never how
			// far a correction got.
			if c.wantDir == "" && !strings.Contains(errOut.String(), c.operand) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), c.operand)
			}
		})
	}
}

// The correction is greedy and does not go back.
//
// Measured: in a tree holding `alpha/beta/gamma` and `alpha/betta`, bash
// corrects `alpha/bteta` to `alpha/betta` and then fails `alpha/bteta/gamma`
// outright rather than trying `beta`, whose `gamma` would have resolved. So a
// component is decided once, against what the components before it chose.
//
// Which of two equally close names wins is where this shell and bash part
// company on purpose: bash takes whatever its readdir hands back first — with
// `zzz1` and `zzz2` both one insertion from `zzz` it chose `zzz2`, the older
// of the two — and ours takes the first by name, because readDir sorts. The
// two agree wherever exactly one entry is within an edit.
func TestCdCorrectionCommitsToEachComponent(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"alpha/beta/gamma", "alpha/betta"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(operand string) (string, string) {
		out, errOut := &strings.Builder{}, &strings.Builder{}
		sem := PosixSemantics()
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
			Stdout: out, Stderr: errOut, Interactive: true,
		})
		r.SetCorrectsCdSpelling(true)
		f, err := syntax.Parse("cd "+operand+"\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(r.Dir), out.String()
	}
	// `beta` is the first of the two by name, so this shell commits to it —
	// and the component after it decides whether that was lucky.
	if got, printed := run("alpha/bteta"); !strings.HasSuffix(got, "beta") || printed != "alpha/beta\n" {
		t.Errorf("cd alpha/bteta left %q printing %q", got, printed)
	}
	// Committed, and the walk carries on from there rather than starting
	// over: `beta/gamma` exists, so this resolves.
	if got, _ := run("alpha/bteta/gamma"); !strings.HasSuffix(got, filepath.Join("beta", "gamma")) {
		t.Errorf("cd alpha/bteta/gamma left %q", got)
	}
	// The other direction is the proof there is no backtracking: `betta` has
	// no `delta` under it, and neither does `beta`, so a corrector that
	// re-tried the first component would still fail — but one that *did*
	// backtrack would have to read both, and this reads neither twice.
	if got, _ := run("alpha/bteta/delta"); got != dir {
		t.Errorf("cd alpha/bteta/delta left %q, want the shell where it started", got)
	}
}

// `cd` with no operand and a HOME that is not there names HOME, and does not
// take the shell down.
//
// It indexed an empty argument slice before this: the diagnostic was written
// against `args[0]` and there is no args[0] without an operand, so `cd` in a
// session whose home has been removed — `sudo -i`, a container, a deleted
// account — panicked. Every shell in the panel names the *value of HOME* here,
// measured 2026-09-08: bash, dash, ksh93 and zsh all print
// `/nonexistent-dir`, and none of them prints nothing.
func TestCdWithNoOperandNamesHomeRatherThanCrashing(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "not-there")
	out, errOut := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: errOut,
		Vars: map[string]string{"HOME": gone},
	})
	f, err := syntax.Parse("cd\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), gone) {
		t.Errorf("stderr = %q, want it to name HOME's value %q", errOut.String(), gone)
	}
	if r.ExitStatus() == 0 {
		t.Errorf("status %d, want a failure", r.ExitStatus())
	}
}

// A correction that lands on a *file* is not a correction.
//
// The mistake it guards against is not hypothetical: the second attempt sets
// the directory and clears the error, and the "is this a directory" check the
// first attempt made has already happened by then — so a correction handed
// back without one of its own would put the shell inside a regular file. The
// name typed here is one edit from a file and from nothing else, so a shell
// that took it would take it silently.
func TestCdDoesNotCorrectOntoAFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: errOut, Interactive: true,
	})
	r.SetCorrectsCdSpelling(true)
	f, err := syntax.Parse("cd notse\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if r.Dir != dir {
		t.Errorf("Dir = %q, want the shell where it started", r.Dir)
	}
	if out.String() != "" {
		t.Errorf("stdout = %q, want nothing announced", out.String())
	}
	if !strings.Contains(errOut.String(), "notse") {
		t.Errorf("stderr = %q, want the refusal to name what was typed", errOut.String())
	}
}

// The completer's entry to the same corrector, which is the half bash calls
// `dirspell`. Asserted here rather than only in repl because the *shape* of
// the answer is this package's decision and it is not `cd`'s: a completion
// puts an absolute, cleaned path into somebody's line where `cd` prints the
// operand as it was typed.
func TestCorrectedDirectoryAnswersAbsolutelyAndOnlyWhenAsked(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"documents", "alpha/beta"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		on   bool
		ask  string
		want string
	}{
		{"off, so there is nothing to correct with", false, filepath.Join(dir, "documnets"), ""},
		{"on, and the answer is absolute", true, filepath.Join(dir, "documnets"), filepath.Join(dir, "documents")},
		{"a component further in", true, filepath.Join(dir, "alpha/bteta"), filepath.Join(dir, "alpha/beta")},
		// Cleaned, which is bash's answer: `documents/../documnets/`
		// completes to the same path `documnets/` does.
		{"a `..` on the way", true, filepath.Join(dir, "documents/../documnets"), filepath.Join(dir, "documents")},
		// A file is not a directory to read, and a completer handed one
		// would rewrite the line and then find nothing there.
		{"one edit from a file", true, filepath.Join(dir, "notes.tx"), ""},
		{"nothing within one edit", true, filepath.Join(dir, "nowhere"), ""},
		{"nothing asked", true, "", ""},
		// The two shapes the completer never sends, because it resolves and
		// cleans before it asks — and this is exported, so somebody else
		// will. Written with a concatenation rather than filepath.Join,
		// which would clean the operand before the method ever saw it and
		// leave the branch below untested: mutation testing found exactly
		// that, and both lines survived being deleted.
		{"a relative operand", true, "documnets", filepath.Join(dir, "documents")},
		{"an operand nobody cleaned", true, dir + "/documents/../documnets", filepath.Join(dir, "documents")},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTestRunner(t, &Runner{Dir: dir})
			r.SetCorrectsCompletionSpelling(c.on)
			if got := r.CorrectedDirectory(c.ask); got != c.want {
				t.Errorf("CorrectedDirectory(%q) = %q, want %q", c.ask, got, c.want)
			}
		})
	}
}

// The two names are two switches over one corrector, and neither moves the
// other: a shell told to correct a typed `cd` has not been told to correct a
// Tab, and the other way about.
func TestTheTwoSpellingSwitchesAreIndependent(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	r.SetCorrectsCdSpelling(true)
	if r.CorrectsCompletionSpelling() {
		t.Error("`cdspell` turned `dirspell` on as well")
	}
	r.SetCorrectsCdSpelling(false)
	r.SetCorrectsCompletionSpelling(true)
	if r.CorrectsCdSpelling() {
		t.Error("`dirspell` turned `cdspell` on as well")
	}
	if !r.CorrectsCompletionSpelling() {
		t.Error("`dirspell` did not stay on")
	}
	// And the writing half is a third switch, off until it is asked for.
	if r.ExpandsCompletedDirectory() {
		t.Error("`direxpand` was on before anything set it")
	}
	r.SetExpandsCompletedDirectory(true)
	if !r.ExpandsCompletedDirectory() || !r.CorrectsCompletionSpelling() {
		t.Error("`direxpand` did not settle beside `dirspell`")
	}
}
