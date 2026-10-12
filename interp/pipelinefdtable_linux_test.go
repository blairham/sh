// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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

// copiesOf lists the descriptors in this process, other than the ones in
// skip, that are open on the same file as target. Both ends of a pipe read
// back as the same name, so a case asking about a pipe skips both.
func copiesOf(t *testing.T, target string, skip ...int) []int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	for _, e := range ents {
		n, err := strconv.Atoi(e.Name())
		if err != nil || slices.Contains(skip, n) {
			continue
		}
		if l, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil && l == target {
			out = append(out, n)
		}
	}
	return out
}

// A pipeline element's private copies stay inside the kernel's first
// descriptor table.
//
// Each element runs beside the others with its own copy of every descriptor
// in the table (ownDescriptors), and those copies were placed above every
// number the dialect could publish — 64 or more in every dialect. Taking 64
// grows a table that starts with room for 64, and growing one shared between
// threads waits out an RCU grace period: `true | true` left FDSize at 128
// and cost +11 ms of wall time on the first pipeline of every run (#6402).
//
// Asked of the copy itself rather than of FDSize, because the size only ever
// grows and is not this process's to choose: a CI runner's step starts the
// test binary from a shell holding descriptor 255, and the table is 256 from
// the first instruction. So the element is held open on a read of a pipe this
// case owns, and the case finds the element's copy of that pipe by what it
// points at and reads its number. Where the table did start at 64, the size
// is checked as well.
func TestAPipelineDoesNotGrowTheDescriptorTable(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	const first = 64
	startSize := fdTableSize(t)
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rd.Close(); _ = wr.Close() }()
	own, other := int(rd.Fd()), int(wr.Fd())
	target, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(own))
	if err != nil {
		t.Fatal(err)
	}
	if got := copiesOf(t, target, own, other); len(got) != 0 {
		t.Fatalf("the pipe already has descriptors %v before the run", got)
	}
	done := make(chan string, 1)
	go func() {
		done <- runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable, AllocateDescriptorsFromTen,
			nil, "read x | true\necho \"$x\" ok", rd)
	}()
	var copies []int
	deadline := time.Now().Add(10 * time.Second)
	for len(copies) == 0 && time.Now().Before(deadline) {
		copies = copiesOf(t, target, own, other)
		if len(copies) == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	if _, err := wr.WriteString("line\n"); err != nil {
		t.Fatal(err)
	}
	_ = wr.Close()
	if out := <-done; out != " ok\n" {
		t.Fatalf("the script printed %q, want %q", out, " ok\n")
	}
	if len(copies) == 0 {
		t.Fatal("the element never took a copy of its input — this case measured nothing")
	}
	for _, fd := range copies {
		if fd >= first {
			t.Errorf("the element's copy of its input is descriptor %d, past the kernel's "+
				"first %d-entry table — the first pipeline of a run grows it", fd, first)
		}
	}
	if startSize != first {
		t.Logf("the table started at %d, so the size itself says nothing here", startSize)
		return
	}
	if n := fdTableSize(t); n != first {
		t.Errorf("FDSize is %d after a pipeline, want %d", n, first)
	}
}
