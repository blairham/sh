// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestTheFunctionKeywordsParensAreAPair: `function name` takes the `()` pair
// and any body, or no pair and a compound body; a `(` after the name, even on
// the next line, is the pair's. Measured 2026-10-03 in the pinned image. See
// syntax.Dialect.FunctionKeywordParensAreAPair.
func TestTheFunctionKeywordsParensAreAPair(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"function a { echo B; }; a", "B\n"},
		{"function a\n{ echo B; }; a", "B\n"},
		{"function a() echo B; a", "B\n"},
		{"function a()\necho B; a", "B\n"},
		{"function a() ( echo B ); a", "B\n"},
		{"function a if true; then echo B; fi; a", "B\n"},
		{"function a for i in 1; do echo B; done; a", "B\n"},
		{"function a [[ 1 = 2 ]]; a; echo $?", "1\n"},
		{"function a-b=c { echo B; }; a-b=c", "B\n"},
	} {
		if out, _ := run(t, tc.src); out != tc.want {
			t.Errorf("%q\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
	// And the refusals, through eval so the shell words them.
	for _, tc := range []struct{ src, want string }{
		{"function a echo B", "syntax error: unexpected word\n"},
		{"function a\necho B", "syntax error: unexpected word\n"},
		{"function a ( echo B )", `syntax error: unexpected word (expecting ")")` + "\n"},
		{"function a\n( echo B )", `syntax error: unexpected word (expecting ")")` + "\n"},
		{"function a\n(( 1 ))", `syntax error: unexpected "(" (expecting ")")` + "\n"},
		{"function a zz\n{ echo hi; }", "syntax error: unexpected word\n"},
		{"function a=b\n{ echo hi; }", `syntax error: unexpected "{"` + "\n"},
		{"function a=b; echo after", `syntax error: unexpected ";"` + "\n"},
	} {
		if parses(t, tc.src) {
			t.Errorf("%q parses, want a refusal", tc.src)
		}
		out, _ := run(t, "eval '"+tc.src+"'; echo st=$?")
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("eval %q\n got %q\nwant a suffix %q", tc.src, out, tc.want)
		}
	}
}
