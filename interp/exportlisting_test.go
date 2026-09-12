// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// export -p and readonly -p list what carries the attribute, in the form the
// dialect chose — and nothing else: an unexported name stays out of the
// first, an unmarked one out of the second.

func TestExportListingNamesTheExported(t *testing.T) {
	for _, tc := range []struct {
		name string
		form DeclarationListingForm
		want string
	}{
		{"clustered", DeclareListingClustered, `declare -x V="1"`},
		{"command word", DeclareListingCommandWord, `export V='1'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `export V=1; plain=2; export -p`, func(r *Runner) {
				sem := CoreSemantics()
				sem.ExportListing = tc.form
				if tc.form == DeclareListingClustered {
					sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
				} else {
					sem.DeclareValueQuoting = ListingQuoteAlwaysEscaped
				}
				r.Semantics = &sem
			})
			if st != 0 || !strings.Contains(out, tc.want) {
				t.Errorf("out = %q st=%d, want %q listed", out, st, tc.want)
			}
			if strings.Contains(out, "plain") {
				t.Errorf("out = %q, want the unexported name kept out", out)
			}
		})
	}
}

func TestReadonlyListingNamesTheReadonly(t *testing.T) {
	out, st := run(t, `readonly R=2; free=3; readonly -p`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ReadonlyListing = DeclareListingCommandWord
		sem.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		r.Semantics = &sem
	})
	if st != 0 || !strings.Contains(out, "readonly R=2") {
		t.Errorf("out = %q st=%d, want the readonly named", out, st)
	}
	if strings.Contains(out, "free") {
		t.Errorf("out = %q, want the unmarked name kept out", out)
	}
}

// The two forms `export -p` can take part company over the *attributes*, and
// a plain exported scalar cannot tell them apart — both write `export V=1`.
// So the discriminating rows are a name carrying a letter and a name holding
// a compound: one form writes the letters and changes the command word where
// `export` cannot carry the value, and the other writes its own word and the
// value alone (#1061). The compound's *elements* are written either way —
// that was the second half of the same fault, and it is not what tells the
// forms apart.
func TestTheExportListingFormDecidesWhetherLettersAreWritten(t *testing.T) {
	const src = `typeset -i -x n=5; typeset -a -x a=(1 2); export -p`
	for _, tc := range []struct {
		name string
		form DeclarationListingForm
		want string
	}{
		{"command word", DeclareListingCommandWord, "export a=( 1 2 )\nexport n=5\n"},
		{"export spelled", DeclareListingExportSpelled, "typeset -ax a=( 1 2 )\nexport -i n=5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src, func(s *Semantics) {
				s.DeclareOptions = "aAgiprx"
				s.ExportListing = tc.form
				s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
			}, Diagnostics{})
			if got := onlyProbeNames(out); got != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q/%d stderr %q, want %q", got, st, errs, tc.want)
			}
		})
	}
}

// A lone `+` given to `export` or `readonly` — see
// Semantics.SignAloneIsAnOptionWordToExport, which is a field of its own
// because a shell that reads the sign for `typeset` may still refuse it here
// (#1756).

func plusRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	out, errs, st := declRun(t, src, func(s *Semantics) {
		s.SignAloneIsAnOptionWordToExport = a
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	}, Diagnostics{})
	return onlyProbeNames(out), errs, st
}

// onlyProbeNames drops the rows a listing writes about the environment the
// test process happens to be running in — TMPDIR and its like. The subject
// here is which *names the script made* a sign selects, and a row that
// depends on the machine would make the assertion one about `go test`.
func onlyProbeNames(out string) string {
	var kept []string
	for _, line := range strings.SplitAfter(out, "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "E"), strings.HasPrefix(line, "R"),
			strings.HasPrefix(line, "plain"), strings.HasPrefix(line, "--"),
			strings.HasPrefix(line, "export E"), strings.HasPrefix(line, "readonly R"),
			strings.HasPrefix(line, "export a"), strings.HasPrefix(line, "export -i n"),
			strings.HasPrefix(line, "export n"), strings.HasPrefix(line, "typeset -ax a"):
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

// The headline, both ways and both builtins: the sign is a listing under one
// answer and a name nobody may declare under the other. The filter is the
// builtin's own attribute, which is what makes the two rows different — the
// exported name is out of `readonly +` and the frozen one out of `export +`.
func TestASignAloneIsAnOptionWordToExportIsAnAxis(t *testing.T) {
	const src = `export E=1; readonly R=2; plain=3; export +; echo --; readonly +`
	out, errs, st := plusRun(t, src, Yes)
	if want := "E\n--\nR\n"; out != want || st != 0 || errs != "" {
		t.Errorf("yes: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	out, _, st = plusRun(t, src, No)
	if out != "" || st == 0 {
		t.Errorf("no: got %q/%d, want the sign refused as a name", out, st)
	}
}

// Names alone: the sign that lists with values is the minus, and this one
// leaves them off. The pair in one test, because either row alone reads as a
// listing that happens to have no value to write.
func TestThePlusListingWritesNamesAndTheDashWritesValues(t *testing.T) {
	out, _, st := plusRun(t, `export E=1; export +`, Yes)
	if want := "E\n"; out != want || st != 0 {
		t.Errorf("plus: got %q/%d, want %q", out, st, want)
	}
	out, _, st = declRun(t, `export E=1; export -p`, func(s *Semantics) {
		s.ExportListing = DeclareListingCommandWord
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	}, Diagnostics{})
	if want := "export E=1\n"; onlyProbeNames(out) != want || st != 0 {
		t.Errorf("dash: got %q/%d, want %q", out, st, want)
	}
}

// Only the sign on its own. An operand alongside it is the line the operand
// path already answers, so the axis is not even asked — which is what keeps a
// name that happens to be written after a sign from being swallowed.
func TestTheSignIsOnlyReadWhenItIsTheWholeLine(t *testing.T) {
	out, _, st := plusRun(t, `export E=1; export + E`, Yes)
	if out != "" || st == 0 {
		t.Errorf("got %q/%d, want the operand line refused", out, st)
	}
}

// And the refusal is real where nothing answered, which is what says the two
// readings above are decisions rather than a question nothing reaches.
func TestTheSignAloneToExportAxisIsAsked(t *testing.T) {
	out, errs, _ := plusRun(t, `export +`, Unspecified)
	if !strings.Contains(errs, "a bare `+` given to `export` or `readonly`") {
		t.Errorf("got %q stderr %q, want a refusal naming the axis", out, errs)
	}
}
