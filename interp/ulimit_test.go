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
	for _, word := range []string{"+1999", "-1999", " 99", "0x10", "1000+999", "99abc", ""} {
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
			{Prefix: "open files                          (-n) ", Res: ResourceOpenFiles, Scale: 1},
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
