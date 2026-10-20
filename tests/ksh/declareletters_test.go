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

// `-T` names a **type** here, where zsh's ties a scalar to an array. The row
// that separates the two readings is the one a script written for that shell
// writes: the two names never meet.
func TestTheBigTLetterNamesATypeRatherThanTyingTwoNames(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -T TS ts\nTS=a:b:c\necho \"n=${#ts[@]} [${ts[*]}]\"")
	if want := "n=0 []\n"; out != want || st != 0 {
		t.Errorf("writing the scalar: got %q (status %d), want %q at 0", out, st, want)
	}
	// And the other way about, in a shell of its own so that neither run can
	// be answered by what the other left behind.
	out, st = runKsh(t, t.TempDir(), "typeset -T TS ts\nts=(x y z)\necho \"TS=[$TS]\"")
	if want := "TS=[]\n"; out != want || st != 0 {
		t.Errorf("writing the array: got %q (status %d), want %q at 0", out, st, want)
	}
}

// The first operand is the type and is remembered by name; the rest are
// variables of it and are not created at all, so a bare `-T` writes back one
// line for a three-operand declaration. Both signs list, and a type name is
// not a parameter.
func TestATypeNameIsRememberedAndIsNotAParameter(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `typeset -T a b c
typeset -T
typeset +T
echo "[${a-UNSET}]"
typeset -p a`)
	if want := "typeset -T a\ntypeset -T a\n[UNSET]\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The name may ride on the letter, which is what makes the rest of the option
// word a name and not more letters: `-Tl` is a type called `l` and not the
// lower-case attribute.
func TestATypeNameRidesOnTheLetter(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -TPt v\ntypeset -Tl TS ts\ntypeset -T")
	if want := "typeset -T Pt\ntypeset -T l\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// So a second letter can only arrive in a word of its own or in front of the
// `T`, and either way the line is the usage block alone and fatal. `integer
// -T` is the same refusal by a second route: that word is `typeset -li` here.
func TestTheBigTLetterTakesNoOtherLetterBesideIt(t *testing.T) {
	for _, src := range []string{
		"typeset -T -l TS ts", "typeset -l -T TS ts", "typeset -xT TS ts",
		// `integer` is `typeset -li` here — a prelude alias rather than a
		// word of its own — so the line a script writes as `integer -T` is
		// this one, and it is refused for the two letters in front of the
		// `T` rather than for anything about the second name.
		"typeset -li -T TS ts",
	} {
		out, st := runKsh(t, t.TempDir(), src+"\necho after")
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
	// The control, and the same one the move letter has: `-p` rides along.
	out, st := runKsh(t, t.TempDir(), "typeset -pT\necho after")
	if want := "after\n"; out != want || st != 0 {
		t.Errorf("typeset -pT: got %q (status %d), want %q at 0", out, st, want)
	}
}

// The two refusals the operands can earn, which are worded differently
// because the first operand is a type and the rest are variables — and a
// value on any of them is the third, since a type is defined by a compound
// assignment and by nothing else.
func TestTheBigTLetterBlamesTheTypeAndItsVariablesDifferently(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bad type name", "typeset -T ':' ts", "typeset: .sh.type.:: no parent"},
		{"a bad variable name", "typeset -T TS ts ':'", "typeset: :: invalid variable name"},
		{"a bad variable under an attached type", "typeset -TPt ':'", "typeset: :: invalid variable name"},
		{"a value on the type", "typeset -T TS=1 ts", "TS: type definition requires compound assignment"},
		{"a value on a variable", "typeset -T TS ts=1 tt=2", "ts: type definition requires compound assignment"},
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

// `-H` is the fourth letter of that shape, and the one whose two readings
// agree about the value and part at every listing: here it is an inert
// attribute that is said back as a letter, where zsh's withholds the value
// and never writes the letter at all. Measured 2026-09-13 on ksh93u+.
func TestTheBigHLetterIsAnInertAttributeRatherThanAHiddenValue(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -H h=hid\necho \"[$h]\"\ntypeset -p h")
	if want := "[hid]\ntypeset -H h=hid\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
	// And the listing a bare `set` writes, which is the row the other shell
	// answers with a bare name: this one writes the value like any other.
	out, st = runKsh(t, t.TempDir(), "typeset -H zzh=hid\nset")
	if want := "zzh=hid\n"; !strings.Contains(out, want) || st != 0 {
		t.Errorf("bare set: got %q (status %d), want a line %q at 0", out, st, want)
	}
}

// The attribute is the name's, outlives a write, comes off with the plus form
// and goes away with the name.
func TestTheBigHAttributeFollowsTheNameAndNotTheValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a later write keeps it", "typeset -H h=1\nh=2\ntypeset -p h", "typeset -H h=2\n"},
		{"no value is still the letter", "typeset -H q\ntypeset -p q", "typeset -H q\n"},
		{"the plus form takes it off", "typeset -H h=hid\ntypeset +H h\ntypeset -p h", "h=hid\n"},
		{"unset takes the name and the letter", "typeset -H h=1\nunset h\ntypeset -p h\necho rc=$?", "rc=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Where the letter stands among the others, measured a pair at a time: after
// the export, readonly and kind letters and before the case letters. The
// spelling on the way in decides nothing — `-Hl` and `-lH` list alike.
func TestTheBigHLetterIsListedInItsMeasuredPlace(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -x -H h=1", "typeset -x -H h=1\n"},
		{"typeset -r -H h=1", "typeset -r -H h=1\n"},
		{"typeset -H -l h=ab", "typeset -H -l h=ab\n"},
		{"typeset -H -u h=AB", "typeset -H -u h=AB\n"},
		{"typeset -x -r -H -u h=AB", "typeset -x -r -H -u h=AB\n"},
		{"typeset -Hl h=ab", "typeset -H -l h=ab\n"},
		{"typeset -lH h=ab", "typeset -H -l h=ab\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src+"\ntypeset -p h")
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The letter and the integer attribute are exclusive under this reading, and
// the refusal is the usage block alone and fatal. `integer -H` is the same
// conflict by a second route: that word is `typeset -li` here, so it arrives
// with the attribute already on and no `i` written anywhere. zsh takes the
// pair and lists `typeset -i n`, which is what makes this the reading's rule
// rather than the letter's.
func TestTheBigHLetterAndTheIntegerAttributeAreExclusive(t *testing.T) {
	for _, src := range []string{"typeset -iH n=5", "typeset -H -i n=5", "integer -H n=5"} {
		out, st := runKsh(t, t.TempDir(), src+"\necho after")
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
}
