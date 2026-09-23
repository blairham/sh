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
			// The same remark from a shell that already leads its own group,
			// which is a different *answer* and not a second spelling of the
			// one above. It survives the mask on purpose: this shell wrote it
			// where the reference wrote a number until #4250, and a mask that
			// covered both would have hidden that for as long as it existed.
			name: "but not the -1 it writes when it leads the group already",
			in:   "bash: cannot set terminal process group (-1): Inappropriate ioctl for device",
			want: "bash: cannot set terminal process group (-1): Inappropriate ioctl for device",
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

// What the mask is worth once it is applied: two shells naming their own
// process groups are the same line, and everything else still differs.
//
// It is applied in [normalize] rather than reported beside the count, which
// is a change of mind #4250 earned — see pid.go. These rows are the reason it
// is safe to have made: each pairs a difference that must survive with one
// that must not.
func TestTheAppliedMaskLeavesEveryRealDifference(t *testing.T) {
	const remark = "bash: cannot set terminal process group (%s): Inappropriate ioctl for device"
	line := func(n string) string { return replaceOnce(remark, "%s", n) }
	for _, c := range []struct {
		name         string
		mine, theirs string
		same         bool
	}{
		{
			"two processes naming their own groups are one line",
			line("34551"), line("34608"), true,
		},
		{
			// The disagreement #4250 fixed, which the mask must still show.
			"a number against a -1 is not",
			line("34551"), line("-1"), false,
		},
		{
			"nor are two shells that both lead their own group and say so",
			line("-1"), line("34608"), false,
		},
		{
			"and a line number that looks like a pid is untouched",
			"./f.tests: line 34551: oops", "./f.tests: line 34608: oops", false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := normalize(c.mine, "<none>", "") == normalize(c.theirs, "<none>", "")
			if got != c.same {
				t.Errorf("normalize(%q) == normalize(%q) is %v, want %v",
					c.mine, c.theirs, got, c.same)
			}
		})
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
