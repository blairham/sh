// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `readonly` here takes POSIX's `-p` and nothing else. Measured 2026-09-12,
// ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin`:
//
//	readonly -p zz     status 0
//	readonly -a zz     readonly: -a: unknown option
//	                   Usage: readonly [-p] [name[=value]...]
//
// and `-A`, `-f` and `-n` are refused in the same words.
//
// It is worth a test of its own because of what the missing letter used to
// cost. The interpreter fixed `readonly`'s letters at `paAf`, so this shell
// accepted `-a` and then walked into
// Semantics.ReadonlyRecordsTheCompoundAttribute — an axis it has no answer
// for, and could not have one for, since the letter that raises the question
// does not exist here. The refusal a script saw was about a disagreement
// between shells rather than about the option it had written (#2277).
func TestReadonlyTakesOnlyThePosixLetter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the array letter", `readonly -a zz`, "readonly: -a: unknown option"},
		{"the table letter", `readonly -A zz`, "readonly: -A: unknown option"},
		{"the function letter", `readonly -f zz`, "readonly: -f: unknown option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{
				Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if !strings.Contains(out, tc.want) ||
				!strings.Contains(out, "Usage: readonly [-p] [name[=value]...]") {
				t.Errorf("got %q, want %q and this shell's usage line",
					strings.TrimSpace(out), tc.want)
			}
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("got %q — an axis this shell cannot be asked must be "+
					"closed at the option, not by choosing an answer for it", out)
			}
		})
	}
}
