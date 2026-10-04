// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// An arithmetic expansion closes only at a `))` where its own parentheses
// are balanced; a lone `)` there is text, and running out first is the
// unclosed `$((`. Measured 2026-10-04 on dash 0.5.12. The last row is the
// control: the lone `)` is text and the `))` behind it closes the expansion.
func TestAnArithmeticExpansionClosesOnlyAtTwoParens(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $((echo a) )`, "Syntax error: Missing '))'"},
		{`echo "[$((echo ab cde) )]"`, "Syntax error: Missing '))'"},
		{`echo "[$(( (1+2)) )]"`, "Syntax error: Missing '))'"},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Errorf("%s: parsed, want a refusal", tc.src)
			continue
		}
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.src, got, tc.want)
		}
	}
	if _, err := syntax.Parse(`echo $(( (1+2)) ))`, dash.Dialect()); err != nil {
		t.Errorf("a lone ) then a closing )): %v, want it to parse", err)
	}
}

// An empty expression is quoted back as written, blanks and all: `$(( ))` is
// `expecting primary: " "` in dash 0.5.12, measured 2026-10-04.
func TestAnEmptyArithmeticExpressionIsQuotedAsWritten(t *testing.T) {
	out, _ := answersRun(t, `echo $((  ))`)
	if want := `arithmetic expression: expecting primary: "  "`; !strings.Contains(out, want) {
		t.Errorf("= %q, want it to contain %q", out, want)
	}
}
