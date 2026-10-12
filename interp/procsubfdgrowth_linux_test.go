// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// fdTargets maps every descriptor in this process to what it points at.
func fdTargets(t *testing.T) map[int]string {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[int]string, len(ents))
	for _, e := range ents {
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if l, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil {
			out[n] = l
		}
	}
	return out
}

// A process substitution keeps every descriptor it opens inside the kernel's
// first descriptor table, in every rule a dialect publishes by.
//
// The shell's own end of the pipe was moved above every number the dialect
// could publish, which is 64 or more in every rule: 64 for bash's, 67 and 75
// for the two that count up. Taking 64 grows a table that starts with room
// for 64, and growing one shared between threads waits out an RCU grace
// period, so the first `<(…)` of a run cost 8 to 13 ms of wall time on Linux
// and `cat <(echo x)` left FDSize at 128 where `echo x | cat` leaves 64
// (#6406).
//
// Asked of the descriptors themselves, for the reason
// TestAPipelineDoesNotGrowTheDescriptorTable gives: a CI runner starts the
// binary holding 255, so the size says nothing there. The body is held open
// on a read of a pipe this case owns, and every descriptor the run has opened
// by then is read off /proc/self/fd. The case requires that it saw the
// substitution's pipe at all — two new descriptors onto one pipe that is not
// the case's own — so a run that had already finished cannot pass it. And it
// waits for the body's own copy of that input as well, which is taken only
// once the pipe is made and placed: two descriptors onto the pipe are also
// what the moment after os.Pipe looks like, before anything has moved.
func TestAProcessSubstitutionDoesNotGrowTheDescriptorTable(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	const first = 64
	startSize := fdTableSize(t)
	for _, rule := range []struct {
		name  string
		where SubstEndPlacement
		base  DescriptorAllocationBase
	}{
		{"bash's", SubstitutionEndsAtTheTopOfTheTable, AllocateDescriptorsFromTen},
		{"ksh93's", SubstitutionEndsAtTheLowestFreeNumber, AllocateDescriptorsFromTen},
		{"zsh's", SubstitutionEndsWhereAnyDescriptorGoes, AllocateDescriptorsPastTheShellsInput},
	} {
		rd, wr, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		before := fdTargets(t)
		own := before[int(rd.Fd())]
		done := make(chan string, 1)
		go func() {
			done <- runSubstPlacement(t, rule.where, rule.base, nil,
				"read y < <(read x; echo \"$x\")\necho \"$y\" ok", rd)
		}()
		var seen map[int]string
		saw := false
		for deadline := time.Now().Add(10 * time.Second); !saw && time.Now().Before(deadline); {
			seen = fdTargets(t)
			onto := map[string]int{}
			for fd, l := range seen {
				if _, ok := before[fd]; !ok {
					onto[l]++
				}
			}
			for l, k := range onto {
				if k >= 2 && l != own && onto[own] > 0 {
					saw = true
				}
			}
			if !saw {
				time.Sleep(time.Millisecond)
			}
		}
		if _, err := wr.WriteString("line\n"); err != nil {
			t.Fatal(err)
		}
		_ = wr.Close()
		if out := <-done; out != "line ok\n" {
			t.Fatalf("%s rule: the script printed %q, want %q", rule.name, out, "line ok\n")
		}
		_ = rd.Close()
		if !saw {
			t.Fatalf("%s rule: never saw the substitution's pipe open — this case measured nothing", rule.name)
		}
		for fd, l := range seen {
			if _, ok := before[fd]; !ok && fd >= first {
				t.Errorf("%s rule: the run opened descriptor %d (%s), past the kernel's first "+
					"%d-entry table — the first substitution of a run grows it", rule.name, fd, l, first)
			}
		}
	}
	if startSize != first {
		t.Logf("the table started at %d, so the size itself says nothing here", startSize)
		return
	}
	if n := fdTableSize(t); n != first {
		t.Errorf("FDSize is %d after the substitutions, want %d", n, first)
	}
}
