// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The performance gate: what makes "no dialect can be slower than the
// original" a thing that fails rather than a thing that is reported.
//
// The rest of this package measures and does not assert, for the reason its
// own comment gives — a millisecond is a fact about the machine as much as
// about the code, and a number committed to this tree would be stale within a
// day. That reasoning is untouched and it is why nothing here holds a
// threshold either. What is asserted is a *comparison*, taken in one
// interleaved run against the shell the dialect claims to be, which is the
// only form of this claim that means anything: "faster than the bash on this
// machine, in this minute" survives being run on a different machine, and
// "under 4.3 ms" does not.
//
// #1403 is the requirement. The maintainer's words: "performance is a must
// requirement before 0.0.0, no dialect can be slower than the original."

// Pair is one dialect measured against the shell it claims to be.
//
// Both halves are Subjects rather than paths, so the reference is described
// the same way ours is and reached by the same code. Which reference goes
// with which dialect is the claim the dialect binary makes about itself, and
// it is a value here rather than a lookup for the reason the rest of this
// repository keeps its answers in tables.
type Pair struct {
	// Name is the dialect, and is what a failure is reported under.
	Name string
	// Ours is the binary this repository builds.
	Ours Subject
	// Real is the shell it claims to be — the oracle panel's own entry, so
	// that the shell being compared against is the shell the corpus was
	// recorded from. See internal/oracle/shell.go: a Homebrew bash 5 and
	// the bash 3.2 Apple ships are different programs with the same name,
	// and crediting one with the other's startup is a measurement of
	// neither.
	Real Subject
}

// Pairs is the four dialects and their originals.
//
// dir is where the binaries this repository builds were put. A pair whose
// reference is not installed on this machine has an empty Real.Path and is
// skipped by the gate, which is how a machine without ksh93 still gets the
// other three answers instead of no answer at all.
//
// Only the `-c` route is described here, and that is the acceptance criterion
// rather than an omission: #1403 asks for "ours < real on both a bare `-c ':'`
// and a representative workload", and the route to a prompt is #1383's
// question, measured by the benchmarks above and by a rich rc they read. A
// prompt also cannot be compared this way — two of the four dialects have no
// rc file of their own to be pointed at — so folding it in here would mean a
// gate that quietly covered two dialects.
func Pairs(dir string) []Pair {
	ours := func(bin string) Subject {
		return Subject{Path: filepath.Join(dir, bin), CommandArgs: []string{"-c"}}
	}
	real := func(candidates ...string) Subject {
		return Subject{Path: firstPath(candidates...), CommandArgs: []string{"-c"}}
	}
	// The candidate lists are the oracle panel's, verbatim, and zsh's order
	// matters: /opt/homebrew/bin/zsh is 5.9.2 and the /bin/zsh Apple ships
	// is not the shell this project is graded against.
	return []Pair{
		{
			Name: "bash",
			Ours: ours("our-bash"),
			Real: real("/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/bin/bash", "/usr/bin/bash"),
		},
		{
			Name: "zsh",
			Ours: ours("our-zsh"),
			Real: real("/opt/homebrew/bin/zsh", "/usr/local/bin/zsh", "/bin/zsh", "/usr/bin/zsh"),
		},
		{
			Name: "ksh",
			Ours: ours("our-ksh"),
			Real: real("/bin/ksh", "/usr/bin/ksh", "/opt/homebrew/bin/ksh93"),
		},
		{
			Name: "dash",
			Ours: ours("our-dash"),
			Real: real("/bin/dash", "/usr/bin/dash"),
		},
	}
}

// The two programs every pair is timed on.
//
// bareProgram is what a script's every subshell pays and is as close to
// nothing as a shell can be asked to do: `:` is a builtin in all six panel
// members that does nothing at all. What is timed is everything around it.
//
// It cannot check its own work, and that is why there are two. A shell that
// ignored `-c` entirely would run `:` in no time and pass — so the pair is
// gated on the *workload* as well, whose answer is checked, and the bare case
// is additionally required to exit 0. A shell that refused `:` exits non-zero
// and is reported as a refusal rather than as a very good time.
const bareProgram = ":"

// workloadProgram is a representative workload in the common-denominator
// language, so that all four dialects and all four references run the same
// text: a two-thousand-iteration `while`, a `for` over ten words, a `case`,
// and two substring expansions. Nothing in it is any dialect's own.
//
// It ends by printing what it computed, and that is the whole reason it is
// shaped this way rather than as a bare loop. This repository has now been
// caught three times by an instrument that timed a shell *refusing* the
// construct under test — the clock faithfully measures a shell doing nothing,
// and a refusal posts the best number in the table (docs/design/startup.md).
// A number is only accepted here if the run that produced it also produced
// workloadAnswer, so this gate cannot report a time for work that was not
// done.
const workloadProgram = `i=0
n=0
while [ $i -lt 2000 ]; do
	i=$((i + 1))
	n=$((n + 1))
done
for w in a b c d e f g h i j; do
	case $w in
	a|e|i|o|u) n=$((n + 1)) ;;
	*) n=$((n + 2)) ;;
	esac
done
s=abcdefghij
printf 'ANS:%s:%s:%s\n' "$n" "${s#abc}" "${s%hij}"
`

// workloadAnswer is what every shell in the panel prints for that program:
// 2000 loop iterations, plus one for each of the three vowels among the ten
// words and two for each of the other seven, and the two ends taken off a
// ten-letter string.
const workloadAnswer = "ANS:2017:defghij:abcdefg\n"

// Case is one of the two programs a pair is timed on.
type Case struct {
	// Name is what the case is reported under.
	Name string
	// Program is the text handed to `-c`.
	Program string
	// Answer is what the program must print for its time to count. Empty
	// means the program prints nothing and only its exit status is checked
	// — see bareProgram for why that is enough there and not enough alone.
	Answer string
}

// Cases are the two programs, in the order a report should read them.
func Cases() []Case {
	return []Case{
		{Name: "bare", Program: bareProgram},
		{Name: "workload", Program: workloadProgram, Answer: workloadAnswer},
	}
}

// RunProgram runs one program through a subject's `-c` route and returns how
// long the whole invocation took and what it wrote.
//
// The output comes back rather than being discarded, which is what lets the
// caller refuse a time for work that was not done. Standard error is captured
// with it so that a refusal can be quoted in the failure instead of
// disappearing.
func RunProgram(s Subject, program string) (time.Duration, string, error) {
	argv := append(append([]string{}, s.CommandArgs...), program)
	cmd := exec.Command(s.Path, argv...)
	cmd.Env = bareEnv(s.Env)
	var out, errOut bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &out, &errOut
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start)
	if err != nil {
		return took, out.String(), fmt.Errorf("%w (stderr %q)", err, strings.TrimSpace(errOut.String()))
	}
	return took, out.String(), nil
}

// Result is what one subject measured on one case.
type Result struct {
	// Best is the fastest single invocation seen.
	//
	// The minimum rather than the mean, and that is a decision about this
	// machine rather than a preference. Measured here under load averages
	// between 8 and 31 — other agents run on it — the batch mean moved by
	// more than a factor of three between batches while the minimum moved
	// by a tenth, because the minimum is the sample that got a clean run
	// and a mean is a measurement of the load. What is wanted is how much
	// work the program does, and the least-contended spawn is the closest
	// this can get to it.
	Best time.Duration
	// Samples is how many invocations that minimum was drawn from.
	Samples int
}

// Comparison is one pair on one case, both halves measured together.
type Comparison struct {
	Pair Pair
	Case Case
	Ours Result
	Real Result
	// Err is set where a measurement could not honestly be made — a
	// refusal, an exit status, or an answer that was not the panel's. A
	// comparison carrying one is a failure and never a pass, whatever the
	// times say.
	Err error
}

// Ratio is ours over the original. Below 1 is the requirement met.
func (c Comparison) Ratio() float64 {
	if c.Real.Best <= 0 {
		return 0
	}
	return float64(c.Ours.Best) / float64(c.Real.Best)
}

// Passed reports whether this comparison meets the requirement.
//
// Strictly faster, with no tolerance and no epsilon. "No dialect can be
// slower than the original" and "our zsh MUST be faster than zsh" are the
// requirement as written, and a gate that allowed 5% would be answering a
// question nobody asked. A comparison that could not be measured has not
// passed — see Err.
func (c Comparison) Passed() bool { return c.Err == nil && c.Ours.Best < c.Real.Best }

// Measure times every pair on every case, interleaved, and returns what it
// found.
//
// Interleaved is the whole method and not a detail. Within a batch each
// subject gets one invocation in turn, so a load spike lands on ours and on
// the original alike rather than on whichever happened to be running when it
// arrived. Measuring one subject to completion and then the next is what makes
// a busy machine able to invent a 2x difference between two copies of the same
// binary, and this package has produced exactly that.
//
// batches × per invocations of each subject. Two loops rather than one so that
// the minimum is drawn from samples spread across the whole run instead of
// from one contiguous window of it.
//
// Every subject is run twice before the clock starts. The first execution of a
// freshly linked binary on macOS pays a one-time validation cost that a shell
// installed months ago does not, and 288 ms of it landed in the first sample
// the day this package was written — a fact about a build, not about a shell.
func Measure(pairs []Pair, cases []Case, batches, per int) []Comparison {
	type slot struct {
		pair Pair
		kase Case
		ours *Result
		real *Result
		err  *error
	}
	out := make([]Comparison, 0, len(pairs)*len(cases))
	for _, p := range pairs {
		for _, c := range cases {
			out = append(out, Comparison{Pair: p, Case: c})
		}
	}
	var slots []slot
	for i := range out {
		c := &out[i]
		if c.Pair.Real.Path == "" {
			c.Err = errors.New("the reference shell is not installed here, so there is nothing to compare against")
			continue
		}
		c.Ours.Best, c.Real.Best = time.Duration(1<<62), time.Duration(1<<62)
		slots = append(slots, slot{c.Pair, c.Case, &c.Ours, &c.Real, &c.Err})
	}

	// Warm, and check the answer while doing it, so that a subject which
	// cannot do the work is known before any of its times are recorded.
	for _, s := range slots {
		for _, side := range []struct {
			who  string
			subj Subject
		}{{"ours", s.pair.Ours}, {s.pair.Name, s.pair.Real}} {
			for range 2 {
				_, got, err := RunProgram(side.subj, s.kase.Program)
				if err != nil {
					*s.err = fmt.Errorf("%s %s refused the %s program: %w", side.who, s.pair.Name, s.kase.Name, err)
					break
				}
				if s.kase.Answer != "" && got != s.kase.Answer {
					*s.err = fmt.Errorf("%s %s answered %q for the %s program where the panel answers %q, so its time would be a measurement of work it did not do",
						side.who, s.pair.Name, got, s.kase.Name, s.kase.Answer)
					break
				}
			}
		}
	}

	for range batches {
		for range per {
			for _, s := range slots {
				if *s.err != nil {
					continue
				}
				record := func(subj Subject, who string, into *Result) {
					took, got, err := RunProgram(subj, s.kase.Program)
					if err != nil {
						*s.err = fmt.Errorf("%s %s stopped answering the %s program: %w", who, s.pair.Name, s.kase.Name, err)
						return
					}
					if s.kase.Answer != "" && got != s.kase.Answer {
						*s.err = fmt.Errorf("%s %s answered %q for the %s program where the panel answers %q",
							who, s.pair.Name, got, s.kase.Name, s.kase.Answer)
						return
					}
					into.Samples++
					if took < into.Best {
						into.Best = took
					}
				}
				record(s.pair.Ours, "ours", s.ours)
				record(s.pair.Real, s.pair.Name, s.real)
			}
		}
	}
	for i := range out {
		c := &out[i]
		if c.Ours.Samples == 0 {
			c.Ours.Best = 0
		}
		if c.Real.Samples == 0 {
			c.Real.Best = 0
		}
	}
	return out
}

// Report renders a comparison table, and is what a failure quotes.
//
// A number with no reference beside it is not evidence, so ours and the
// original are always printed together with the ratio between them — and the
// load average is printed above, because on a machine with other work on it
// that is the difference between a result and an anecdote.
func Report(cs []Comparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "load average: %s\n", LoadAverage())
	fmt.Fprintf(&b, "%-10s %-9s %10s %10s %8s %7s  %s\n", "dialect", "case", "ours", "original", "ratio", "samples", "verdict")
	sorted := append([]Comparison(nil), cs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Pair.Name < sorted[j].Pair.Name })
	for _, c := range sorted {
		verdict := "FASTER"
		if c.Err != nil {
			verdict = "NOT MEASURED"
		} else if !c.Passed() {
			verdict = "SLOWER"
		}
		fmt.Fprintf(&b, "%-10s %-9s %8.2fms %8.2fms %7.2fx %7d  %s\n",
			c.Pair.Name, c.Case.Name, ms(c.Ours.Best), ms(c.Real.Best), c.Ratio(), c.Ours.Samples, verdict)
		if c.Err != nil {
			fmt.Fprintf(&b, "%-10s %-9s   %v\n", "", "", c.Err)
		}
	}
	return b.String()
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// LoadAverage is what the machine was doing while the measurement was taken.
//
// Reported with every result rather than looked up afterwards. Read by running
// `uptime`, which is test infrastructure reaching for a tool rather than a
// shell reaching for one — there is no portable syscall for it in Go, and this
// package is already the one that spawns processes for a living.
func LoadAverage() string {
	out, err := exec.Command("uptime").Output()
	if err != nil {
		return "unknown"
	}
	text := string(out)
	if i := strings.Index(text, "load average"); i >= 0 {
		return strings.TrimSpace(text[i:])
	}
	if i := strings.Index(text, "load averages"); i >= 0 {
		return strings.TrimSpace(text[i:])
	}
	return strings.TrimSpace(text)
}
