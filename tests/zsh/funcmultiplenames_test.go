// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `function clipcopy clippaste { … }` — one body defining several functions,
// run rather than only parsed.
//
// Behavioral because parsing is not the promise. The point of the construct
// is that `$0` inside the body is the name that was *called*, so a reading
// that defined one function and aliased the rest to it would parse perfectly
// and answer every call with the same name (#1680).
func TestOneBodyDefinesEveryNameAndKnowsWhichOneRan(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`function a b { echo "$0"; }; a; b`, "a\nb"},
		{`function f1 f2 f3 { echo "$0"; }; f3; f1; f2`, "f3\nf1\nf2"},
		// The arguments are the call's own, which is the other half of what
		// makes each name a function rather than a label on one.
		{`function a b { echo "$0 $#"; }; a x; b x y z`, "a 1\nb 3"},
		// One body, so a name removed leaves the others standing.
		{`function a b { echo "$0"; }; unfunction a; b; print -rl -- ${(ko)functions}`, "b\nb"},
		// A name already defined is replaced, and so is every other name in
		// the list — the definition is not a merge.
		{`a(){ echo old; }; function a b { echo "new $0"; }; a; b`, "new a\nnew b"},
		// The same name twice is one function and not an error.
		{`function a a { echo "$0"; }; a; print -rl -- ${(ko)functions}`, "a\na"},
		// The listing writes each name back as its own definition, in the
		// POSIX spelling this shell lists everything in.
		{`function a b { echo hi; }; functions b`, "b () {\n\techo hi\n}"},
		// The continuation spelling an Oh-My-Zsh library is written in.
		{"function man \\\n  dman \\\n  debman {\n  echo \"colored $0\"\n}\nman; debman", "colored man\ncolored debman"},
		// The hybrid parens close the list rather than being part of the
		// last name.
		{`function a b() { echo "$0"; }; a; b`, "a\nb"},
		{`function a b () { echo "$0"; }; a; b`, "a\nb"},
		// A name may hold an expansion here, in the list as in the front of
		// it, and it is fixed at the definition.
		{`w=x; function p_$w q_$w { echo "$0"; }; p_x; q_x`, "p_x\nq_x"},
		// A quoted name is a name whatever is in it, in the list too.
		{`function a "b c" d { echo "[$0]"; }; "b c"; d`, "[b c]\n[d]"},
		// A redirection on the definition belongs to the body, so every name
		// carries it — measured, and the row that says the names share one
		// body rather than each getting a copy of the brace group alone.
		{`function a b { echo "$0"; } >out; a; b; cat out`, "b"},
		// A reserved word after a name is a name, and the table is what says
		// so: `while` is defined here. Listed rather than called, because
		// `while` in command position is still the keyword and a call to it
		// would wait for a body — which is what the shell being modeled does
		// too, so the row would hang rather than fail.
		{`function a while { echo "$0"; }; print -rl -- ${(ko)functions}`, "a\nwhile"},
		// And one name is still one name, which is the control.
		{`function a { echo "$0"; }; a`, "a"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The construct the issue was found on, whole: an Oh-My-Zsh clipboard library
// ends with two names over one body, and `$0` is how the body knows which of
// them the script called. Refused before, taking the file with it.
func TestTheClipboardShapeDefinesBothAndDispatchesOnDollarZero(t *testing.T) {
	src := `detect-clipboard() { echo detected; }
function clipcopy clippaste {
  unfunction clipcopy clippaste
  detect-clipboard || true
  echo "was $0"
}
clippaste
print -rl -- ${(ko)functions}
`
	out, st := answersRun(t, src)
	want := "detected\nwas clippaste\ndetect-clipboard\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
