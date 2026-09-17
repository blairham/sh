// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// runWithDiagnostics runs a snippet with the wording under the test's
// control, keeping the two streams apart: every question here is about what
// the shell said and whether the rest of the script ran.
func runWithDiagnostics(t *testing.T, diag Diagnostics, src string) (out, errOut string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Diagnostics: &diag})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

// A `break` or `continue` whose count the shell will not read is refused and
// the script ends, which every column in the panel does and ours did in none
// of them (#2800).
//
// The wordings are named by what they *do* rather than by which shell writes
// them, and each vector below is a value some dialect holds: one sentence for
// any unreadable count, a separate one for a number that is not positive, and
// a sentence that quotes the number rather than the word.
func TestACountLoopControlWillNotRead(t *testing.T) {
	oneSentence := Diagnostics{
		LoopControlCount: "%[1]s: unreadable count: %[2]s",
	}
	fromNumericArgument := Diagnostics{
		NumericArgument: "%[1]s: not a number: %[2]s",
	}
	partingTheTwo := Diagnostics{
		LoopControlCount:           "%[1]s: unreadable count: %[2]s",
		LoopControlCountOutOfRange: "%[1]s: %[2]s: out of range",
	}
	namingTheNumber := Diagnostics{
		LoopControlCount:               "%[1]s: not positive: %[2]s",
		LoopControlCountNamesTheNumber: true,
		LoopControlCountStatus:         1,
	}

	for _, tc := range []struct {
		name    string
		diag    Diagnostics
		src     string
		wantErr string
		wantOut string
		wantSt  int
		why     string
	}{
		{
			"a word that is no number",
			oneSentence,
			`for i in 1 2; do break abc; echo tail; done; echo after`,
			"break: unreadable count: abc", "", 2,
			"neither the rest of the body nor the line after the loop runs",
		},
		{
			"the other builtin, one reader",
			oneSentence,
			`for i in 1 2; do continue abc; done; echo after`,
			"continue: unreadable count: abc", "", 2,
			"the same sentence with the other name in it",
		},
		{
			"the sentence a dialect already had for a bad number",
			fromNumericArgument,
			`for i in 1 2; do break abc; done; echo after`,
			"break: not a number: abc", "", 2,
			"an empty LoopControlCount falls back to NumericArgument, which is three columns' answer",
		},
		{
			"a number that is not positive, said the same way",
			oneSentence,
			`for i in 1 2; do break 0; echo tail; done; echo after`,
			"break: unreadable count: 0", "", 2,
			"with no separate sentence the dialect ends the script for this too",
		},
		{
			"a number that is not positive, parted",
			partingTheTwo,
			`for i in 1 2; do break 0; echo tail; done; echo after`,
			"break: 0: out of range", "after\n", 0,
			"the second sentence and the surviving script are one value: the loop still ends and `tail` never runs",
		},
		{
			"a count that is not positive ends every loop it can reach",
			partingTheTwo,
			`for i in 1 2; do for j in a b; do break 0; echo tail; done; echo mid; done; echo "st=$?"`,
			"break: 0: out of range", "st=1\n", 0,
			"not the count taken as one: neither `mid` nor a second pass of the outer loop runs, and the loops end at status 1",
		},
		{
			"the other builtin ends them too",
			partingTheTwo,
			`for i in 1 2 3; do echo "top$i"; continue 0; echo tail; done; echo "st=$?"`,
			"continue: 0: out of range", "top1\nst=1\n", 0,
			"a refused `continue` is not a `continue`: the complaint is made once and the loop does not go on to a second pass",
		},
		{
			"and the status is the builtin's own",
			partingTheTwo,
			`for i in 1 2; do break 0 && echo and; echo body; done; echo "st=$?"`,
			"break: 0: out of range", "st=1\n", 0,
			"the control is taken even though the builtin failed, so nothing behind it on the list runs",
		},
		{
			"a negative count, parted",
			partingTheTwo,
			`for i in 1 2; do break -1; done; echo after`,
			"break: -1: out of range", "after\n", 0,
			"the same half of the split, reached from the other end of the range",
		},
		{
			"the number rather than the word",
			namingTheNumber,
			`for i in 1 2; do break abc; done; echo after`,
			"break: not positive: 0", "", 1,
			"a word that is no number reads as nought, which is what the one dialect that quotes the number writes",
		},
		{
			"a negative count, named as the number it is",
			namingTheNumber,
			`for i in 1 2; do break -1; done; echo after`,
			"break: not positive: -1", "", 1,
			"the number is the operand's own here, so this is not a nought for everything",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, st := runWithDiagnostics(t, tc.diag, tc.src)
			if !strings.Contains(errOut, tc.wantErr) {
				t.Errorf("stderr %q, want it to contain %q — %s", errOut, tc.wantErr, tc.why)
			}
			if out != tc.wantOut {
				t.Errorf("stdout %q, want %q — %s", out, tc.wantOut, tc.why)
			}
			if st != tc.wantSt {
				t.Errorf("status %d, want %d — %s", st, tc.wantSt, tc.why)
			}
		})
	}
}

// The control the rule above needs: a count the shell *can* read is still
// taken, and a `break` with none still leaves one loop.
func TestACountLoopControlDoesRead(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no count at all", `for i in 1 2; do break; done; echo after`, "after\n"},
		{"a count of one", `for i in 1 2; do break 1; done; echo after`, "after\n"},
		{"a count of two", `for i in 1; do for j in 1; do break 2; done; echo inner; done; echo after`, "after\n"},
		{"leading zeros", `for i in 1 2; do break 01; done; echo after`, "after\n"},
		{"blanks around it", `for i in 1 2; do break " 1 "; done; echo after`, "after\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, st := runWithDiagnostics(t, Diagnostics{
				LoopControlCount: "%[1]s: unreadable count: %[2]s",
			}, tc.src)
			if out != tc.want || st != 0 || errOut != "" {
				t.Errorf("got %q / %q at %d, want %q at 0 with nothing said", out, errOut, st, tc.want)
			}
		})
	}
}
