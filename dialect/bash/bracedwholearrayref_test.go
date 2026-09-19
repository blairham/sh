// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `$r` and `${r}` on a reference aimed at the whole of an array, which this
// shell reads two different ways — the splice and a scalar read of the
// reference.
//
// This is the only column that can be asked: zsh has no `-n` letter at all,
// bash 3.2 has none, and ksh93 refuses this target at the declaration
// (#3124). So the core holds the reading and this file holds the measurement
// — taken 2026-09-19 against bash 5.3.20 at /opt/homebrew, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, under `-c`, a script file
// and standard input alike.

// The fields, which is the half that is a wrong **value** rather than a
// missing diagnostic: three elements one at a time for the bare spelling and
// one joined field for the braced one.
func TestTheTwoSpellingsOfAWholeArrayReferenceGiveDifferentFields(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(aa bb cc)
declare -n r=a[@]
printf '<%s>' "$r"; echo
printf '<%s>' "${r}"; echo
printf '<%s>' ${r}; echo`)
	want := "<aa><bb><cc>\n<aa bb cc>\n<aa><bb><cc>\n"
	if out != want || st != 0 {
		t.Errorf("the spellings = %q (status %d), want %q", out, st, want)
	}
}

// An operator applies to that joined string and not to the elements, which is
// the discriminator saying the braced spelling is not `${a[*]}` either: a
// range subscript written over `[*]` takes *elements* and this takes
// characters.
func TestAnOperatorOnABracedWholeArrayReferenceReadsTheJoin(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(aa bb cc)
declare -n r=a[@]
echo "[${r:0:2}]"
echo "[${a[*]:0:2}]"
echo "[${r@Q}]"`)
	want := "[aa]\n[aa bb]\n['aa bb cc']\n"
	if out != want || st != 0 {
		t.Errorf("the operators = %q (status %d), want %q", out, st, want)
	}
}

// The length is the exception and it is measured: the element **count** under
// either spelling, where the join is seven characters long.
func TestTheLengthOfAWholeArrayReferenceIsTheCount(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "a=(aaa bbb)\ndeclare -n r=a[@]\necho \"[${#r}]\"")
	if out != "[2]\n" || st != 0 {
		t.Errorf("the length = %q (status %d), want %q", out, st, "[2]\n")
	}
}

// A list with **no elements** is unset through the braced reference where one
// holding a single empty element is set, which is what the `set -u` row below
// rests on and is measurable without the option.
func TestAnEmptyArrayIsUnsetThroughABracedReference(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(); declare -n r=a[@]; echo "[${r-D}][${r+S}]"
b=(""); declare -n s=b[@]; echo "[${s-D}][${s+S}]"
c=(x); declare -n u=c[@]; echo "[${u+S}]"
declare -n v=d[@]; echo "[${v+S}]"`)
	want := "[D][]\n[][S]\n[S]\n[]\n"
	if out != want || st != 0 {
		t.Errorf("the set-ness = %q (status %d), want %q", out, st, want)
	}
}

// And `set -u`, which is the row #3125 was filed for: the braced spelling
// refuses and names the **reference**, where the bare one is silent because a
// splice of an array nobody set is not an unbound parameter here.
func TestNounsetReachesTheBracedSpellingOfAWholeArrayReference(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "set -u\ndeclare -n r=a[@]\necho \"[$r]\"\necho OK")
	if out != "[]\nOK\n" || st != 0 {
		t.Errorf("the bare spelling = %q (status %d), want it silent", out, st)
	}
	out, st = runBash(t, t.TempDir(), "set -u\ndeclare -n r=a[@]\necho \"[${r}]\"\necho OK")
	if !strings.Contains(out, "r: unbound variable") || strings.Contains(out, "OK") || st == 0 {
		t.Errorf("the braced spelling = %q (status %d), want the reference named", out, st)
	}
	out, st = runBash(t, t.TempDir(), "set -u\na=()\ndeclare -n r=a[@]\necho \"[${r}]\"\necho OK")
	if !strings.Contains(out, "r: unbound variable") || strings.Contains(out, "OK") || st == 0 {
		t.Errorf("a list with no elements = %q (status %d), want the reference named", out, st)
	}
	out, st = runBash(t, t.TempDir(), "set -u\na=(\"\")\ndeclare -n r=a[@]\necho \"[${r}]\"\necho OK")
	if out != "[]\nOK\n" || st != 0 {
		t.Errorf("one empty element = %q (status %d), want it silent", out, st)
	}
	// The control that keeps this about the spelling: a subscript of its own
	// is not the whole-array reference at all and is silent under either.
	out, st = runBash(t, t.TempDir(), "set -u\ndeclare -n r=a[@]\necho \"[${r[@]}]\"\necho OK")
	if out != "[]\nOK\n" || st != 0 {
		t.Errorf("a subscripted reference = %q (status %d), want it silent", out, st)
	}
}
