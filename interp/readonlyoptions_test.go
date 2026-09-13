// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `readonly`'s option letters come from the vector — see
// Semantics.ReadonlyOptions. Named for the axis and never for a shell.
//
// The letters were a literal in the interpreter until #2277, which is a
// defect with a particular shape worth keeping a test on: a builtin whose
// letters differ by shell and are fixed in the substrate does not merely
// accept too much, it makes an *axis* reachable in a shell that cannot be
// asked about it. Three columns took `readonly -a` and then walked into
// ReadonlyRecordsTheCompoundAttribute, an unanswered axis, and refused with a
// message about the disagreement — when the honest answer is that the letter
// does not exist there.
func TestReadonlyOptionLettersComeFromTheVector(t *testing.T) {
	for _, tc := range []struct {
		name, letters, src string
		want               func(out string) bool
		why                string
	}{
		{
			"the kind letter where the vector has it",
			"paAf",
			`readonly -a a; echo st=$?`,
			func(out string) bool { return strings.Contains(out, "st=0") },
			"the letter is in the set, so it is read and the name is frozen",
		},
		{
			"the kind letter where the vector does not",
			"p",
			`readonly -a a; echo st=$?`,
			func(out string) bool { return strings.Contains(out, "-a") && strings.Contains(out, "invalid option") },
			"the letter is not in the set, so it is refused before there is a " +
				"name to ask an axis about",
		},
		{
			"the function letter is its own question",
			"pa",
			`readonly -f zz; echo st=$?`,
			func(out string) bool { return strings.Contains(out, "-f") && strings.Contains(out, "invalid option") },
			"`-f` is in the set for two of the panel's shells and in no other, " +
				"so it is a letter of the set rather than something `-a` implies",
		},
		{
			"POSIX's own letter is always there",
			"p",
			`readonly r=1; readonly -p`,
			func(out string) bool { return strings.Contains(out, "r=") },
			"`-p` is the whole of what POSIX gives the builtin, so the " +
				"narrowest set still lists",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				s := permissive()
				s.ReadonlyOptions = tc.letters
				s.ReadonlyRecordsTheCompoundAttribute = Yes
				r.Semantics = &s
			})
			if !tc.want(out) {
				t.Errorf("with letters %q, `%s` wrote %q — %s",
					tc.letters, tc.src, out, tc.why)
			}
		})
	}
}

// An empty set is POSIX's `p` and not "no letters at all", which is the
// convention ReadOptions and UnsetOptions already follow. It matters because
// the zero value is what a Semantics literal built by hand carries, and a
// `readonly -p` refused there would be a listing that vanished for a reason
// nobody stated.
func TestReadonlyOptionsUnsetIsThePosixLetter(t *testing.T) {
	out, _ := run(t, `readonly r=1; readonly -p`, func(r *Runner) {
		s := permissive()
		s.ReadonlyOptions = ""
		r.Semantics = &s
	})
	if !strings.Contains(out, "r=") {
		t.Errorf("with no letters stated, `readonly -p` wrote %q, want the listing", out)
	}
}
