// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `return` reads its operand before it judges the place, and reports the
// operand's complaint first.
//
// Two axes decide what comes out, and the three rows below are every
// combination that produces anything. ReturnOutsideAFunctionIsRefused says
// the place draws a complaint at all; BadOptionToSpecialBuiltinFatal says a
// refused operand ends the script, which is what keeps the place from ever
// being judged. Ordering the other way round — the place first — left the
// operand unread wherever both applied, and the status is 2 either way, so
// nothing graded on status could see it (#2762).
func TestAReturnOperandIsJudgedBeforeThePlace(t *testing.T) {
	for _, tc := range []struct {
		name         string
		refusesPlace Answer
		fatalOperand Answer
		want         []string
		absent       string
	}{
		{
			// Both complaints, the operand's first.
			name: "reported and not fatal", refusesPlace: Yes, fatalOperand: No,
			want: []string{"operand: abc", "nowhere to return to", "ret=2"},
		},
		{
			// The operand ends the script, so nothing judges the place.
			name: "reported and fatal", refusesPlace: Yes, fatalOperand: Yes,
			want: []string{"operand: abc", "ret=2"}, absent: "nowhere to return to",
		},
		{
			// A dialect that lets a `return` outside a function stand still
			// refuses the word it cannot read.
			name: "the place is not refused", refusesPlace: No, fatalOperand: No,
			want: []string{"operand: abc", "ret=2"}, absent: "nowhere to return to",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "( return abc )\necho \"ret=$?\"\n"
			f, err := syntax.Parse(src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.StatusArgument = StatusArgNumeric
			sem.ReturnOutsideAFunctionIsRefused = tc.refusesPlace
			sem.BadOptionToSpecialBuiltinFatal = tc.fatalOperand
			diag := Diagnostics{
				NumericArgument:        "operand: %[2]s",
				ReturnOutsideAFunction: "nowhere to return to",
			}
			var buf bytes.Buffer
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf,
				Semantics: &sem, Diagnostics: &diag, Name: "mysh",
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			at := -1
			for _, w := range tc.want {
				i := strings.Index(got, w)
				if i < 0 {
					t.Fatalf("said %q, want %q in it", got, w)
				}
				if i < at {
					t.Errorf("said %q, want %q after what came before it", got, w)
				}
				at = i
			}
			if tc.absent != "" && strings.Contains(got, tc.absent) {
				t.Errorf("said %q, want nothing about %q", got, tc.absent)
			}
		})
	}
}
