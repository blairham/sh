// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// notReadingTheAmpersand is the bare core with one axis answered: several rows
// below write a quote or a backslash *inside* a quoted expansion, which is the
// one reading of a `${ }` operand the panel divides on, and the core refuses an
// unanswered disagreement. `No` is the word reading, where the operand's own
// quotes quote and are removed — see replacementquoting_test.go, which is where
// that axis is measured. It is answered in both states here so that the two
// states differ by the option and by nothing else.
func notReadingTheAmpersand(r *Runner) { replacementQuoting(No)(r) }

// readingTheAmpersand is that plus the option on, which is how every row that
// expects a match written into its replacement gets one. It is off in the bare
// core, so a test that forgot it would be asserting the other state.
func readingTheAmpersand(r *Runner) {
	notReadingTheAmpersand(r)
	r.SetMatchOption(ReplacementAmpersandIsTheMatch, true)
}

// With [ReplacementAmpersandIsTheMatch] on, an unquoted `&` in a pattern
// substitution's replacement is the text the pattern took.
//
// The rows are the whole of what "the text the pattern took" means, and each
// one distinguishes it from a plausible wrong answer: the pattern itself
// (which is what the history modifier's `&` can only ever be), the whole
// value, or the first character of the match.
func TestTheReplacementAmpersandIsTheTextTheMatchTook(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a literal pattern",
			`v=abc; printf "[%s]" "${v/b/[&]}"`,
			"[a[b]c]",
		},
		{
			// The `&` is the match and not the pattern, which a literal
			// pattern cannot tell apart.
			"a pattern with metacharacters is the span it took",
			`v=aXbXc; printf "[%s]" "${v/X*X/<&>}"`,
			"[a<XbX>c]",
		},
		{
			"a one-character wildcard",
			`v=hello; printf "[%s]" "${v/?/<&>}"`,
			"[<h>ello]",
		},
		{
			"a global substitution reads each match into its own copy",
			`v=abcabc; printf "[%s]" "${v//[ab]/<&>}"`,
			"[<a><b>c<a><b>c]",
		},
		{
			"twice in one replacement is the match twice",
			`v=hello; printf "[%s]" "${v//l/&&}"`,
			"[hellllo]",
		},
		{
			"anchored at the front",
			`v=abcabc; printf "[%s]" "${v/#a/<&>}"`,
			"[<a>bcabc]",
		},
		{
			"anchored at the end",
			`v=abcabc; printf "[%s]" "${v/%c/<&>}"`,
			"[abcab<c>]",
		},
		{
			// Nothing matched, so the replacement is never read and the `&`
			// never stands for anything.
			"a pattern that matches nothing leaves the value alone",
			`v=abc; printf "[%s]" "${v/zzz/<&>}"`,
			"[abc]",
		},
		{
			"every element of a list is read on its own",
			`v=abc; w=xbz; printf "[%s]" "${v/b/<&>}" "${w/b/<&>}"`,
			"[a<b>c][x<b>z]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src, readingTheAmpersand)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// Quoting is what turns the reading off, character by character — the same
// channel that decides whether a `*` in the *pattern* half is a pattern.
//
// Each row is the option **on**, so a row that passed by the reading never
// having happened would fail the test above. The enclosing quotes are
// deliberately present on the first four: they are not what is asked, and a
// fix that read the operand's quoting off the `${ }` around it would make
// every one of these a literal `&` and the test above would still pass.
func TestAQuotedReplacementAmpersandIsAnOrdinaryCharacter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"double-quoted",
			`v=abc; printf "[%s]" "${v/b/"&"}"`,
			"[a&c]",
		},
		{
			"single-quoted",
			`v=abc; printf "[%s]" "${v/b/'&'}"`,
			"[a&c]",
		},
		{
			"dollar-single-quoted",
			`v=abc; printf "[%s]" "${v/b/$'&'}"`,
			"[a&c]",
		},
		{
			"backslashed",
			`v=abc; printf "[%s]" "${v/b/[\&]}"`,
			"[a[&]c]",
		},
		{
			// The discriminating pair: one expansion, two quotings, and the
			// only thing between them is whether the `&` the value carries
			// is read.
			"from an unquoted expansion",
			`v=abc; r='&'; printf "[%s]" "${v/b/$r}"`,
			"[abc]",
		},
		{
			"from a quoted expansion",
			`v=abc; r='&'; printf "[%s]" "${v/b/"$r"}"`,
			"[a&c]",
		},
		{
			"an unquoted expansion in the middle of written text",
			`v=abc; r='&'; printf "[%s]" "${v/b/<$r>}"`,
			"[a<b>c]",
		},
		{
			// The enclosing quotes are not the operand's quoting, which is
			// the reading replacementWord already settles for the rest of
			// the operand.
			"the quotes around the whole expansion do not reach the operand",
			`v=abc; printf "[%s]" "${v/b/[&]}"`,
			"[a[b]c]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src, readingTheAmpersand)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// A backslash in the text an expansion brought is an escape only in front of
// an `&` or another backslash, and stands as itself in front of anything else.
//
// This is the half that differs from the history modifier's rule, where a
// backslash stands for whatever byte follows it — see expandAmpersand, which
// takes the rule as an argument for exactly this reason. A row written down
// rather than expanded cannot see the difference: ordinary quote removal has
// already taken the backslash off by the time the replacement is read.
func TestABackslashInAnExpandedReplacementEscapesOnlyItselfAndTheAmpersand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"before an ampersand it leaves the ampersand",
			`v=abc; r='[\&]'; printf "[%s]" "${v/b/$r}"`,
			"[a[&]c]",
		},
		{
			"before another backslash it leaves one backslash",
			`v=abc; r='[\\]'; printf "[%s]" "${v/b/$r}"`,
			`[a[\]c]`,
		},
		{
			// Order is observable: a pass resolving the escapes first would
			// turn this into an escaped ampersand and answer `a[\&]c`.
			"a doubled backslash then an ampersand is a backslash and the match",
			`v=abc; r='[\\&]'; printf "[%s]" "${v/b/$r}"`,
			`[a[\b]c]`,
		},
		{
			"before an ordinary character it stands as itself",
			`v=abc; r='[\a]'; printf "[%s]" "${v/b/$r}"`,
			`[a[\a]c]`,
		},
		{
			"at the very end there is nothing to escape",
			`v=abc; r='x\'; printf "[%s]" "${v/b/$r}"`,
			`[ax\c]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src, readingTheAmpersand)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// With the option **off** an `&` is one ordinary character wherever it came
// from, and a backslash the replacement carries is left where it stands.
//
// The mirror of the two tests above, run through the same sources, because an
// option is only an option if both of its states are measured: a fix that read
// the ampersand unconditionally passes everything above.
func TestWithoutTheOptionAReplacementAmpersandIsOrdinaryText(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"written down",
			`v=abc; printf "[%s]" "${v/b/[&]}"`,
			"[a[&]c]",
		},
		{
			"global",
			`v=abcabc; printf "[%s]" "${v//b/<&>}"`,
			"[a<&>ca<&>c]",
		},
		{
			"anchored",
			`v=abcabc; printf "[%s]" "${v/#a/<&>}"`,
			"[<&>bcabc]",
		},
		{
			"from an unquoted expansion",
			`v=abc; r='&'; printf "[%s]" "${v/b/$r}"`,
			"[a&c]",
		},
		{
			// The row that says the *escape* half moved with the reading and
			// not on its own: with the option on this is `a[&]c`.
			"a backslash an expansion brought keeps its backslash",
			`v=abc; r='[\&]'; printf "[%s]" "${v/b/$r}"`,
			`[a[\&]c]`,
		},
		{
			// And the row that says what did not move: this backslash is
			// gone before the replacement is read, in both states.
			"a written backslash is removed by quote removal either way",
			`v=abc; printf "[%s]" "${v/b/[\&]}"`,
			"[a[&]c]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src, notReadingTheAmpersand)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// The option moves while the shell runs, and what it switches is read at each
// substitution rather than once.
func TestTheReplacementAmpersandOptionIsReadAtEachSubstitution(t *testing.T) {
	src := `v=abc; printf "[%s]" "${v/b/[&]}"`
	on, _ := run(t, src, readingTheAmpersand)
	off, _ := run(t, src, notReadingTheAmpersand)
	if on == off {
		t.Fatalf("both states answered %q; the option changes nothing", on)
	}
	out, status := run(t, `v=abc; printf "[%s]" "${v/b/[&]}"`, func(r *Runner) {
		readingTheAmpersand(r)
		r.SetMatchOption(ReplacementAmpersandIsTheMatch, false)
	})
	if out != off {
		t.Errorf("after turning it back off, got %q, want %q", out, off)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
}

// Every MatchOption has a bit of its own in the set that stores them.
//
// The field was a uint8 holding exactly eight options, so the ninth shifted
// off the end and read as off however it was set — a dialect turning it on,
// the option's own builtin reporting it off, and the behavior never running.
// A constant in the package fails to compile if the list outgrows the field
// again; this is the other half, and it is the half that would notice two
// options sharing a bit.
func TestEveryMatchOptionIsStoredSeparately(t *testing.T) {
	all := []MatchOption{
		UnmatchedPatternIsEmpty,
		PatternsMatchHidden,
		GlobFoldsCase,
		MatchFoldsCase,
		StarStarCrossesDirectories,
		StarStarAloneCrossesDirectories,
		ExtendedPatternOperators,
		TrailingGroupIsPartOfThePattern,
		ReplacementAmpersandIsTheMatch,
	}
	for _, o := range all {
		// testrunner:bare — nothing here runs a script, so the runner needs
		// no directory and no cleanup: the subject is the bit the setter
		// writes and the getter reads.
		r := &Runner{}
		r.SetMatchOption(o, true)
		if !r.MatchOption(o) {
			t.Errorf("option %d does not read back as on", o)
		}
		for _, other := range all {
			if other == o {
				continue
			}
			if r.MatchOption(other) {
				t.Errorf("setting option %d also turned option %d on", o, other)
			}
		}
	}
}
