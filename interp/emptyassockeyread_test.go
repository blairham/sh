// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Reading a key that came out empty is *reported* in one column and silent in
// two, and every column answers the same empty string at the same status.
//
// The other face of EmptyAssociativeKeyIsAnError, which is the store: that one
// refuses, names the subscript as it was written and reports 1, where this one
// names the table and lets the read finish. One column does both and words
// them differently, which is what says they are two questions (#1972).
func TestAnEmptyKeyReadMayBeReported(t *testing.T) {
	const src = `typeset -A m; m[k]=v; w=; echo "[${m[$w]}]"; echo after=$?`

	for _, tc := range []struct {
		name     string
		answer   Answer
		reported bool
	}{
		{"reported", Yes, true},
		{"silent", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyIsReportedWhenRead = tc.answer
			out, st := run(t, src, withSem(sem))
			// The value and the status are the same under both answers,
			// which is the whole of why this is a report and not a value.
			if !strings.Contains(out, "[]\nafter=0\n") {
				t.Errorf("%s = %q, want the empty value at status 0", src, out)
			}
			if st != 0 {
				t.Errorf("%s: status %d, want 0", src, st)
			}
			if said := strings.Contains(out, "bad array subscript"); said != tc.reported {
				t.Errorf("%s = %q, want it %sreported", src, out,
					map[bool]string{true: "", false: "not "}[tc.reported])
			}
		})
	}
}

// The subject is the **name** and not the subscript, which is the whole
// difference in wording from the store's refusal one branch over.
func TestAnEmptyKeyReadNamesTheTable(t *testing.T) {
	sem := testSemantics()
	sem.EmptyAssociativeKeyIsReportedWhenRead = Yes
	const src = `typeset -A m; m[k]=v; w=; echo "[${m[$w]}]"`
	out, _ := run(t, src, withSem(sem))
	if !strings.Contains(out, "m: bad array subscript") {
		t.Errorf("%s = %q, want the table named", src, out)
	}
	if strings.Contains(out, "m[") {
		t.Errorf("%s = %q, want no subscript in the subject", src, out)
	}
}

// Once per read, and the word is finished rather than abandoned: nothing here
// sets the failed-expansion flag.
func TestAnEmptyKeyReadIsReportedPerReadAndCarriesOn(t *testing.T) {
	sem := testSemantics()
	sem.EmptyAssociativeKeyIsReportedWhenRead = Yes
	const src = `typeset -A m; m[k]=v; w=; echo "[${m[$w]}]${m[$w]}"; echo after`
	out, st := run(t, src, withSem(sem))
	if n := strings.Count(out, "bad array subscript"); n != 2 {
		t.Errorf("%s = %q, want the sentence twice, got %d", src, out, n)
	}
	if !strings.Contains(out, "[]\n") || !strings.Contains(out, "after") || st != 0 {
		t.Errorf("%s = %q (status %d), want the word finished and the script running on", src, out, st)
	}
}

// Where the axis is asked, and where the same emptiness means something else.
func TestTheEmptyKeyReadAxisIsAskedOnlyOverAnEmptyKey(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		asked     bool
	}{
		{"an empty key through a parameter", `typeset -A m; m[k]=v; w=; echo "[${m[$w]}]"`, true},
		// A blank key is a key: the control that says this is emptiness and
		// not whitespace.
		{"a blank key", `typeset -A m; m[k]=v; echo "[${m[ ]}]"`, false},
		{"an ordinary key", `typeset -A m; m[k]=v; echo "[${m[k]}]"`, false},
		// An indexed name reads its subscript as an expression, and the
		// emptiness it can object to is a different axis again.
		{"an indexed name", `a=(1 2); w=; echo "[${a[$w]}]"`, false},
		// The whole-array spellings are not keys at all.
		{"the whole table", `typeset -A m; m[k]=v; echo "[${m[@]}]"`, false},
		// And storing under one is the neighbouring question.
		{"storing under an empty key", `typeset -A m; w=; m[$w]=4`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyIsAnError = No
			sem.EmptyAssociativeKeyIsReportedWhenRead = Unspecified
			out, _ := run(t, tc.src, withSem(sem))
			said := strings.Contains(out, "key on a keyed table came out empty")
			if said != tc.asked {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "asked", false: "not asked"}[tc.asked])
			}
		})
	}
}
