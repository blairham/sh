// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A prefix that is a *chain* of the borrowed texts the shell is inside rather
// than one name and one line — see Diagnostics.BorrowedTextRendersTheCallStack
// for the six measured rows it is read off. This names the field and no shell;
// what one dialect writes is asserted in dialect/ksh.

func chainDiagnostics(on bool) Diagnostics {
	return Diagnostics{
		Location: LocationLineWord,
		// Deliberately *not* LocationNamesTheCurrentFile: the chain's
		// outermost component is the name the diagnostic would have carried
		// on its own, and a dialect that renames it after the innermost file
		// would be answering the same question twice. No dialect combines
		// the two, and the one that renders a chain names the shell.
		SourceFileIsTheBuiltin:          true,
		SourceFileNaming:                SourceBeforeLocation,
		BorrowedTextRendersTheCallStack: on,
	}
}

// TestTheChainIsOffByDefault: the zero value writes one name and one line, as
// it always did. It matters that this is asserted rather than assumed — the
// neighboring SourceNaming enum has a *shell's* answer as its zero value, and
// #2463 records what that cost.
func TestTheChainIsOffByDefault(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", "echo one\necho \"${NOPE?gone}\"\n")
	out, _ := sourceRun(t, dir, ". ./p.sh\n", permissive(), chainDiagnostics(false))
	if strings.Contains(out, "[") {
		t.Errorf("out = %q, want no chain in it — the field is off", out)
	}
	if want := "testsh: line 2: "; !strings.Contains(out, want) {
		t.Errorf("out = %q, want the ordinary prefix %q", out, want)
	}
}

// TestTheChainNamesEveryBorrowedText: with it on, each text the shell is
// inside is a component carrying the line in it that entered the next, and
// the innermost carries the location instead.
func TestTheChainNamesEveryBorrowedText(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.sh", "echo one\necho \"${NOPE?gone}\"\n")
	write(t, dir, "s.sh", "echo s1\n. ./p.sh\n")
	// The outermost component carries a bracket here because this vector
	// names a line for the top level. The shell that renders a chain does
	// not, for a `-c` program — `ksh -c '. ./p.sh'` is `ksh: .: line 3:`,
	// with no bracket — which is that route's location showing through
	// rather than a second rule; see dialect/ksh.
	for _, tc := range []struct{ name, src, want string }{
		{"one text", ". ./p.sh\n", "testsh[1]: .: line 2: "},
		{"a text inside a text", ". ./s.sh\n", "testsh[1]: .[2]: .: line 2: "},
		// A function frame is not a component: the bracket is the line the
		// `.` was written on, which here is inside the body.
		{"a function is not a component", "f() { . ./p.sh; }\nf\n", "testsh[1]: .: line 2: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := sourceRun(t, dir, tc.src, permissive(), chainDiagnostics(true))
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want %q in it", out, tc.want)
			}
		})
	}
}
