// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `( … )` subshell is a job of its own in one dialect, so the jobs it starts
// are numbered from two — and it never moves the current-job marker, so the
// `+` and `-` stay on whatever *numbers* the parent's were on.
//
// **The two halves are one answer because one mechanism produces both.** A
// shell that numbered from two and still marked its own job would be wrong on
// every row here, and so would one that never marked and numbered from one:
// the rows where a mark appears are exactly the rows where the subshell's job
// number happens to collide with an inherited one.
//
// Measured 2026-09-28 against zsh 5.9.2, `sleep … &` repeated in the parent
// and then `( sleep … & print ${(kv)jobstates} )`, each row a fresh run, 3–5
// repeats each and stable. The last three rows of the first table were
// **predicted from the rule and then measured**, which is what says the marks
// are read off inherited numbers rather than simply cleared.
func TestASubshellNumbersItsOwnJobsFromTwoAndDoesNotMarkThem(t *testing.T) {
	for _, tc := range []struct {
		name       string
		parentJobs int
		want       string // number and marker of the subshell's own job
	}{
		{"no job in the parent leaves the mark on nobody", 0, "2:"},
		{"one, and the `+` stays on the number it was on", 1, "2:"},
		{"two, where the `+` was on number two — so it lands here", 2, "2:+"},
		{"three, where number two held the `-`", 3, "2:-"},
		{"four, where neither mark is on number two", 4, "2:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var src strings.Builder
			for range tc.parentJobs {
				src.WriteString("sleep 0.4 &\n")
			}
			src.WriteString(`( sleep 0.3 & jobs )` + "\n")
			if got := subshellJobRows(t, src.String()); got != tc.want {
				t.Errorf("subshell's job = %q, want %q", got, tc.want)
			}
		})
	}
	// Three jobs started *inside* the subshell, which is where "numbered from
	// two" and "never marked" are told apart from "the first one is special".
	// Both rows were predicted and then measured.
	for _, tc := range []struct {
		name       string
		parentJobs int
		want       string
	}{
		{"a parent of one marks none of the three", 1, "2: 3: 4:"},
		{"a parent of two puts its `+` on the first", 2, "2:+ 3: 4:"},
		{"and a parent of three puts `-` and `+` on the first two", 3, "2:- 3:+ 4:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var src strings.Builder
			for range tc.parentJobs {
				src.WriteString("sleep 0.4 &\n")
			}
			src.WriteString(`( sleep 0.3 & sleep 0.3 & sleep 0.3 & jobs )` + "\n")
			if got := subshellJobRows(t, src.String()); got != tc.want {
				t.Errorf("subshell's jobs = %q, want %q", got, tc.want)
			}
		})
	}
}

// The boundary the rule is keyed on is a real `( … )`, not a clone and not a
// compound — and those three agree on every shape but the last two here.
//
// Measured the same day: `( … )`, `( … ) | cat`, `$( … )`, a backquoted
// substitution, `<( … )` and `( … ) &` all number a job they start **2**,
// while `{ … } | cat` and `{ … } &` number it **1** and mark it. A rule
// written against the clone would be wrong on the last two; one written
// against the compound would be wrong on the same two.
func TestWhichBoundariesNumberASubshellsJobsFromTwo(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"a parenthesised subshell", `( sleep 0.3 & jobs )`, "2:"},
		{"parentheses as a pipeline element", `( sleep 0.3 & jobs ) | cat`, "2:"},
		{"and backgrounded", `( sleep 0.3 & jobs ) & sleep 0.2`, "2:"},
		// The two that part the three candidate nouns.
		{"a group as a pipeline element numbers from one", `{ sleep 0.3 & jobs ; } | cat`, "1:+"},
		{"and so does a backgrounded group", `{ sleep 0.3 & jobs ; } & sleep 0.2`, "1:+"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := subshellJobRows(t, "sleep 0.4 &\n"+tc.src+"\n"); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// A dialect that has not answered numbers a subshell's job `[1]` and marks it,
// which is what six of the seven columns do and what this tree did before the
// axis existed.
func TestADialectThatHasNotAnsweredNumbersASubshellsJobFromOne(t *testing.T) {
	out, st := run(t, "sleep 0.4 &\n( sleep 0.3 & jobs )\n", func(r *Runner) {
		sem := CoreSemantics()
		sem.JobsShowBackgroundCommand, sem.JobsListNewestFirst = Yes, No
		sem.SubshellJobTable = SubshellJobsCleared
		r.Semantics = &sem
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := jobRowsOf(out); got != "1:+" {
		t.Errorf("unanswered = %q, want the subshell's job at 1 and marked", got)
	}
}

// subshellJobRows runs src under a zsh-shaped answer and reports the listing
// as `number:marker` pairs, which is the whole of what the rows above are
// about.
func subshellJobRows(t *testing.T, src string) string {
	t.Helper()
	out, st := run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.JobsShowBackgroundCommand, sem.JobsListNewestFirst = Yes, No
		// The dialect that makes a `( … )` a job of its own also clears the
		// parent's rows from a subshell, so the listing below is the
		// subshell's own jobs and nothing else.
		sem.SubshellJobTable = SubshellJobsCleared
		sem.SubshellIsAJobInItsOwnTable = Yes
		r.Semantics = &sem
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	return jobRowsOf(out)
}

// jobRowsOf reads `[n]<marker>` off a `jobs` listing.
func jobRowsOf(out string) string {
	var rows []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		close := strings.IndexByte(line, ']')
		if close < 0 {
			continue
		}
		mark := ""
		if rest := strings.TrimPrefix(line[close+1:], " "); strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-") {
			mark = rest[:1]
		}
		rows = append(rows, fmt.Sprintf("%s:%s", line[1:close], mark))
	}
	return strings.Join(rows, " ")
}
