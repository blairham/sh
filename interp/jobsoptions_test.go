// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `jobs`' option letters. Every one of them was read and thrown away before
// this, so `jobs -p` printed the whole listing and `kill $(jobs -p)` was a
// line that killed nothing.

// jobsSemantics is a listing with the questions this file is not about
// already answered, so a refusal here is about the letter under test.
func jobsSemantics(letters string) Semantics {
	sem := CoreSemantics()
	sem.JobsListFinishedJobs = Yes
	sem.JobsListNewestFirst = No
	sem.JobsShowBackgroundCommand = Yes
	sem.JobsOptions = letters
	// The majority answer, so a test about a *letter* is not also a test
	// about which listing finishes with a job. TestOnlyOneDialectFinishes…
	// below asks that axis itself.
	sem.PidListingFinishesWithAJob = No
	return sem
}

func jobsRun(t *testing.T, src string, adjust func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := jobsSemantics("lprs")
		if adjust != nil {
			adjust(&sem)
		}
		r.Semantics = &sem
	})
}

// The whole of #469: `-p` is the process ids and nothing else in three of the
// four shells, and the job's process *group* added to an ordinary listing in
// the fourth. Both are reachable, and neither is the core's to pick.
func TestJobsDashPIsTheProcessIdsAlone(t *testing.T) {
	const src = `sleep 0.3 & echo "bang=$!"; jobs -p; wait`

	t.Run("the ids alone", func(t *testing.T) {
		out, st := jobsRun(t, src, func(s *Semantics) { s.JobsPidsOnlyOption = Yes })
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		bang, listed := twoLines(t, out)
		if listed != bang {
			t.Errorf("listed %q, want the job's process id %q and nothing else", listed, bang)
		}
	})

	t.Run("the id inside a listing", func(t *testing.T) {
		out, st := jobsRun(t, src, func(s *Semantics) { s.JobsPidsOnlyOption = No })
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		bang, listed := twoLines(t, out)
		if listed == bang || !strings.Contains(listed, bang) || !strings.Contains(listed, "[1]") {
			t.Errorf("listed %q, want a numbered row carrying the id %q", listed, bang)
		}
	})

	t.Run("unanswered, the core refuses rather than choosing", func(t *testing.T) {
		out, _ := jobsRun(t, src, nil)
		if !strings.Contains(out, "printing process ids and nothing else") {
			t.Errorf("out = %q, want a refusal naming the axis", out)
		}
	})
}

// `-l` is the one letter every shell in the panel has, so the core prints it
// without asking anything.
func TestJobsDashLPutsTheProcessIdInTheRow(t *testing.T) {
	out, st := jobsRun(t, `sleep 0.3 & echo "bang=$!"; jobs -l; wait`, nil)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	bang, listed := twoLines(t, out)
	if !strings.Contains(listed, bang) || !strings.HasPrefix(listed, "[1]") {
		t.Errorf("listed %q, want the numbered row to carry the id %q", listed, bang)
	}
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want no question asked for a letter every shell shares", out)
	}
}

// The two format letters are exclusive and the last one given decides, which
// is the same in every shell that has both.
func TestJobsFormatLettersTheLastOneWins(t *testing.T) {
	for _, tc := range []struct {
		name, opts string
		wantIDOnly bool
	}{
		{"-lp is the ids", "-lp", true},
		{"-pl is the listing", "-pl", false},
		{"-l -p is the ids", "-l -p", true},
		{"-p -l is the listing", "-p -l", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := jobsRun(t, `sleep 0.3 & echo "bang=$!"; jobs `+tc.opts+`; wait`,
				func(s *Semantics) { s.JobsPidsOnlyOption = Yes })
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			bang, listed := twoLines(t, out)
			if idOnly := listed == bang; idOnly != tc.wantIDOnly {
				t.Errorf("listed %q, want ids-only=%v", listed, tc.wantIDOnly)
			}
		})
	}
}

// A letter the dialect's set does not carry is refused. Silently ignoring one
// is the failure this whole change is about, and it is worse here than
// elsewhere: a name the engine recognizes never reaches the exec seam, so an
// unimplemented `jobs` shadows whatever is on the machine.
func TestJobsRefusesALetterTheDialectDoesNotHave(t *testing.T) {
	out, st := run(t, `jobs -r; echo "st=$?"`, func(r *Runner) {
		sem := jobsSemantics("lp")
		r.Semantics = &sem
	})
	if !strings.Contains(out, "invalid option") || !strings.Contains(out, "st=2") {
		t.Errorf("out = %q status %d, want -r refused where the letter set has no r", out, st)
	}
}

// `-r` and `-s` pick a state. Alone they mean the same thing in both shells
// that have them; together they are the one place those two disagree.
func TestJobsStateFilters(t *testing.T) {
	// A running job and nothing else, so `-r` shows it and `-s` does not.
	const src = `sleep 0.3 & jobs %s; echo "end"; wait`

	t.Run("-r keeps a running job", func(t *testing.T) {
		out, _ := jobsRun(t, strings.Replace(src, "%s", "-r", 1), nil)
		if !strings.Contains(out, "[1]") {
			t.Errorf("out = %q, want the running job listed", out)
		}
	})

	t.Run("-s leaves it out", func(t *testing.T) {
		out, _ := jobsRun(t, strings.Replace(src, "%s", "-s", 1), nil)
		if strings.Contains(out, "[1]") {
			t.Errorf("out = %q, want nothing listed", out)
		}
	})

	t.Run("both, adding up", func(t *testing.T) {
		out, _ := jobsRun(t, strings.Replace(src, "%s", "-rs", 1),
			func(s *Semantics) { s.JobsStateFiltersAccumulate = Yes })
		if !strings.Contains(out, "[1]") {
			t.Errorf("out = %q, want a job in either state listed", out)
		}
	})

	t.Run("both, the last letter deciding", func(t *testing.T) {
		out, _ := jobsRun(t, strings.Replace(src, "%s", "-rs", 1),
			func(s *Semantics) { s.JobsStateFiltersAccumulate = No })
		if strings.Contains(out, "[1]") {
			t.Errorf("out = %q, want -s to have decided", out)
		}
	})

	t.Run("both, unanswered", func(t *testing.T) {
		out, _ := jobsRun(t, strings.Replace(src, "%s", "-rs", 1), nil)
		if !strings.Contains(out, "listing a job in either state") {
			t.Errorf("out = %q, want a refusal naming the axis", out)
		}
	})

	t.Run("one letter asks nothing", func(t *testing.T) {
		out, _ := jobsRun(t, strings.Replace(src, "%s", "-r", 1), nil)
		if strings.Contains(out, "no dialect was chosen") {
			t.Errorf("out = %q, want no question where one filter decides it", out)
		}
	})
}

// A filtered listing and an ids-only one do not finish with a job the way a
// listing of states does: the job is still there for the next bare `jobs`.
func TestJobsOnlyAStateListingForgetsAFinishedJob(t *testing.T) {
	for _, tc := range []struct {
		name, opts string
		wantKept   bool
	}{
		{"a bare listing consumes it", "", false},
		{"the long listing consumes it", "-l", false},
		{"the ids keep it", "-p", true},
		{"a filtered listing keeps it", "-r", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `sleep 0.05 & sleep 0.3; jobs ` + tc.opts + `; echo "---"; jobs`
			out, _ := jobsRun(t, src, func(s *Semantics) { s.JobsPidsOnlyOption = Yes })
			after := out[strings.Index(out, "---"):]
			if kept := strings.Contains(after, "[1]"); kept != tc.wantKept {
				t.Errorf("after = %q, want kept=%v", after, tc.wantKept)
			}
		})
	}
}

// **Whether a pid listing finishes with the job is a dialect's answer**, and
// the axis is asked only where there is a finished job for it to be about.
//
// The `Yes` row is what one column of the panel does and the `No` row is what
// two others do; the third row is why the axis cannot be inferred from the
// listing form alone — a `jobs -p` over a job that is still running raises no
// question in any of them, so a fix that asked unconditionally would refuse
// every ordinary `jobs -p` in a shell with no answer (#602).
func TestAPidListingMayFinishWithAJob(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answer   Answer
		wantKept bool
	}{
		{"kept for the next listing", No, true},
		{"finished with, as a state listing would", Yes, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `sleep 0.05 & sleep 0.3; jobs -p; echo "---"; jobs`
			out, _ := jobsRun(t, src, func(s *Semantics) {
				s.JobsPidsOnlyOption = Yes
				s.PidListingFinishesWithAJob = tc.answer
			})
			after := out[strings.Index(out, "---"):]
			if kept := strings.Contains(after, "[1]"); kept != tc.wantKept {
				t.Errorf("after = %q, want kept=%v", after, tc.wantKept)
			}
		})
	}

	t.Run("a running job asks nothing", func(t *testing.T) {
		out, _ := jobsRun(t, `sleep 0.3 & jobs -p; wait`, func(s *Semantics) {
			s.JobsPidsOnlyOption = Yes
			s.PidListingFinishesWithAJob = Unspecified
		})
		if strings.Contains(out, "no dialect was chosen") {
			t.Errorf("out = %q, want no question where no job has finished", out)
		}
	})
}

// Operands settle their own order and each row keeps the job's own number —
// `jobs %2` printed `[1]` before this, because the row counted from the start
// of the slice the listing had been handed.
func TestJobsOperandsKeepTheirOrderAndTheirNumbers(t *testing.T) {
	out, st := jobsRun(t, `sleep 0.3 & sleep 0.4 & jobs %2 %1; wait`, nil)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("listed %d rows, want 2:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "[2]") || !strings.HasPrefix(lines[1], "[1]") {
		t.Errorf("rows = %q, want them in the order written, each with its own number", lines)
	}
}

// One job named twice is listed twice, and the number is the job's rather
// than the row's — the shape a listing of one job could never have shown.
func TestJobsOneJobNamedTwice(t *testing.T) {
	out, _ := jobsRun(t, `sleep 0.3 & sleep 0.4 & jobs %2 %2; wait`, nil)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "[2]") || !strings.HasPrefix(lines[1], "[2]") {
		t.Errorf("rows = %q, want the same job listed twice as [2]", lines)
	}
}

// A spec that names nothing is reported after the jobs written before it have
// been listed, which is unanimous and was the other way around here.
func TestJobsABadSpecIsReportedAfterTheRowsBeforeIt(t *testing.T) {
	out, st := jobsRun(t, `sleep 0.3 & jobs %1 %9; wait`, nil)
	row, complaint := strings.Index(out, "[1]"), strings.Index(out, "no such job")
	if row < 0 || complaint < 0 || row > complaint {
		t.Errorf("out = %q status %d, want the row and then the complaint", out, st)
	}
}

// twoLines splits an `echo bang=$!` and the single listing line after it.
func twoLines(t *testing.T, out string) (bang, listed string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "bang=") {
		t.Fatalf("out = %q, want the process id and one listing line", out)
	}
	return strings.TrimPrefix(lines[0], "bang="), strings.TrimRight(lines[1], " ")
}
