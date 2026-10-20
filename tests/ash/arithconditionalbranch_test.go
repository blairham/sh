// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// The two branches of a conditional are read at different levels here, and
// this column is the one that shows both ends of it at once: the then branch
// takes the sequence operator and the else branch will not begin with a bare
// store.
//
// Measured 2026-09-27 against BusyBox v1.37.0 in the panel's own image,
// `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`,
// with a `GOOS=linux GOARCH=arm64` build of `cmd/ash` beside it:
// `$(( 1 ? 2 , 3 : 4 ))` is 3, as it is in bash 5.3.20 and ksh93u+
// 2012-08-01 and unlike zsh 5.9.2; and `$(( 1 ? 2 : x = 3 ))` is `arithmetic
// syntax error` at 2, which is byte for byte what `$(( 7 = 4 ))` earns here
// (#4775, #4776).
//
// The comma-after-the-conditional row is the control that keeps the first
// finding off the operator itself, and the parenthesized row is what says the
// else branch is at a *level* rather than refusing stores.
func TestTheTwoConditionalBranchesAreReadAtDifferentLevels(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{"a comma in the then branch", `echo "$(( 1 ? 2 , 3 : 4 ))"`, "3", 0},
		{"a comma after the conditional", `echo "$(( 1 ? 2 : 3 , 4 ))"`, "4", 0},
		{"both", `echo "$(( 1 ? 2 , 3 : 4 , 5 ))"`, "5", 0},
		{"the sequence operator on its own", `echo "$(( 1 , 2 ))"`, "2", 0},
		{"a bare store in the then branch", `echo "$(( 1 ? x = 2 : 3 ))" "$x"`, "2 2", 0},
		{"a bare store in the else branch", `echo "$(( 1 ? 2 : y = 3 ))"`, "syntax error", 2},
		{"a bare store in the else branch, taken", `echo "$(( 0 ? 2 : y = 3 ))"`, "syntax error", 2},
		{"the same refusal with no conditional", `echo "$(( 7 = 4 ))"`, "syntax error", 2},
		{"parenthesized in the else", `echo "$(( 0 ? 2 : (w = 3) ))" "$w"`, "3 3", 0},
		{"an ordinary conditional", `echo "$(( 1 ? 2 : 3 ))"`, "2", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src)
			if !strings.Contains(out, tc.want) || st != tc.st {
				t.Errorf("%s gave %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
