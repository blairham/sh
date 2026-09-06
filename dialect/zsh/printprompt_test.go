// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `print -P`, measured against zsh 5.9.2 (2026-09-06). It is the `${(%)…}`
// flag under another spelling and it runs the same expansion: what one of
// them carries the other carries, and what one refuses the other refuses in
// the same words.

// The order of the two passes, and that neither reads the other's result.
// Both directions are needed: one row alone is satisfied by doing the passes
// the wrong way round.
func TestPrintDashPRunsAfterTheBackslashEscapesAndNotOverThem(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -P '\045\045'
print -P '%%%%'
print -P 'a\tb'
print -rP 'a\tb%%'`)
	// `\045\045` is two backslash escapes making `%%`, which the prompt pass
	// then reads as one `%`. `%%%%` never meets the backslash pass at all and
	// is two `%%` making `%%` — the `%` that the first `%%` produced is not
	// looked at again.
	// `-r` suppresses the backslash pass and leaves this one: the tab stays
	// two characters and the `%%` still becomes one `%`. Without the `%%`
	// the line cannot tell a `-r` that suppresses both passes from one that
	// suppresses the right one.
	want := "%\n%%\na\tb\na\\tb%\n"
	if out != want || st != 0 {
		t.Errorf("print -P = %q (status %d), want %q", out, st, want)
	}
}

// `%` with nothing after it is dropped rather than written — in both
// spellings, because it is one expansion. It used to be written through here,
// which was the one place this said more than the shell it copies.
func TestATrailingPercentIsDroppedInBothSpellings(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -P 'x%'
v='x%'
print -r -- "[${(%)v}]"
u='%'
print -r -- "[${(%)u}]"`)
	want := "x\n[x]\n[]\n"
	if out != want || st != 0 {
		t.Errorf("a trailing percent = %q (status %d), want %q", out, st, want)
	}
}

// One expansion, so one refusal, in one wording — and the refusal names the
// escape rather than the word, which is what lets a reader tell which of
// several was the one this shell could not answer. The builtin route says the
// sentence alone because the location has already named `print`; the
// expansion route quotes the construct back, because `${(%)…}` can hold
// several words.
func TestBothSpellingsRefuseTheSameEscapeByName(t *testing.T) {
	// The builtin's refusal is a status the script goes on from, and the
	// expansion's ends the script — which is not this change's doing but the
	// expansion's own rule for a word it could not produce. Two runs rather
	// than one, so the second is reached at all.
	out, st := runZsh(t, t.TempDir(), `print -P '[%q]' 2>&1
print -r -- "print=$?"
print -r -- "after"`)
	want := "zsh:print:1: the %q prompt escape is not implemented\nprint=1\nafter\n"
	if out != want || st != 0 {
		t.Errorf("the builtin's refusal = %q (status %d), want %q", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `v='[%q]'
print -r -- "[${(%)v}]" 2>&1
print -r -- "after"`)
	want = "zsh:2: ${(%)v}: the %q prompt escape is not implemented\n"
	if out != want || st != 1 {
		t.Errorf("the expansion's refusal = %q (status %d), want %q with 1", out, st, want)
	}
}

// A refused escape writes *nothing*: a `print` that put out the operands it
// managed and then complained would leave a script holding a line it could
// not tell apart from a whole one.
func TestARefusedEscapeWritesNoPartialLine(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -lP 'first' 'second%q' 'third' 2>/dev/null
print -r -- "st=$?"`)
	want := "st=1\n"
	if out != want || st != 0 {
		t.Errorf("a refused operand = %q (status %d), want %q", out, st, want)
	}
}

// The escapes that are carried, in the spelling a script uses to find its own
// path. `%N` is the shell's own name under `-c`, where there is no file.
func TestPrintDashPCarriesTheEscapesTheExpansionCarries(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `print -P '%N'
v='%N'
print -P '%%'
print -r -- "[${(%)v}]"`)
	want := "zsh\n%\n[zsh]\n"
	if out != want || st != 0 {
		t.Errorf("the carried escapes = %q (status %d), want %q", out, st, want)
	}
}

// `-P` is no longer among the letters named as missing, and the ones that
// still are still are.
func TestPrintDashPIsNoLongerRefusedAsMissing(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `print -P 'x' 2>&1
print -D 'x' 2>&1
print -r -- "st=$?"`)
	if strings.Contains(out, "-P is not implemented yet") {
		t.Errorf("got %q, want -P implemented", out)
	}
	if !strings.Contains(out, "zsh:print:2: -D is not implemented yet") {
		t.Errorf("got %q, want -D still named as missing", out)
	}
}

// A builtin refusal leaves the next expansion's refusal fatal.
//
// Said plainly about what this does *not* prove: the expansion machinery's
// failure flag is reset at the start of every command — see the three flags
// cleared together in interp's command path — so a builtin that set it could
// not be caught here, and a mutant that sets it survives. The rule the code
// states is still the right one and the reason is not this test: nothing is
// being expanded, and the builtin's own status is the answer. What this
// pins is the behavior a reader would want to check first, which is that
// one refusal does not disarm the next.
func TestTheBuiltinsRefusalDoesNotDisarmTheNextExpansionFailure(t *testing.T) {
	// The second refusal is written before the redirection on its own line
	// takes effect — a word is expanded before the command it belongs to is
	// set up — so it stands in the output. What this asserts is the line
	// *after* it, which must not be reached.
	out, st := runZsh(t, t.TempDir(), `print -P '[%q]' 2>/dev/null
v='%q'
print -r -- "[${(%)v}]"
print -r -- "reached"`)
	want := "zsh:3: ${(%)v}: the %q prompt escape is not implemented\n"
	if out != want || st != 1 {
		t.Errorf("after a builtin refusal = %q (status %d), want %q with 1", out, st, want)
	}
}

// Each operand is expanded on its own, the way the backslash pass already is.
func TestPrintDashPExpandsEachOperandSeparately(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -lP '%%' '%N' '%%'`)
	want := "%\nzsh\n%\n"
	if out != want || st != 0 {
		t.Errorf("print -lP = %q (status %d), want %q", out, st, want)
	}
}
