// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A math error in `(( ))` is fatal here and a reporting statement everywhere
// else, which is the half ConditionArithmeticErrorIsFatal could not carry —
// zsh abandons the word-spelled condition and stays for the parenthesized
// form, so the two constructs do not group.
//
// Measured against ksh93u+ 2012-08-01, 2026-09-11.
func TestAMathErrorInAnArithmeticCommandAbandonsTheInput(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		// Both ways an expression can fail: one the arithmetic parser cannot
		// read at all, and one it reads and then cannot evaluate.
		{"will not parse", "echo one; (( 1+ )); echo two"},
		{"will not evaluate", "echo one; (( 1/0 )); echo two"},
		// And the construct standing as a condition, which is the same
		// question: it is about the expression and not about what the status
		// is read for.
		{"as a condition of &&", "echo one; (( 1+ )) && echo yes; echo two"},
		{"as an if condition", "echo one; if (( 1+ )); then echo yes; fi; echo two"},
	} {
		out, st := answersRun(t, tc.src)
		if !strings.Contains(out, "one") {
			t.Errorf("%s: got %q, want what came before it to have run", tc.name, out)
		}
		if strings.Contains(out, "two") || strings.Contains(out, "yes") {
			t.Errorf("%s: got %q, want nothing after it to have run", tc.name, out)
		}
		if st != 1 {
			t.Errorf("%s: status %d, want 1", tc.name, st)
		}
	}
}

// And it gives up a sourced file alone, which is this shell's ordinary reach
// for an error rather than anything of the construct's: `. ./s.sh; echo after`
// still prints `after`, for the parenthesized spelling and the word-spelled
// condition alike.
func TestAFatalMathErrorGivesUpTheSourcedFileAlone(t *testing.T) {
	for _, line := range []string{"(( 1+ ))", "[[ 1+ -eq 0 ]]"} {
		dir := t.TempDir()
		script := filepath.Join(dir, "s.sh")
		if err := os.WriteFile(script, []byte(line+"\necho insrc\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, _ := answersRun(t, ". "+script+"; echo after")
		if strings.Contains(out, "insrc") {
			t.Errorf("%s: got %q, want the rest of the file abandoned", line, out)
		}
		if !strings.Contains(out, "after") {
			t.Errorf("%s: got %q, want the caller to carry on past the file", line, out)
		}
	}
}
