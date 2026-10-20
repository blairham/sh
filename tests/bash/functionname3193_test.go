// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The `function` keyword's name is any word written bare (#3193).
//
// Measured 2026-09-16 on bash 5.3.20 and bash 3.2.57, `eval "function $n {
// echo r; }"` a name at a time from a script file. The accepted set here was
// the identifier rule plus punctuation, which is narrower than this shell by
// `= [ { } ~ ? *` at least — and `a*b` is the row that says the name is never
// matched against the filesystem: it defines, and calling it runs the body.
func TestTheFunctionKeywordTakesAnyBareName(t *testing.T) {
	for _, name := range []string{
		"a=2", "f=", "[", "a~b", "a?b", "a*b", "a{b", "a}b", "x[y",
		// The ones that already defined, kept so a change that widened the
		// set by narrowing another is visible.
		"a!b", "a#b", "a-b", "a.b", "a@b", "a]b", "a^b", "a%b", "a,b", "a:b", "a/b", "a+b",
	} {
		src := "function " + name + " { echo ran; }\necho \"def=$?\"\n'" + name + "'"
		out, st := runBash(t, t.TempDir(), src)
		if st != 0 || out != "def=0\nran\n" {
			t.Errorf("%s: out %q status %d, want the definition and the call", name, out, st)
		}
	}
	// A word carrying quoting or an expansion is not one, and is refused as
	// it was written with the script carrying on — which is the reading this
	// shell already had and which the bare rule must not take away.
	for _, tc := range []struct{ name, quoted string }{
		{`'f'`, `'f'`},
		{`"f"`, `"f"`},
		{`\f`, `\f`},
		{`a\*b`, `a\*b`},
		{`a"b"c`, `a"b"c`},
		{`x$y`, `x$y`},
		{`x${y}`, `x${y}`},
		{`$x`, `$x`},
	} {
		src := "function " + tc.name + " { echo ran; }\necho \"def=$?\""
		out, st := runBash(t, t.TempDir(), src)
		if st != 0 || !strings.Contains(out, "`"+tc.quoted+"': not a valid identifier") ||
			!strings.HasSuffix(out, "def=1\n") {
			t.Errorf("%s: out %q status %d, want the refusal naming the word and 1", tc.name, out, st)
		}
	}
}
