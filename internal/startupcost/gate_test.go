// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/startupcost"
)

// The performance gate, and the tests that prove it can bite.
//
// #1403 is the requirement: "performance is a must requirement before 0.0.0,
// no dialect can be slower than the original", and "our zsh MUST be faster
// than zsh". Faster, not comparable. The benchmarks in this package report;
// this one fails.

// gateVariable is what turns the gate on.
//
// A separate target rather than part of `go test ./...`, and the reason is the
// same one `make startup` is a separate target for: this spawns several
// thousand processes, takes minutes, and is the one measurement in the tree
// whose answer depends on what else the machine is doing. Wiring it into the
// unit lane would make every unrelated change's build depend on the load
// average, which is how a gate gets deleted.
//
// What makes it a gate rather than a report is that it *fails*, and what keeps
// it honest when nobody is looking is that the three tests below it always
// run: they prove a slower dialect is caught, a refusing one is not scored as
// a pass, and the answer the workload is checked against is the panel's. Those
// need no measurement and cost no time, so the machinery cannot rot unnoticed
// between releases.
const gateVariable = "SH_PERFGATE"

// TestNoDialectIsSlowerThanItsOriginal is the release gate for v0.0.0.
func TestNoDialectIsSlowerThanItsOriginal(t *testing.T) {
	if os.Getenv(gateVariable) == "" {
		t.Skipf("the performance gate is off; run `make perfgate` (%s=1) to measure it", gateVariable)
	}
	dir := buildDirFor(t)
	pairs := startupcost.Pairs(dir)
	// Enough samples that the minimum is a clean spawn rather than a lucky
	// one, and few enough that the whole gate is minutes and not hours.
	// Spread over batches so the minimum is not drawn from one contiguous
	// window of the machine's day; see startupcost.Measure.
	const batches, per = 5, 20
	got := startupcost.Measure(pairs, startupcost.Cases(), batches, per)
	report := startupcost.Report(got)
	t.Logf("\n%s", report)

	slower, notGating, unmeasured := startupcost.Failures(got)
	// An unmeasured comparison is not a pass. A reference shell that is not
	// installed is the only acceptable form of it, and it is reported rather
	// than swallowed so that a gate run on a machine missing half the panel
	// cannot read as a green one.
	for _, u := range unmeasured {
		t.Errorf("not measured, so not passed: %s", u)
	}
	// Logged and not failed. These rows lost, and the reason they do not stop
	// a release is in Cases: the bare case's floor on Linux is the Go
	// runtime's process start, which is not this repository's to win. Printed
	// every run anyway, because a regression in what every subshell pays is
	// worth a reader's attention even when it is not a gate.
	if len(notGating) > 0 {
		t.Logf("slower, reported but not gating (see startupcost.Cases and #2813): %s",
			strings.Join(notGating, ", "))
	}
	if len(slower) > 0 {
		t.Errorf("these dialects are slower than the shell they claim to be on the gating case, which #1403 does not allow before v0.0.0: %s\n%s",
			strings.Join(slower, ", "), report)
	}
}

// TestTheBareCaseIsMeasuredButDoesNotGate is #2813's decision made executable.
//
// The bar changed shape rather than loosening, and the difference between
// those two is exactly one property: a slower *workload* still fails. So both
// halves are asserted here, on hand-built comparisons that cost no
// measurement — the same reason the rest of the guards in this file run in
// the unit lane. A version of this test that only checked the bare half would
// go on passing if Gating were deleted and nothing gated at all.
func TestTheBareCaseIsMeasuredButDoesNotGate(t *testing.T) {
	t.Parallel()
	lost := func(c startupcost.Case) startupcost.Comparison {
		return startupcost.Comparison{
			Pair: startupcost.Pair{Name: "behind"},
			Case: c,
			Ours: startupcost.Result{Best: at(4 * time.Millisecond), Samples: 4},
			Real: startupcost.Result{Best: at(2 * time.Millisecond), Samples: 4},
		}
	}
	var bare, workload startupcost.Case
	for _, c := range startupcost.Cases() {
		switch c.Name {
		case "bare":
			bare = c
		case "workload":
			workload = c
		}
	}
	if bare.Name == "" || workload.Name == "" {
		t.Fatalf("Cases() no longer offers both a bare and a workload case: %+v", startupcost.Cases())
	}

	gating, reported, unmeasured := startupcost.Failures([]startupcost.Comparison{lost(bare)})
	if len(gating) != 0 {
		t.Errorf("a slower bare row failed the release gate: %v — #2813 settled that the bare floor on Linux is the Go runtime's process start, not this repository's to win", gating)
	}
	if len(reported) != 1 {
		t.Errorf("a slower bare row was not reported at all (%v); not gating means it does not stop a release, not that it may go missing", reported)
	}
	if len(unmeasured) != 0 {
		t.Errorf("a measured row was counted as unmeasured: %v", unmeasured)
	}

	gating, reported, _ = startupcost.Failures([]startupcost.Comparison{lost(workload)})
	if len(gating) != 1 {
		t.Errorf("a slower workload row did not fail the release gate (%v), so the bar has no teeth left: the workload is interpreter throughput and is the half #1403 still asks us to win", gating)
	}
	if len(reported) != 0 {
		t.Errorf("a slower workload row was demoted to a mere report: %v", reported)
	}

	// The verdict a reader sees has to separate them too. A bare row printing
	// a bare "SLOWER" is what let "perfgate is failing" travel for a week.
	bareRow := startupcost.Report([]startupcost.Comparison{lost(bare)})
	if !strings.Contains(bareRow, "SLOWER (not gating)") {
		t.Errorf("the report does not mark a slower bare row as non-gating, so it reads as a release blocker:\n%s", bareRow)
	}
	workRow := startupcost.Report([]startupcost.Comparison{lost(workload)})
	if strings.Contains(workRow, "not gating") {
		t.Errorf("the report marked a slower workload row as non-gating, which is the one row that does gate:\n%s", workRow)
	}
}

// TestAnUnmeasuredBareRowStillFails is the hole the change above could have
// opened.
//
// "The bare case does not gate" is about a row that was measured and lost. A
// bare row that could not be measured is a different animal — a reference
// shell missing, a refusal, an answer that was not the panel's — and it has
// to keep failing, or a machine with no bash on it reads as green on half the
// table. Failures returns it in its own bucket for that reason, and this says
// so.
func TestAnUnmeasuredBareRowStillFails(t *testing.T) {
	t.Parallel()
	c := startupcost.Comparison{
		Pair: startupcost.Pair{Name: "no-reference"},
		Case: startupcost.Case{Name: "bare", Gating: false},
		Err:  errors.New("the reference shell is not installed here"),
	}
	gating, reported, unmeasured := startupcost.Failures([]startupcost.Comparison{c})
	if len(unmeasured) != 1 {
		t.Errorf("an unmeasurable bare row was not reported as unmeasured (%v); not gating must not swallow a measurement that never happened", unmeasured)
	}
	if len(gating) != 0 || len(reported) != 0 {
		t.Errorf("an unmeasurable row was also counted as a timing loss: gating=%v reported=%v", gating, reported)
	}
}

// TestTheGateCatchesADialectThatIsSlower is what says the gate is wired the
// right way round.
//
// A guard nothing has ever tripped is a guard nobody knows the shape of, and
// this package has the scar: a benchmark whose shell *refused* the construct
// it was timing reported no regression at all, three separate times
// (docs/design/startup.md). So the failure is manufactured here on purpose.
//
// The slow half is a **wrapper script** that burns a fixed amount of time and
// then execs the reference shell with the same arguments, so it is the same
// shell reached by a slower road: the answer is identical and only the time
// differs. That needs no second shell installed to make the point.
//
// The first draft of this test tried to make one side slower by giving it a
// heavier *program*, and it could not work: Measure hands both halves of a
// pair the same Case.Program, by construction, because a comparison between
// two different programs is not a comparison. It came out at 1.00x — 54.70ms
// against 54.97ms, both sides running the heavy loop — and failed three runs
// in four, which is a flaky test manufacturing false kills rather than a gate
// being checked. What is slow here has to be the subject.
func TestTheGateCatchesADialectThatIsSlower(t *testing.T) {
	t.Parallel()
	real, ok := anyReference()
	if !ok {
		t.Skip("no reference shell here to compare against itself")
	}
	slow := real
	slow.Path = slowWrapper(t, real.Path)
	pair := startupcost.Pair{Name: "deliberately-slow", Ours: slow, Real: real}
	kase := startupcost.Case{Name: "workload", Program: workloadProgramFor(t), Answer: workloadAnswerFor(t)}
	got := startupcost.Measure([]startupcost.Pair{pair}, []startupcost.Case{kase}, 1, 3)
	if len(got) != 1 {
		t.Fatalf("measured %d comparisons, want 1", len(got))
	}
	if got[0].Err != nil {
		t.Fatalf("the deliberately slow subject could not be measured: %v", got[0].Err)
	}
	if got[0].Passed() {
		t.Errorf("a shell reached by a road that burns tens of milliseconds first was reported as faster than the same shell (%.2fx), so the gate cannot catch a slow dialect:\n%s",
			got[0].Ratio(), startupcost.Report(got))
	}
}

// slowWrapper is a shell that is the given shell plus a fixed delay.
//
// A busy loop rather than `sleep`, because what is being manufactured is a
// slow *start* and sleep is the one thing a scheduler is free to return from
// early. The count is large enough that the delay is tens of milliseconds —
// an order of magnitude above the spread this package sees at load 30 — so
// three samples settle it and the test does not become the flaky thing it is
// checking for.
//
// It ends in `exec`, so the process the gate times really is the reference
// shell answering the reference shell's answer; nothing about the output can
// differ.
func slowWrapper(t *testing.T, real string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slow-shell")
	body := "#!/bin/sh\ni=0\nwhile [ $i -lt 40000 ]; do i=$((i + 1)); done\nexec " + real + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestAComparisonThatCouldNotBeMeasuredNeverPasses is the assertion the
// refusal test below cannot make.
//
// It is here because the obvious probe does not discriminate. When Measure
// gives up on a subject it leaves that side with no samples and a zero time,
// so `ours < original` is `0 < 0` and Passed answers false whether or not it
// consults Err — which means the refusal test would go on passing with the
// Err check deleted. The dangerous case is a subject that answered for a
// while and then stopped: real times on both sides, a real Err, and a
// comparison that would read as a pass. Built by hand, because Measure cannot
// be made to produce it on demand.
func TestAComparisonThatCouldNotBeMeasuredNeverPasses(t *testing.T) {
	t.Parallel()
	c := startupcost.Comparison{
		Pair: startupcost.Pair{Name: "stopped-halfway"},
		Case: startupcost.Case{Name: "workload"},
		Ours: startupcost.Result{Best: at(time.Millisecond), Samples: 4},
		Real: startupcost.Result{Best: at(10 * time.Millisecond), Samples: 4},
		Err:  errors.New("stopped answering partway through"),
	}
	if c.Passed() {
		t.Error("a comparison carrying an error was reported as a pass on the strength of its times, so a shell that stopped working halfway through would clear the gate")
	}
	if !strings.Contains(startupcost.Report([]startupcost.Comparison{c}), "NOT MEASURED") {
		t.Error("the report does not mark a comparison that carries an error as unmeasured")
	}
}

// TestATieIsNotAPass pins the one word the requirement turns on.
//
// "No dialect can be slower than the original" and "our zsh MUST be faster
// than zsh" — faster, not comparable. So the comparison is strict, with no
// tolerance and no epsilon, and this is what says so: a `<=` in Passed would
// let a dialect that merely drew level clear a gate that asks it to win.
//
// Not hypothetical. Measured the day this landed, ours-zsh came in at 4.26ms
// against real zsh's 4.28ms on the bare case, which is a tie dressed as a
// 1.00x win — exactly the reading this refuses, and the reason the figure was
// reported as a tie rather than claimed.
func TestATieIsNotAPass(t *testing.T) {
	t.Parallel()
	tie := func(ours, real time.Duration) startupcost.Comparison {
		return startupcost.Comparison{
			Pair: startupcost.Pair{Name: "level"},
			Case: startupcost.Case{Name: "bare"},
			Ours: startupcost.Result{Best: at(ours), Samples: 4},
			Real: startupcost.Result{Best: at(real), Samples: 4},
		}
	}
	if tie(3*time.Millisecond, 3*time.Millisecond).Passed() {
		t.Error("a dialect exactly level with the shell it claims to be was reported as faster; the requirement is faster, not comparable")
	}
	// The neighbors, so that this pins the boundary rather than merely
	// disliking equality: one nanosecond either side has to answer.
	if !tie(3*time.Millisecond-1, 3*time.Millisecond).Passed() {
		t.Error("a dialect one nanosecond faster than the original did not pass, so the comparison is not strict but backwards")
	}
	if tie(3*time.Millisecond+1, 3*time.Millisecond).Passed() {
		t.Error("a dialect one nanosecond slower than the original passed")
	}
}

// at is a Timing whose two clocks agree, for the tests that are about the
// comparison rather than about which clock it reads.
//
// CPU set as well as wall rather than left zero, deliberately: a zero CPU
// figure means "the kernel gave no accounting for this run" elsewhere in the
// package, and a fixture that quietly relied on that would be asserting
// something other than what it says.
func at(d time.Duration) startupcost.Timing { return startupcost.Timing{Wall: d, CPU: d} }

// TestTheReportSaysWhetherARedRowIsWorkOrLoad is what makes the CPU column
// worth printing rather than merely present.
//
// The two kinds of red are the whole reason this package was hard to read: a
// dialect that really does more work, and a run that spent its time waiting
// behind something else on the machine. #1403's closing run recorded zsh/bare
// as FASTER at load 26 and SLOWER at load 60 on the same tree, which is the
// second kind wearing the first kind's verdict — and "perfgate is failing"
// was then passed on for a week with nobody able to say which it was.
//
// Both rows here are SLOWER on the wall clock. What separates them is the CPU
// ratio, so the assertion is that the report carries it.
func TestTheReportSaysWhetherARedRowIsWorkOrLoad(t *testing.T) {
	t.Parallel()
	row := func(name string, ours, real startupcost.Timing) startupcost.Comparison {
		return startupcost.Comparison{
			Pair: startupcost.Pair{Name: name},
			Case: startupcost.Case{Name: "bare"},
			Ours: startupcost.Result{Best: ours, Samples: 4},
			Real: startupcost.Result{Best: real, Samples: 4},
		}
	}
	doesMoreWork := row("works-harder",
		startupcost.Timing{Wall: 4 * time.Millisecond, CPU: 4 * time.Millisecond},
		startupcost.Timing{Wall: 2 * time.Millisecond, CPU: 2 * time.Millisecond})
	waitedBehindTheMachine := row("waited",
		startupcost.Timing{Wall: 4 * time.Millisecond, CPU: time.Millisecond},
		startupcost.Timing{Wall: 2 * time.Millisecond, CPU: 2 * time.Millisecond})

	if doesMoreWork.Passed() || waitedBehindTheMachine.Passed() {
		t.Fatal("both rows are meant to be slower on the wall clock, which is what makes the CPU column the thing that tells them apart")
	}
	if got := doesMoreWork.CPURatio(); got != 2 {
		t.Errorf("a dialect burning twice the CPU reports a CPU ratio of %.2f, want 2.00", got)
	}
	if got := waitedBehindTheMachine.CPURatio(); got != 0.5 {
		t.Errorf("a dialect burning half the CPU reports a CPU ratio of %.2f, want 0.50", got)
	}
	report := startupcost.Report([]startupcost.Comparison{doesMoreWork, waitedBehindTheMachine})
	for _, want := range []string{"2.00x", "0.50x", "ours cpu", "orig cpu"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not carry %q, so a reader cannot tell a slow dialect from a busy machine:\n%s", want, report)
		}
	}
}

// TestAMissingCPUAccountingIsNotTheFastestShellInTheTable is the same refusal
// the rest of this package makes about a shell that will not do the work.
//
// A run the kernel gave no accounting for comes back with a CPU of zero, and
// zero is the smallest number there is: taken as a sample it would become the
// minimum and the dialect would be reported as costing nothing at all. That
// is the failure this package has already been caught making three times in
// another guise (docs/design/startup.md), so the ratio has to answer 0 —
// "nothing to compare" — rather than a very good number.
func TestAMissingCPUAccountingIsNotTheFastestShellInTheTable(t *testing.T) {
	t.Parallel()
	c := startupcost.Comparison{
		Pair: startupcost.Pair{Name: "unaccounted"},
		Case: startupcost.Case{Name: "bare"},
		Ours: startupcost.Result{Best: startupcost.Timing{Wall: time.Millisecond}, Samples: 4},
		Real: startupcost.Result{Best: startupcost.Timing{Wall: 2 * time.Millisecond}, Samples: 4},
	}
	if got := c.CPURatio(); got != 0 {
		t.Errorf("a comparison with no CPU accounting on either side reports a CPU ratio of %.2f, want 0 for `nothing to compare`", got)
	}
}

// TestTheGateRefusesAShellThatWillNotDoTheWork is the other half, and it is
// the one the history of this package demands.
//
// A shell that cannot run the program is the fastest shell in any table. Here
// the subject exits without running anything, which is exactly the shape of a
// refused construct: the clock is perfectly happy and the answer never comes.
// The gate must report that as unmeasured, and Passed must be false for it
// whatever the times say.
func TestTheGateRefusesAShellThatWillNotDoTheWork(t *testing.T) {
	t.Parallel()
	real, ok := anyReference()
	if !ok {
		t.Skip("no reference shell here to compare against")
	}
	if _, err := os.Stat("/usr/bin/true"); err != nil {
		t.Skipf("no /usr/bin/true here: %v", err)
	}
	// `true` takes the arguments and ignores them, which is a shell that
	// accepted `-c` and ran nothing — and it starts faster than any shell.
	refusing := startupcost.Subject{Path: "/usr/bin/true", CommandArgs: []string{"-c"}}
	pair := startupcost.Pair{Name: "will-not-work", Ours: refusing, Real: real}
	got := startupcost.Measure([]startupcost.Pair{pair}, []startupcost.Case{{
		Name:    "workload",
		Program: workloadProgramFor(t),
		Answer:  workloadAnswerFor(t),
	}}, 1, 2)
	if len(got) != 1 {
		t.Fatalf("measured %d comparisons, want 1", len(got))
	}
	if got[0].Err == nil {
		t.Fatalf("a program that ran nothing was measured rather than refused, so the gate can be passed by not working:\n%s",
			startupcost.Report(got))
	}
	if got[0].Passed() {
		t.Error("a comparison that could not be measured was reported as a pass")
	}
	if !strings.Contains(startupcost.Report(got), "NOT MEASURED") {
		t.Errorf("the report does not say the comparison was not measured:\n%s", startupcost.Report(got))
	}
}

// TestTheWorkloadAnswerIsThePanelsOwn keeps the gate's checked answer honest.
//
// The workload's whole value is that a refusal cannot pass it, and that rests
// entirely on the expected answer being what the real shells actually print.
// An answer that had drifted — or been updated to match ours — would turn the
// check into a tautology, so it is verified against every reference shell
// installed here rather than trusted.
func TestTheWorkloadAnswerIsThePanelsOwn(t *testing.T) {
	t.Parallel()
	var checked int
	for _, p := range startupcost.Pairs(t.TempDir()) {
		if p.Real.Path == "" {
			continue
		}
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			_, got, err := startupcost.RunProgram(p.Real, workloadProgramFor(t))
			if err != nil {
				t.Fatalf("%s refused the gate's workload: %v", p.Name, err)
			}
			if got != workloadAnswerFor(t) {
				t.Errorf("%s answers %q for the gate's workload where the gate expects %q, so the gate is checking against the wrong answer",
					p.Name, got, workloadAnswerFor(t))
			}
		})
		checked++
	}
	if checked == 0 {
		t.Skip("no reference shells here to check the answer against")
	}
}

// TestEveryDialectHasAReferenceToBeGradedAgainst keeps the gate from quietly
// covering fewer dialects than it claims.
//
// The four binaries this repository builds are the four the requirement names.
// A pair that lost its reference — a renamed path, a dialect added without one
// — would be skipped by Measure with an Err, which the gate reports; but a
// dialect missing from Pairs altogether would be silently ungraded, and
// nothing else in the tree would notice.
func TestEveryDialectHasAReferenceToBeGradedAgainst(t *testing.T) {
	t.Parallel()
	want := map[string]bool{"bash": false, "zsh": false, "ksh": false, "dash": false}
	for _, p := range startupcost.Pairs("/nonexistent") {
		if _, ok := want[p.Name]; !ok {
			t.Errorf("the gate grades %q, which is not one of the four dialects", p.Name)
			continue
		}
		want[p.Name] = true
		if p.Ours.Path == "" {
			t.Errorf("%s has no binary of ours to grade", p.Name)
		}
		if len(p.Ours.CommandArgs) == 0 {
			t.Errorf("%s has no `-c` route, so the gate cannot time it", p.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s is not graded by the performance gate at all", name)
		}
	}
}

// TestTheGateHasBothCases guards the pair of programs.
//
// The bare case cannot check its own work and the workload can; a gate left
// with only the first would be back to timing whatever a shell felt like
// doing, which is the trap docs/design/startup.md records three instances of.
func TestTheGateHasBothCases(t *testing.T) {
	t.Parallel()
	cases := startupcost.Cases()
	if len(cases) != 2 {
		t.Fatalf("the gate has %d cases, want a bare one and a checked workload", len(cases))
	}
	var checked int
	for _, c := range cases {
		if c.Program == "" {
			t.Errorf("case %q has no program", c.Name)
		}
		if c.Answer != "" {
			checked++
		}
	}
	if checked == 0 {
		t.Error("no case checks what it printed, so the gate would score a refusal as the fastest shell in the table")
	}
}

// anyReference is a reference shell installed on this machine, for the tests
// that need *a* real shell rather than a particular one.
func anyReference() (startupcost.Subject, bool) {
	for _, p := range startupcost.Pairs("/nonexistent") {
		if p.Real.Path != "" {
			return p.Real, true
		}
	}
	return startupcost.Subject{}, false
}

// workloadProgramFor and workloadAnswerFor read the gate's own program and
// answer back out of it, so that these tests cannot drift from what the gate
// actually runs. Taken from Cases rather than copied, because a copy is how a
// test comes to prove something about text nothing uses.
func workloadProgramFor(tb testing.TB) string {
	tb.Helper()
	for _, c := range startupcost.Cases() {
		if c.Answer != "" {
			return c.Program
		}
	}
	tb.Fatal("the gate has no case that checks its answer")
	return ""
}

func workloadAnswerFor(tb testing.TB) string {
	tb.Helper()
	for _, c := range startupcost.Cases() {
		if c.Answer != "" {
			return c.Answer
		}
	}
	tb.Fatal("the gate has no case that checks its answer")
	return ""
}
