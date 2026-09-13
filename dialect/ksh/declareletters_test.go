// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// Two letters this shell spells the way zsh spells them and means something
// else entirely by. Measured against ksh93u+ 2012-08-01 on 2026-09-13;
// interp/declaremove.go and interp/declaremapping.go hold the readings and
// the dialect-neutral rows.

// `-m` moves a parameter here. The name of the row is the whole point: in the
// other shell with the letter the same line lists every parameter whose name
// matches the pattern `qa`.
func TestTheMLetterMovesAParameter(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `qa=1
typeset -m qb=qa
echo "[${qa-UNSET}][${qb-UNSET}]"`)
	if want := "[UNSET][1]\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// An operand with no `=` reads the source out of the named parameter's
// *value*, so a value that is not an identifier is what gets blamed — and a
// based integer is blamed as the script would see it rather than as the
// number underneath.
func TestTheMLetterBlamesTheValueOfABareOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a plain value", "qa=1\ntypeset -m qa", "typeset: 1: invalid variable name"},
		{"a based integer", "typeset -i16 qa=255\ntypeset -m qa", "typeset: 16#ff: invalid variable name"},
		{"a pattern, which is no name either", "qa=1\ntypeset -m 'q*'", "typeset: q*: invalid variable name"},
		{"a bad destination keeps its whole operand", "qa=1\ntypeset -m 'q*'=9", "typeset: q*=9: invalid variable name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src+"\necho after")
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if strings.Contains(out, "after") || st != 1 {
				t.Errorf("got %q (status %d), want the script ended with 1", out, st)
			}
		})
	}
}

// Any other attribute letter refuses the line with the usage block alone, and
// fatally — `typeset` is one of this shell's special builtins. `integer -m` is
// the same refusal arriving by a second route, since that word is `typeset -i`
// and the `i` is the letter that conflicts.
func TestTheMLetterTakesNoAttributeLetterBesideIt(t *testing.T) {
	for _, src := range []string{"typeset -mx 'q*'", "typeset -m -x 'q*'", "typeset -fm 'q*'", "integer -m qb=qa"} {
		out, st := runKsh(t, t.TempDir(), "qa=1\n"+src+"\necho after")
		if !strings.Contains(out, "Usage: typeset [-bflmnprstuxACHS]") {
			t.Errorf("%s: got %q, want the usage block", src, out)
		}
		if strings.Contains(out, "unknown option") || strings.Contains(out, "not implemented") {
			t.Errorf("%s: got %q, want no complaint above the usage block", src, out)
		}
		if strings.Contains(out, "after") || st != 2 {
			t.Errorf("%s: got %q (status %d), want the script ended with 2", src, out, st)
		}
	}
	// The control: `-p` is the one letter that neither conflicts nor acts.
	out, st := runKsh(t, t.TempDir(), "qa=1\ntypeset -pm 'q*'\necho after")
	if want := "after\n"; out != want || st != 0 {
		t.Errorf("typeset -pm: got %q (status %d), want %q at 0", out, st, want)
	}
}

// `-M` names a character mapping here, where zsh's `functions -M` registers a
// math function. There are two mappings and each lists back as the attribute
// letter it stands for.
func TestTheBigMLetterNamesACharacterMapping(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `v=AbC
typeset -M tolower v
echo "[$v]"
typeset -p v`)
	if want := "[abc]\ntypeset -l v=abc\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// A name that is not one of the two is refused and named — which is what the
// corpus rows written for the *other* shell's facility land on, since
// `functions -M mf 1 1 g` here is `typeset -f -M mf …` and `mf` is read as
// the mapping.
func TestAnUnknownMappingIsNamedEvenOnAFunctionLine(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `g() { :; }
functions -M mf 1 1 g
echo after`)
	if !strings.Contains(out, "typeset: mf: unknown mapping name") {
		t.Errorf("got %q, want the mapping named", out)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("got %q (status %d), want the script ended with 1", out, st)
	}
}

// And a `--` puts the operands out of the letter's reach, which is the
// spelling a plugin loader written for the other shell uses: with no mapping
// to name, a `-f` line is the usage block and a plain one says what is
// missing.
func TestADoubleDashLeavesTheMappingLetterWithNoName(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "g() { :; }\nfunctions -M -- mf 1 1 g\necho after")
	if !strings.Contains(out, "Usage: typeset [-bflmnprstuxACHS]") || strings.Contains(out, "after") || st != 2 {
		t.Errorf("got %q (status %d), want the usage block and the script ended with 2", out, st)
	}
	out, st = runKsh(t, t.TempDir(), "typeset -M -- mf 1 1 g\necho after")
	if !strings.Contains(out, "typeset: -M requires argument when operands are specified") ||
		strings.Contains(out, "after") || st != 1 {
		t.Errorf("got %q (status %d), want the missing argument named and the script ended with 1", out, st)
	}
}
