// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The instrument this file guards is the corpus's *invocation* surface: the
// cases that spell an argv out rather than letting the harness write `-c`.
//
// It exists because the measuring apparatus here has been the defective part
// more often than the code, and each time it read as a result rather than as
// a bug. Two of those are the shape this file is aimed at. The dialect suites
// built Runners with a nil Dialect, so every one of them asserted *core*
// answers while claiming to assert a shell's (#849) — and fixing it made zero
// tests fail, which was the finding. The conformance harness normalized a
// shell's own name by its binary's basename, so real zsh's column normalized
// by accident and the implementation's did not, and 281 rows were graded on
// the name of a build directory (#848).
//
// The same failure is available here and would look the same: a case that
// writes `-lc` in its Args, and an argv assembly that drops it. Every column
// then answers for a plain `-c`, every column agrees, and the row records a
// unanimous fact about an invocation nobody performed. Nothing is red. The
// score goes *up*, because a shape no shell was asked about is a shape no
// shell can fail.
//
// So the check is not "does the corpus have login cases". It is "do the words
// the case wrote reach the shell", asserted over the whole corpus, plus a
// deliberate violation handed to the same checker to prove the checker can
// say no.

// argvOmissions reports what the built argv failed to carry of the words a
// case wrote, or "" when it carried all of them.
//
// It is a function rather than a test body so that the negative half below can
// hand it an argv that is wrong on purpose. A detector that reports nothing
// passes every run, and a silent one is indistinguishable from a clean tree.
func argvOmissions(c Case, dir string, argv []string) string {
	if len(c.Args) == 0 {
		return ""
	}
	want := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		a = strings.ReplaceAll(a, ArgSnippet, c.Snippet)
		a = strings.ReplaceAll(a, ArgScript, filepath.Join(dir, "case.sh"))
		want = append(want, a)
	}
	if len(argv) < len(want) {
		return fmt.Sprintf("%s: argv %q holds %d words, fewer than the %d the case wrote",
			c.ID, argv, len(argv), len(want))
	}
	if tail := argv[len(argv)-len(want):]; !slices.Equal(tail, want) {
		return fmt.Sprintf("%s: argv ends %q, want the case's own words %q", c.ID, tail, want)
	}
	return ""
}

// TestEveryInvocationCasesOwnWordsReachTheShell is the positive half, over
// the real corpus.
//
// Asserted as the argv's tail rather than by searching it, because position
// is the whole of what an invocation case measures: `-c -l cmd` and `-c cmd
// -l` hold the same three words and ask opposite questions — the first
// passes an option and the second passes an operand that becomes `$0` — so a
// check that only asked whether `-l` was present would pass both while
// measuring one.
func TestEveryInvocationCasesOwnWordsReachTheShell(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours", Args: []string{"-dialect", "bash"}}, Path: "/bin/sh"}
	dir := t.TempDir()
	n := 0
	for _, c := range Corpus {
		if len(c.Args) == 0 {
			continue
		}
		n++
		if got := argvOmissions(c, dir, command(t.Context(), sh, c, dir).Args); got != "" {
			t.Error(got)
		}
	}
	// A count, because the loop above passes vacuously over an empty corpus
	// and a corpus that stopped setting Args is exactly the regression this
	// file is about. The floor is the population at the time of writing, less
	// room to delete a case deliberately.
	if n < 70 {
		t.Errorf("only %d cases spell an argv out; the invocation surface has stopped being graded", n)
	}
}

// TestAnArgvThatDroppedTheCasesWordsIsReported is the deliberate violation.
//
// Every case in the corpus passes the check above, so on its own that proves
// nothing: a checker that returned "" unconditionally would pass it too, and
// would go on passing it after the bug it exists to catch had landed. This
// hands it the three shapes the bug takes and asserts the whole rendered line
// for each, location included — a Contains would match a checker that had
// stopped saying which case, which is the half a reader needs.
func TestAnArgvThatDroppedTheCasesWordsIsReported(t *testing.T) {
	c := Case{
		ID: "t/login-bundle", Category: "harness invocation",
		Args:    []string{"-lc", ArgSnippet},
		Snippet: `echo ran`,
		Why:     "a case whose shape is dropped on purpose",
	}
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{
			// The bug in full: the assembly ignored Args and wrote the
			// harness's own default, so the shell was asked a plain `-c`.
			name: "the shape was replaced by the default route",
			argv: []string{"/bin/sh", "-c", "echo ran"},
			want: `t/login-bundle: argv ends ["-c" "echo ran"], want the case's own words ["-lc" "echo ran"]`,
		},
		{
			// The subtler one: the letters arrived unbundled, which is a
			// different invocation and in two shells a different answer.
			name: "the bundle was split into two words",
			argv: []string{"/bin/sh", "-l", "-c", "echo ran"},
			want: `t/login-bundle: argv ends ["-c" "echo ran"], want the case's own words ["-lc" "echo ran"]`,
		},
		{
			name: "the argv is shorter than the case",
			argv: []string{"-lc"},
			want: `t/login-bundle: argv ["-lc"] holds 1 words, fewer than the 2 the case wrote`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := argvOmissions(c, t.TempDir(), tc.argv); got != tc.want {
				t.Errorf("argvOmissions = %q, want %q", got, tc.want)
			}
		})
	}
	// And the other direction, which is the one that makes the three above
	// mean something: the correct argv must report nothing, or the checker is
	// merely noisy rather than discriminating.
	ok := []string{"/bin/sh", "-dialect", "bash", "-lc", "echo ran"}
	if got := argvOmissions(c, t.TempDir(), ok); got != "" {
		t.Errorf("argvOmissions on a correct argv = %q, want no complaint", got)
	}
}

// TestNoTwoCasesAskTheSameQuestion guards the other way an instrument passes
// for the wrong reason: a case that duplicates another measures nothing and
// scores a point for it.
//
// The identity of a case is everything the shell is handed — argv, program,
// standard input, the name it was called by, and the environment — because
// those are the only inputs there are. Two cases alike in all five are one
// case counted twice, and the corpus percentage is a headline number.
//
// Standard input counts for two of them rather than one: what is on it, and
// whether it is open at all. Those are different questions and one shell
// answers them differently, so a key that read only the bytes would call the
// null-device case and the closed-descriptor case one measurement — which is
// how this test failed the moment Case.StdinClosed existed, correctly (#1038).
// It found two pairs already in the tree when it was written, and they were
// listed rather than deleted. Both were deliberate cross-references — a case
// filed under two chapters, each Why pointing at the other — so removing
// either would delete evidence somebody wrote on purpose, and `corpus-guard`
// treats a lost case as a regression whatever the reason. What is worth
// knowing is recorded here instead: the corpus percentage counts each of the
// remaining facts twice, and the maintainer can decide whether a chapter's
// cross-reference should be a case at all.
//
// One of the two pairs is gone, and not by being deduplicated: both its cases
// ran `a=(); set -- "${a[@]}"` and both were retired, because that line does
// not build an empty array in ksh93 at all (#1379). The chapters each carry
// a row of their own now, asking different questions, so the pair is not a
// pair any more.
//
// The list is by pair, so a *new* duplicate of one of these still fails.
var knownDuplicateCases = map[string]string{
	"syntax/an-unmatched-double-quote": "token/an-unterminated-quote-at-end-of-input",
}

func TestNoTwoCasesAskTheSameQuestion(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	dir := t.TempDir()
	seen := map[string]string{}
	for _, c := range Corpus {
		cmd := command(t.Context(), sh, c, dir)
		key := fmt.Sprintf("%q|%q|%q|%v|%q|%q", cmd.Args[1:], c.Snippet,
			strings.ReplaceAll(c.Stdin, ArgSnippet, c.Snippet), c.StdinClosed,
			c.Argv0, c.Env)
		first, dup := seen[key]
		switch {
		case !dup:
			seen[key] = c.ID
		case knownDuplicateCases[c.ID] == first:
			// Grandfathered above. Left in the map under the first ID, so a
			// third case with the same invocation is still reported.
		default:
			t.Errorf("%s asks exactly what %s asks: same argv, program, input, name and environment", c.ID, first)
		}
	}
	// The list must not outlive what it excuses: a pair that is fixed or
	// renamed stops being found here, and an entry nothing matches is a claim
	// about the corpus that has quietly stopped being true.
	for id, first := range knownDuplicateCases {
		if !slices.ContainsFunc(Corpus, func(c Case) bool { return c.ID == id }) ||
			!slices.ContainsFunc(Corpus, func(c Case) bool { return c.ID == first }) {
			t.Errorf("knownDuplicateCases names %s and %s; the corpus no longer holds both", id, first)
		}
	}
}
