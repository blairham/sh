// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import "regexp"

// The second way a reference disagrees with its own second run, and the
// second thing that costs a file its score outright.
//
// reorder.go has the first: an associative array's key order, which is that
// shell's hash table and not a fact about either shell. This one is smaller
// and blunter. A shell told to be interactive with no terminal anywhere
// reports the process group it could not hand the terminal to, **by number**
// — and the number is a different one every run. Measured 2026-09-21 over
// all 83 files of the fetched bash column, one pair of reference runs each:
// `history.tests` is the only file in the whole suite that will not
// reproduce its own run, it differs in exactly seven lines, and all seven
// are that number. It is a 410-line file with 225 differing lines at a
// matching status of 0 — the largest such row in the run — and until this it
// scored nothing at all, so it was absent from every ranking #2298's passes
// were picked from rather than being at the top of one (#2298).
//
// # Where this is applied, and where it deliberately is not
//
// [repeats] only, exactly as [canonicalOrder] is. Against the reference's own
// second run there is nothing to argue about: one shell disagreeing with
// *itself* about a number the kernel chose is not a fact about either shell.
//
// Against **our** run it is not applied, and [processGroups] counts what that
// costs instead of hiding it. That is a floor and not a defect, which took
// measuring twice to establish — see below.
//
// # The floor, and why the route matters (#4012)
//
// The number bash writes there is **two answers**, and which one a run sees
// is decided by the shell's standing in its own session rather than by
// anything about the shell. Measured 2026-09-21 against bash 5.3.20, one
// harness starting the same binary with the same command string twice and
// changing exactly one thing between them:
//
//	started into a group of its own    cannot set terminal process group (-1)
//	started into its caller's group    cannot set terminal process group (78796)
//
// The second is the group and not the parent — a run with a wrapper between
// the leader and the shell, leader 84507 and wrapper 84516, wrote 84507 — and
// three alternating repeats gave the same pair every time.
//
// Only the first of those was measured when this file was written, and it
// read as a plain disagreement: bash wrote `-1` where this shell wrote its
// own process group, so a mask over the comparison would have hidden a real
// difference in what the two shells report. It was a real difference, it was
// on the **leader** route, and it is fixed — interp's terminalProcessGroup
// now writes what bash writes on both standings.
//
// **The suite's inner shells are on the other route**, which is what makes
// the seven lines a floor. [runFile] gives each suite file's shell a process
// group of its own so the run can be ended as a group, and every inner
// `$THIS_SH -i` a file starts inherits that group rather than leading one. So
// both shells take the second branch, both write their own process group
// correctly, and the two numbers still differ — because they are two
// different processes, which is not a thing any implementation can close.
// Fixing the disagreement removed a parity bug and did not remove one of the
// seven lines.
//
// So the arithmetic is: the fetched bash column's differing-line figure
// carries seven lines that no change to this shell can ever take out, and
// [Report.ProcessGroups] is where that is counted on every run rather than
// being rediscovered by the next pass.
//
// **Whether a matching number is now worth masking in the comparison is left
// open**, and it is a decision rather than an oversight. With both standings
// answered alike there is less for a mask to hide than there was, but a mask
// works on a line and cannot tell which of the two routes produced it, so it
// would also cover a future disagreement on either. The count below says what
// masking would be worth without spending anything to find out.
//
// # Anchored on the role, not on the digits
//
// #3988 is the failure this is written against: a mask over `$$` that was
// not anchored ate a line number that merely happened to equal the pid, on
// about one run in a hundred, and the file it was in answered two different
// ways. A mask is at its worst when it over-matches, because what it costs
// is the determinism it was added to buy and nothing says so.
//
// So this matches the *role* — a number inside the parentheses of a remark
// about a process group — and not a number that happens to look like one. A
// broader mask would be worse than none here for a sharper reason than
// usual: this decides whether a file is **scored**, so a genuinely
// non-deterministic file passed through it would have the reference's own
// per-run difference graded against us as though it were our disagreement.
var pidOfAProcessGroup = regexp.MustCompile(`(process group \()-?\d+(\))`)

// withoutTheRunsPid takes that number out and leaves everything else,
// including the parentheses that make it recognizable.
func withoutTheRunsPid(out string) string {
	return pidOfAProcessGroup.ReplaceAllString(out, "${1}<pid>${2}")
}

// reproduced is what [repeats] asks: did the reference reproduce its own run, up
// to the two things neither run was asked for?
//
// One function for both tolerances so that a third one cannot be added to
// half of the question. Each is argued where it is defined — the order in
// reorder.go, the process group above.
func reproduced(a, b string) bool {
	return sameButForOrder(withoutTheRunsPid(a), withoutTheRunsPid(b))
}

// processGroups is how many of two runs' differing lines go away when the
// process group's number is ignored.
//
// The floor the file comment argues, counted. It is the same shape as
// [reordered] and stands on the same terms: a per-side rewrite, clamped into
// the differing lines it is reported beside, **subtracted from nothing**. The
// raw count stays what the two runs did.
//
// Unlike [reordered] it is neither an upper nor a lower bound but the figure
// itself, and the reason is the anchor above: the only lines it can reach are
// lines where both runs wrote a remark naming a process group in the same
// place, which is a line neither shell was asked for and neither can match.
// It is still reported rather than applied, because what a report may
// subtract is a question for whoever reads it.
func processGroups(mine, theirs []string, differing int) int {
	if differing <= 0 {
		return 0
	}
	common, longest, _ := agreement(withoutThePids(mine), withoutThePids(theirs))
	return min(max(differing-(longest-common), 0), differing)
}

// withoutThePids is [withoutTheRunsPid] over one side's lines. A pure
// function of one run, so it cannot be told what the other one wrote.
func withoutThePids(ls []string) []string {
	out := make([]string, len(ls))
	for i, line := range ls {
		out[i] = withoutTheRunsPid(line)
	}
	return out
}
