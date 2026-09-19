// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// ksh93u+ 2012-08-01 prints one table on two kernels and seven of its rows are
// the kernel's rather than the engine's. Measured 2026-09-18 on macOS arm64 and
// on Debian bullseye's linux/arm64 build of the same version, `cmd/ksh`
// cross-compiled and run in the same container (#3693):
//
//	row                                  macOS           Linux
//	locks (-x)                           not supported   unlimited
//	message queue size (Kibytes) (-q)    not supported   800
//	nice (-e)                            not supported   0
//	rtprio (-r)                          not supported   0
//	sigpend (-i)                         undefined       192129
//	pipe buffer size (bytes) (-p)        512             4096
//	socket buffer size (bytes) (-b)      512             4096
//
// `swap (-w)` and `threads (-T)` print `not supported` on **both** kernels, so
// those two are the engine's own sentence and stay fixed. The layout does not
// move: the same twenty rows, the same order, the same labels, the same widths.
//
// The limits are the hooks' answers rather than this process's, so the test
// asks the same question on two pretend kernels in one run — which is the only
// way either column can be checked on a machine that is one of them.

// ulimitKernel runs a snippet against a ksh runner whose kernel has the limits
// has() names, each reading the number limits() gives it.
func ulimitKernel(t *testing.T, has func(interp.Resource) bool, limits map[interp.Resource]int64, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "ksh", Dir: t.TempDir(), Dialect: presetDialect(),
	}
	r.HasRlimit = has
	r.GetRlimit = func(res interp.Resource) (int64, int64, error) {
		v, ok := limits[res]
		if !ok {
			v = interp.RlimitInfinity
		}
		return v, v, nil
	}
	r.SetRlimit = func(interp.Resource, int64, int64) error { return nil }
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return buf.String(), st
}

// The two kernels, as the hooks see them: one that numbers all six of the
// limits this engine's table reaches for, and one that numbers none of them.
var (
	kshLinuxKernel = map[interp.Resource]int64{
		interp.ResourceFileLocks:          interp.RlimitInfinity,
		interp.ResourceMessageQueues:      819200,
		interp.ResourceSchedulingPriority: 0,
		interp.ResourceRealtimePriority:   0,
		interp.ResourcePendingSignals:     192129,
		interp.ResourcePipeBuffer:         4096,
	}
	kshDarwinKernel = map[interp.Resource]int64{
		interp.ResourcePipeBuffer: 512,
	}
)

func kshLinuxHas(interp.Resource) bool { return true }

func kshDarwinHas(res interp.Resource) bool {
	switch res {
	case interp.ResourceFileLocks, interp.ResourceMessageQueues,
		interp.ResourceSchedulingPriority, interp.ResourceRealtimePriority,
		interp.ResourcePendingSignals:
		return false
	}
	return true
}

func TestTheSevenKernelRowsOfThisTableAreNotTheEnginesOwnSentence(t *testing.T) {
	for _, tc := range []struct {
		kernel string
		has    func(interp.Resource) bool
		limits map[interp.Resource]int64
		want   map[string]string
	}{
		{
			kernel: "a kernel with all six of these limits",
			has:    kshLinuxHas,
			limits: kshLinuxKernel,
			want: map[string]string{
				"locks                          (-x)": "unlimited",
				"message queue size (Kibytes)   (-q)": "800",
				"nice                           (-e)": "0",
				"rtprio                         (-r)": "0",
				"sigpend                        (-i)": "192129",
				"pipe buffer size (bytes)       (-p)": "4096",
				"socket buffer size (bytes)     (-b)": "4096",
				"swap size (Kibytes)            (-w)": "not supported",
				"threads                        (-T)": "not supported",
			},
		},
		{
			kernel: "a kernel with none of them",
			has:    kshDarwinHas,
			limits: kshDarwinKernel,
			want: map[string]string{
				"locks                          (-x)": "not supported",
				"message queue size (Kibytes)   (-q)": "not supported",
				"nice                           (-e)": "not supported",
				"rtprio                         (-r)": "not supported",
				"sigpend                        (-i)": "undefined",
				"pipe buffer size (bytes)       (-p)": "512",
				"socket buffer size (bytes)     (-b)": "512",
				"swap size (Kibytes)            (-w)": "not supported",
				"threads                        (-T)": "not supported",
			},
		},
	} {
		out, st := ulimitKernel(t, tc.has, tc.limits, `ulimit -a`)
		if st != 0 {
			t.Fatalf("%s: `ulimit -a` exited %d: %q", tc.kernel, st, out)
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		// Twenty rows on either kernel: a limit this kernel lacks is a row
		// this engine keeps and answers with a sentence, where the other
		// four columns drop the row and refuse the letter (#2806).
		if len(lines) != 20 {
			t.Errorf("%s: %d rows, want the twenty this engine prints on both:\n%s", tc.kernel, len(lines), out)
		}
		got := make(map[string]string, len(lines))
		for _, line := range lines {
			// The label ends at the option letter in parentheses, which is
			// the last one on the line: no value here has a bracket in it.
			end := strings.LastIndexByte(line, ')')
			if end < 0 {
				t.Errorf("%s: unreadable row %q", tc.kernel, line)
				continue
			}
			got[line[:end+1]] = strings.TrimSpace(line[end+1:])
		}
		for label, want := range tc.want {
			if got[label] != want {
				t.Errorf("%s: %q reads %q, want %q", tc.kernel, label, got[label], want)
			}
		}
	}
}

// TestAPipeBufferIsReadFromTheKernelAndStillRefusesToBeSet: the two buffer rows
// are the one pair whose number moves with the kernel while the refusal does
// not. `ulimit -p 100` is `ulimit: pipe: is read only` at 1 on both, measured
// with the same script file on each.
func TestAPipeBufferIsReadFromTheKernelAndStillRefusesToBeSet(t *testing.T) {
	for _, tc := range []struct {
		letter, name string
	}{{"p", "pipe"}, {"b", "sbsize"}} {
		out, st := ulimitKernel(t, kshLinuxHas, kshLinuxKernel, "ulimit -"+tc.letter+" 100")
		if want := "ksh: ulimit: " + tc.name + ": is read only\n"; st != 1 || out != want {
			t.Errorf("-%s 100: %q at %d, want %q at 1", tc.letter, out, st, want)
		}
	}
}

// TestALimitThisKernelHasIsSetThroughTheSameLetter is the control the table
// above cannot be: a row whose sentence is gone must be a limit in every
// respect, not only in what it prints. `ulimit -x 100` is silent at 0 on the
// kernel that has file locks and the read-only refusal on the one that does
// not — both measured on ksh93u+ 2012-08-01.
func TestALimitThisKernelHasIsSetThroughTheSameLetter(t *testing.T) {
	if out, st := ulimitKernel(t, kshLinuxHas, kshLinuxKernel, `ulimit -x 100`); st != 0 || out != "" {
		t.Errorf("-x 100 where the kernel has file locks: %q at %d, want silence at 0", out, st)
	}
	out, st := ulimitKernel(t, kshDarwinHas, kshDarwinKernel, `ulimit -x 100`)
	if want := "ksh: ulimit: locks: is read only\n"; st != 1 || out != want {
		t.Errorf("-x 100 where it does not: %q at %d, want %q at 1", out, st, want)
	}
}
