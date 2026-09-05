// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// limitPair is one resource's two limits, kept in a map rather than in the
// process — so these tests assert exact numbers and change nothing outside
// themselves. The whole reason the hooks exist.
type limitPair struct{ soft, hard int64 }

func ulimitRun(t *testing.T, held map[Resource]limitPair, tweak func(*Semantics), src string) (string, int, map[Resource]limitPair) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UlimitBlockIsKilobyte = No
	sem.UlimitHasResidentSet = Yes
	sem.UlimitHasProcessCount = Yes
	sem.UlimitSetsBothLimits = Yes
	if tweak != nil {
		tweak(&sem)
	}
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.GetRlimit = func(res Resource) (int64, int64, error) {
		p := held[res]
		return p.soft, p.hard, nil
	}
	r.SetRlimit = func(res Resource, soft, hard int64) error {
		held[res] = limitPair{soft, hard}
		return nil
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st, held
}

func limits(pairs map[Resource]limitPair) map[Resource]limitPair { return pairs }

// Reading, in the units each resource is printed in.
func TestUlimitReads(t *testing.T) {
	held := limits(map[Resource]limitPair{
		ResourceCPUTime:   {3600, RlimitInfinity},
		ResourceStack:     {8372224, 67092480},
		ResourceOpenFiles: {256, 512},
		ResourceFileSize:  {1024, RlimitInfinity},
	})
	for _, c := range []struct{ src, want string }{
		{`ulimit -t`, "3600"},       // seconds
		{`ulimit -Ht`, "unlimited"}, // no limit is a word
		{`ulimit -s`, "8176"},       // kilobytes: 8372224/1024
		{`ulimit -Hs`, "65520"},     // 67092480/1024
		{`ulimit -n`, "256"},        // a count
		{`ulimit -f`, "2"},          // 512-byte blocks: 1024/512
		{`ulimit`, "2"},             // bare ulimit is -f
		{`ulimit -St`, "3600"},      // -S is the default
	} {
		out, _, _ := ulimitRun(t, held, nil, c.src)
		if strings.TrimSpace(out) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
	// bash counts the block as a kilobyte, which halves the same number.
	out, _, _ := ulimitRun(t, held, func(s *Semantics) { s.UlimitBlockIsKilobyte = Yes }, `ulimit -f`)
	if strings.TrimSpace(out) != "1" {
		t.Errorf("kilobyte block: %q, want 1", strings.TrimSpace(out))
	}
}

// Setting, and which of the two limits moves.
func TestUlimitSets(t *testing.T) {
	// Neither flag: three of the four lower the ceiling with the floor.
	held := limits(map[Resource]limitPair{ResourceCPUTime: {100, RlimitInfinity}})
	_, _, held = ulimitRun(t, held, nil, `ulimit -t 50`)
	if got := held[ResourceCPUTime]; got.soft != 50 || got.hard != 50 {
		t.Errorf("both: soft=%d hard=%d, want 50 and 50", got.soft, got.hard)
	}
	// zsh moves only the floor.
	held = limits(map[Resource]limitPair{ResourceCPUTime: {100, RlimitInfinity}})
	_, _, held = ulimitRun(t, held, func(s *Semantics) { s.UlimitSetsBothLimits = No }, `ulimit -t 50`)
	if got := held[ResourceCPUTime]; got.soft != 50 || got.hard != RlimitInfinity {
		t.Errorf("soft only: soft=%d hard=%d, want 50 and unlimited", got.soft, got.hard)
	}
	// -H moves the ceiling alone, -S the floor alone.
	held = limits(map[Resource]limitPair{ResourceCPUTime: {100, 200}})
	_, _, held = ulimitRun(t, held, nil, `ulimit -Ht 150`)
	if got := held[ResourceCPUTime]; got.soft != 100 || got.hard != 150 {
		t.Errorf("-H: soft=%d hard=%d, want 100 and 150", got.soft, got.hard)
	}
	held = limits(map[Resource]limitPair{ResourceCPUTime: {100, 200}})
	_, _, held = ulimitRun(t, held, nil, `ulimit -St 150`)
	if got := held[ResourceCPUTime]; got.soft != 150 || got.hard != 200 {
		t.Errorf("-S: soft=%d hard=%d, want 150 and 200", got.soft, got.hard)
	}
	// The units apply on the way in as well as out.
	held = limits(map[Resource]limitPair{ResourceStack: {0, 0}})
	_, _, held = ulimitRun(t, held, nil, `ulimit -s 100`)
	if got := held[ResourceStack]; got.soft != 100*1024 {
		t.Errorf("stack: %d, want %d", got.soft, 100*1024)
	}
	// And `unlimited` is read back from the word.
	held = limits(map[Resource]limitPair{ResourceCPUTime: {5, 5}})
	_, _, held = ulimitRun(t, held, nil, `ulimit -t unlimited`)
	if got := held[ResourceCPUTime]; got.soft != RlimitInfinity {
		t.Errorf("unlimited: %d, want %d", got.soft, RlimitInfinity)
	}
}

// Two of the ten letters are not universal.
func TestUlimitResourcesThatNotEveryShellHas(t *testing.T) {
	held := limits(map[Resource]limitPair{ResourceResidentSet: {1024, 1024}, ResourceProcesses: {50, 50}})
	if out, st, _ := ulimitRun(t, held, nil, `ulimit -m`); st != 0 || strings.TrimSpace(out) != "1" {
		t.Errorf("-m where it exists: %q status %d", out, st)
	}
	out, st, _ := ulimitRun(t, held, func(s *Semantics) { s.UlimitHasResidentSet = No }, `ulimit -m`)
	if st == 0 || !strings.Contains(out, "m") {
		t.Errorf("-m where it does not: %q status %d, want it refused as an option", out, st)
	}
	if out, st, _ := ulimitRun(t, held, nil, `ulimit -u`); st != 0 || strings.TrimSpace(out) != "50" {
		t.Errorf("-u where it exists: %q status %d", out, st)
	}
	out, st, _ = ulimitRun(t, held, func(s *Semantics) { s.UlimitHasProcessCount = No }, `ulimit -u`)
	if st == 0 {
		t.Errorf("-u where it does not: %q status %d, want it refused", out, st)
	}
}

// Without hooks it says so rather than pretending — the failure #117 was about.
func TestUlimitWithoutHooks(t *testing.T) {
	f, err := syntax.Parse(`ulimit -n 256`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if st == 0 || buf.String() == "" {
		t.Errorf("status %d saying %q, want a refusal", st, buf.String())
	}
}
