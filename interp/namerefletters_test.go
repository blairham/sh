// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The `n` letter meeting another one, in the two readings a dialect has of
// it: decide the pair, or refuse to read it at all.
//
// Where the pair is decided, what decides it is that **a reference's own cell
// holds the target's name** — so a letter that shapes a value shapes that
// name. See Runner.namerefAim and Semantics.NamerefLetterStandsAlone (#3137,
// #3170, #3171).

// runNamerefLetters runs src where the `n` letter is read beside every other
// one, which is the reading that has a table to get right.
func runNamerefLetters(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
	}, func(r *Runner) { r.Semantics = &sem })
}

// A case letter written on the `-n` line folds the **name the reference is
// aimed at**, and the letter stays on the reference to say so.
func TestACaseLetterOnAReferenceFoldsTheNameItIsAimedAt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "the upper letter",
			src:  `v=1; V=BIGV; typeset -nu r=v; typeset -p r; echo "[$r]"`,
			want: "declare -nu r='V'\n[BIGV]\n",
		},
		{
			name: "the lower letter",
			src:  `v=1; V=BIGV; typeset -nl r=V; typeset -p r; echo "[$r]"`,
			want: "declare -nl r='v'\n[1]\n",
		},
		{
			// The target may be an element, and the fold reaches the whole
			// word: it is one string in one cell and nothing here knows a
			// subscript from a name.
			name: "an element target",
			src:  `a=(x y); A=(p q); typeset -nu r=a[1]; typeset -p r; echo "[$r]"`,
			want: "declare -nu r='A[1]'\n[q]\n",
		},
		{
			// The same letter arriving on an *earlier* line shapes nothing,
			// because the `-n` declaration discards what it found standing.
			// The pair is what makes the row above about this line rather
			// than about the name.
			name: "the letter from an earlier line",
			src:  `v=1; typeset -u r; typeset -n r=v; typeset -p r; echo "[$r]"`,
			want: "declare -n r='v'\n[1]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runNamerefLetters(t, tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, status, tc.want)
			}
		})
	}
}

// And the fold is done **by the declaration**, so a second `-n` over the same
// reference takes the letter off and leaves the folded name standing.
func TestASecondReferenceDeclarationDropsTheFoldingLetter(t *testing.T) {
	out, status := runNamerefLetters(t,
		`v=1; typeset -nu r=v; typeset -p r; typeset -n r; typeset -p r`)
	want := "declare -nu r='V'\ndeclare -n r='V'\n"
	if out != want || status != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, status, want)
	}
}

// The integer letter is the end of the same rule: what the cell holds becomes
// a **number**, and a number is not a name, so the declaration has nowhere to
// aim. Refused at 1 and **in silence** — the word as written passed the check
// that has a sentence.
func TestAnIntegerReferenceHasNoNameToAimAt(t *testing.T) {
	out, status := runNamerefLetters(t,
		`v=1; typeset -ni r=v; echo "st=$?"; echo "[$r]"`)
	if !strings.HasPrefix(out, "st=1\n") {
		t.Errorf("out %q does not refuse the declaration at 1", out)
	}
	if strings.Contains(out, "invalid variable name") {
		t.Errorf("out %q says something about a word the shell took", out)
	}
	if strings.Contains(out, "[1]") {
		t.Errorf("out %q reads through a reference the declaration refused", out)
	}
	if status != 0 {
		t.Errorf("status %d, want the script to carry on", status)
	}
	// And the word as written still gets the ordinary complaint, which is
	// what keeps the silence above a measurement rather than a swallowed
	// refusal.
	out, _ = runNamerefLetters(t, "typeset -ni r=1; echo \"st=$?\"")
	if !strings.Contains(out, "invalid variable name for name reference") {
		t.Errorf("out %q loses the complaint about the word as written", out)
	}
}

// The fold runs **in front of** the self-reference check and the written word
// is still tested too. Three rows, because either test alone passes two of
// them and fails the third.
func TestTheSelfReferenceCheckSeesBothTheWordAndTheFold(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"the fold makes it one", `typeset -nl r=R; echo "st=$?"`, true},
		{"the word is one", `typeset -nu r=r; echo "st=$?"`, true},
		{"neither is", `typeset -nu r=R; echo "st=$?"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runNamerefLetters(t, tc.src)
			said := strings.Contains(out, "invalid self reference")
			if said != tc.refused {
				t.Errorf("out %q, want refused=%v", out, tc.refused)
			}
		})
	}
}

// The other reading: the `n` letter is refused **in company**, whatever the
// other letter is and whichever sign either carries. The complaint is the
// builtin's bare usage line, with none of the sentence a letter the builtin
// has not got would earn.
func TestTheReferenceLetterCanBeRefusedInCompany(t *testing.T) {
	for _, src := range []string{
		"typeset -ni r=v",
		"typeset -rn r=v",
		"typeset -n -i r=v",
		"typeset -i -n r=v",
		"typeset +n -i r",
	} {
		t.Run(src, func(t *testing.T) {
			out, status := runNamerefLettersAlone(t, src+"\necho AFTER")
			if !strings.Contains(out, "Usage: typeset") {
				t.Errorf("out %q does not write the usage line", out)
			}
			if !strings.HasPrefix(out, "Usage: typeset") {
				t.Errorf("out %q writes something in front of the usage line", out)
			}
			if strings.Contains(out, "AFTER") {
				t.Errorf("out %q carried on past the refusal", out)
			}
			if status != 2 {
				t.Errorf("status %d, want 2", status)
			}
		})
	}
	// The letter alone is still read, which is what makes the rows above
	// about the company rather than about the letter.
	out, status := runNamerefLettersAlone(t, `v=1; typeset -n r=v; echo "[$r]"`)
	if out != "[1]\n" || status != 0 {
		t.Errorf("out %q status %d, want the reference read at 0", out, status)
	}
}

// runNamerefLettersAlone runs src where the `n` letter refuses company, and
// where the builtin's refusal ends the script — the two together are what one
// dialect was measured doing.
func runNamerefLettersAlone(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	sem.NamerefLetterStandsAlone = Yes
	sem.TypesetBadOptionFatal = Yes
	return runGrammar(t, src, nil, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{
			BuiltinUsage: map[string]string{"typeset": "Usage: typeset [-nirx] [name[=value]...]"},
			// Unprefixed, because the refusal is *bare*: what a case here
			// asserts is that nothing of the shell's own stands in front of
			// the block, and a location would be indistinguishable from a
			// complaint.
			BuiltinUsageUnprefixed: true,
		}
	})
}
