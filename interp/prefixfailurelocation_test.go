// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Who a value in an assignment prefix that will not expand is located as —
// Diagnostics.PrefixFailureIsTheBuiltins. The rule is keyed on whether the
// prefix is the builtin's **environment** or a store this shell keeps, and the
// eighteen measured rows are in interp/prefixfailurelocation.go. Named for the
// field and never for a shell.

// prefixFailureLocation runs one command with a failing prefix and hands back
// the location its complaint was written with.
func prefixFailureLocation(t *testing.T, command string, isTheBuiltins bool) string {
	t.Helper()
	sem := permissive()
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = Yes
	out, _ := sourceRun(t, t.TempDir(),
		"f() { :; }\na=$((1/0)) "+command+"\n", sem,
		Diagnostics{
			Location:                   LocationLineWord,
			BuiltinLocation:            LocationBracketLine,
			PrefixFailureIsTheBuiltins: isTheBuiltins,
		})
	line, _, _ := strings.Cut(out, "\n")
	prefix, _, ok := strings.Cut(line, "division by zero")
	if !ok {
		t.Fatalf("%q: no complaint about the division in %q", command, out)
	}
	return prefix
}

// The rule, both halves, at the answer that has it. A regular builtin's prefix
// is the environment that builtin is handed, so the builtin speaks; a special
// builtin's persists, so it is an ordinary assignment and this shell does.
func TestAFailedPrefixIsLocatedByWhoTheValueBelongsTo(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		command string
		want    string
	}{
		{"true", "testsh[2]: "},
		{":", "testsh: line 2: "},
		{"f", "testsh: line 2: "},
		{"/nonexistent/zz", "testsh: line 2: "},
	} {
		if got := prefixFailureLocation(t, row.command, true); got != row.want {
			t.Errorf("%q: located %q, want %q", row.command, got, row.want)
		}
	}
}

// And the other answer, which is every column but one: the shell's own
// location for all four, because a builtin never speaks for a prefix there.
// The regular-builtin row is the only one that moves between the two answers,
// which is what says the field is doing the work rather than the routes.
func TestAFailedPrefixCanAlwaysBeTheShellsOwn(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"true", ":", "f", "/nonexistent/zz"} {
		if got := prefixFailureLocation(t, command, false); got != "testsh: line 2: " {
			t.Errorf("%q: located %q, want the shell's own location", command, got)
		}
	}
}

// The control that keeps this off "an arithmetic failure past the first line":
// the same failure in an ordinary word, on the same line, under the same
// answer, is the shell's. A rule that shape would move this row too.
func TestAFailedWordIsNeverTheBuiltinsWhereverItStands(t *testing.T) {
	t.Parallel()
	sem := permissive()
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = Yes
	out, _ := sourceRun(t, t.TempDir(), "x=1\necho $((1/0))\n", sem,
		Diagnostics{
			Location:                   LocationLineWord,
			BuiltinLocation:            LocationBracketLine,
			PrefixFailureIsTheBuiltins: true,
		})
	if !strings.HasPrefix(out, "testsh: line 2: ") {
		t.Errorf("got %q, want the shell's own location", out)
	}
}
