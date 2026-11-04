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
	"syscall"
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
//
// #2813 settled which rows that counts. The workload gates; the bare case is
// measured and reported and does not. See Cases — the short version is that
// the bar was read off macOS numbers, and on Linux what is underneath the
// bare case is the Go runtime's process start rather than anything here.
//
// #6496 settled what the workload row is graded on: its time with each
// shell's own bare start subtracted, both from the same run. The start keeps
// its own row, so nothing it costs goes unprinted; it is only no longer paid
// for twice. See Case.NetOf.

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
	// Gating is whether losing this case fails the release bar, as against
	// being measured and reported. See Cases for why the two programs
	// answer differently, which is #2813's finding.
	Gating bool
	// NetOf names the case whose time is taken off this one's before it is
	// graded, each shell's own and from the same run. Empty means the case
	// is graded on its whole time.
	//
	// The workload is graded net of the bare case (#6496). Its whole time is
	// a start plus the interpretation, and the start is already the bare
	// row: grading the workload on its whole time charges that start a
	// second time, on the one row that gates. Net of it, the workload is the
	// thing Cases says it is for, which is interpreter throughput. The whole
	// time is still printed beside it, so the subtraction hides nothing.
	//
	// A case graded net of another cannot be graded at all without it. If
	// the start is missing from the run, or could not be measured, or is
	// not shorter than the case it is taken off, the comparison is
	// unmeasured and fails, rather than being graded on whatever is left.
	NetOf string
}

// Cases are the two programs, in the order a report should read them.
//
// **Only the workload gates**, and that is #2813's answer rather than a
// softening of #1403. The bar was written from macOS numbers where bash
// starts in 6.76 ms; the same bash on an idle ubuntu runner starts in 0.73 ms,
// an order of magnitude apart, while ours barely moves between the two. So
// the margin the bar was read from was macOS being slow at starting bash, and
// not anything this repository did.
//
// What is left underneath the bare case on a platform where spawning a
// process is cheap is the Go runtime's own start, which is already measured:
// an empty Go binary costs more CPU to start than /bin/dash costs to do its
// whole job. That is a fact about the language this shell is written in, and
// no amount of work in this tree moves it — a gate nobody can pass teaches
// nobody anything, and would have held v0.0.0 indefinitely for a reason with
// no fix.
//
// The workload is a different matter and is why the bar survives with teeth.
// It is interpreter throughput, it is ours to win, and the same ubuntu run had
// bash at 1.53x and zsh at 2.20x — losing, but losing by an amount that is
// work rather than physics. Every dialect still has to beat the shell it
// claims to be there, strictly, with no epsilon.
//
// **It is graded net of the bare case**, and that is #6496's answer
// (2026-11-04). The workload's whole time contains a start, and that start is
// the same Go runtime floor the paragraphs above take out of the bar. Graded
// whole, the gating row was paying for it anyway: across main's last 41 runs
// the whole-time column read bash 1.23–1.38x and zsh 1.40–1.60x, while the
// same runs with each shell's own start subtracted read bash 0.87–1.01x and
// zsh 1.02–1.30x. The first would need the interpreter 29–62% faster to pay
// for a start no interpreter work can reach. The second is the interpreter's
// own number, and it is the one the row now gates on. Both are printed; see
// Report.
//
// The bare case keeps being measured and keeps being printed. It is what a
// script's every subshell pays, a regression in it is worth seeing, and
// deleting the row would lose the only number that shows the floor. Report
// marks its verdict so that a red bare row cannot be misread as a gate
// failure; see Report.
func Cases() []Case {
	return []Case{
		{Name: "bare", Program: bareProgram, Gating: false},
		{Name: "workload", Program: workloadProgram, Answer: workloadAnswer, Gating: true, NetOf: "bare"},
	}
}

// Timing is what one invocation cost, measured two ways.
//
// Both, because neither alone is enough here and the two answer different
// questions. Wall is what a person waits for and is the requirement as
// written; CPU is how much work the program did, and it is the only half of
// this measurement that is a fact about the code rather than about the
// machine.
//
// The distinction is not academic in this repository. #1403's own closing run
// has zsh/bare at 4.26 ms against 4.28 ms (FASTER) at load average 26 and
// 4.89 ms against 4.83 ms (SLOWER) at load 60 — **the same tree, the same
// binaries, and the verdict flipped**, because taking the minimum over a
// hundred interleaved samples bounds the contention a sample can have paid
// but does not remove it. A number that decides a release bar should not move
// when somebody else starts a build.
type Timing struct {
	// Wall is how long the invocation took from fork to exit.
	Wall time.Duration
	// CPU is the child's own user plus system time, from the kernel's
	// accounting rather than from a clock this process read — so it counts
	// what the program executed and not what it waited behind.
	//
	// It is a sum across the child's threads, which matters for exactly one
	// side of this comparison: our shells are Go programs with a garbage
	// collector and the references are single-threaded C. So our CPU can
	// exceed our wall where theirs cannot, and the CPU column is the harder
	// bar for us of the two rather than a kinder one.
	CPU time.Duration
}

// RunProgram runs one program through a subject's `-c` route and returns what
// the invocation cost and what it wrote.
//
// The output comes back rather than being discarded, which is what lets the
// caller refuse a time for work that was not done. Standard error is captured
// with it so that a refusal can be quoted in the failure instead of
// disappearing.
func RunProgram(s Subject, program string) (Timing, string, error) {
	argv := append(append([]string{}, s.CommandArgs...), program)
	var out, errOut bytes.Buffer
	var took Timing
	err := retryingTextFileBusy(func() error {
		out.Reset()
		errOut.Reset()
		took = Timing{}
		cmd := exec.Command(s.Path, argv...)
		cmd.Env = bareEnv(s.Env)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &out, &errOut
		start := time.Now()
		err := cmd.Run()
		took.Wall = time.Since(start)
		// Nil where the process was never started — the ETXTBSY path below
		// is one — and there is no accounting for a child that did not run.
		if cmd.ProcessState != nil {
			took.CPU = cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
		}
		return err
	})
	if err != nil {
		return took, out.String(), fmt.Errorf("%w (stderr %q)", err, strings.TrimSpace(errOut.String()))
	}
	return took, out.String(), nil
}

// How long an exec refused with ETXTBSY is waited out before the refusal is
// taken at its word.
//
// The window it is waiting for is one child's fork to its exec, which is
// microseconds of work on a machine with room and milliseconds on one without,
// so the pause is the generous end of that and the bound is short enough that
// a real refusal is a failure rather than a hang. A second of patience against
// a gate run that is minutes long.
const (
	textFileBusyAttempts = 8
	textFileBusyPause    = 125 * time.Millisecond
)

// retryingTextFileBusy runs the command again while the kernel refuses to exec
// its file because something still has it open for writing.
//
// This package writes an executable and hands its path to something that execs
// it — slowWrapper does — and `os.WriteFile` closes its own descriptor before
// it returns, so the race is not with itself. It is golang/go#22315: Go opens
// files `O_CLOEXEC`, so a child forked anywhere else in the process loses the
// descriptor *at its exec*, but it holds it, open for writing, for the whole
// window between its fork and that exec. This package forks constantly, by
// design and in parallel, so the window is open all the time and an exec of
// the same file inside it is ETXTBSY. It failed `main` that way, and the merge
// queue's runs are the ones under the most concurrent load.
//
// Bounded, and the error is returned rather than swallowed: a refusal that
// survives every attempt is a real one and has to fail the gate, because a
// subject that will not run is the fastest subject in any table — the failure
// this package exists to refuse, and which it has already been caught making
// three times (docs/design/startup.md).
//
// Retrying here rather than in the caller because the fault is in the exec and
// not in what any one caller is doing with it: every timed run in this package
// starts a process from a path, and the one that hit this is not the only one
// that could. Nothing about a time is spoiled by it — an attempt that never
// ran the program contributes no sample, and the clock is started again for
// the attempt that does.
//
// Linux enforces the rule; macOS was measured not to, which is why only the
// ubuntu job ever saw the failure and why the tests for this skip elsewhere.
func retryingTextFileBusy(run func() error) error {
	err := run()
	for attempt := 1; attempt < textFileBusyAttempts && errors.Is(err, syscall.ETXTBSY); attempt++ {
		time.Sleep(textFileBusyPause)
		err = run()
	}
	return err
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
	//
	// The two halves are minimized independently, and not from the same
	// invocation. That is deliberate: each is the least-contended sample of
	// the quantity it measures, and insisting they come from one run would
	// mean picking a winner between the two and reporting the loser's
	// second-best number.
	Best Timing
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
	// Start is the comparison this one is graded net of: the same pair on
	// the case Case.NetOf names, from the same run. Measure fills it in. It
	// is nil for a case graded on its whole time, and a case that names one
	// and has none cannot be graded; see Unmeasured.
	Start *Comparison
}

// Ratio is ours over the original on wall time, the whole of each run. For a
// case graded on its whole time it is the graded figure; for one graded net
// of its start it is printed beside NetRatio and does not decide anything.
func (c Comparison) Ratio() float64 { return ratio(c.Ours.Best.Wall, c.Real.Best.Wall) }

// Net is what each side cost with its own start taken off, on both clocks.
//
// ok is false for a case graded on its whole time, and for one whose start is
// missing. A wall half that comes out at zero or less is returned as it is:
// Unmeasured is what refuses it. A CPU half is zero wherever any of the four
// CPU figures it is made from is, because a run the kernel gave no
// accounting for is not a run that cost nothing (see Measure).
func (c Comparison) Net() (ours, real Timing, ok bool) {
	if c.Case.NetOf == "" || c.Start == nil {
		return Timing{}, Timing{}, false
	}
	less := func(whole, start Timing) Timing {
		t := Timing{Wall: whole.Wall - start.Wall}
		if whole.CPU > 0 && start.CPU > 0 {
			t.CPU = whole.CPU - start.CPU
		}
		return t
	}
	return less(c.Ours.Best, c.Start.Ours.Best), less(c.Real.Best, c.Start.Real.Best), true
}

// NetRatio is ours over the original on wall time, each with its own start
// subtracted. It is what Passed consults for a case graded net of its start,
// and zero for a case that is not.
func (c Comparison) NetRatio() float64 {
	ours, real, ok := c.Net()
	if !ok || ours.Wall <= 0 {
		return 0
	}
	return ratio(ours.Wall, real.Wall)
}

// NetCPURatio is NetRatio on the work done rather than the time taken, and is
// reported beside it for the reason CPURatio is reported beside Ratio.
func (c Comparison) NetCPURatio() float64 {
	ours, real, ok := c.Net()
	if !ok || ours.CPU <= 0 {
		return 0
	}
	return ratio(ours.CPU, real.CPU)
}

// Unmeasured says why this comparison could not honestly be graded, and is
// nil when it could.
//
// Err is the first reason. A case graded net of its start adds three more,
// because the subtraction needs a start to subtract: none was measured beside
// it, or that one could not be measured, or a side's whole run took no longer
// than its own start. The last cannot happen with a real start and a real
// workload; if it does, the two numbers are not measuring what they claim
// to, and grading what is left would score a shell on noise.
func (c Comparison) Unmeasured() error {
	if c.Err != nil {
		return c.Err
	}
	if c.Case.NetOf == "" {
		return nil
	}
	if c.Start == nil {
		return fmt.Errorf("graded net of the %s case, which was not measured beside it", c.Case.NetOf)
	}
	if c.Start.Err != nil {
		return fmt.Errorf("graded net of the %s case, which could not be measured: %w", c.Case.NetOf, c.Start.Err)
	}
	ours, real, _ := c.Net()
	if ours.Wall <= 0 || real.Wall <= 0 {
		return fmt.Errorf(
			"graded net of the %s case, and a whole run took no longer than its own start (ours %.2fms, original %.2fms left)",
			c.Case.NetOf,
			ms(ours.Wall),
			ms(real.Wall),
		)
	}
	return nil
}

// CPURatio is the same comparison on the work done rather than the time taken.
//
// Reported beside Ratio and deliberately not gated on. The requirement in
// #1403 is about how long a person waits, so moving the verdict onto CPU
// would be moving the release bar rather than measuring it better — but a
// wall verdict with no CPU figure beside it cannot say whether a dialect is
// slower because of what it does or because of what the machine was doing,
// and that is the question every reader of a red gate has asked so far.
func (c Comparison) CPURatio() float64 { return ratio(c.Ours.Best.CPU, c.Real.Best.CPU) }

func ratio(ours, real time.Duration) float64 {
	if real <= 0 {
		return 0
	}
	return float64(ours) / float64(real)
}

// Passed reports whether this comparison meets the requirement.
//
// Strictly faster, with no tolerance and no epsilon. "No dialect can be
// slower than the original" and "our zsh MUST be faster than zsh" are the
// requirement as written, and a gate that allowed 5% would be answering a
// question nobody asked. A comparison that could not be measured has not
// passed — see Err.
//
// This is the comparison and not the bar. A bare row that loses is a real
// loss and says so here; whether that loss stops a release is Gating's
// question, and the two are kept apart so that #2813's decision changed which
// rows are counted rather than what a row means.
//
// For a case graded net of its start, what is compared is the two net wall
// times (#6496), and the same strictness holds: a net tie is not a pass.
func (c Comparison) Passed() bool {
	if c.Unmeasured() != nil {
		return false
	}
	if ours, real, ok := c.Net(); ok {
		return ours.Wall < real.Wall
	}
	return c.Ours.Best.Wall < c.Real.Best.Wall
}

// Gating reports whether losing this comparison fails the release bar.
//
// See Cases: the workload gates and the bare case does not, because what is
// underneath the bare case on Linux is the Go runtime's process start rather
// than any code here.
func (c Comparison) Gating() bool { return c.Case.Gating }

// Failures sorts a run into the three things a caller has to say about it.
//
// In the package rather than in the test that fails, deliberately. The bar is
// now a *selection* — which rows count — and a selection made inside a test
// body is a rule with no test of its own. Here it is ordinary code, so the
// one-line unit tests beside it can pin "a slower bare row does not fail the
// gate" and "a slower workload row does" without measuring anything, which is
// the property this package already relies on to keep the gate from rotting
// between releases.
//
// unmeasured comes first and is never folded into the other two: a comparison
// that could not honestly be taken is a failure whatever case it was on, so a
// machine missing half the panel cannot read as green. That includes the
// bare case — not gating means "being slower here does not fail the bar", not
// "this row may quietly go missing".
func Failures(cs []Comparison) (gating, reported, unmeasured []string) {
	for _, c := range cs {
		name := c.Pair.Name + "/" + c.Case.Name
		switch err := c.Unmeasured(); {
		case err != nil:
			unmeasured = append(unmeasured, name+": "+err.Error())
		case c.Passed():
		case c.Gating():
			gating = append(gating, name)
		default:
			reported = append(reported, name)
		}
	}
	return gating, reported, unmeasured
}

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
//
// A case graded net of another (Case.NetOf) is handed that case's comparison
// for the same pair once every sample is in; see linkStarts.
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
		unset := Timing{Wall: time.Duration(1 << 62), CPU: time.Duration(1 << 62)}
		c.Ours.Best, c.Real.Best = unset, unset
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
					*s.err = fmt.Errorf(
						"%s %s answered %q for the %s program where the panel answers %q, so its time would be a measurement of work it did not do",
						side.who,
						s.pair.Name,
						got,
						s.kase.Name,
						s.kase.Answer,
					)
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
					if took.Wall < into.Best.Wall {
						into.Best.Wall = took.Wall
					}
					// A CPU figure of zero is not a very fast run, it is a
					// run the kernel gave no accounting for, and taking it
					// as a minimum would report the fastest possible shell.
					if took.CPU > 0 && took.CPU < into.Best.CPU {
						into.Best.CPU = took.CPU
					}
				}
				record(s.pair.Ours, "ours", s.ours)
				record(s.pair.Real, s.pair.Name, s.real)
			}
		}
	}
	for i := range out {
		c := &out[i]
		clearUnmeasured(&c.Ours)
		clearUnmeasured(&c.Real)
	}
	linkStarts(out)
	return out
}

// linkStarts gives each comparison graded net of another case the same pair's
// comparison on that case, from the same run.
//
// From the same run and not from anywhere else, and that is the whole of why
// the subtraction is honest: the start and the whole run were interleaved on
// one machine in one minute, so a runner that is slow at starting processes
// is slow on both. A copy rather than a pointer into the slice, so nothing a
// caller does to the slice afterwards can change what a row was graded on.
//
// A case whose start is not among the cases measured is left without one, and
// Unmeasured reports it rather than grading the whole time instead.
func linkStarts(cs []Comparison) {
	for i := range cs {
		c := &cs[i]
		if c.Case.NetOf == "" {
			continue
		}
		for j := range cs {
			if j != i && cs[j].Pair.Name == c.Pair.Name && cs[j].Case.Name == c.Case.NetOf {
				start := cs[j]
				c.Start = &start
				break
			}
		}
	}
}

// clearUnmeasured turns the sentinel a minimum starts at back into a zero, so
// that a side nothing was recorded for reports nothing rather than reporting
// a hundred and forty-six years.
//
// Per half rather than per Result, because the CPU half can be the only one
// missing: a kernel that gave no accounting for any of the samples leaves the
// wall minimum real and the CPU minimum untouched.
func clearUnmeasured(r *Result) {
	const sentinel = time.Duration(1 << 62)
	if r.Samples == 0 || r.Best.Wall == sentinel {
		r.Best.Wall = 0
	}
	if r.Samples == 0 || r.Best.CPU == sentinel {
		r.Best.CPU = 0
	}
}

// Report renders a comparison table, and is what a failure quotes.
//
// A number with no reference beside it is not evidence, so ours and the
// original are always printed together with the ratio between them — and the
// load average is printed above, because on a machine with other work on it
// that is the difference between a result and an anecdote.
//
// Both clocks, side by side, because a red row on its own does not say which
// kind of red it is. Where the wall ratio is above 1 and the CPU ratio is
// too, the dialect really is doing more work; where the wall ratio is above 1
// and the CPU ratio is below it, the run was waiting behind something else on
// the machine and the row is about the machine. Nobody reading this table has
// been able to tell those apart before, which is how "perfgate is failing"
// has been passed on second-hand for a week without anybody knowing what it
// meant.
//
// A case graded net of its start carries four more columns: each side's time
// less its own start, the ratio between those, and the same on CPU. The
// verdict on that row is read from the net ratio, and the whole-run ratio
// stays beside it so the subtraction is visible rather than taken on trust.
// A row graded on its whole time prints a dash there. The line above the
// table says which figure each verdict reads, because a table whose verdict
// column disagrees with the ratio next to it reads as a bug otherwise.
func Report(cs []Comparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "load average: %s\n", LoadAverage())
	b.WriteString("graded on: `net` where a row has one (its time less the same shell's own bare start, " +
		"both from this run), `ratio` otherwise\n")
	fmt.Fprintf(&b, "%-10s %-9s %10s %10s %8s %10s %10s %8s %10s %10s %8s %8s %7s  %s\n",
		"dialect", "case", "ours", "original", "ratio", "ours cpu", "orig cpu", "cpu",
		"ours net", "orig net", "net", "net cpu", "samples", "verdict")
	sorted := append([]Comparison(nil), cs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Pair.Name < sorted[j].Pair.Name })
	for _, c := range sorted {
		verdict := "FASTER"
		err := c.Unmeasured()
		switch {
		case err != nil:
			verdict = "NOT MEASURED"
		case c.Passed():
		case c.Gating():
			verdict = "SLOWER"
		default:
			// Still slower, and still worth seeing — but not the bar.
			// Spelled out in the row itself rather than left to a
			// footnote, because the one thing #2813 cost this project
			// was a week of "perfgate is failing" passed on second-hand
			// by people who could not tell which rows meant it.
			verdict = "SLOWER (not gating)"
		}
		fmt.Fprintf(&b, "%-10s %-9s %8.2fms %8.2fms %7.2fx %8.2fms %8.2fms %7.2fx %s %7d  %s\n",
			c.Pair.Name, c.Case.Name,
			ms(c.Ours.Best.Wall), ms(c.Real.Best.Wall), c.Ratio(),
			ms(c.Ours.Best.CPU), ms(c.Real.Best.CPU), c.CPURatio(),
			netColumns(c),
			c.Ours.Samples, verdict)
		if err != nil {
			fmt.Fprintf(&b, "%-10s %-9s   %v\n", "", "", err)
		}
	}
	return b.String()
}

// netColumns is the four net columns of a report row, or dashes where the row
// is graded on its whole time or has no start to subtract.
func netColumns(c Comparison) string {
	ours, real, ok := c.Net()
	if !ok {
		return fmt.Sprintf("%10s %10s %8s %8s", "-", "-", "-", "-")
	}
	return fmt.Sprintf("%8.2fms %8.2fms %7.2fx %7.2fx", ms(ours.Wall), ms(real.Wall), c.NetRatio(), c.NetCPURatio())
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
