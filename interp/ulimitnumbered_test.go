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

// A `ulimit` option whose *operand* names the resource by the kernel's own
// number — see Diagnostics.UlimitNumberedOption. Named for the field rather
// than for the one column that has such an option; the preset's pick is
// asserted in dialect/zsh.
//
// The numbering is Runner.RlimitOrder, which is the platform's, so the test
// supplies one of its own: that is the whole claim, and a fixed order would
// assert about this machine instead.

func numberedRun(t *testing.T, order []Resource, src string) (string, int, map[Resource]limitPair) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	held := map[Resource]limitPair{
		ResourceCPUTime:   {3600, RlimitInfinity},
		ResourceProcesses: {700, 700},
		ResourceOpenFiles: {256, 512},
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UlimitBlockIsKilobyte = No
	sem.UlimitSetsBothLimits = Yes
	dg := Diagnostics{
		UlimitBadOption:       "ulimit: -%[1]s: invalid option",
		UlimitBadOptionStatus: 1,
		UlimitListing: []UlimitListingRow{
			{Prefix: "-t: cpu ", Letter: 't', Res: ResourceCPUTime, Scale: 1},
			{Prefix: "-u: proc ", Letter: 'u', Res: ResourceProcesses, Scale: 1},
			{Prefix: "-n: files ", Letter: 'n', Res: ResourceOpenFiles, Scale: 1},
		},
		UlimitNumberedOption: UlimitNumberedOption{
			Letter:      'N',
			NeedsNumber: "number required after -%[1]s",
			BadNumber:   "invalid number: %[1]s",
			OutOfRange:  "no limit there",
		},
	}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.RlimitOrder = order
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

// TestTheNumberedOptionReadsThisKernelsNumbering is the discriminator: the
// same operand is a different resource under a different order, which is what
// says the number is the platform's rather than an index into a table of the
// dialect's.
func TestTheNumberedOptionReadsThisKernelsNumbering(t *testing.T) {
	cpuFirst := []Resource{ResourceCPUTime, ResourceProcesses, ResourceOpenFiles}
	filesFirst := []Resource{ResourceOpenFiles, ResourceProcesses, ResourceCPUTime}
	for _, c := range []struct {
		order []Resource
		src   string
		want  string
	}{
		{cpuFirst, `ulimit -N 1`, "700"},
		{filesFirst, `ulimit -N 1`, "700"},
		{cpuFirst, `ulimit -N 2`, "256"},
		{filesFirst, `ulimit -N 2`, "3600"},
		{cpuFirst, `ulimit -N 0`, "3600"},
		{filesFirst, `ulimit -N 0`, "256"},
		// Attached and separate are the same option, and a leading zero is
		// a decimal digit rather than a refusal.
		{cpuFirst, `ulimit -N2`, "256"},
		{cpuFirst, `ulimit -N 02`, "256"},
		// An empty operand is nought.
		{cpuFirst, `ulimit -N ""`, "3600"},
		// The letter run before it is read as usual.
		{cpuFirst, `ulimit -HN 1`, "700"},
	} {
		out, st, _ := numberedRun(t, c.order, c.src)
		if st != 0 || strings.TrimSpace(out) != c.want {
			t.Errorf("%s under %v: %q at %d, want %q", c.src, c.order, out, st, c.want)
		}
	}
	// And it sets, through the same row the letter would have set.
	out, st, held := numberedRun(t, cpuFirst, `ulimit -N 1 900`)
	if st != 0 || strings.TrimSpace(out) != "" {
		t.Errorf("setting: %q at %d, want silence at 0", out, st)
	}
	if got := held[ResourceProcesses]; got.soft != 900 {
		t.Errorf("setting left %v, want the soft limit at 900", got)
	}
}

// Its three complaints, each at the builtin's own bad-option status.
func TestTheNumberedOptionHasThreeComplaints(t *testing.T) {
	order := []Resource{ResourceCPUTime, ResourceProcesses, ResourceOpenFiles}
	for _, c := range []struct{ src, want string }{
		{`ulimit -N`, "number required after -N"},
		{`ulimit -aN`, "number required after -N"},
		{`ulimit -N x`, "invalid number: x"},
		{`ulimit -Nx`, "invalid number: x"},
		{`ulimit -N -1`, "invalid number: -1"},
		{`ulimit -N 9`, "no limit there"},
	} {
		out, st, _ := numberedRun(t, order, c.src)
		if st != 1 || !strings.Contains(out, c.want) {
			t.Errorf("%s: %q at %d, want %q at 1", c.src, out, st, c.want)
		}
	}
	// With no numbering supplied there is no number to read, so every
	// operand is out of range rather than being answered from a table of
	// this engine's own.
	if out, st, _ := numberedRun(t, nil, `ulimit -N 0`); st != 1 || !strings.Contains(out, "no limit there") {
		t.Errorf("with no order: %q at %d, want the refusal", out, st)
	}
}

// And a dialect with no such option keeps its ordinary bad-option refusal,
// which is what the zero value means.
func TestWithoutTheOptionTheLetterIsStillBad(t *testing.T) {
	out, st := ulimitRunPlain(t, `ulimit -N 0`)
	if st == 0 || !strings.Contains(out, "invalid option") {
		t.Errorf("%q at %d, want the bad-option refusal", out, st)
	}
}

func ulimitRunPlain(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, _ := ulimitRun(t, limits(map[Resource]limitPair{}), nil, src)
	return out, st
}
