// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `${| cmd;}` is the current-shell substitution valued from what the body left
// in `$REPLY` rather than from what it printed — the one expansion that lets a
// function return a value without a subshell and without the caller naming a
// variable.
//
// Every row measured 2026-09-13 against bash 5.3.15, the one shell in the panel
// that has it. Run through braceRun, which is the blank form's door with the
// pipe form's flag folded in: two helpers would have been two places for a fix
// to land.
func TestAReplySubstitutionIsValuedFromReply(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the value is what the body left in REPLY", "echo \"[${| REPLY=hi;}]\"\n", "[hi]\n"},
		{
			// The whole point, and the row the issue was filed on.
			"including what a function left there",
			"f() { REPLY=zz; }\necho \"[${| f;}]\"\n", "[zz]\n",
		},
		{
			// The discriminating row against reading this as the blank
			// form with a marker: its body's output is the value and this
			// one's is not captured at all.
			"and not what it printed",
			"echo \"[${| echo printed; REPLY=val;}]\"\n", "printed\n[val]\n",
		},
		{
			"what it assigns survives, because it is this shell",
			"x=0\necho \"[${| x=9; REPLY=r;}]\"\necho \"x=[$x]\"\n", "[r]\nx=[9]\n",
		},
		{"an unset REPLY is an empty value", "echo \"[${| true;}]\"\n", "[]\n"},
		{"an empty body is an empty value", "echo \"[${|}]\"\n", "[]\n"},
		{
			// No blank is needed after the pipe: the `|` is the marker, not
			// the first character of the body. `${ x}` needs its blank.
			"the pipe needs no blank after it",
			"echo \"[${|REPLY=hi;}]\"\n", "[hi]\n",
		},
		{"one inside another", "echo \"[${| REPLY=${| REPLY=in;};}]\"\n", "[in]\n"},
		{"and one inside the blank form", "echo \"[${| REPLY=${ echo in;};}]\"\n", "[in]\n"},
		{"quoted, it is one field", "echo \"[${| REPLY='a b';}]\"\n", "[a b]\n"},
		{
			// Not trimmed, where the blank form's output is: this is a
			// parameter's value and not captured output.
			"trailing newlines stay, where the blank form's go",
			"v='a\n\n'\necho \"[${| REPLY=$v;}]\"\n", "[a\n\n]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := braceRun(t, c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// `$REPLY` is hidden for the body and put back afterwards, and absent and empty
// are two states on the way back.
//
// Measured 2026-09-13 on bash 5.3.15: the body starts with no REPLY whatever
// the script had set, the script's own value is there again afterwards, and a
// REPLY that was unset before is unset after rather than empty. Three rows,
// because a save that kept only the value would pass the first two.
func TestAReplySubstitutionLocalizesReply(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the body starts with none of the script's",
			"REPLY=outer\necho \"[${| echo \"in=[${REPLY+SET}]\"; REPLY=x;}]\"\n",
			"in=[]\n[x]\n",
		},
		{
			"and the script's is there afterwards",
			"REPLY=outer\nv=${| REPLY=inner;}\necho \"[$v][$REPLY]\"\n", "[inner][outer]\n",
		},
		{
			"one that was unset before is unset after, not empty",
			"unset REPLY\nv=${| REPLY=inner;}\necho \"[$v][${REPLY+SET}]\"\n", "[inner][]\n",
		},
		{
			"and one that was empty before is empty after, not unset",
			"REPLY=\nv=${| REPLY=inner;}\necho \"[$v][${REPLY+SET}]\"\n", "[inner][SET]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := braceRun(t, c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The name is *hidden* for the body rather than deleted, and the difference is
// an inherited one: a deleted name still reads through to the environment.
//
// Measured 2026-09-13 — `REPLY=outer bash -c 'echo "[${| true; }]"'` is `[]` on
// bash 5.3.15, not `[outer]`. Without the hiding this row is the one that moves,
// and it is the only one that can: every other row here writes REPLY from the
// script, where deleting and hiding are the same thing.
func TestAReplySubstitutionHidesAnInheritedReply(t *testing.T) {
	got := braceRun(t, "echo \"[${| true;}]\"\necho \"after=[$REPLY]\"\n", "REPLY=inherited")
	if want := "[]\nafter=[inherited]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The status is the body's last command's, for the same reason the blank form's
// is: it is this runner's status, set where every other command sets it.
//
// The assignment shape is the only one that shows it — after `echo ${| false;}`
// the status is the echo's — which is the same control the blank form's test
// carries.
func TestAReplySubstitutionCarriesItsStatus(t *testing.T) {
	if got := braceRun(t, "y=${| false;}\necho \"st=$?\"\n"); !strings.Contains(got, "st=1") {
		t.Errorf("got %q, want st=1", got)
	}
	if got := braceRun(t, "y=${| true;}\necho \"st=$?\"\n"); !strings.Contains(got, "st=0") {
		t.Errorf("got %q, want st=0", got)
	}
}

// The writer is never taken away, which is the half of this that is *not* the
// blank form: whatever the body prints goes where the shell's output was going,
// and what follows the substitution is written as well.
func TestAReplySubstitutionNeverTakesTheWriter(t *testing.T) {
	got := braceRun(t, "y=${| echo loose; REPLY=v;}\necho \"after=[$y]\"\n")
	if !strings.Contains(got, "loose") {
		t.Errorf("got %q, want the body's output written rather than captured", got)
	}
	if !strings.Contains(got, "after=[v]") {
		t.Errorf("got %q, want the value from REPLY and the next line written", got)
	}
}
