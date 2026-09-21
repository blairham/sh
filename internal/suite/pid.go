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
// Against **our** run it is not applied, and that is not an oversight. The
// two shells do not always write the same number there — measured the same
// day with all three streams redirected, bash 5.3.20 writes `(-1)` where
// this shell writes its own process group — so a mask over the comparison
// would hide a real disagreement about what the shell reports. So the
// number stays in the differing-line count as a floor, and #4012 is where
// that arithmetic is written down.
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
