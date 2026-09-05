// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A substring range is this shell's history-modifier syntax as well, so a
// segment beginning with an unquoted letter is a modifier rather than an
// arithmetic offset — and `i` names no modifier. Measured against zsh 5.9.2
// (2026-09-05); the other three take the substring and answer `cd`.
func TestARangeThatNamesAVariableIsRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abcdef; i=2; echo "[${x:i:2}]"; echo after`)
	if !strings.Contains(out, "unrecognized modifier `i'") {
		t.Errorf("got %q, want the modifier named", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the command abandoned at a failure", out, st)
	}
}

// The length is the same question, reached by a different call site.
func TestALengthThatNamesAVariableIsRefusedToo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abcdef; i=2; echo "[${x:2:i}]"; echo after`)
	if !strings.Contains(out, "unrecognized modifier `i'") || st == 0 {
		t.Errorf("got %q (status %d), want the modifier named at a failure", out, st)
	}
}

// The modifiers this dialect performs. The variables are set on purpose: with
// `h` unset the arithmetic reading would answer from offset 0 and the case
// would look like a disagreement about the whole string.
func TestTheModifiersThisShellPerforms(t *testing.T) {
	for _, c := range []struct{ mod, want string }{
		{"h", "/tmp/Dir"},
		{"t", "File.Txt"},
		{"r", "/tmp/Dir/File"},
		{"e", "Txt"},
		{"l", "/tmp/dir/file.txt"},
		{"u", "/TMP/DIR/FILE.TXT"},
		{"h:t", "Dir"},
	} {
		src := `x=/tmp/Dir/File.Txt; h=9; t=9; echo "[${x:` + c.mod + `}]"`
		out, st := runZsh(t, t.TempDir(), src)
		if got, want := strings.TrimSpace(out), "["+c.want+"]"; got != want || st != 0 {
			t.Errorf(":%s = %q (status %d), want %q", c.mod, got, st, want)
		}
	}
}

// An offset and then a modifier, which is what says the two readings are
// segments of one range rather than alternatives.
func TestAModifierMayFollowAnOffset(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=/tmp/Dir/File.Txt; t=3; echo "[${x:2:t}]"`)
	if got := strings.TrimSpace(out); got != "[File.Txt]" || st != 0 {
		t.Errorf("got %q (status %d), want %q", got, st, "[File.Txt]")
	}
}

// A segment that does not begin with a letter is a range here as it is
// everywhere else — an underscore, a leading space, a parenthesis, an
// expansion that already happened, and a quoted letter.
func TestARangeThatDoesNotBeginWithALetterIsAnOffset(t *testing.T) {
	for _, src := range []string{
		`x=abcdef; _q=1; echo "[${x:_q:2}]"`,
		`x=abcdef; i=1; echo "[${x: i:2}]"`,
		`x=abcdef; i=1; echo "[${x:(i):2}]"`,
		`x=abcdef; i=1; echo "[${x:$i:2}]"`,
		`x=abcdef; h=1; echo "[${x:"h":2}]"`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if got := strings.TrimSpace(out); got != "[bc]" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", src, got, st, "[bc]")
		}
	}
}

// A good modifier with something after it in the same segment is refused with
// nothing named, where an unknown letter is named. Two shapes of one sentence.
func TestALeftoverAfterAModifierIsRefusedWithoutAName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=/tmp/a.b; echo "[${x:ha}]"; echo after`)
	if !strings.Contains(out, "unrecognized modifier") || strings.Contains(out, "`") {
		t.Errorf("got %q, want the complaint with nothing named", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the command abandoned at a failure", out, st)
	}
}

// The modifiers this shell has and this implementation does not perform are
// refused out loud, not passed through. Recorded rather than reproduced: each
// needs the working directory, the disk, the command search or this shell's
// quoting table, and a value that was never computed must not be handed back
// as though it had been.
func TestAModifierThatIsNotPerformedIsRefused(t *testing.T) {
	for _, mod := range []string{"a", "A", "P", "c", "q", "Q", "s"} {
		out, st := runZsh(t, t.TempDir(), `x=/tmp/a.b; echo "[${x:`+mod+`}]"`)
		if !strings.Contains(out, "not implemented") || st == 0 {
			t.Errorf(":%s = %q (status %d), want a refusal saying so", mod, out, st)
		}
	}
}
