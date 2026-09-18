// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The sweep is expensive enough that CI stopped running the whole of it on
// every leg, and this is the check that the whole of it is still run
// somewhere.
//
// TestNoDialectCombinationPanics is the property that no combination of
// dialect flags can crash the parser, and it is proved over every vector and
// every case. CI answers it in two sharded jobs and skips it on the
// uninstrumented `Build and test` leg, where it was 236s of the critical path
// for an answer those two jobs had already produced.
//
// That arrangement has a failure mode with no symptom: drop the sharded jobs,
// or narrow their matrix, and every check stays green while the sweep runs
// nowhere. The file that shrinks a run is the file that has to say where the
// whole of it lives, so this reads the workflow and holds the two halves
// against each other. It is the same rule vectorShard applies to a malformed
// shard spec — silently sweeping nothing is a green check for a sweep that
// never ran.
//
// Deliberately text rather than parsed YAML. The question is about two lines
// of one file, the repository pins its dependency surface on purpose
// (internal/depsurface), and a YAML parser pulled in for a test that greps
// would be a worse trade than the brittleness it buys off.
func TestTheSweepIsProvedWhereThisLegStopsPayingForIt(t *testing.T) {
	t.Parallel()
	const sweep = "TestNoDialectCombinationPanics"
	workflow := readWorkflow(t)

	var skipped, proved bool
	for _, line := range strings.Split(workflow, "\n") {
		if !strings.Contains(line, sweep) {
			continue
		}
		switch {
		case strings.Contains(line, "-skip"):
			skipped = true
		case strings.Contains(line, "-run"):
			proved = true
		}
	}
	if !skipped {
		// Nothing is being traded away, so there is nothing to hold against
		// it. Said rather than passed silently: a check that stops applying
		// should say so, or the next reader takes it for a check that passed.
		t.Skip("no CI leg skips the sweep, so nothing here is owed a second run of it")
	}
	if !proved {
		t.Fatalf("a CI leg skips %s and no job runs it: the sweep runs nowhere and every check is green", sweep)
	}

	offsets, stride := shardMatrix(t, workflow)
	if stride == 0 {
		t.Fatalf("a CI leg skips %s and no SH_DIALECTCOMBO_SHARD matrix says who answers for the whole of it", sweep)
	}
	for want := range stride {
		if !offsets[want] {
			t.Errorf("the shard matrix is missing %d/%d, so that slice of the vectors is swept by nobody", want, stride)
		}
	}
}

// shardMatrix reads the shard specs out of the workflow's matrix and returns
// the offsets present and the stride they share.
//
// A second stride is a failure rather than a merge: `0/2` and `0/3` together
// cover neither sweep, and the arithmetic that says so is exactly the
// arithmetic nobody does by eye.
func shardMatrix(t *testing.T, workflow string) (map[int]bool, int) {
	t.Helper()
	spec := regexp.MustCompile(`\b(\d+)/(\d+)\b`)
	offsets := map[int]bool{}
	stride := 0
	for _, line := range strings.Split(workflow, "\n") {
		if !strings.Contains(line, "shard:") || strings.Contains(line, "matrix.shard") {
			continue
		}
		for _, m := range spec.FindAllStringSubmatch(line, -1) {
			o, _ := strconv.Atoi(m[1])
			s, _ := strconv.Atoi(m[2])
			if stride != 0 && stride != s {
				t.Fatalf("two shard strides in one matrix, %d and %d: neither is a partition of the sweep", stride, s)
			}
			offsets[o], stride = true, s
		}
	}
	return offsets, stride
}

// readWorkflow finds the workflow from wherever `go test` put this package's
// working directory, which is the package directory rather than the root.
func readWorkflow(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above this package, so the workflow cannot be found")
		}
		dir = parent
	}
	b, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
