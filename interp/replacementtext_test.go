// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// replacementDir is the directory these rows are measured in. Every name in it
// is one a replacement's metacharacters would find if they were live — `Q` for
// a bracket expression and for `?`, and four longer names for `*` — which is
// the only way a row can tell "the text stood" from "the pattern happened to
// miss".
func replacementDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"Q", "axcd", "aQcd", "ax", "bx"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A replacement is **text**. Its metacharacters are not a pattern, and they
// never reach the filesystem on the replacement's own account.
//
// The first row is what the fault looked like: the replacement went through
// the same word expansion an ordinary word gets, so `*` listed the directory,
// the listing was joined with spaces, and the result was pushed into the middle
// of the value — at status 0, with nothing said. The bracket row is the loud
// half of the same fault, and it is loud only where an unmatched pattern is
// fatal, which is why the quiet one is written first (#1337).
//
// **Every expansion here is quoted**, and that is what makes the rows measure
// the operator. Quoted, nothing that comes out of the expansion is re-read as
// a pattern, so what prints is exactly what the operator produced.
//
// The obvious alternative does not work, and it is worth writing down because
// it reads like the stronger assertion: `y=${x//b/*}` and then printing `$y`
// cannot see this at all. An assignment's value is expanded with pathname
// expansion suspended for the whole word, nested operands included, so the
// replacement stops globbing there for a reason that has nothing to do with
// the replacement — every row written that way passes with the fault in place.
func TestAReplacementIsLiteralText(t *testing.T) {
	dir := replacementDir(t)
	for _, tc := range []struct{ src, want string }{
		{`x=abcd; printf "[%s]" "${x//b/*}"`, `[a*cd]`},
		{`x=abcd; printf "[%s]" "${x//b/[Q]}"`, `[a[Q]cd]`},
		{`x=abcd; printf "[%s]" "${x//b/?}"`, `[a?cd]`},
		{`x=abcd; printf "[%s]" "${x/b/*}"`, `[a*cd]`},
		{`x=abcd; printf "[%s]" "${x/#a/*}"`, `[*bcd]`},
		{`x=abcd; printf "[%s]" "${x/%d/*}"`, `[abc*]`},
		// The pattern half is a pattern, which is what makes the rows above
		// a statement about the *replacement* and not about the operator.
		{`x=abcd; printf "[%s]" "${x/b?/Z}"`, `[aZd]`},
		// And the replacement is still expanded — it is a word. What an
		// expansion produces is text, so `$r` contributes both its words and
		// the space between them as one field. Neither row can fail on the
		// fault above; they are here to say what the fix must not take away.
		{`x=abcd; r="p q"; printf "[%s]" "${x/b/$r}"`, `[ap qcd]`},
		{`x=abcd; printf "[%s]" "${x/b/$(echo m n)}"`, `[am ncd]`},
	} {
		out, st := run(t, tc.src, inDir(dir))
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// The elementwise path is the same rule, and it is a separate call site: an
// operator over a whole array expands its operands once and applies them to
// every element. A fix that reached only the scalar path would leave the
// listing being substituted here, which is the shape a second copy of a rule
// has produced in this tree before.
func TestAReplacementIsLiteralTextForEveryElement(t *testing.T) {
	dir := replacementDir(t)
	src := `a=(ab cb); b=("${a[@]//b/*}"); printf "[%s]" "${b[0]}" "${b[1]}"`
	out, st := run(t, src, inDir(dir))
	if got := strings.TrimSpace(out); got != `[a*][c*]` || st != 0 {
		t.Errorf("%s = %q (status %d), want [a*][c*] at 0", src, got, st)
	}
}
