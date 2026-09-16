// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// DeclarationCommandWord decides whether a declaration utility's operand is an
// assignment only when the utility's name was written as the command word, or
// whenever the utility runs.
//
// Each row reaches `export` a different way and splits `x y` only where the
// reading keyed on the written word cannot see a declaration. The literal row
// is the control: it keeps the value whole under both readings, so a runner
// that ignored the axis entirely would fail every other row under one of them.
func TestADeclarationIsRecognizedByTheWordAsTheAxisSays(t *testing.T) {
	rows := []struct {
		name, line string
		// splitWhenWritten is the value under DeclarationByUnquotedLiteralWord;
		// under DeclarationByUtilityName every row keeps it whole.
		splitWhenWritten bool
	}{
		{"the literal word", `export v=$b`, false},
		{"an expansion", `cmd=export; $cmd v=$b`, true},
		{"a backslash", `\export v=$b`, true},
		{"single quotes", `'export' v=$b`, true},
		{"double quotes", `"export" v=$b`, true},
		{"a quoted part", `ex'port' v=$b`, true},
		{"an empty expansion in front", `e=; $e export v=$b`, true},
	}
	for _, reading := range []DeclarationCommandWordReading{
		DeclarationByUtilityName, DeclarationByUnquotedLiteralWord,
	} {
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				sem := PosixSemantics()
				sem.DeclarationCommandWord = reading
				out, _ := run(t, "b='x y'\n"+row.line+"\n"+`echo "[$v]"`,
					func(r *Runner) { r.Semantics = &sem })
				want := "[x y]"
				if reading == DeclarationByUnquotedLiteralWord && row.splitWhenWritten {
					want = "[x]"
				}
				if got := strings.TrimSpace(out); got != want {
					t.Errorf("reading %d: %s wrote %q, want %q", reading, row.line, got, want)
				}
			})
		}
	}
}
