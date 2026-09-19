// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a refused reference declaration leaves behind — see
// interp/declarationtakenback.go, where the rows are measured.
//
// One table over both answers rather than a test per refusal, because the
// point is the **difference** between them: a reported refusal takes the
// operand back and the silent one does not, and either half alone passes with
// the other broken.
func runDeclarationTakenBack(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	// Three axes a row walks past on its way to this one, answered flat so
	// that an unanswered one cannot stand in for the refusal a row is looking
	// for: whether `-p` reports a name that is not there — which is how a row
	// says the name is gone — whether `typeset` needs a keyword-defined
	// function to declare a local, and whether the export letter reaches past
	// the function.
	sem.DeclarePrintReportsAMissingName = Yes
	sem.TypesetLocalNeedsKeywordFunction = No
	sem.ExportLetterDeclaresAGlobal = No
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
	}, func(r *Runner) { r.Semantics = &sem })
}

// A refusal the declaration reports leaves the name exactly as the operand
// found it: the letters this line applied are gone, the letters the name
// already carried stand, and a name the line brought into being is not there.
func TestAReportedRefusalLeavesTheNameAsTheOperandFoundIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The control: with nothing for the letters to land on, the two
			// answers are the same and this row is about the name.
			name: "a name the operand brought into being",
			src:  `typeset -nx r=1x; typeset -p r; echo "st=$?"`,
			want: "st=1\n",
		},
		{
			name: "a name that was already standing",
			src:  `p=orig; typeset -nx p=1x; typeset -p p`,
			want: "declare -- p='orig'\n",
		},
		{
			// The letters the *line* wrote go and the letters the name had
			// stay, which is what says this is the name put back rather than
			// the operand's letters taken off.
			name: "a letter from an earlier line",
			src:  `typeset -l q=AB; typeset -nx q=1x; typeset -p q`,
			want: "declare -l q='ab'\n",
		},
		{
			name: "the self reference",
			src:  `typeset -nu u=u; typeset -p u; echo "st=$?"`,
			want: "st=1\n",
		},
		{
			// The valueless form, whose target is the value the name was
			// already holding — a refusal there is the same answer.
			name: "a target the name was holding",
			src:  `e=/; typeset -nx e; typeset -p e`,
			want: "declare -- e='/'\n",
		},
		{
			// And the array refusal, which is the third wording and not a
			// third rule.
			name: "a reference over an array",
			src:  `a=(x y); typeset -nx a; typeset -p a`,
			want: "declare -a a=([0]='x' [1]='y')\n",
		},
		{
			// In a function the local is not made either, so the name reads
			// as the caller's and the caller gets it back unlettered.
			name: "inside a function",
			src:  `g=G; f(){ typeset -nx g=1x; typeset -p g; }; f; typeset -p g`,
			want: "declare -- g='G'\ndeclare -- g='G'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runDeclarationTakenBack(t, tc.src)
			out = withoutComplaints(out)
			if out != tc.want || status != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, status, tc.want)
			}
		})
	}
}

// The silent refusal is the other answer: the letters stand. Only a name
// brought into being at the top level goes, and a local made by the call
// stands with its letters whatever the aiming did.
func TestTheSilentRefusalKeepsTheLettersItApplied(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a name that was already standing",
			src:  `w=old; typeset -ni w=v; typeset -p w`,
			want: "declare -i w='old'\n",
		},
		{
			name: "a local the call made",
			src:  `f(){ typeset -ni z=v; typeset -p z; }; f`,
			want: "declare -i z\n",
		},
		{
			name: "a name the operand brought into being",
			src:  `typeset -ni r=v; typeset -p r; echo "st=$?"`,
			want: "st=1\n",
		},
		{
			// The same name written through `-g`, which takes no shadow: the
			// binding is the top level's and goes with it.
			name: "a global written from inside a function",
			src:  `f(){ typeset -gni t=v; }; f; typeset -p t; echo "st=$?"`,
			want: "st=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runDeclarationTakenBack(t, tc.src)
			out = withoutComplaints(out)
			if out != tc.want || status != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, status, tc.want)
			}
		})
	}
}

// withoutComplaints drops the diagnostic lines, which are another subject's —
// these rows are about what stands afterwards, and the wordings are pinned in
// interp/namerefletters_test.go and by the corpus.
func withoutComplaints(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.Contains(line, "invalid") ||
			strings.Contains(line, "reference variable") ||
			strings.Contains(line, "not found") {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}
