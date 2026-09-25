// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Where a process substitution's far end lands in the descriptor table — the
// N of the `/dev/fd/N` the word expands to.
//
// Four shells have the construct and each numbers it differently. Measured
// 2026-09-21 with `echo <(true) <(true) <(true)`, ash through the pinned
// BusyBox image: bash 5.3.20 and 3.2.57 `63 62 61`, zsh 5.9.2 `11 12 13`,
// ksh93 `3 4 5`, BusyBox 1.37.0 `64 65 66`. This shell answered `10 11 12` in
// every dialect. See Semantics.SubstitutionEndPlacement.
//
// # Against the kernel's own answer, not against digits
//
// These numbers come from the kernel's table for the **whole test binary**, so
// a parallel test that opens a file moves them: this case was first written
// asserting the three digits and answered `63 58 62` and `13 15 14` under
// `go test ./interp/`, which is the rule working beside other tests rather
// than the rule being wrong.
//
// What it asserted next was the **region** each rule allocates from — upward
// rules never below their base, the downward one never above 63 — and that
// was a bound on a digit in disguise. `3` and `63` are a statement about how
// many descriptors the host had open, and on the macOS runner, which handed
// the binary some seventy of them, every one of the three answers was in the
// seventies and this case failed for a diff that touched no Go at all
// (#4459).
//
// So the run is compared against **what this rule's own walk finds free**,
// taken from the kernel the way the neighbors below take theirs. That is
// exactly as portable as a region, strictly sharper — a region cannot see a
// rule that lands one number off — and it says the thing the rule says rather
// than a thing about the table. The table is quiet in the first place because
// this case is measured in a child of the test binary that has nothing else in
// it; see measuredInAQuietDescriptorTable, without which the walk has nothing
// free to find and says so.
//
// A retry for the interloper, on the same terms as the neighbors: another test
// may take a number *during* a run, and can only ever make the answer worse,
// so one clean attempt is the claim.
//
// The digits themselves are pinned where the shell is the only thing in the
// process: against the real shells through each built dialect binary, which is
// where the four rows above were measured, and by the bash column of
// `make bash-suite`, which runs a whole file of them.
func TestASubstitutionEndTakesTheDialectsRegion(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	for _, tc := range []struct {
		name  string
		where SubstEndPlacement
		base  DescriptorAllocationBase
		// from and to are the rule's own walk: where it starts looking and
		// the last number it would look at.
		from, to int
	}{
		{
			"the top of the table, down from 63", SubstitutionEndsAtTheTopOfTheTable,
			AllocateDescriptorsFromTen, 63, 10,
		},
		{
			"above the top of the table, up from 64", SubstitutionEndsAboveTheTopOfTheTable,
			AllocateDescriptorsFromTen, 64, 64 + 63,
		},
		{
			"where any descriptor goes, up from the base of 11",
			SubstitutionEndsWhereAnyDescriptorGoes, AllocateDescriptorsFromEleven, 11, 11 + 63,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const attempts = 3
			var got, want []int
			for i := range attempts {
				want = freeFdsAlong(t, tc.from, tc.to, 3)
				got = substNumbers(t, runSubstPlacement(t, tc.where, tc.base, nil,
					"echo <(true) <(true) <(true)", nil))
				if len(got) != 3 {
					t.Fatalf("got %v, want three numbers", got)
				}
				if sameFds(got, want) {
					return
				}
				t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
			}
			t.Errorf("got %v over %d attempts, want %v — the numbers this rule's own "+
				"walk from %d found free", got, attempts, want, tc.from)
		})
	}
}

// A number the script parked before the word expanded is stepped over.
//
// Measured the same day on bash 5.3.20: `exec 63</dev/null; echo <(true)
// <(true)` is `/dev/fd/62 /dev/fd/61`, so the descent skips what is held
// rather than colliding with it or stopping at it. The collision this rules
// out is the one firstProcSubFd was originally set to 10 to avoid — two
// entries on one number in the table childFiles builds, with which of them
// survived decided by map iteration order.
func TestASubstitutionEndStepsOverAParkedNumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil,
		"exec 63</dev/null\necho <(true) <(true)", nil))
	for _, n := range got {
		if n == 63 {
			t.Errorf("got %v, want the parked 63 stepped over", got)
		}
	}
}

// The top of the table gives way to the open-file limit.
//
// Measured 2026-09-21 by sweeping `ulimit -n`, two substitutions in one
// command: bash is `63 62` at every limit of 64 and above and `3 4` at 63 and
// below — so the test is whether 63 is a legal descriptor, the same constant
// and the same condition Semantics.CoprocessEndPlacement records. What it
// gives way *to* is the lowest free number, which is ksh93's rule, so this
// axis's second value is what the third one degrades into rather than an
// invention.
//
// Both halves are asserted, and against the kernel rather than against digits
// for the reason the case above this one gives. The old shape compared the
// answer with the limit it was told about and with the literal 63, and both
// are claims about the host: on the macOS runner, holding seventy descriptors,
// a correct fallback to the lowest free number was in the seventies and read
// as a number "above the top of the table" and "not below the limit of 63"
// (#4459). Where the walk goes is the rule; which digits the walk finds is the
// table, and measuredInAQuietDescriptorTable is what makes the table one the
// walk can be seen in.
func TestTheSubstitutionTopOfTheTableGivesWayToTheOpenFileLimit(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	for _, soft := range []int64{63, 64, 1024, RlimitInfinity} {
		t.Run(strconv.FormatInt(soft, 10), func(t *testing.T) {
			limit := func(Resource) (int64, int64, error) { return soft, soft, nil }
			// The descent is walked only where its top is a legal descriptor.
			// Below that the rule gives way to the walk up from the first
			// number above stdio, which is what every other dialect does.
			from, to := 63, 10
			if soft != RlimitInfinity && soft <= 63 {
				from, to = 3, 3+63
			}
			const attempts = 3
			var got, want []int
			for i := range attempts {
				want = freeFdsAlong(t, from, to, 1)
				got = substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
					AllocateDescriptorsFromTen, limit, "echo <(true)", nil))
				if len(got) != 1 {
					t.Fatalf("got %v, want one number", got)
				}
				if sameFds(got, want) {
					return
				}
				t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
			}
			t.Errorf("got %v over %d attempts, want %v — the first number the walk "+
				"from %d found free under a limit of %d", got, attempts, want, from, soft)
		})
	}
}

// A wish the kernel refuses is a wish that missed, not the end of the search.
//
// This is the half a list of preferred numbers gets wrong by default. Asking
// for 63 under a low `ulimit -n` is EMFILE or EINVAL rather than a number,
// and a search that reported that error would turn a construct every shell
// answers into a diagnostic: measured while writing this, `ulimit -n 64` and
// a substitution was `too many open files` at 1 where bash prints a path and
// carries on.
func TestARefusedSubstitutionNumberIsNotTheEndOfTheSearch(t *testing.T) {
	limit := func(Resource) (int64, int64, error) { return 64, 64, nil }
	out := runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, limit, "echo <(true)", nil)
	if !strings.HasPrefix(out, "/dev/fd/") {
		t.Errorf("got %q, want a substitution path", out)
	}
}

// substNumbers is the descriptor numbers of a run's substitution paths.
func substNumbers(t *testing.T, out string) []int {
	t.Helper()
	var ns []int
	for _, f := range strings.Fields(strings.TrimSpace(out)) {
		n, err := strconv.Atoi(strings.TrimPrefix(f, "/dev/fd/"))
		if err != nil {
			t.Fatalf("not a substitution path: %q", f)
		}
		ns = append(ns, n)
	}
	return ns
}

// runSubstPlacement runs src with process substitution on, under the given
// placement and allocation base, optionally under an embedder-supplied
// open-file limit.
//
// Multi-digit descriptor numbers are on because one case writes `exec
// 63</dev/null`, which is the only way for a script to reach a number the
// shell would otherwise pick for itself.
// stdin is the shell's input, and nil leaves it unset. It is a parameter
// because whether it is a **real descriptor** decides whether one of these
// cases can see anything at all: ownDescriptors gives a concurrent body its
// own copy of every *os.File in the table, and with no file there is nothing
// to copy. See TestABodysOwnCopyOfTheTableIsNotInTheAnswer.
func runSubstPlacement(t *testing.T, where SubstEndPlacement,
	base DescriptorAllocationBase, limit func(Resource) (int64, int64, error), src string,
	stdin io.Reader,
) string {
	t.Helper()
	d := syntax.Core()
	d.ProcessSubstitution = true
	d.FdVariableRedirections = true
	d.MultiDigitFdNumber = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.SubstitutionEndPlacement = where
	sem.FirstAllocatedDescriptor = base
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdin: stdin, Stdout: out, Stderr: &strings.Builder{}, GetRlimit: limit,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// Several live substitutions take *consecutive* numbers, starting at the
// lowest number that was actually free — because the shell's own plumbing is
// not in the answer.
//
// `os.Pipe` hands out the two lowest free numbers, so before #4120 each
// substitution's own pipe and its shell-side end sat inside the region the
// published number comes from and the next substitution had to step over
// them. ksh93 answers `echo <(true) <(true) <(true)` with `/dev/fd/3
// /dev/fd/4 /dev/fd/5`; this shell answered `/dev/fd/6 /dev/fd/9 /dev/fd/12`,
// two or three apart. The rule was right the whole time and the floor was our
// own descriptors. See raiseShellEnd and reparkPreferred.
//
// # Against the kernel's own answer, not against digits
//
// The numbers come from the table for the whole test binary, so a parallel
// test holding a descriptor moves them and this file's neighbors assert
// regions for that reason. A *region* cannot see this fault at all, though —
// every reading below is inside the region — so what is compared instead is
// the run against **what the kernel would answer for the first park**, taken
// here by asking for a descriptor and giving it straight back. That is
// exactly as portable as a region and strictly sharper:
//
//	this shell          4 5 6    the three numbers that were free
//	without the raise   4 6 8    each shell-side end stepped over
//	without the repark  5 6 7    the pipe's own original held the best one
//	without either      6 8 10   both, which is the fault as filed
//
// Measured in this harness on 2026-09-21, which is also why a span test was
// no good: the first two mutations span 4 against the true span of 2, and a
// bound loose enough to be stable let both through. A retry covers the
// interloper case instead — another test may take a number *during* a run,
// and can only ever make the answer worse, so one clean attempt is the claim.
func TestSeveralSubstitutionEndsTakeConsecutiveNumbers(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	const attempts = 3
	var got, want []int
	for i := range attempts {
		want = lowestFreeFds(t, 3)
		got = substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
			AllocateDescriptorsFromTen, nil, "echo <(true) <(true) <(true)", nil))
		if len(got) != 3 {
			t.Fatalf("got %v, want three numbers", got)
		}
		if sameFds(got, want) {
			return
		}
		t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
	}
	t.Errorf("got %v over %d attempts, want %v — the numbers that were free, which is "+
		"what the rule allocates from; this shell's own pipe ends are being counted "+
		"in the answer", got, attempts, want)
}

// lowestFreeFds is the n numbers the kernel would hand out next, asked for
// and given straight back.
//
// F_DUPFD answers with the lowest free number at or above the one it is
// given, which is the same question a substitution's park asks — so these are
// the run's expected answers rather than a guess at them. Asked for all n at
// once and released together, because that is what a run holding n live
// substitutions does.
//
// **A set and not a first number plus one each time**, which is what the
// first version of this was and what failed on Linux. The low numbers are not
// a contiguous free run: `go test ./interp/` does not import driver, so
// nothing has kept the Go runtime off them, and on Linux its epoll descriptor
// and eventfd sit at 11 and 12 for the life of the binary. The correct answer
// there is `10 13 14`, and an assertion of "10, 11, 12" called it a defect.
func lowestFreeFds(t *testing.T, n int) []int {
	t.Helper()
	var got []int
	for range n {
		fd, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(syscall.Stdin),
			uintptr(syscall.F_DUPFD_CLOEXEC), uintptr(3))
		if errno != 0 {
			t.Fatalf("asking for a descriptor: %v", errno)
		}
		got = append(got, int(fd))
	}
	for _, fd := range got {
		_ = syscall.Close(fd)
	}
	return got
}

// freeFdsAlong is the first n numbers a walk from `from` towards `to` finds
// free — the same question a rule's wish list asks, asked of the kernel.
//
// The twin of lowestFreeFds, and here because that one cannot express a walk
// that goes *down*: F_DUPFD answers with the lowest free number at or above
// the one it is given, which is the upward rules' question and the opposite of
// the descent from the top of the table. So this one reads the table rather
// than taking from it — see fdIsOpen — which also means it holds nothing while
// it looks, and n numbers come back in the order the rule would meet them.
//
// Running out is a failure and not an empty answer. A walk that finds fewer
// than n free numbers is a walk in a table this rule cannot be seen in, which
// is exactly the state #4459 was reported from; saying so names the cause
// rather than leaving a case to fail on digits further down.
func freeFdsAlong(t *testing.T, from, to, n int) []int {
	t.Helper()
	step := 1
	if to < from {
		step = -1
	}
	var got []int
	for fd := from; len(got) < n; fd += step {
		if !fdIsOpen(fd) {
			got = append(got, fd)
		}
		if fd == to {
			break
		}
	}
	if len(got) != n {
		t.Fatalf("only %v free between %d and %d, want %d numbers — this process was "+
			"handed a table this rule cannot be seen in", got, from, to, n)
	}
	return got
}

// sameFds reports whether a run's numbers are the ones that were free.
func sameFds(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// And the tidy-up gives way to the open-file limit without giving up the
// number.
//
// The floor is above everything the dialect could publish, which under a low
// `ulimit -n` is past the end of the table — so the duplicate is refused and
// the shell's end stays where os.Pipe put it, in the way. bash answers
// `/dev/fd/3` at `ulimit -n 8` and this shell answered `/dev/fd/5`.
//
// **The re-park is what recovers it, and nothing else had to be built.** A
// descending list of floors bounded by the limit was written first, on the
// reasoning that a refused tidy-up leaves the end crowding the answer; it
// changed no measurement anywhere, because closing the pipe's own original
// frees the number the shell end is sitting next to and the second pass then
// takes it. It was deleted rather than kept as an untested path — the
// mutation that disabled it failed nothing, which is the whole argument.
//
// Against the kernel's own answer for the same reason the case above is, and
// this one learned it the hard way: written against the digit 3 with a slot
// of tolerance it passed alone and failed inside `go test ./interp/`, where
// the other cases hold descriptors and the lowest free number under a limit
// of 8 was 6. A bound on a digit is a bound on what else the binary happens
// to have open.
func TestTheShellEndIsTidiedAwayUnderALowOpenFileLimit(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	limit := func(Resource) (int64, int64, error) { return 8, 8, nil }
	const attempts = 3
	var got []int
	for i := range attempts {
		want := lowestFreeFds(t, 1)
		got = substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
			AllocateDescriptorsFromTen, limit, "echo <(true)", nil))
		if len(got) != 1 {
			t.Fatalf("got %v, want one number", got)
		}
		if sameFds(got, want) {
			return
		}
		t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
	}
	t.Errorf("got %v over %d attempts, want the lowest free number under the limit — "+
		"the shell's own end is still holding one", got, attempts)
}

// A body's own copy of the descriptor table is not in the answer either.
//
// `ownDescriptors` gives a shell that runs *beside* this one its own copy of
// every descriptor in the table — the half of a fork a goroutine does not get
// — and it took them with `syscall.Dup`, which answers with the lowest free
// number. So each substitution body left a descriptor sitting in the region
// the next substitution's number comes from: measured 2026-09-21, one per
// body at 4 and then 6, which is what left ksh93's `3 4 5` reading `3 5 7`
// after the pipe's own two ends had been moved out of the way. `dupFile`
// takes Runner.privateFdFloor now, which is the same floor by the same
// derivation.
//
// **The input has to be a real file for this case to see anything.** The copy
// is of every *os.File in the table and there is no descriptor behind a
// strings.Reader, so with the harness's usual input nothing is duplicated and
// the case passes however `dupFile` is written — which is exactly what it did
// while being written, and is why the input is a parameter.
func TestABodysOwnCopyOfTheTableIsNotInTheAnswer(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	const attempts = 3
	var got, want []int
	for i := range attempts {
		want = lowestFreeFds(t, 3)
		got = substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
			AllocateDescriptorsFromTen, nil, "echo <(true) <(true) <(true)", f))
		if len(got) != 3 {
			t.Fatalf("got %v, want three numbers", got)
		}
		if sameFds(got, want) {
			return
		}
		t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
	}
	t.Errorf("got %v over %d attempts, want %v — a body's private copy of the table "+
		"is being counted in the answer", got, attempts, want)
}

// A substitution nested inside another one's body takes the number the
// enclosing one published — not the number below it.
//
// Measured 2026-09-21 on bash 5.3.20: `cat <(echo <(true))` is `/dev/fd/63`,
// the outer's own number handed out a second time. bash runs the body in a
// **fork** and closes the outer end there, so the number is genuinely free by
// the time the inner one asks. A body here is a clone of the Runner in the one
// process there is, so the outer end is still parked in the real table and the
// descent stepped over it to 62 — one differing line per nested substitution
// (#4119). See substFdView, which is what releases the number instead.
//
// # Against the shell's own flat answer, not against digits
//
// The numbers come from the table for the whole test binary, so the neighbors
// in this file assert regions — and a region cannot see this fault, which is
// one number wide. What is compared instead is the **same script's flat
// answer**: a substitution of its own on the line before, whose pipe is gone
// again by the time the nested one runs, so the two must land on the same
// number whatever number that is. Before the fix the second line was one
// below the first; a parallel test taking a descriptor moves both together.
func TestANestedSubstitutionTakesTheEnclosingNumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil,
		"echo <(true)\nread line < <(echo <(true))\necho \"$line\"", nil))
	if len(got) != 2 {
		t.Fatalf("got %v, want two numbers", got)
	}
	if got[0] != got[1] {
		t.Errorf("got %v, want the nested substitution on the same number as the flat "+
			"one — the enclosing end is being stepped over rather than released", got)
	}
}

// And the shell's own open of that number reaches the pipe it named.
//
// The other half of the release, and the half that is not cosmetic. A nested
// substitution publishes a number the enclosing pipe is *still parked on* in
// this process, because a clone is not a fork: commands get the published
// number through the table childFiles builds, but an open done by the shell
// itself — the redirection of `read x < <(cmd)` — goes through the real
// table and would reach the enclosing pipe. See Runner.substOpenPath.
//
// Mutation-proven 2026-09-22: with the mapping removed this script does not
// finish at all, because the shell reads the outer pipe and nothing is ever
// going to write the line it is waiting for. So it is run on a goroutine
// under a deadline, and a regression is a failure rather than a hang.
func TestTheShellsOwnOpenOfANestedSubstitutionReachesIt(t *testing.T) {
	const src = "read outer < <(read inner < <(echo deep); echo \"[$inner]\")\necho \"$outer\""
	d := syntax.Core()
	d.ProcessSubstitution = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.SubstitutionEndPlacement = SubstitutionEndsAtTheTopOfTheTable
	sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: &strings.Builder{},
	})
	// The run on a goroutine and the assertions on this one: t.Fatal belongs
	// to the goroutine running the test, and the buffer is read only after
	// the channel says the run is over.
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), f)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != "[deep]" {
			t.Errorf("got %q, want %q — the shell opened the published number and "+
				"reached the enclosing pipe", got, "[deep]")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the run did not finish: the shell is reading a pipe nobody is going " +
			"to write, which is the enclosing substitution's rather than its own")
	}
}
