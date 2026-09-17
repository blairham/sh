// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// readArrayRun runs src with the `-a` spelling of the array letter, which is
// the one this is about: the two spellings are two shells' and they answer
// this differently, so the behavior rides the letter rather than an axis.
func readArrayRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return readRun(t, func(s *Semantics) {
		s.ReadOptions = "a:dnrst"
		s.DeclareOptions = "aAfFgilnprux"
		s.UnsetOptions = "vfn"
		s.NamerefCycleIsRefused = No
		s.BadNameToDeclarationFatal = No
		s.NamerefArrayRefusal = NamerefArrayCheckedLastOnTheAttribute
	}, Diagnostics{}, src)
}

// The `-a` array's name is a **plain** name, where an ordinary `read` operand
// may carry a subscript.
//
// One name rule was doing for both, so `read -a 'A[0]'` answered 0 and made a
// parameter *called* `A[0]` holding the words — the array the script meant was
// never written and nothing said so. Measured 2026-09-17 on bash 5.3.20:
// bash refuses the name and writes nothing, at 1, with no `A` afterwards.
func TestTheArrayLetterTakesAPlainNameWhereAnOperandTakesASubscript(t *testing.T) {
	out, _ := readArrayRun(t, `read -a 'A[0]' <<< "x y"; echo "st=$?"
echo "[${A[*]-GONE}]"`)
	if !strings.Contains(out, "`A[0]': not a valid identifier") {
		t.Errorf("got %q, want the array name refused", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want status 1", out)
	}
	if !strings.Contains(out, "[GONE]") {
		t.Errorf("got %q, want nothing written", out)
	}

	// The operand's own rule is untouched, which is what says this is the
	// *array letter's* question and not a name rule for the builtin.
	out, _ = readArrayRun(t, `B=(p q); read 'B[1]' <<< "z"; echo "st=$?"
echo "[${B[*]}]"`)
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "[p z]") {
		t.Errorf("got %q, want the element written and no refusal", out)
	}

	// And an ordinary array name still fills.
	out, _ = readArrayRun(t, `read -a A <<< "x y"; echo "st=$?"
echo "[${A[*]}]"`)
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "[x y]") {
		t.Errorf("got %q, want the array filled", out)
	}
}

// And the name is judged **through a reference**, so a reference aimed at an
// element is refused rather than quietly making a parameter with a subscript
// in its name.
//
// The same rule and the same route `mapfile` takes. #3478 closed this shape
// for `mapfile`, `readarray` and `unset`; `read` was the writer left out, and
// it is the one the suite reaches.
func TestTheArrayLetterJudgesTheNameAReferencePointsAt(t *testing.T) {
	out, _ := readArrayRun(t, `typeset -n e=XXX[0]
read -a e <<< "A B C"; echo "st=$?"
echo "[${XXX[*]-GONE}]"`)
	if !strings.Contains(out, "`XXX[0]': not a valid identifier") {
		t.Errorf("got %q, want the target refused and named", out)
	}
	if !strings.Contains(out, "st=1") || !strings.Contains(out, "[GONE]") {
		t.Errorf("got %q, want status 1 and nothing written", out)
	}

	// A reference aimed at a plain name fills the target, which it always
	// did — the control that says the redirect is a judgement and not a wall.
	out, _ = readArrayRun(t, `typeset -n r=A
read -a r <<< "x y"; echo "st=$?"
echo "[${A[*]}]"`)
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "[x y]") {
		t.Errorf("got %q, want the reference's target filled", out)
	}
}
