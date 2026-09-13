// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// This shell names borrowed text while it is *running* and not only when it
// fails to parse, and it writes the name in front of the location:
// `ash: ./p.sh: line 2: NOPE: parameter not set`. That is the fourth
// arrangement of the same three fields — dash names the text after the
// location, bash and zsh put it where the shell's own name goes, ksh93
// renders the whole chain — and it is
// interp.Diagnostics.BorrowedTextIsNamedAtRunTime plus
// SourceFileNaming/EvalNaming rather than a mechanism of its own (#2520).
//
// Measured 2026-09-12, BusyBox v1.37.0 in the pinned alpine image. Eight
// arrangements, run as one script so that the columns cannot drift apart:
//
//	dot from a script                    ./s.sh: ./p.sh: line 2: NOPE: …
//	a dot nested two deep                ./s.sh: ./p.sh: line 2: NOPE: …
//	eval from a script                   ./s.sh: eval: line 11: NOPE: …
//	a dot inside a function              ./s.sh: ./p.sh: line 2: NOPE: …
//	a function defined in a sourced
//	  file, called after it returned     ./s.sh: line 1: NOPE: …
//	an eval inside a sourced file        ./s.sh: eval: line 2: NOPE: …
//	command not found inside an eval     ./s.sh: eval: line 20: nosuchcmd: …
//	command not found inside a dot       ./s.sh: ./nf.sh: line 2: nosuchcmd: …
//
// The fifth row is the one that makes the rule falsifiable: naming is over
// the innermost borrowed text *still being read*, so a source that has
// returned leaves nothing to name even though the failing body came from its
// file. An implementation that named the defining file would pass the other
// seven.
//
// **The panel on this machine cannot answer any of this.** There is no
// BusyBox ash here, so every line above came out of the container; the
// sibling's answer is not this one's, since dash writes the name after the
// location.
func TestBorrowedTextIsNamedInFrontOfTheLocation(t *testing.T) {
	const mk = "printf 'echo one\\necho $NOPE\\n' > p.sh\n"
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"a sourced file, failing in the file",
			mk + "set -u\n. ./p.sh\n",
			": ./p.sh: line 2: NOPE: parameter not set",
			"the path the script wrote, in front of the file's own line",
		},
		{
			"a sourced file, failing under a function it called",
			mk + "set -u\ng() { . ./p.sh; }\ng\n",
			": ./p.sh: line 2: NOPE: parameter not set",
			"a function frame above the borrowed text does not end it",
		},
		{
			"a sourced file two levels down",
			mk + "printf 'echo s1\\n. ./p.sh\\n' > s.sh\nset -u\n. ./s.sh\n",
			": ./p.sh: line 2: NOPE: parameter not set",
			"the innermost text and nothing about the way in — this shell writes no chain",
		},
		{
			"text eval is running",
			"set -u\neval 'echo e\necho $NOPE'\n",
			": eval: line ",
			"the builtin's own name for text that came from no file. The " +
				"*digit* is deliberately not pinned here: this runner is on " +
				"no invocation route, and ash numbers a `-c` program from 0 " +
				"where a script starts at 1, so a number written down here " +
				"would be one route's answer labelled as the dialect's. It " +
				"is Semantics.EvalTextContinuesTheCallersLines' question " +
				"and the corpus row `eval/the-borrowed-text-in-the-prefix` " +
				"grades it against the record, from a script",
		},
		{
			"a command not found inside a sourced file",
			"printf 'echo d1\\nnosuchcmd\\n' > nf.sh\n. ./nf.sh\n",
			": ./nf.sh: line 2: nosuchcmd: not found",
			"not only the expansion failures — any run-time diagnostic inside the text",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runIn(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// The other half, and the one a wider rule would fail: the source has
// returned, so there is no borrowed text to name and the prefix is the plain
// one. Measured in the same run as the table above.
func TestTheNameGoesWhenTheSourceReturns(t *testing.T) {
	out, _ := runIn(t, "printf 'f() { echo $NOPE; }\\n' > fn.sh\nset -u\n. ./fn.sh\nf\n")
	if !strings.Contains(out, "NOPE: parameter not set") {
		t.Fatalf("got %q, want the unset-parameter failure", out)
	}
	if strings.Contains(out, "fn.sh") {
		t.Errorf("got %q, want no borrowed name: the source returned before `f` ran, "+
			"so the innermost borrowed text is nothing at all", out)
	}
}
