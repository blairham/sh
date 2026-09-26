// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `hash` handed a word with an `=` in it, and `hash -m` handed a pattern —
// the two ways one dialect's `hash` reaches its table that the rest of the
// panel has no spelling for (#4456, #4445).

// TestHashAndAnOperandWithAnEquals is the assignment form over the axis that
// decides it.
//
// The two readings are: an entry written straight into the table, so the name
// runs the value with no search; and an ordinary name to look up, which is
// what every other column does with the same word — and which then misses,
// because no PATH holds a name with an `=` in it.
//
// The *run* is the half a status cannot show. A shell that read the operand
// as a definition and then failed to record it would answer 0 here and leave
// the name unrunnable, which is exactly the silent success the loud side of
// this rule exists to avoid.
func TestHashAndAnOperandWithAnEquals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		defines Answer
		wantRun string
		wantSt  string
	}{
		{"an entry written into the table", Yes, "V1\n", "st=0"},
		{"a name to look up, and missed", No, "", "st=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := withDirs(t, `hash zzname=$PWD/d1/zzc; echo st=$?; zzname`,
				func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") },
				func(r *Runner) {
					s := testSemantics()
					s.HashDefinesAnEntryFromAnAssignment = tc.defines
					// Held fixed at the reading that *searches* for a slashed
					// operand rather than passing it over, so the row below
					// reaches the name lookup and the two rows differ by this
					// axis alone. Left at the passing-over reading, both rows
					// are a silent 0 and the axis reads as inert.
					s.HashIgnoresAnOperandWithASlash = No
					s.HashSearchesPathAlone = Yes
					s.HashReportsAMissingName = Yes
					r.Semantics = &s
				})
			if !strings.Contains(out, tc.wantSt+"\n") {
				t.Errorf("out=%q, want %s", out, tc.wantSt)
			}
			ran := strings.Contains(out, "V1\n")
			if ran != (tc.wantRun != "") {
				t.Errorf("out=%q, want the name to run: %v", out, tc.wantRun != "")
			}
		})
	}
}

// The name need not be the base name of the file it points at, and the value
// is taken as written: neither half is looked at, so a value that is not a
// path at all goes in and comes back out.
func TestHashTakesBothHalvesOfAnAssignmentAsWritten(t *testing.T) {
	out, st := run(t, `hash zzname=notapath; echo st=$?; hash`, func(r *Runner) {
		s := testSemantics()
		s.HashDefinesAnEntryFromAnAssignment = Yes
		r.Semantics = &s
	})
	if st != 0 || !strings.Contains(out, "st=0\n") {
		t.Errorf("out=%q st=%d, want a silent success", out, st)
	}
	if !strings.Contains(out, "notapath") {
		t.Errorf("out=%q, want the value in the table as written", out)
	}
}

// `hash -m` over the axis that decides whether the letter exists at all.
//
// Under Yes the operands are patterns and the answer is a set; under No the
// letter is not in the set this builtin takes and the call is a bad option,
// which is what it already was in every column but one.
func TestHashDashMReadsItsOperandsAsPatterns(t *testing.T) {
	for _, tc := range []struct {
		name     string
		patterns Answer
		wantA    bool
		wantB    bool
		wantBad  bool
	}{
		{"a pattern over the table", Yes, true, false, false},
		{"not a letter this dialect has", No, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := withDirs(t, `zza; zzb; hash -m "zza*"`,
				func(d1, _ string) {
					hashable(t, d1, "zza", "echo ranA")
					hashable(t, d1, "zzb", "echo ranB")
				},
				func(r *Runner) {
					s := testSemantics()
					s.HashReadsOperandsAsPatterns = tc.patterns
					r.Semantics = &s
				})
			// The listing lines, not the lines the two commands printed when
			// they ran — which is why the programs say `ranA` and the names
			// they are hashed under are what the listing writes.
			listing := strings.ReplaceAll(out, "ranA\n", "")
			listing = strings.ReplaceAll(listing, "ranB\n", "")
			if got := strings.Contains(listing, "zza"); got != tc.wantA {
				t.Errorf("listing=%q, want the matching entry: %v", listing, tc.wantA)
			}
			if got := strings.Contains(listing, "zzb"); got != tc.wantB {
				t.Errorf("listing=%q, want the entry the pattern missed: %v", listing, tc.wantB)
			}
			if got := strings.Contains(out, "-m"); got != tc.wantBad {
				t.Errorf("out=%q, want the letter refused: %v", out, tc.wantBad)
			}
		})
	}
}

// A pattern is a question about the table and not about a command, so one
// that reaches nothing is silence at 0 where a *name* that resolves to
// nothing is the dialect's complaint at 1. Both rows run on one shell, so the
// difference is the letter and not the vector.
func TestHashAPatternThatReachesNothingIsNotAMissingName(t *testing.T) {
	out, st := withDirs(t, `zza; hash -m "nothinglikethis*"; echo pat=$?; hash nosuchzz; echo name=$?`,
		func(d1, _ string) { hashable(t, d1, "zza", "echo ranA") },
		func(r *Runner) {
			s := testSemantics()
			s.HashReadsOperandsAsPatterns = Yes
			s.HashReportsAMissingName = Yes
			r.Semantics = &s
		})
	if !strings.Contains(out, "pat=0\n") || !strings.Contains(out, "name=1\n") {
		t.Errorf("out=%q st=%d, want the pattern silent at 0 and the name reported at 1", out, st)
	}
	if strings.Contains(out, "nothinglikethis") {
		t.Errorf("out=%q, want nothing said about the pattern", out)
	}
}

// `hash -m` with no operand at all writes **nothing**, which is where it
// parts from the bare listing: a shell that treated an empty operand list as
// "everything" would write the table here and agree with the reference on
// every other row.
func TestHashDashMWithNoOperandIsNotTheBareListing(t *testing.T) {
	out, _ := withDirs(t, `zza; hash -m; echo m=$?; hash; echo bare=$?`,
		func(d1, _ string) { hashable(t, d1, "zza", "echo ranA") },
		func(r *Runner) {
			s := testSemantics()
			s.HashReadsOperandsAsPatterns = Yes
			r.Semantics = &s
		})
	before, after, ok := strings.Cut(out, "m=0\n")
	if !ok {
		t.Fatalf("out=%q, want `hash -m` to report 0", out)
	}
	if strings.Contains(strings.ReplaceAll(before, "ranA\n", ""), "zza") {
		t.Errorf("out=%q, want `hash -m` to write nothing", out)
	}
	if !strings.Contains(after, "zza") {
		t.Errorf("out=%q, want the bare listing to write the entry", out)
	}
}

// An operand with an `=` is read as an assignment *before* the slash rule
// decides anything, because the slash that matters in `n=/bin/ls` is in the
// value. A shell that asked the other way round would pass the whole word
// over and record nothing, at 0 — which reads as success.
func TestHashReadsTheEqualsBeforeTheSlashRule(t *testing.T) {
	out, _ := withDirs(t, `hash zzname=$PWD/d1/zzc; echo st=$?; zzname`,
		func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") },
		func(r *Runner) {
			s := testSemantics()
			s.HashDefinesAnEntryFromAnAssignment = Yes
			// The reading that passes a slashed operand over in silence,
			// which is the one that would swallow this word.
			s.HashIgnoresAnOperandWithASlash = Yes
			r.Semantics = &s
		})
	if !strings.Contains(out, "st=0\n") || !strings.Contains(out, "V1\n") {
		t.Errorf("out=%q, want the entry made and the name runnable", out)
	}
}
