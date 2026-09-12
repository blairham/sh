// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which number the shell counts up from when it picks a descriptor for
// itself.
//
// Measured 2026-09-12, `<shell> -c 'exec {fd}< /etc/hosts; echo $fd'`: bash
// 5.3.15 and ksh93 say 10, zsh 5.9.2 says 11. dash and bash 3.2 have no such
// grammar (#1752).
//
// A number a script can print, compare and hand to a child, which is why it
// is worth a field: the corpus rows about `zsocket` and `sysopen -u name`
// were written so that the allocated number never appears, because it was
// wrong in one column.
func TestTheDescriptorTheShellPicksHasABase(t *testing.T) {
	for _, tc := range []struct {
		name string
		base DescriptorAllocationBase
		want string
	}{
		{"from ten", AllocateDescriptorsFromTen, "10\n"},
		{"from eleven", AllocateDescriptorsFromEleven, "11\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = tc.base
			out, st := run(t, `exec {fd}< /dev/null; echo $fd`, withSem(sem))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The base is where the counting *starts*, not the number every allocation
// gets: a second one takes the next free number up from it.
func TestTheSecondPickedDescriptorIsTheNextFreeOne(t *testing.T) {
	sem := permissive()
	sem.FirstAllocatedDescriptor = AllocateDescriptorsFromEleven
	out, st := run(t, `exec {a}< /dev/null; exec {b}< /dev/null; echo "$a $b"`, withSem(sem))
	if want := "11 12\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// And it is clear of the single digits a script addresses itself, under
// either answer: a descriptor the script parked at 9 is not what the shell
// hands back.
func TestAPickedDescriptorNeverCollidesWithASingleDigit(t *testing.T) {
	for _, base := range []DescriptorAllocationBase{
		AllocateDescriptorsFromTen, AllocateDescriptorsFromEleven,
	} {
		sem := permissive()
		sem.FirstAllocatedDescriptor = base
		out, st := run(t, `exec 9< /dev/null; exec {fd}< /dev/null; echo $fd`, withSem(sem))
		if want := itoa(baseNumber(base)); out != want+"\n" || st != 0 {
			t.Errorf("%v: got %q (status %d), want %q at 0", base, out, st, want)
		}
	}
}

func baseNumber(b DescriptorAllocationBase) int {
	if b == AllocateDescriptorsFromEleven {
		return 11
	}
	return 10
}
