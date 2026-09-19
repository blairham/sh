// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"syscall"
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

// ulimitRunTable is ulimitRun with a table: the rows a dialect lists, the
// limits this pretend kernel has, and the order it numbers them in.
func ulimitRunTable(t *testing.T, held map[Resource]limitPair, dg Diagnostics, has func(Resource) bool, order []Resource, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UlimitBlockIsKilobyte = No
	sem.UlimitSetsBothLimits = Yes
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.GetRlimit = func(res Resource) (int64, int64, error) {
		p := held[res]
		return p.soft, p.hard, nil
	}
	r.SetRlimit = func(res Resource, soft, hard int64) error {
		held[res] = limitPair{soft, hard}
		return nil
	}
	r.HasRlimit, r.RlimitOrder = has, order
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// residentSetTable is a table naming the two resources the substrate's own
// letters leave out, each on the letter the measured tables give it.
var residentSetTable = Diagnostics{UlimitListing: []UlimitListingRow{
	{Prefix: "resident set ", Letter: 'm', Res: ResourceResidentSet, Scale: 1024},
	{Prefix: "processes ", Letter: 'u', Res: ResourceProcesses, Scale: 1},
}}

// TestUlimitLettersComeFromTheTable: a shell reads exactly the letters its own
// `ulimit -a` prints, which is what makes the set answerable per platform —
// the row goes and the letter goes with it (#2805).
//
// The substrate's own fallback, for a caller that has described no table, is
// POSIX's: it names neither the resident set nor the process count, and two
// of the measured columns do not have them either.
func TestUlimitLettersComeFromTheTable(t *testing.T) {
	held := limits(map[Resource]limitPair{ResourceResidentSet: {1024, 1024}, ResourceProcesses: {50, 50}})
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`ulimit -m`, "1"},
		{`ulimit -u`, "50"},
	} {
		out, st := ulimitRunTable(t, held, residentSetTable, nil, nil, tc.src)
		if st != 0 || strings.TrimSpace(out) != tc.want {
			t.Errorf("%s against a table that names it: %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
		out, st, _ = ulimitRun(t, held, nil, tc.src)
		if st == 0 {
			t.Errorf("%s with no table: %q status %d, want it refused — the substrate's letters are POSIX's", tc.src, out, st)
		}
	}
	// And the letter is the row's own rather than the resource's: one column
	// spells the process count `-p`, which is the pipe buffer in two others.
	dg := Diagnostics{UlimitListing: []UlimitListingRow{
		{Prefix: "process ", Letter: 'p', Res: ResourceProcesses, Scale: 1},
	}}
	if out, st := ulimitRunTable(t, held, dg, nil, nil, `ulimit -p`); st != 0 || strings.TrimSpace(out) != "50" {
		t.Errorf("-p against a table that spells the process count with it: %q status %d", out, st)
	}
	if out, st := ulimitRunTable(t, held, dg, nil, nil, `ulimit -u`); st == 0 {
		t.Errorf("-u against that table: %q status %d, want it refused", out, st)
	}
}

// TestALetterLeavesWithItsRow: a limit this kernel does not have is not a row
// that reads zero and not a letter that reads one — both go, and nothing else
// moves. This is the macOS half of #2805, asserted on a machine that may be
// either.
func TestALetterLeavesWithItsRow(t *testing.T) {
	held := limits(map[Resource]limitPair{ResourceFileLocks: {77, 77}, ResourceCPUTime: {60, 60}})
	dg := Diagnostics{UlimitListing: []UlimitListingRow{
		{Prefix: "cpu time  ", Letter: 't', Res: ResourceCPUTime, Scale: 1},
		{Prefix: "file locks ", Letter: 'x', Res: ResourceFileLocks, Scale: 1},
	}}
	everything := func(Resource) bool { return true }
	if out, st := ulimitRunTable(t, held, dg, everything, nil, `ulimit -x`); st != 0 || strings.TrimSpace(out) != "77" {
		t.Errorf("-x on a kernel that has it: %q status %d", out, st)
	}
	without := func(res Resource) bool { return res != ResourceFileLocks }
	out, st := ulimitRunTable(t, held, dg, without, nil, `ulimit -x`)
	if st == 0 {
		t.Errorf("-x on a kernel that has no such limit: %q status %d, want it refused", out, st)
	}
	if out, st := ulimitRunTable(t, held, dg, without, nil, `ulimit -a`); st != 0 || out != "cpu time  60\n" {
		t.Errorf("`ulimit -a` without that limit: %q status %d, want the other row alone", out, st)
	}
}

// TestAFixedRowReadsItsSentenceAndRefusesToBeSet: one column lists rows that
// are a sentence rather than a limit, reads the letter on every platform
// because nothing behind it is the kernel's, and refuses to set one in its own
// words with the row's own short name.
func TestAFixedRowReadsItsSentenceAndRefusesToBeSet(t *testing.T) {
	held := limits(map[Resource]limitPair{})
	dg := Diagnostics{
		UlimitListing: []UlimitListingRow{
			{Prefix: "locks  ", Letter: 'x', Fixed: "not supported", Name: "locks"},
		},
		UlimitReadOnly: "ulimit: %[1]s: is read only",
	}
	none := func(Resource) bool { return false }
	if out, st := ulimitRunTable(t, held, dg, none, nil, `ulimit -x`); st != 0 || strings.TrimSpace(out) != "not supported" {
		t.Errorf("reading a fixed row: %q status %d, want its sentence at 0", out, st)
	}
	out, st := ulimitRunTable(t, held, dg, none, nil, `ulimit -x 5`)
	if st != 1 || !strings.Contains(out, "ulimit: locks: is read only") {
		t.Errorf("setting a fixed row: %q status %d, want the read-only refusal at 1", out, st)
	}
}

// TestTheKernelsOwnOrderPicksTheRowsAndTheirSequence is the other shape a
// table takes: one column lists in the order the kernel numbers its limits,
// one row per number, so a kernel that spells two limits with one number gets
// one row and the other letter is not read at all (#2806).
func TestTheKernelsOwnOrderPicksTheRowsAndTheirSequence(t *testing.T) {
	held := limits(map[Resource]limitPair{
		ResourceResidentSet:  {1024, 1024},
		ResourceAddressSpace: {2048, 2048},
		ResourceCPUTime:      {60, 60},
	})
	dg := Diagnostics{
		UlimitListing: []UlimitListingRow{
			{Prefix: "-t: cpu time ", Letter: 't', Res: ResourceCPUTime, Scale: 1},
			{Prefix: "-m: resident set ", Letter: 'm', Res: ResourceResidentSet, Scale: 1024},
			{Prefix: "-v: address space ", Letter: 'v', Res: ResourceAddressSpace, Scale: 1024},
		},
		UlimitListingInKernelOrder: true,
	}
	// A kernel that numbers the two alike, so its order carries one of them.
	shared := []Resource{ResourceCPUTime, ResourceAddressSpace}
	out, st := ulimitRunTable(t, held, dg, nil, shared, `ulimit -a`)
	if st != 0 || out != "-t: cpu time 60\n-v: address space 2\n" {
		t.Errorf("`ulimit -a` in the kernel's order: %q status %d", out, st)
	}
	if out, st := ulimitRunTable(t, held, dg, nil, shared, `ulimit -m`); st == 0 {
		t.Errorf("-m where that kernel prints no such row: %q status %d, want it refused", out, st)
	}
	// And a kernel that numbers them apart prints both, in its own sequence
	// rather than in the one the table is written in.
	apart := []Resource{ResourceCPUTime, ResourceResidentSet, ResourceAddressSpace}
	out, st = ulimitRunTable(t, held, dg, nil, apart, `ulimit -a`)
	if st != 0 || out != "-t: cpu time 60\n-m: resident set 1\n-v: address space 2\n" {
		t.Errorf("`ulimit -a` where the two are numbered apart: %q status %d", out, st)
	}
	if out, st := ulimitRunTable(t, held, dg, nil, apart, `ulimit -m`); st != 0 || strings.TrimSpace(out) != "1" {
		t.Errorf("-m there: %q status %d, want it read", out, st)
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

// The words `hard` and `soft` in a limit's place, which name the limits this
// resource has now. Two axes rather than one: zsh takes `hard` and refuses
// `soft`, so a single switch would have to be wrong about one of them.
func TestUlimitKeywordOperands(t *testing.T) {
	both := func(s *Semantics) { s.UlimitTakesHardKeyword, s.UlimitTakesSoftKeyword = Yes, Yes }
	held := limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
	out, st, held := ulimitRun(t, held, both, `ulimit -n hard`)
	if st != 0 || out != "" {
		t.Errorf("-n hard: %q status %d, want it taken in silence", out, st)
	}
	if got := held[ResourceOpenFiles]; got.soft != 4096 {
		t.Errorf("-n hard: soft=%d, want the ceiling 4096", got.soft)
	}
	// The floor written back, which is what `soft` is for beside -H.
	held = limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
	_, st, held = ulimitRun(t, held, both, `ulimit -Hn soft`)
	if got := held[ResourceOpenFiles]; st != 0 || got.hard != 256 {
		t.Errorf("-Hn soft: hard=%d status %d, want the floor 256", got.hard, st)
	}
	// The keyword is spent before a name can be looked up, which is the one
	// line where this shell and ksh93 answer differently for the same text.
	held = limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
	_, _, held = ulimitRun(t, held, both, "hard=333\nulimit -n hard")
	if got := held[ResourceOpenFiles]; got.soft != 4096 {
		t.Errorf("hard=333 then -n hard: soft=%d, want the ceiling 4096 and not the variable", got.soft)
	}
	// And where the dialect has no such word it is a number it could not read.
	for _, c := range []struct {
		src   string
		tweak func(*Semantics)
	}{
		{`ulimit -n hard`, nil},
		{`ulimit -n soft`, nil},
		{`ulimit -n soft`, func(s *Semantics) { s.UlimitTakesHardKeyword = Yes }},
	} {
		held = limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
		out, st, held = ulimitRun(t, held, c.tweak, c.src)
		if st == 0 || !strings.Contains(out, "invalid number") {
			t.Errorf("%s without the word: %q status %d, want a bad-number refusal", c.src, out, st)
		}
		if got := held[ResourceOpenFiles]; got.soft != 256 {
			t.Errorf("%s without the word: soft moved to %d", c.src, got.soft)
		}
	}
}

// A signed number is a number to Go's reader and to no shell on the panel.
// Four dialects set a limit from `+1999` at status 0 and in silence until
// #3060, which is the shape #2298 is about.
func TestUlimitRefusesWhatIsNotAPlainNumber(t *testing.T) {
	// The empty operand is deliberately not here: bash 5.3 refuses it and
	// bash 3.2, zsh, ksh93, dash and BusyBox ash take it as a line that
	// changes nothing, so it is an axis of its own rather than part of this
	// one. This shell has always refused it, which is what #3064 is for.
	for _, word := range []string{"+1999", "-1999", " 99", "0x10", "1000+999", "99abc"} {
		held := limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
		out, st, held := ulimitRun(t, held, nil, `ulimit -n `+quoteOperand(word))
		if st == 0 {
			t.Errorf("-n %q: status 0 saying %q, want it refused", word, out)
		}
		if got := held[ResourceOpenFiles]; got.soft != 256 {
			t.Errorf("-n %q: soft moved to %d", word, got.soft)
		}
	}
	// The word every shell does take is still taken.
	held := limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
	_, st, held := ulimitRun(t, held, nil, `ulimit -n unlimited`)
	if got := held[ResourceOpenFiles]; st != 0 || got.soft != RlimitInfinity {
		t.Errorf("-n unlimited: soft=%d status %d", got.soft, st)
	}
}

func quoteOperand(s string) string { return "'" + s + "'" }

// ksh93's reading: the operand is an expression, and a bare name in it must
// be set — which is the one place this position is stricter than `$(( ))`.
func TestUlimitArithmeticOperand(t *testing.T) {
	arith := func(s *Semantics) { s.UlimitOperandIsArithmetic = Yes }
	for _, c := range []struct {
		src  string
		want int64
	}{
		{`ulimit -n '+1999'`, 1999},
		{`ulimit -n 1000+999`, 1999},
		{`ulimit -n 0x10`, 16},
		{`ulimit -n ' 99'`, 99},
		{"hard=333\nulimit -n hard", 333},
	} {
		held := limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
		out, st, held := ulimitRun(t, held, arith, c.src)
		if got := held[ResourceOpenFiles]; st != 0 || got.soft != c.want {
			t.Errorf("%s: soft=%d status %d saying %q, want %d", c.src, got.soft, st, out, c.want)
		}
	}
	// An unset name is the refusal, not a zero: `ulimit -n hard` is how that
	// shell says it has no keyword.
	held := limits(map[Resource]limitPair{ResourceOpenFiles: {256, 4096}})
	out, st, held := ulimitRun(t, held, arith, `ulimit -n hard`)
	if st == 0 {
		t.Errorf("-n hard as arithmetic: status 0 saying %q, want the unset name refused", out)
	}
	if got := held[ResourceOpenFiles]; got.soft != 256 {
		t.Errorf("-n hard as arithmetic: soft moved to %d", got.soft)
	}
}

// The kernel's refusal names the resource and keeps the C string's capital.
func TestUlimitCannotChangeNamesTheResource(t *testing.T) {
	f, err := syntax.Parse(`ulimit -n 200`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{
		UlimitCannotChange: "ulimit: %[1]s: cannot modify limit: %[3]s",
		UlimitListing: []UlimitListingRow{
			{Prefix: "open files                          (-n) ", Letter: 'n', Res: ResourceOpenFiles, Scale: 1},
		},
	}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.GetRlimit = func(Resource) (int64, int64, error) { return 100, 100, nil }
	r.SetRlimit = func(Resource, int64, int64) error { return syscall.EPERM }
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if want := "ulimit: open files: cannot modify limit: Operation not permitted"; !strings.Contains(buf.String(), want) {
		t.Errorf("output = %q, want it to contain %q", buf.String(), want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}
