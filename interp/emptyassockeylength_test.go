// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The *length* of a table's element under a key that came out empty is a
// third answer to one emptiness, and not a louder version of the read's
// report: the subject, the status and what happens next all differ.
//
// Measured 2026-09-12, `-c`, with `typeset -A m; m[k]=v` and `w=`:
//
//	shell         ${#m[$w]}
//	bash 5.3.15   `[$w]: bad array subscript`, status 1, nothing after runs
//	ksh93u+       `0`, `after`, status 0
//	zsh 5.9.2     `0`, `after`, status 0
//
// The plain read is the control that says the length operator is doing this
// and not the key lookup: `${m[$w]}` on the same line writes `m: bad array
// subscript` and answers the empty string at 0 (#2286).

const emptyKeyLengthSrc = `typeset -A m; m[k]=v; w=; echo "[${#m[$w]}]"; echo after`

func TestTheLengthOfAnEmptyKeyMayBeRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		refused bool
	}{
		{"refused", Yes, true},
		{"answered", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyRefusesTheLength = tc.answer
			out, st := run(t, emptyKeyLengthSrc, withSem(sem))
			if tc.refused {
				// Nonzero rather than a number: which status a failed
				// expansion leaves is a dialect's answer and not this axis's.
				// The shipped bash dialect answers 1, matching the shell.
				if st == 0 {
					t.Errorf("status = %d, want the shell ended; out = %q", st, out)
				}
				if !strings.Contains(out, "bad array subscript") {
					t.Errorf("out = %q, want the refusal", out)
				}
				// Abandoned rather than reported: neither the echo the
				// expansion is in nor the one after it runs.
				if strings.Contains(out, "[0]") || strings.Contains(out, "after") {
					t.Errorf("out = %q, want the word abandoned and the script stopped", out)
				}
				return
			}
			if st != 0 || !strings.Contains(out, "[0]\nafter\n") {
				t.Errorf("out = %q st=%d, want the absent element's 0 and the script running on", out, st)
			}
			if strings.Contains(out, "bad array subscript") {
				t.Errorf("out = %q, want nothing said", out)
			}
		})
	}
}

// The subject is the subscript **as written, brackets and all, with no name**
// — which is neither of the two subjects the neighbouring shapes use.
func TestTheRefusedLengthNamesTheSubscriptAsWritten(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m; m[k]=v; w=; echo "${#m[$w]}"`, `[$w]: bad array subscript`},
		{`typeset -A m; m[k]=v; w=; echo "${#m[${w}]}"`, `[${w}]: bad array subscript`},
	} {
		sem := testSemantics()
		sem.EmptyAssociativeKeyRefusesTheLength = Yes
		out, _ := run(t, tc.src, withSem(sem))
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q — the source text and not what it expanded to", tc.src, out, tc.want)
		}
		if strings.Contains(out, "m[") || strings.Contains(out, "m:") {
			t.Errorf("%s = %q, want no name in the subject", tc.src, out)
		}
	}
}

// A subshell dies alone. The refusal ends the shell it is in, which is the
// ordinary shape of a failed expansion here, so the parent carries on at 0.
func TestTheRefusedLengthEndsOnlyTheShellItIsIn(t *testing.T) {
	sem := testSemantics()
	sem.EmptyAssociativeKeyRefusesTheLength = Yes
	const src = `typeset -A m; m[k]=v; w=; ( echo "[${#m[$w]}]" ); echo after`
	out, st := run(t, src, withSem(sem))
	if st != 0 || !strings.Contains(out, "after") {
		t.Errorf("%s = %q st=%d, want the parent running on at 0", src, out, st)
	}
	if strings.Contains(out, "[0]") {
		t.Errorf("%s = %q, want the subshell's word abandoned", src, out)
	}
}

// Where the axis is asked, and where the same emptiness means something else.
// Every row but the first is a shape the refusal must not reach.
func TestTheEmptyKeyLengthAxisIsAskedOnlyWhereItWasMeasured(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		asked     bool
	}{
		{"the length of an empty key on a written table", `typeset -A m; m[k]=v; w=; echo "${#m[$w]}"`, true},
		// Emptied by an assignment rather than holding anything: still a
		// table something has written to, and still refused.
		{"a table emptied after being written", `typeset -A m; m[k]=v; unset "m[k]"; w=; echo "${#m[$w]}"`, true},
		// A name the declaration's letters merely brought into being is
		// silent — measured, and the pair declaredOnlyCompound keeps.
		{"a table the letters only declared", `typeset -A m; w=; echo "${#m[$w]}"`, false},
		// The plain read is the neighbouring axis and not this one.
		{"the plain read of the same element", `typeset -A m; m[k]=v; w=; echo "${m[$w]}"`, false},
		// A blank key is a key: emptiness and not whitespace.
		{"a blank key", `typeset -A m; m[k]=v; echo "${#m[ ]}"`, false},
		{"an ordinary key", `typeset -A m; m[k]=v; echo "${#m[k]}"`, false},
		// An indexed name reads its subscript as an expression.
		{"an indexed name", `a=(1 2); w=; echo "${#a[$w]}"`, false},
		// And the whole-table spellings are not keys at all.
		{"the whole table", `typeset -A m; m[k]=v; echo "${#m[@]}"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyIsAnError = No
			sem.EmptyAssociativeKeyIsReportedWhenRead = No
			sem.EmptyAssociativeKeyRefusesTheLength = Unspecified
			out, _ := run(t, tc.src, withSem(sem))
			said := strings.Contains(out, "the length of a keyed table's element under an empty key")
			if said != tc.asked {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "asked", false: "not asked"}[tc.asked])
			}
		})
	}
}

// A dialect that answers the read and not the length gives each its own
// sentence, which is what says they are two questions and not one.
func TestTheReadReportAndTheLengthRefusalAreTwoQuestions(t *testing.T) {
	sem := testSemantics()
	sem.EmptyAssociativeKeyIsReportedWhenRead = Yes
	sem.EmptyAssociativeKeyRefusesTheLength = No
	const src = `typeset -A m; m[k]=v; w=; echo "[${m[$w]}]"; echo "[${#m[$w]}]"; echo after`
	out, st := run(t, src, withSem(sem))
	if st != 0 {
		t.Fatalf("status = %d, want 0; out = %q", st, out)
	}
	// One sentence, from the read alone: the length is answered in silence.
	if n := strings.Count(out, "bad array subscript"); n != 1 {
		t.Errorf("out = %q, want the sentence once, got %d", out, n)
	}
	if !strings.Contains(out, "[]\n") || !strings.Contains(out, "[0]\n") || !strings.Contains(out, "after") {
		t.Errorf("out = %q, want the read's empty string, the length's 0 and the script running on", out)
	}
}
