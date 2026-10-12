// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// fdTableSize is the size of this process's descriptor table, as the kernel
// reports it in /proc/self/status. It only ever grows.
func fdTableSize(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "FDSize:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				t.Fatalf("reading %q: %v", line, err)
			}
			return n
		}
	}
	t.Fatal("no FDSize line in /proc/self/status")
	return 0
}

// A pipeline does not grow the kernel's descriptor table.
//
// Each element runs beside the others with its own copy of every descriptor
// in the table (ownDescriptors), and those copies were placed above every
// number the dialect could publish — 64 or more in every dialect. Taking 64
// grows a table that starts with room for 64, and growing one shared between
// threads waits out an RCU grace period: `true | true` left FDSize at 128
// and cost +11 ms of wall time on the first pipeline of every run (#6402).
//
// In a quiet table, because the size never shrinks and this binary's own
// table has long since grown by the time the case runs. The input is a real
// file, because a copy is only taken of one; and the case proves its own
// instrument afterwards, by taking 64 by hand and watching the size move.
func TestAPipelineDoesNotGrowTheDescriptorTable(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	const first = 64
	if n := fdTableSize(t); n != first {
		t.Fatalf("the table starts at %d, want %d — nothing below can be measured", n, first)
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	out := runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable, AllocateDescriptorsFromTen,
		nil, "true | true\n{ true | true; } | { read x; echo ok; }\ntrue & wait", f)
	if out != "ok\n" {
		t.Fatalf("the script printed %q, want %q", out, "ok\n")
	}
	if n := fdTableSize(t); n != first {
		t.Errorf("FDSize is %d after a pipeline, want %d — an element's private "+
			"copies landed past the kernel's first table", n, first)
	}
	fd, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(syscall.Stdin),
		uintptr(syscall.F_DUPFD_CLOEXEC), uintptr(first))
	if errno != 0 {
		t.Fatalf("taking descriptor %d by hand: %v", first, errno)
	}
	defer func() { _ = syscall.Close(int(fd)) }()
	if n := fdTableSize(t); n <= first {
		t.Errorf("FDSize is still %d with descriptor %d open — the instrument cannot "+
			"see the growth this case is about", n, first)
	}
}
