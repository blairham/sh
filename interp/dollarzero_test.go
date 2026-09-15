// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `$0` and the call stack.
//
// The panel splits three ways. One shell moves `$0` as the shell is called
// into things: it names the function being run, or the file being sourced,
// and goes back to the script's name when that call returns. One moves it
// only for a function spelled with the `function` keyword, and for nothing
// else. The rest report the script's name however deep the shell is. The axis
// is Semantics.DollarZeroNames and the measurement is the `axis/dollar-zero-*`
// and `cmd/function-keyword-*` rows of docs/spec/measurements.md.

// zeroSemantics is permissive() with the axis under test answered, so a test
// about `$0` is not tripped by an unrelated axis going unanswered.
func zeroSemantics(a DollarZeroScope) Semantics {
	s := permissive()
	s.DollarZeroNames = a
	return s
}

// TestDollarZeroNamesTheInnermostCall walks every route the panel splits on.
//
// The cases are the corpus's, run here against the substrate so that the
// stack discipline is pinned where it is implemented rather than only where
// it is graded.
func TestDollarZeroNamesTheInnermostCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		src   string
		on    string // with the axis answered yes
		off   string // and no
	}{
		{
			name:  "a sourced file names itself, and only while it runs",
			files: map[string]string{"inc.sh": `echo "in=[$0]"`},
			src:   "echo \"before=[$0]\"\n. ./inc.sh\necho \"after=[$0]\"",
			on:    "before=[testsh]\nin=[./inc.sh]\nafter=[testsh]\n",
			off:   "before=[testsh]\nin=[testsh]\nafter=[testsh]\n",
		},
		{
			name:  "a function defined in a sourced file names the function",
			files: map[string]string{"inc.sh": "f() { echo \"in=[$0]\"; }"},
			src:   ". ./inc.sh\nf",
			on:    "in=[f]\n",
			off:   "in=[testsh]\n",
		},
		{
			name: "a file sourced by a sourced file",
			files: map[string]string{
				"deep.sh": `echo "deep=[$0]"`,
				"mid.sh":  "echo \"mid=[$0]\"\n. ./deep.sh\necho \"mid-again=[$0]\"",
			},
			src: ". ./mid.sh",
			on:  "mid=[./mid.sh]\ndeep=[./deep.sh]\nmid-again=[./mid.sh]\n",
			off: "mid=[testsh]\ndeep=[testsh]\nmid-again=[testsh]\n",
		},
		{
			// The case that settles which question the axis is really
			// asking. A rule written as "a function is on the stack" gets
			// the middle line wrong.
			name:  "a file sourced from inside a function names the file",
			files: map[string]string{"inc.sh": `echo "in=[$0]"`},
			src:   "f() { echo \"fn=[$0]\"; . ./inc.sh; echo \"fn-again=[$0]\"; }\nf",
			on:    "fn=[f]\nin=[./inc.sh]\nfn-again=[f]\n",
			off:   "fn=[testsh]\nin=[testsh]\nfn-again=[testsh]\n",
		},
		{
			// The operand and not the file: the search joins a path the
			// script never wrote, and `$0` is the word it did write. See
			// Frame.Operand.
			name:  "a file found on PATH names the operand",
			files: map[string]string{"inc.sh": `echo "in=[$0]"`},
			src:   ". inc.sh",
			on:    "in=[inc.sh]\n",
			off:   "in=[testsh]\n",
		},
		{
			// `eval` pushes no frame, so it is transparent to this in every
			// shell — including the one that moves `$0` everywhere else.
			name: "eval is transparent",
			src:  `eval 'echo "in=[$0]"'`,
			on:   "in=[testsh]\n",
			off:  "in=[testsh]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, answer := range []struct {
				a    DollarZeroScope
				want string
			}{{DollarZeroIsTheInnermostCall, tc.on}, {DollarZeroIsTheShellsOwnName, tc.off}} {
				dir := t.TempDir()
				for name, body := range tc.files {
					write(t, dir, name, body+"\n")
				}
				out, st := sourceRun(t, dir, tc.src, zeroSemantics(answer.a), Diagnostics{})
				if out != answer.want {
					t.Errorf("axis %v: got %q, want %q", answer.a, out, answer.want)
				}
				if st != 0 {
					t.Errorf("axis %v: status %d, want 0", answer.a, st)
				}
			}
		})
	}
}

// TestDollarZeroOutsideAnyCallNeverAsksTheAxis. At the top level there is
// nothing for `$0` to name but the shell, so the question does not arise —
// and a core that refuses every unanswered axis must not refuse `echo $0` in
// a script that has called nothing.
func TestDollarZeroOutsideAnyCallNeverAsksTheAxis(t *testing.T) {
	sem := zeroSemantics(DollarZeroScopeUnspecified)
	out, st := sourceRun(t, t.TempDir(), `echo "[$0]"`, sem, Diagnostics{})
	if out != "[testsh]\n" || st != 0 {
		t.Errorf("got %q status %d, want the shell's own name and success", out, st)
	}
}

// TestThePresetsAnswerForThemselves. The standard's preset answers with the
// shell's own name — the majority reading, and the one the standard's own
// text has no reason to move — and the bare core answers nothing at all,
// which is this package's discipline rather than an omission.
//
// Both asserted by running the preset rather than by reading the field, and
// with nothing overriding it: every other test here sets the axis by hand, so
// flipping the value the preset stores failed nothing in the whole tree until
// this existed.
func TestThePresetsAnswerForThemselves(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "echo \"in=[$0]\"\n")
	const src = "f() { echo \"fn=[$0]\"; }\nf\n. ./inc.sh\n"

	// permissive() is the standard's preset with the axes `.` needs on its
	// failure paths answered, and it says nothing about this one.
	out, st := sourceRun(t, dir, src, permissive(), Diagnostics{})
	if want := "fn=[testsh]\nin=[testsh]\n"; out != want || st != 0 {
		t.Errorf("the standard's preset: said %q status %d, want %q and 0", out, st, want)
	}

	// And the core, where nothing has been chosen: a refusal per site.
	const refusal = "testsh: $0 naming the function or sourced file it is inside: " +
		"the shells disagree here and no dialect was chosen\n"
	out, st = sourceRun(t, dir, src, CoreSemantics(), Diagnostics{})
	if want := refusal + refusal; out != want || st != 2 {
		t.Errorf("the bare core: said %q status %d, want %q and 2", out, st, want)
	}
}

// TestDollarZeroInsideACallRefusesWhenNothingAnswered pins the whole rendered
// refusal, location included: an unanswered axis reached from inside a call
// is a refusal and not a guess, and either answer would have been a different
// program.
//
// The two cases are written to raise it on different lines, so a location
// taken from the wrong frame cannot pass both.
func TestDollarZeroInsideACallRefusesWhenNothingAnswered(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "# a comment\n# another\necho \"in=[$0]\"\n")
	const tail = "$0 naming the function or sourced file it is inside: " +
		"the shells disagree here and no dialect was chosen\n"

	for _, tc := range []struct{ name, src, want string }{
		{
			// The function's body starts on line 2 of the script, and the
			// call is on line 4.
			"inside a function",
			"f() {\n  echo \"in=[$0]\"\n}\ntrue\nf",
			"testsh: line 2: " + tail,
		},
		{
			// And a sourced file counts from its own top, so this is line 3
			// of a file the `.` on line 2 of the script pulled in.
			"inside a sourced file",
			"true\n. ./inc.sh",
			"testsh: line 3: " + tail,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := sourceRun(t, dir, tc.src, zeroSemantics(DollarZeroScopeUnspecified),
				Diagnostics{Location: LocationLineWord})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestASourcedFilesFrameCarriesBothTheOperandAndTheFile. The two are the same
// string on every route but one, so a single field would have looked right
// until a PATH search made them differ — and then `$0` would have reported a
// path the script never wrote.
//
// Both halves are asserted here because the fix is exactly that they are two
// fields: `$0` takes the operand and a location takes the file, from the same
// frame, at the same time.
func TestASourcedFilesFrameCarriesBothTheOperandAndTheFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	// PATH is the subdirectory, so the file the search opens is `sub/inc.sh`
	// where the operand is the bare `inc.sh`.
	write(t, sub, "inc.sh", "echo \"in=[$0]\"\n")
	write(t, sub, "bad.sh", "nosuchcmd-xyz\n")
	dg := Diagnostics{Location: LocationTightLine, LocationNamesTheCurrentFile: true}

	out, _ := sourceRunOnPath(t, dir, "sub", ". inc.sh", zeroSemantics(DollarZeroIsTheInnermostCall), dg)
	if out != "in=[inc.sh]\n" {
		t.Errorf("got %q, want the operand the script wrote", out)
	}
	out, _ = sourceRunOnPath(t, dir, "sub", ". bad.sh", zeroSemantics(DollarZeroIsTheInnermostCall), dg)
	if !strings.HasPrefix(out, "sub/bad.sh:1: ") {
		t.Errorf("got %q, want the location to name the joined path", out)
	}
}

// sourceRunOnPath is sourceRun with PATH pointed somewhere other than the
// directory the script runs in, which is what separates the operand `.` was
// given from the file the search finds.
func sourceRunOnPath(t *testing.T, dir, path, src string, sem Semantics, dg Diagnostics) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": path}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// TestADigitRunWorthZeroIsDollarZero. `${00}` and `${000}` are the shell's own
// name, not the empty string: a positional parameter's digits are read as a
// *number*, so a leading zero is not part of a name and a run worth nothing
// names what `$0` names.
//
// Unanimous across the panel — measured 2026-09-15 on bash 5.3, bash 3.2, bash
// as `sh`, ksh93, dash, zsh and BusyBox ash — so it is a fact here rather than
// an axis, and it is asked of this package because every dialect inherits it.
//
// This answered the empty string until #2879, which was reachable only through
// the braced spelling while every dialect read `$00` as `$0` with a `0` left
// over. `Dialect.MultiDigitPositional` gives one of them the unbraced route to
// it, which is why the two land together.
//
// The axis is carried through rather than bypassed: a run worth zero is `$0`
// wherever it stands, so inside a call it follows `$0` and does not fall back
// to the script's name.
func TestADigitRunWorthZeroIsDollarZero(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		on, off string
	}{
		{
			name: "at the top level",
			src:  `echo "[${0}][${00}][${000}]"`,
			on:   "[testsh][testsh][testsh]\n",
			off:  "[testsh][testsh][testsh]\n",
		},
		{
			name: "inside a call it follows $0",
			src:  "f() { echo \"[${0}][${00}]\"; }\nf",
			on:   "[f][f]\n",
			off:  "[testsh][testsh]\n",
		},
		{
			// The control that says the run is read as a number and not
			// merely stripped of zeros: `${01}` is the *first* parameter and
			// `${010}` the tenth, neither of which is `$0`.
			name: "a run worth more is a positional",
			src:  `set -- p q r s t u v w x y; echo "[${01}][${010}][${0010}]"`,
			on:   "[p][y][y]\n",
			off:  "[p][y][y]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, answer := range []struct {
				a    DollarZeroScope
				want string
			}{{DollarZeroIsTheInnermostCall, tc.on}, {DollarZeroIsTheShellsOwnName, tc.off}} {
				out, st := sourceRun(t, t.TempDir(), tc.src, zeroSemantics(answer.a), Diagnostics{})
				if out != answer.want || st != 0 {
					t.Errorf("axis %v: got %q status %d, want %q and 0", answer.a, out, st, answer.want)
				}
			}
		})
	}
}

// TestDollarZeroNamesTheInnermostKeywordFunction is the third reading, and
// every case here is one the other two get wrong.
//
// It is a separate table rather than a third column on the one above because
// the rows are different rows: what separates this reading from "the
// innermost call" is never a function on its own but a *frame that does not
// answer* — a `name()` function, a sourced file — sitting above one that
// does. A table whose sources only ever call one thing cannot tell them
// apart.
func TestDollarZeroNamesTheInnermostKeywordFunction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		src   string
		// One want per reading, so each row says what all three do.
		call, keyword, shell string
	}{
		{
			name: "the keyword spelling names the function and the other does not",
			src: "function kf { echo \"kf=[$0]\"; }\npf() { echo \"pf=[$0]\"; }\nkf\npf\n" +
				"echo \"top=[$0]\"",
			call:    "kf=[kf]\npf=[pf]\ntop=[testsh]\n",
			keyword: "kf=[kf]\npf=[testsh]\ntop=[testsh]\n",
			shell:   "kf=[testsh]\npf=[testsh]\ntop=[testsh]\n",
		},
		{
			// The row that shows this is a different frame and not the
			// innermost call narrowed to a spelling: the file is on top of
			// the stack and does not answer, and does not hide the function
			// under it either.
			name:    "a file sourced from inside a keyword function is still the function",
			files:   map[string]string{"inc.sh": `echo "in=[$0]"`},
			src:     "function kf { . ./inc.sh; }\nkf\n. ./inc.sh",
			call:    "in=[./inc.sh]\nin=[./inc.sh]\n",
			keyword: "in=[kf]\nin=[testsh]\n",
			shell:   "in=[testsh]\nin=[testsh]\n",
		},
		{
			name:    "a name() function called from inside a keyword one is still the outer",
			src:     "pf() { echo \"pf=[$0]\"; }\nfunction outer { pf; }\nouter\npf",
			call:    "pf=[pf]\npf=[pf]\n",
			keyword: "pf=[outer]\npf=[testsh]\n",
			shell:   "pf=[testsh]\npf=[testsh]\n",
		},
		{
			name:    "and it goes back when the call returns",
			src:     "function kf { echo \"in=[$0]\"; }\nkf\necho \"after=[$0]\"",
			call:    "in=[kf]\nafter=[testsh]\n",
			keyword: "in=[kf]\nafter=[testsh]\n",
			shell:   "in=[testsh]\nafter=[testsh]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, answer := range []struct {
				a    DollarZeroScope
				want string
			}{
				{DollarZeroIsTheInnermostCall, tc.call},
				{DollarZeroIsTheInnermostKeywordFunction, tc.keyword},
				{DollarZeroIsTheShellsOwnName, tc.shell},
			} {
				dir := t.TempDir()
				for name, body := range tc.files {
					write(t, dir, name, body+"\n")
				}
				out, st := sourceRun(t, dir, tc.src, zeroSemantics(answer.a), Diagnostics{})
				if out != answer.want || st != 0 {
					t.Errorf("axis %v: got %q status %d, want %q and 0", answer.a, out, st, answer.want)
				}
			}
		})
	}
}
