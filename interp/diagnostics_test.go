// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

func TestSyntaxErrorStatusIsADialectAnswer(t *testing.T) {
	// The number is the SyntaxErrorStatus field's to give — measured per
	// dialect across eight distinct syntax errors, and asserted per preset in
	// the dialect packages. It was hardcoded to 2 under a comment claiming
	// every shell in the panel agreed, which is true of half of them.
	for _, tc := range []struct {
		name string
		diag Diagnostics
		want int
	}{
		{"a dialect's own number", Diagnostics{SyntaxErrorStatus: 3}, 3},
		{"another", Diagnostics{SyntaxErrorStatus: 1}, 1},
		// The substrate answers for itself rather than refusing: a status is
		// not a claim about another shell, and the process must exit with
		// some number.
		{"core", CoreDiagnostics(), 2},
		{"zero value", Diagnostics{}, 2},
	} {
		if got := tc.diag.SyntaxStatus(); got != tc.want {
			t.Errorf("%s: SyntaxStatus() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestCommandSubstitutionCarriesTheDialectStatus covers the second place a
// script is parsed. A seam that only reached whatever read the file first
// would be wrong inside `$( )`, which re-parses.
func TestCommandSubstitutionCarriesTheDialectStatus(t *testing.T) {
	for _, want := range []int{1, 2, 3} {
		f, err := syntax.Parse("x=$(if true); echo after", syntax.Core())
		if err != nil {
			t.Fatalf("the outer script must parse: %v", err)
		}
		var buf bytes.Buffer
		sem := permissive()
		diag := Diagnostics{SyntaxErrorStatus: want}
		r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag})
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		// Fatal in every measured dialect: none of them reach the next
		// command.
		if bytes.Contains(buf.Bytes(), []byte("after")) {
			t.Errorf("status %d: the script continued past a parse error: %q", want, buf.String())
		}
		if st != want {
			t.Errorf("status = %d, want %d", st, want)
		}
	}
}

// UnmatchedNearMaxBytes is a length rather than a code path, so the two
// answers worth pinning are the limit doing its job and zero meaning "all of
// it" — the answer three of the four presets give, and the one a dialect gets
// by saying nothing.
//
// Named for the field and not for a shell, which is the rule for a test in
// this package: what the dialect that elides actually prints is pinned beside
// that dialect.
func TestUnmatchedNearMaxBytesCutsTheQuotedWordAndZeroKeepsIt(t *testing.T) {
	const src = "v=$(echo bbbbbbbbbbbbbbbbbbbbbbbb\n"
	_, err := syntax.Parse(src, syntax.Dialect{})
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	const whole = "v=$(echo bbbbbbbbbbbbbbbbbbbbbbbb"
	for _, tc := range []struct {
		why   string
		limit int
		want  string
	}{
		{"zero keeps the whole word", 0, whole},
		{"a negative limit is zero", -1, whole},
		{"a limit shorter than the word cuts and marks it", 20, whole[:20] + "..."},
		{"a limit the word exactly reaches marks it uncut", len(whole), whole + "..."},
		{"a limit longer than the word leaves it alone", len(whole) + 1, whole},
	} {
		d := Diagnostics{
			UnmatchedCmdSubst:     "near `%[3]s'",
			UnmatchedNearMaxBytes: tc.limit,
		}
		if got, want := d.ParseFailure(err), "near `"+tc.want+"'"; got != want {
			t.Errorf("%s (limit %d):\n got %q\nwant %q", tc.why, tc.limit, got, want)
		}
	}
}
