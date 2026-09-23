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
// Applied in [normalize], so it reaches [repeats] and the comparison against
// our own run alike. That is a change of mind and the argument for it is
// below: it was reported beside the count and not applied, for as long as
// there was a real disagreement it could have hidden.
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
// So the arithmetic was: the fetched bash column's differing-line figure
// carried seven lines that no change to this shell could ever take out.
//
// # Why it is masked now, having been counted and not masked before
//
// The objection that kept it out of the comparison was exact: a mask works on
// a line and cannot tell which of the two routes produced it, so one that
// covered the number would also cover a future disagreement on either. That
// objection is answered rather than overruled, and in two pieces.
//
// The first is #4250. This shell used to put every child of every script in a
// process group of its own, so its inner shells took the **leader** route and
// wrote `-1` where the reference wrote a number. That was a real
// disagreement, it was under the mask's anchor, and masking then would have
// hidden it. It is fixed: a foreground command leads a group only where there
// is a monitor and a terminal, so our inner shells inherit the file's group
// exactly as the reference's do, and both now write a number.
//
// The second is the anchor itself, narrowed: the mask matches a number the
// run chose and **not** the `-1`. So the two routes stay apart — a shell that
// went back to leading its own group would write `-1` against the reference's
// number and the line would differ, loudly, as it did before. What is hidden
// is the digits of two numbers that can never be equal, because they belong
// to two processes.
//
// With both halves in place the seven lines are not a floor any more; they
// are a difference nobody was ever going to close, taken out of the count the
// way the run's own directory and the shell's own path are.
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
// **A number the run chose, and not the `-1`.** The two are different answers
// rather than two spellings of one: `-1` is what a shell writes when it
// already leads its own group, and a number is what it writes when it does
// not. A mask that covered both would hide a real disagreement — this shell
// wrote `-1` where the reference wrote a number until #4250, for the whole of
// the time the mask existed, and applying it then would have made that bug
// invisible instead of counted. So the digits go and the branch stays.
var pidOfAProcessGroup = regexp.MustCompile(`(process group \()\d+(\))`)

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
