// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package emulatesweep

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/oracle"
)

// ourZsh builds the shipped zsh binary, which is the one binary this sweep can
// always point at. The controls are asserted against it rather than against a
// zsh that may not be installed: a control suite that skips on the machine
// where the answer matters is the shape this file exists to prevent.
func ourZsh(t *testing.T) Binary {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "zsh")
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, "github.com/blairham/sh/cmd/zsh")
	cmd.Dir = filepath.Dir(filepath.Dir(root)) // the module root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building cmd/zsh: %v\n%s", err, out)
	}
	return Binary{Name: "ours", Path: bin, Version: "under test"}
}

// TestTheControlsFireOnOurOwnBinary. This shell's answer to the population is
// a null — zero of 4485 snippets move with the mode — and a null is also what
// a harness that cannot see a parse error reports. So the two controls are
// asserted here, against the binary that is always in the tree, and a sweep
// that has been broken into reporting 0 fails this rather than reading as
// success.
func TestTheControlsFireOnOurOwnBinary(t *testing.T) {
	t.Parallel()
	b := ourZsh(t)
	dir := t.TempDir()
	for _, c := range Controls {
		got, err := Ask(context.Background(), dir, b, "control", c.Snippet)
		if err != nil {
			t.Fatalf("%q: %v", c.Snippet, err)
		}
		if got.Refused != c.Want {
			t.Errorf("%q: got %s, want %s — %s", c.Snippet, got, Verdict{Refused: c.Want}, c.Why)
		}
	}
}

// TestAControlThatCannotFireIsAnError. The controls are only worth having if a
// binary that would defeat them is refused, so both are pointed at a program
// that answers every invocation the same way: `true` never refuses, which must
// fail the positive control, and `false` always does, which must fail the
// negative one. Without this the control block is itself an unfalsified null.
func TestAControlThatCannotFireIsAnError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		prog string
		want string
	}{
		{"true", "{ fi; }"},
		{"false", "echo hi"},
	} {
		path, err := exec.LookPath(tc.prog)
		if err != nil {
			t.Skipf("no %s on this machine: %v", tc.prog, err)
		}
		err = fireControls(context.Background(), t.TempDir(), Binary{Name: tc.prog, Path: path})
		if err == nil {
			t.Fatalf("%s passed the controls; a binary that answers everything alike must not", tc.prog)
		}
		var cf ControlFailure
		if !asControlFailure(err, &cf) {
			t.Fatalf("%s: got %v, want a ControlFailure", tc.prog, err)
		}
		if cf.Control.Snippet != tc.want {
			t.Errorf("%s: failed on %q, want %q", tc.prog, cf.Control.Snippet, tc.want)
		}
	}
}

func asControlFailure(err error, out *ControlFailure) bool {
	cf, ok := err.(ControlFailure) //nolint:errorlint // returned unwrapped by fireControls
	if ok {
		*out = cf
	}
	return ok
}

// TestASweepOfNothingIsAnError. `-only` naming nothing would otherwise report
// the same zero a closed epic reports, which is the empty-input half of the
// null rule: an instrument that can fire still says nothing when the case it
// was pointed at holds nothing.
func TestASweepOfNothingIsAnError(t *testing.T) {
	t.Parallel()
	b := ourZsh(t)
	_, err := Sweep(context.Background(), Options{
		Reference: b, Ours: b,
		Cases: oracle.Corpus,
		Only:  "no-such-case-id-anywhere",
		Jobs:  1,
	})
	if err == nil {
		t.Fatal("a sweep selecting no cases returned a result; it must be an error")
	}
	if !strings.Contains(err.Error(), "no cases selected") {
		t.Errorf("got %v, want a refusal naming the empty selection", err)
	}
}

// TestTheReferenceMovesOnTheDiscriminator is the positive control over the
// *population* rather than over the harness: it points the sweep at the one
// snippet #4734 measured by hand and requires the reference to move on it.
// A sweep whose reference never moves has nothing to grade against, and that
// is indistinguishable from a closed epic unless something checks.
func TestTheReferenceMovesOnTheDiscriminator(t *testing.T) {
	t.Parallel()
	ref, err := ResolveReference()
	if err != nil {
		t.Skipf("no reference zsh here: %v", err)
	}
	got, err := Ask(context.Background(), t.TempDir(), ref, "discriminator", "d1() { print hi }")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Moves() {
		t.Fatalf("the reference answered %s for the short function body; it must move with the mode", got)
	}
	if !got.Refused[1] {
		t.Errorf("got %s, want a refusal under `emulate %s`", got, Modes[1])
	}
}

// TestRelativeIgnoresAnOrdinaryGrammarGap. The bar counts what the *mode*
// changed, so a construct one shell has and the other has not — which shifts
// all three modes together — must not be counted here. Several hundred corpus
// rows are in that state, and counting them would drown the signal the epic is
// graded on.
func TestRelativeIgnoresAnOrdinaryGrammarGap(t *testing.T) {
	t.Parallel()
	refusedThroughout := Verdict{Refused: [3]bool{true, true, true}}
	acceptedThroughout := Verdict{Refused: [3]bool{false, false, false}}
	if refusedThroughout.Relative() != acceptedThroughout.Relative() {
		t.Error("a construct refused in every mode and one accepted in every mode must read alike")
	}
	if refusedThroughout.Moves() || acceptedThroughout.Moves() {
		t.Error("neither answer moves with the mode")
	}
}

// TestAReversedCaseCounts. Two corpus snippets are refused by zsh's own mode
// and accepted by a mode, which is the evidence that the modes are a vector
// rather than a narrowing — so a verdict of that shape has to register as
// movement, or a table built on the narrowing assumption would pass.
func TestAReversedCaseCounts(t *testing.T) {
	t.Parallel()
	reversed := Verdict{Refused: [3]bool{true, false, false}}
	if !reversed.Moves() {
		t.Fatalf("%s must move: zsh's own mode refuses it and a mode accepts it", reversed)
	}
	r := &Result{Disagreements: []Disagreement{{ID: "cmd/x", Ref: reversed}}}
	if got := r.Split().Reversed; got != 1 {
		t.Errorf("Split().Reversed = %d, want 1", got)
	}
}

// TestSplitCountsEachMoverOnce over the four shapes #4734 measured, so a
// re-derivation of the 80/54/19/2 split cannot silently double-count.
func TestSplitCountsEachMoverOnce(t *testing.T) {
	t.Parallel()
	r := &Result{Disagreements: []Disagreement{
		{ID: "a/both", Ref: Verdict{Refused: [3]bool{false, true, true}}},
		{ID: "b/sh", Ref: Verdict{Refused: [3]bool{false, true, false}}},
		{ID: "c/ksh", Ref: Verdict{Refused: [3]bool{false, false, true}}},
		{ID: "d/rev", Ref: Verdict{Refused: [3]bool{true, false, false}}},
		{ID: "e/still", Ref: Verdict{Refused: [3]bool{false, false, false}}},
	}}
	got := r.Split()
	want := Split{Both: 1, ShOnly: 1, KshOnly: 1, Reversed: 1}
	if got != want {
		t.Errorf("Split() = %+v, want %+v", got, want)
	}
}

// TestByPrefixSeparatesTheFamilies. Each epic row owns a corpus prefix, so a
// row that made its own family worse while another improved has to be visible
// — a falling total is not a per-row check.
func TestByPrefixSeparatesTheFamilies(t *testing.T) {
	t.Parallel()
	r := &Result{Disagreements: []Disagreement{
		{ID: "pat/one"}, {ID: "pat/two"}, {ID: "cond/one"}, {ID: "bare"},
	}}
	got := r.ByPrefix()
	for name, want := range map[string]int{"pat": 2, "cond": 1, "bare": 1} {
		if got[name] != want {
			t.Errorf("ByPrefix()[%q] = %d, want %d", name, got[name], want)
		}
	}
}

// TestTheReportNamesTheControlsAndThePrefixes. The report is what a row is
// closed from, so both halves the null rule asks for have to be on the page:
// the controls that were fired, and the split the total would otherwise hide.
func TestTheReportNamesTheControlsAndThePrefixes(t *testing.T) {
	t.Parallel()
	r := &Result{
		Reference: Binary{Name: "reference", Path: "/opt/zsh", Version: "zsh 5.9.2"},
		Ours:      Binary{Name: "ours", Path: "build/zsh", Version: "cmd/zsh"},
		Total:     4485, RefMoves: 155, OurMoves: 0,
		Disagreements: []Disagreement{
			{ID: "pat/one", Ref: Verdict{Refused: [3]bool{false, true, true}}},
		},
	}
	out := r.Report()
	for _, want := range []string{"{ fi; }", "echo hi", "zsh 5.9.2", "cmd/zsh", "by corpus prefix", "pat"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
}

// TestEveryCorpusPrefixIsANonEmptyWord, so the per-family counts the epic's
// rows are cut along cannot silently collapse into one bucket.
func TestEveryCorpusPrefixIsANonEmptyWord(t *testing.T) {
	t.Parallel()
	seen := make(map[string]int)
	for _, c := range oracle.Corpus {
		p := Prefix(c.ID)
		if p == "" {
			t.Fatalf("case %q has an empty prefix", c.ID)
		}
		seen[p]++
	}
	if len(seen) < 2 {
		t.Fatalf("the corpus reduced to %d prefix(es); the per-family split would say nothing", len(seen))
	}
}

// TestAShellThatNeverStartedIsAnError. A binary that does not start answers
// every mode alike, which is exactly what a shell the mode does not reach
// looks like — so it has to be an error rather than a verdict. This is not
// hypothetical: the first run of this sweep was handed a relative `-bin`,
// every run happens in a scratch directory of its own, and nothing started.
func TestAShellThatNeverStartedIsAnError(t *testing.T) {
	t.Parallel()
	_, err := Ask(context.Background(), t.TempDir(),
		Binary{Name: "absent", Path: filepath.Join(t.TempDir(), "no-such-shell")},
		"control", "echo hi")
	if err == nil {
		t.Fatal("a shell that never started produced a verdict; it must be an error")
	}
}

// TestAHungRunIsNeitherRefusedNorAccepted. The reference really does hang on
// `cmd/function-keyword-with-a-name-holding-a-dollar` under `emulate ksh` —
// measured 2026-09-27 against zsh 5.9.2 — and the first full run of this sweep
// died on it. Folding a hang into "not refused" would have been worse: it
// would have counted as the mode reaching nothing, which is this shell's whole
// answer today, so the one shape that must never be manufactured is a zero.
func TestAHungRunIsNeitherRefusedNorAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hang := filepath.Join(dir, "hangs")
	if err := os.WriteFile(hang, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil { //nolint:gosec // a fixture that must be executable
		t.Fatal(err)
	}
	got, err := Ask(context.Background(), t.TempDir(),
		Binary{Name: "hangs", Path: hang, Timeout: 250 * time.Millisecond},
		"hang", "echo hi")
	if err != nil {
		t.Fatalf("a hang must be a verdict column, not an error: %v", err)
	}
	if !got.Unmeasured() {
		t.Fatalf("got %s, want every mode unmeasured", got)
	}
	for i := range got.Refused {
		if got.Refused[i] {
			t.Errorf("mode %s read as refused; a hang is not a refusal", Modes[i])
		}
	}
	if got.Moves() {
		t.Error("a snippet nothing answered must not read as moving with the mode")
	}
	if want := "[? ? ?]"; got.String() != want {
		t.Errorf("String() = %s, want %s", got, want)
	}
}

// TestAHungControlIsAControlFailure, because an instrument whose positive
// control did not finish has not been seen to fire.
func TestAHungControlIsAControlFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hang := filepath.Join(dir, "hangs")
	if err := os.WriteFile(hang, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil { //nolint:gosec // a fixture that must be executable
		t.Fatal(err)
	}
	err := fireControls(context.Background(), t.TempDir(),
		Binary{Name: "hangs", Path: hang, Timeout: 250 * time.Millisecond})
	if err == nil {
		t.Fatal("a binary that answers nothing passed the controls")
	}
}
