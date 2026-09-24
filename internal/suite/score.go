// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Three numbers, and the reason there are three is that each of them lies on
// its own.
//
// **Strict** — the whole file byte-identical, status included — reads as
// catastrophe. A suite file is hundreds of assertions and one disagreement
// forfeits every one of them, so a column at 4% may be wrong about 4% of what
// it does.
//
// **Line agreement** reads as triumph for the mirror-image reason: most lines
// of most files are text the shell echoed back, and two shells agree on an
// echo. It is still worth having, and it has to be a longest common
// subsequence rather than a positional comparison — a file that emits one
// extra line near the top agrees on everything afterwards, and lining the two
// outputs up by index would score that zero and call it a total failure.
//
// **Parsed** is neither, and for a while it claimed to be the explanation of
// both. It is not: this shell parses incrementally, exactly as the reference
// does, so a file whose *static* whole-file read is refused still runs up to
// the construct that stopped the read, and is still run and still scored
// here. What a refusal costs is measured rather than asserted — the refused
// files are aggregated as a band of their own, and on the run this wording
// was written against they scored no worse for having been refused.
//
// What parsed measures is the static route — `-n`, a formatter, an editor —
// reading a whole file in the dialect's defaults. That is a real property
// with real consumers in this tree; it is simply not what the other two
// numbers are made of. And part of its gap list can never close: an option
// set at run time decides what a later line means, so a file the reference's
// own `-n` refuses is refused by every static read there is, and the ranking
// separates those out.

// tempPattern matches the per-run directory, whose name differs every time
// and would otherwise be a difference in itself.
var tempPattern = regexp.MustCompile(`(/private)?/(tmp|var/folders)/[^\s:"']*`)

// normalize removes what is true of the run rather than of the shell.
//
// The shell's own path appears in its diagnostics and the two shells have
// different ones, so comparing those would report a difference on every file
// that fails — which is most of the interesting ones. The full path only,
// never the base name: replacing a base name turns unrelated words into
// `<shell>` and reports a difference between two identical outputs.
//
// The third is the process group a shell with no terminal names, and it is
// here for the same reason the other two are: two shells are two processes,
// so the number can never match and nothing about either shell is in it. It
// is masked and not dropped, and only where the number is one the run chose —
// see pid.go, where the anchor and the `-1` it deliberately leaves alone are
// argued.
func normalize(out, shell, dir string) string {
	out = strings.ReplaceAll(out, shell, "<shell>")
	if dir != "" {
		out = strings.ReplaceAll(out, dir, "<dir>")
	}
	out = withoutTheRunsPid(out)
	return tempPattern.ReplaceAllString(out, "<tmp>")
}

// ErrBaseNameDiffers is what a run is refused with when the shell being graded
// is named neither like the reference nor like its own column.
var ErrBaseNameDiffers = errors.New("the graded shell's base name differs from the reference's")

// CheckBaseNames is [normalize]'s precondition, asked before a run rather than
// discovered from its numbers.
//
// normalize above replaces each shell's **full path** and deliberately not its
// base name — replacing a base name would turn unrelated words into `<shell>`.
// The consequence is easy to miss and is the whole of this check: a line where
// a shell names *itself* by base name is left alone, so it is compared
// **literally**, and that is sound only while the two base names are equal.
//
// When they are not, every such line differs — and there is no sign of it in
// the report, which prints a figure like any other. Measured 2026-09-24 in the
// pinned image on one commit, changing nothing but the name of the binary
// handed to -bin:
//
//	-bin …/bash            strict 1/1   0 differing lines
//	-bin …/deep/nested/bash  strict 1/1   0 differing lines
//	-bin …/ourbash         strict 0/1  10 differing lines
//
// Depth is irrelevant; the base name decides it. Five rows of bash's suite
// were reported as regressions on the strength of figures produced that way —
// 31 differing lines, of which 29 were the name — and each one reproduces
// exactly on demand by renaming the binary. That is worse than a wrong answer
// of the usual kind: a number reads as evidence and sends somebody to diagnose
// lines that do not exist.
//
// The refusal is deliberate rather than a warning. A warning is what this
// already had — the recorded trap that *a binary named `bash-base` is partly
// graded on a build artifact's name* — and it did not stop the same fault
// arriving from the other direction.
//
// The one mismatch that is **not** the caller's to fix is tolerated with a
// loud line instead: a reference the machine keeps under another name, which
// is `ksh93` for the `ksh` column and `busybox` for `ash`. There the base names
// cannot be made equal, the comparison is unsound to exactly the same degree,
// and refusing would take the column away rather than fix it — so it says so
// on every run and grades anyway.
func CheckBaseNames(s Suite, ours, reference string) (warning string, err error) {
	ourBase, refBase := filepath.Base(ours), filepath.Base(reference)
	if ourBase == refBase {
		return "", nil
	}
	if ourBase == s.Dialect {
		return fmt.Sprintf(
			"the reference is %s on this machine and the column is %q, so the two base "+
				"names cannot be made equal.\n  Lines where either shell names itself by "+
				"base name are compared literally and will differ. Treat every figure "+
				"below as a ceiling, not a measurement.",
			refBase, s.Dialect), nil
	}
	// Short, and the paragraph that explains it belongs to the caller — the
	// same division ErrDigest next door keeps.
	return "", fmt.Errorf("%w: grading %q against %q", ErrBaseNameDiffers, ourBase, refBase)
}

// lines splits output for comparison, dropping a single trailing empty line
// so that a file ending in a newline and one that does not are not reported
// as differing by a line neither shell wrote.
func lines(out string) []string {
	if out == "" {
		return nil
	}
	ls := strings.Split(out, "\n")
	if len(ls) > 0 && ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// maxLines bounds the comparison. The DP below is O(n·m) and a suite file can
// print tens of thousands of lines; past this the two are compared on their
// first maxLines, which is reported rather than hidden.
const maxLines = 8000

// agreement is the longest common subsequence of two outputs' lines, as a
// fraction of the longer one, and whether the comparison had to be cut short.
//
// The denominator is the longer side rather than the sum, because the
// question is "how much of this run is right" and an answer of 0.99 for one
// extra line is the reading that matches what a person means by it.
func agreement(a, b []string) (common, longest int, truncated bool) {
	if len(a) > maxLines {
		a, truncated = a[:maxLines], true
	}
	if len(b) > maxLines {
		b, truncated = b[:maxLines], true
	}
	// Identical heads and tails are the overwhelming majority of two nearly
	// agreeing outputs, and trimming them turns the quadratic part into the
	// size of the disagreement rather than the size of the file.
	head := 0
	for head < len(a) && head < len(b) && a[head] == b[head] {
		head++
	}
	a, b = a[head:], b[head:]
	tail := 0
	for tail < len(a) && tail < len(b) && a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}
	a, b = a[:len(a)-tail], b[:len(b)-tail]

	longest = head + tail + max(len(a), len(b))
	return head + tail + lcs(a, b), longest, truncated
}

// lcs is the length of the longest common subsequence, two rows at a time.
//
// Only the length is wanted, so the table is never built: a full table over
// two eight-thousand-line outputs is sixty-four million cells, and two rows
// is sixteen thousand.
func lcs(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
				continue
			}
			curr[j] = max(prev[j], curr[j-1])
		}
		prev, curr = curr, prev
		clear(curr)
	}
	return prev[len(b)]
}
