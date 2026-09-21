// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import "testing"

// The mask is anchored on the **role** and not on the digits, which is the
// half that can be wrong in the direction nothing notices.
//
// #3988 is the failure it is written against: an unanchored pid mask ate a
// line number that merely happened to equal the pid, so the file it was in
// answered two different ways and did so on about one run in a hundred. Here
// the cost of over-matching is sharper still — this decides whether a file is
// **scored at all**, so a genuinely non-deterministic file waved through it
// would have the reference's own per-run difference graded against us.
//
// So the rows below pair each thing that must be masked with a number that
// must survive, and every surviving number is one a real run writes.
func TestThePidMaskIsAnchoredOnTheRole(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{
			name: "the number a process-group remark names",
			in:   "bash: cannot set terminal process group (34551): Inappropriate ioctl for device",
			want: "bash: cannot set terminal process group (<pid>): Inappropriate ioctl for device",
		},
		{
			// The same remark from a shell with no group to name, which is
			// what the reference writes with every stream redirected.
			name: "and the -1 it names when there is none",
			in:   "bash: cannot set terminal process group (-1): Inappropriate ioctl for device",
			want: "bash: cannot set terminal process group (<pid>): Inappropriate ioctl for device",
		},
		{
			name: "a line number is not one, even with a pid's value",
			in:   "./f.tests: line 34551: nosuchcmd: command not found",
			want: "./f.tests: line 34551: nosuchcmd: command not found",
		},
		{
			name: "and neither is a number a script printed",
			in:   "34551",
			want: "34551",
		},
		{
			// The words without the parentheses are prose a file can print.
			name: "the words alone are not the role",
			in:   "process group 34551 is the one it wanted",
			want: "process group 34551 is the one it wanted",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := withoutTheRunsPid(c.in); got != c.want {
				t.Errorf("masked to %q, want %q", got, c.want)
			}
		})
	}
}

// And what [repeats] does with it: two runs of one shell differing only in
// that number are the same run, and two differing anywhere else are not.
//
// Both halves, because a tolerance that accepted everything would make every
// file look steady — including one that is genuinely not, whose per-run
// difference would then be scored as though it were ours.
func TestAReferenceReproducesItsRunUpToTheProcessGroup(t *testing.T) {
	const first = "bash: cannot set terminal process group (34551): Inappropriate ioctl for device\n" +
		"bash: no job control in this shell\n$ 1\n"
	const again = "bash: cannot set terminal process group (34608): Inappropriate ioctl for device\n" +
		"bash: no job control in this shell\n$ 1\n"
	if !reproduced(first, again) {
		t.Error("two runs differing only in the process group read as unsteady")
	}
	const moved = "bash: cannot set terminal process group (34608): Inappropriate ioctl for device\n" +
		"bash: no job control in this shell\n$ 2\n"
	if reproduced(first, moved) {
		t.Error("two runs that printed different answers read as steady")
	}
	// The order tolerance is still there, reached through the same function:
	// one shell disagreeing with itself about a hash table's sequence is the
	// other thing neither run was asked for.
	const pairs = `declare -A m=([a]="1" [b]="2" )` + "\n"
	const rotated = `declare -A m=([b]="2" [a]="1" )` + "\n"
	if !reproduced(pairs, rotated) {
		t.Error("the order tolerance did not survive being folded in")
	}
}

// The floor, counted. It is reported beside the differing lines and corrects
// nothing, so what it has to be right about is the two ends: the line two
// shells can never agree on is counted, and an ordinary disagreement is not.
//
// The seven-line figure #4012 records is this function over `history.tests`,
// whose inner interactive shells each write the remark once.
func TestTheProcessGroupFigureCountsTheRemarkAndNotContent(t *testing.T) {
	const remark = "bash: cannot set terminal process group (%s): Inappropriate ioctl for device"
	ours := func(n string) string { return replaceOnce(remark, "%s", n) }
	for _, c := range []struct {
		name         string
		mine, theirs []string
		want         int
	}{
		{
			"two processes naming their own groups is entirely the floor",
			[]string{ours("34551")},
			[]string{ours("34608")},
			1,
		},
		{
			"and so is one standing against the other",
			[]string{ours("34551")},
			[]string{ours("-1")},
			1,
		},
		{
			"seven inner shells are seven lines, which is the figure #4012 records",
			[]string{ours("1"), ours("2"), ours("3"), ours("4"), ours("5"), ours("6"), ours("7")},
			[]string{ours("8"), ours("9"), ours("10"), ours("11"), ours("12"), ours("13"), ours("14")},
			7,
		},
		{
			"a real disagreement beside it is not counted",
			[]string{ours("34551"), "one"},
			[]string{ours("34608"), "two"},
			1,
		},
		{
			"an ordinary disagreement on its own is not",
			[]string{"one", "two"},
			[]string{"one", "three"},
			0,
		},
		{
			"runs that already agree have nothing to report",
			[]string{ours("34551")},
			[]string{ours("34551")},
			0,
		},
		{
			// The anchor again, from the counting side: a line number that
			// happens to look like a pid is a disagreement we own.
			"and a number nobody named a group is not reached",
			[]string{"./f.tests: line 34551: oops"},
			[]string{"./f.tests: line 34608: oops"},
			0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			common, longest, _ := agreement(c.mine, c.theirs)
			if got := processGroups(c.mine, c.theirs, longest-common); got != c.want {
				t.Errorf("processGroups(%q, %q) = %d, want %d", c.mine, c.theirs, got, c.want)
			}
		})
	}
}

// The figure may never claim more than there were, whatever the masking does
// to the alignment — the bound [reordered] is held to, for the same reason.
func TestTheProcessGroupFigureStaysInsideTheDifferingLines(t *testing.T) {
	const remark = "bash: cannot set terminal process group (%s): Inappropriate ioctl for device"
	mine := []string{replaceOnce(remark, "%s", "1"), "one", "two"}
	theirs := []string{replaceOnce(remark, "%s", "2"), "three"}
	common, longest, _ := agreement(mine, theirs)
	differing := longest - common
	got := processGroups(mine, theirs, differing)
	if got < 0 || got > differing {
		t.Errorf("processGroups = %d, outside 0..%d", got, differing)
	}
}

// replaceOnce keeps the rows above readable without pulling `strings` in for
// one call.
func replaceOnce(s, old, with string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + with + s[i+len(old):]
		}
	}
	return s
}
