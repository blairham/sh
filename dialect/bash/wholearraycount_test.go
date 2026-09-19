// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `set -u` refuses `${#a[@]}` here unless the name holds a list, and it is
// the widest of the three readings the panel has: a **scalar** is refused
// alongside a name that was never mentioned, where the same line on zsh 5.9.2
// is a length and on ksh93u+ is `1`.
//
// Measured 2026-09-18 against bash 5.3.20 and bash 3.2.57, `env -i HOME=…
// PATH=/usr/bin:/bin LC_ALL=C` from a script file under `set -u`, with
// `echo after` on the line below. The third row is the control that keeps this
// about the count rather than about the name: `${x[@]}` on the same scalar is
// `abc` at status 0 in the same run (#3125).
func TestACountOfAWholeArrayRefusesANameHoldingNoList(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"set -u\necho \"[${#nope[@]}]\"\necho after\n", "nope: unbound variable"},
		{"set -u\necho \"[${#nope[*]}]\"\necho after\n", "nope: unbound variable"},
		{"x=abc; set -u\necho \"[${#x[@]}]\"\necho after\n", "x: unbound variable"},
	} {
		out, st := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%q = %q, want %q in it", tc.src, out, tc.want)
		}
		if strings.Contains(out, "[") {
			t.Errorf("%q = %q, want the refused command not to have run", tc.src, out)
		}
		if !strings.Contains(out, "after") || st != 0 {
			t.Errorf("%q = %q status %d, want the next line run and the shell alive", tc.src, out, st)
		}
	}
	// The sentence names the **bare** name, where every other subscripted
	// refusal in this dialect writes the brackets back.
	if out, _ := answersRun(t, `set -u; echo "[${#nope[@]}]"`); strings.Contains(out, "nope[") {
		t.Errorf("got %q, want the subject written without the subscript", out)
	}
	for _, tc := range []struct{ src, want string }{
		{"a=(); set -u\necho \"[${#a[@]}]\"\necho after\n", "[0]\nafter\n"},
		{`a=(p q); set -u; echo "[${#a[@]}]"; echo after`, "[2]\nafter\n"},
		{`x=abc; set -u; echo "[${x[@]}]"; echo after`, "[abc]\nafter\n"},
	} {
		if out, st := answersRun(t, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the refusal gives up **the line** and runs the next one, which is the
// opposite of what the same option does one construct over.
//
// Measured 2026-09-18 against bash 5.3.20 and bash 3.2.57 alike, from a script
// file: `echo "c=${#a[@]}"; echo SAME` writes the sentence, never prints
// `SAME`, and `echo "NEXT=$?"` on the next line prints `NEXT=1`; the same
// script with `${#a}` in place of the count ends the shell at 1 and prints
// neither (#3125).
func TestARefusedCountGivesUpTheLineAndNotTheShell(t *testing.T) {
	out, st := answersRun(t, "set -u\necho \"c=${#nope[@]}\"; echo SAME\necho \"NEXT=$?\"\n")
	if strings.Contains(out, "SAME") {
		t.Errorf("got %q, want the rest of the line given up", out)
	}
	if !strings.Contains(out, "NEXT=1") || st != 0 {
		t.Errorf("got %q status %d, want NEXT=1 and a status of 0", out, st)
	}
	// The control: a length rather than a count, on the same unset name, ends
	// the shell in this dialect.
	out, st = answersRun(t, "set -u\necho \"c=${#nope}\"\necho NEXT\n")
	if strings.Contains(out, "NEXT") || st == 0 {
		t.Errorf("a length = %q status %d, want the shell stopped", out, st)
	}
}

// A reference aimed at a whole array reaches the same refusal, because
// `${#r}` on one *is* `${#a[@]}` — which is what the row #3125 filed as a
// brace-versus-bare question about references turns out to be.
//
// Measured 2026-09-18 against bash 5.3.20 from a script file under `set -u`
// with `a` never set: `declare -n r=a[@]; echo "${#r}"` is `a: unbound
// variable`, the line is given up, the next one runs, and the sentence names
// the **array** rather than the reference.
func TestACountThroughAReferenceAimedAtAWholeArray(t *testing.T) {
	out, st := answersRun(t, "set -u\ndeclare -n r=a[@]\necho \"[${#r}]\"\necho after\n")
	if want := "a: unbound variable"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q in it", out, want)
	}
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q status %d, want the next line run at 0", out, st)
	}
	// With the array there, the same reference is the count and says nothing.
	if out, st := answersRun(t, "set -u\na=(p q)\ndeclare -n r=a[@]\necho \"[${#r}]\"\n"); out != "[2]\n" || st != 0 {
		t.Errorf("got %q status %d, want [2] at 0", out, st)
	}
}
