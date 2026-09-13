// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

// POSIX mode moves three of the four listing axes and leaves the fourth
// alone, which is a reading no preset holds and no vector can show.
//
// Measured 2026-09-12, macOS arm64, on bash 5.3.15 and the 3.2.57 macOS
// ships, over `V='a b'; export V; R=2; readonly R`:
//
//	                    default              set -o posix
//	export -p           declare -x V="a b"   export V="a b"
//	readonly -p         declare -r R="2"     readonly R="2"
//	export   (bare)     declare -x V="a b"   export V="a b"
//	readonly (bare)     declare -r R="2"     readonly R="2"
//	declare -p V        declare -x V="a b"   declare -x V="a b"
//
// Both builds agree and `set +o posix` puts the clustered form back, so it is
// a mode the shell enters and leaves rather than the build or the invocation.
// The value quoting does not move either — `export V="a b"` keeps the double
// quotes the clustered form uses — so DeclareValueQuoting stays where the
// dialect put it (#2154).
//
// The tests name the axes rather than a shell, and the fifth line of the
// script is the control that makes this three axes and not one.

// posixListingRun runs src over an empty environment — so a listing sees only
// what the snippet made — with the four listing axes clustered, and enters or
// leaves POSIX mode before the script runs.
func posixListingRun(t *testing.T, src string, mode func(*Runner), set func(*Semantics)) (out string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := testSemantics()
	sem.DeclareListing = DeclareListingClustered
	sem.ExportListing = DeclareListingClustered
	sem.ReadonlyListing = DeclareListingClustered
	sem.BareDeclarationListing = DeclareListingClustered
	sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
	if set != nil {
		set(&sem)
	}
	var o bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &o, Semantics: &sem,
		Name: "testsh", Env: []string{},
	})
	mode(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	// The scratch TMPDIR every test runner is given is exported, so it is in
	// every listing here and is not what any of this is about.
	var kept []string
	for line := range strings.SplitSeq(strings.TrimSuffix(o.String(), "\n"), "\n") {
		if line == "" || strings.Contains(line, "TMPDIR") {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return "", st
	}
	return strings.Join(kept, "\n") + "\n", st
}

// The five listings of #2154's table, in the order the table has them.
const posixListingSrc = `V="a b"; export V; R=2; readonly R
export -p
readonly -p
export
readonly
typeset -p V`

func TestPosixModeMovesThreeListingAxesAndNotTheFourth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		posix bool
		want  string
	}{
		{
			name: "the dialect's own name",
			want: `declare -x V="a b"
declare -r R="2"
declare -x V="a b"
declare -r R="2"
declare -x V="a b"
`,
		},
		{
			name:  "posix mode",
			posix: true,
			want: `export V="a b"
readonly R="2"
export V="a b"
readonly R="2"
declare -x V="a b"
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := posixListingRun(t, posixListingSrc, func(r *Runner) {
				r.SetPosixMode(tc.posix)
				if r.PosixMode() != tc.posix {
					t.Fatalf("the mode is %v; this case asserts nothing without it", r.PosixMode())
				}
			}, nil)
			if st != 0 || out != tc.want {
				t.Errorf("out = %q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// Leaving the mode puts the dialect's own forms back rather than asserting
// the clustered shape, which is the half the saved fields exist for.
func TestLeavingPosixModePutsTheListingFormsBack(t *testing.T) {
	want := `declare -x V="a b"
declare -r R="2"
declare -x V="a b"
declare -r R="2"
declare -x V="a b"
`
	out, st := posixListingRun(t, posixListingSrc, func(r *Runner) {
		r.SetPosixMode(true)
		r.SetPosixMode(false)
	}, nil)
	if st != 0 || out != want {
		t.Errorf("out = %q st=%d, want all five clustered again: %q", out, st, want)
	}
}

// A dialect whose export listing already was the command word keeps it on the
// way out, which one remembered form for all three could not do — and which
// asserting the standard's answer on leaving would break outright. zsh is the
// shell this is about: it writes `export V='a b'` and `typeset -r R=2`, so
// the two axes really do have to be saved apart.
func TestLeavingPosixModeKeepsAListingFormThatWasAlreadyTheCommandWord(t *testing.T) {
	out, st := posixListingRun(t, posixListingSrc, func(r *Runner) {
		r.SetPosixMode(true)
		r.SetPosixMode(false)
	}, func(s *Semantics) { s.ExportListing = DeclareListingCommandWord })
	if st != 0 {
		t.Fatalf("status = %d, want 0; out = %q", st, out)
	}
	if !strings.HasPrefix(out, `export V="a b"`+"\n") {
		t.Errorf("out = %q, want the export listing still the command word after the round trip", out)
	}
	if !strings.Contains(out, `declare -r R="2"`) {
		t.Errorf("out = %q, want the readonly listing back to the clustered form", out)
	}
}

// The mode moves an answer and does not invent one, which is the promise the
// loop and function-name axes already make. An axis nobody answered is still
// refused by name inside the mode.
func TestPosixModeDoesNotAnswerAnUnansweredListingAxis(t *testing.T) {
	out, st := posixListingRun(t, `V=1; export V; export -p`, func(r *Runner) {
		r.SetPosixMode(true)
	}, func(s *Semantics) { s.ExportListing = DeclarationListingUnspecified })
	if st == 0 {
		t.Errorf("out = %q st=%d, want the unanswered axis still refused inside the mode", out, st)
	}
	if strings.Contains(out, "export V=") {
		t.Errorf("out = %q, want no answer where the dialect gave none", out)
	}
}
