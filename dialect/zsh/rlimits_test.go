// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `limit` and `unlimit`, the other two builtins of `zsh/rlimits` — the module
// this shell refused outright, so `B12limit.ztst` skipped itself rather than
// running (#4736).
//
// Measured 2026-09-26 on zsh 5.9.2, `-f` under `env -i PATH=/usr/bin:/bin`,
// on macOS (aarch64-apple-darwin25.4.0) and in the digest-pinned
// `ghcr.io/blairham/sh/zsh@sha256:aab8255c…` image
// (zsh 5.9.2, aarch64-unknown-linux-gnu) — the second is where the seven rows
// macOS has no limit behind were measured, `rt_time`'s `us` suffix among
// them.
//
// Every case here supplies **its own** limits. A test that read the machine's
// would be asking what this laptop grants rather than what the builtin does,
// and would answer differently on a runner.

// limitFixture is a kernel of nine resources in a fixed order, which is the
// macOS numbering — chosen because it is the one with a row the other has
// not, so a case that addressed a row by number would notice.
func limitFixture() *dialecttest.Rlimits {
	return &dialecttest.Rlimits{
		Order: []interp.Resource{
			interp.ResourceCPUTime,
			interp.ResourceFileSize,
			interp.ResourceData,
			interp.ResourceStack,
			interp.ResourceCore,
			interp.ResourceAddressSpace,
			interp.ResourceLockedMemory,
			interp.ResourceProcesses,
			interp.ResourceOpenFiles,
		},
		Soft: map[interp.Resource]int64{
			interp.ResourceStack:     8176 * 1024,
			interp.ResourceCore:      0,
			interp.ResourceProcesses: 10666,
			interp.ResourceOpenFiles: 1048576,
		},
		Hard: map[interp.Resource]int64{
			interp.ResourceStack:     64512 * 1024,
			interp.ResourceProcesses: 16000,
		},
	}
}

func runLimit(t *testing.T, lim *dialecttest.Rlimits, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir(), Rlimits: lim}, src+"\n")
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The listing, in the kernel's own order and with the kernel's own
// membership — the same table `ulimit -a` is laid out by here, which is why
// it is read from the runner rather than written out a second time.
//
// Three of the rows are the measurement rather than decoration. `stacksize`
// is 8176 kB and prints `7MB`, so the division truncates and does not round.
// `coredumpsize` is nought and prints `0kB`, which says a size is never
// written plain. And `maxproc` and `descriptors` are plain counts, which says
// the unit belongs to the resource: a shell that wrote every number as a size
// would put `10kB` in the maxproc row.
func TestLimitListsTheKernelsTableInItsOwnOrder(t *testing.T) {
	out, st := runLimit(t, limitFixture(), "limit")
	want := "cputime         unlimited\n" +
		"filesize        unlimited\n" +
		"datasize        unlimited\n" +
		"stacksize       7MB\n" +
		"coredumpsize    0kB\n" +
		"addressspace    unlimited\n" +
		"memorylocked    unlimited\n" +
		"maxproc         10666\n" +
		"descriptors     1048576\n"
	if out != want || st != 0 {
		t.Errorf("limit = %q (status %d), want %q", out, st, want)
	}
}

// `-h` reads the other limit, and the two rows that differ are what say so.
func TestLimitReadsTheHardLimits(t *testing.T) {
	out, st := runLimit(t, limitFixture(), "limit -h stacksize\nlimit -h maxproc\nlimit stacksize")
	want := "stacksize       63MB\nmaxproc         16000\nstacksize       7MB\n"
	if out != want || st != 0 {
		t.Errorf("limit -h = %q (status %d), want %q", out, st, want)
	}
}

// The three renderings, one value at a time. `1023kB` against `1MB` is the
// pair that fixes the boundary at exactly one megabyte, and `1500` — which
// is kilobytes, so 1536000 bytes — is the one that says the megabyte figure
// is floored rather than rounded to the nearest.
func TestLimitWritesEachResourceInItsOwnUnit(t *testing.T) {
	for _, c := range []struct{ set, want string }{
		{"limit filesize 0", "filesize        0kB\n"},
		{"limit filesize 1", "filesize        1kB\n"},
		{"limit filesize 1023", "filesize        1023kB\n"},
		{"limit filesize 1024", "filesize        1MB\n"},
		{"limit filesize 1500", "filesize        1MB\n"},
		{"limit filesize 1k", "filesize        1kB\n"},
		{"limit filesize 7m", "filesize        7MB\n"},
		{"limit filesize 1g", "filesize        1024MB\n"},
		{"limit filesize unlimited", "filesize        unlimited\n"},
		{"limit cputime 59", "cputime         0:00:59\n"},
		{"limit cputime 90", "cputime         0:01:30\n"},
		{"limit cputime 3661", "cputime         1:01:01\n"},
		{"limit cputime 360000", "cputime         100:00:00\n"},
		{"limit descriptors 100", "descriptors     100\n"},
	} {
		t.Run(c.set, func(t *testing.T) {
			name := strings.Fields(c.set)[1]
			out, st := runLimit(t, limitFixture(), c.set+"\nlimit "+name)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.set, out, st, c.want)
			}
		})
	}
}

// A bare number is **kilobytes** for a size and the resource's own unit
// otherwise, which is the pair below: the same word `100` is 102400 bytes
// under `filesize` and a hundred descriptors under `descriptors`. A shell
// that scaled every operand the same way would pass one of these rows and
// fail the other.
func TestABareNumberIsKilobytesOnlyForASize(t *testing.T) {
	out, st := runLimit(t, limitFixture(),
		"limit filesize 100\nlimit filesize\nlimit descriptors 100\nlimit descriptors")
	want := "filesize        100kB\ndescriptors     100\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
}

// A resource is named by an **unambiguous prefix** or by the kernel's number,
// and the refusals are three different sentences rather than one.
func TestHowALimitResourceIsNamed(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"the whole word", "limit cputime", "cputime         unlimited\n", 0},
		{"a prefix", "limit cpu", "cputime         unlimited\n", 0},
		{"a longer prefix", "limit cput", "cputime         unlimited\n", 0},
		// `c` reaches cputime and coredumpsize, which is what makes it
		// ambiguous — and is why the refusal is its own sentence rather
		// than `no such resource`.
		{"an ambiguous prefix", "limit c", "zsh:limit:1: ambiguous resource specification: c\n", 1},
		{"a word that is nothing", "limit nosuchres", "zsh:limit:1: no such resource: nosuchres\n", 1},
		{"the kernel's number", "limit 2", "datasize        unlimited\n", 0},
		{"a number past the table", "limit 99", "zsh:limit:1: can't read limit: invalid argument\n", 1},
		// The word never reaches the number, so this is the resource
		// complaint and not the numeric one — and it is not `bad option`
		// either, which is what a shell reading `-1` as a letter bundle
		// would say.
		{"a negative number", "limit -1", "zsh:limit:1: no such resource: -1\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runLimit(t, limitFixture(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s = %q (status %d), want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// The letters are `-h` and `-s` and nothing else, on either builtin —
// measured over all 26 on both.
func TestTheLimitBuiltinsTakeTwoLettersAndNoMore(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"limit -x", "zsh:limit:1: bad option: -x\n"},
		{"unlimit -f", "zsh:unlimit:1: bad option: -f\n"},
	} {
		out, st := runLimit(t, limitFixture(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, c.want)
		}
	}
}

// A value is a word, and one that is not a number with an optional scaling
// letter is refused — `1:30` among them, which is the shape a *time* is
// printed in and may not be set from. That pair is the one worth having:
// without it, a reader could take the `H:MM:SS` rendering for a syntax.
func TestALimitValueThatIsNotANumberIsRefused(t *testing.T) {
	for _, word := range []string{"xyz", "1:30", "-5", "1x", "hard"} {
		t.Run(word, func(t *testing.T) {
			out, st := runLimit(t, limitFixture(), "limit filesize "+word)
			want := "zsh:limit:1: unknown scaling factor: " + word + "\n"
			if out != want || st != 1 {
				t.Errorf("out = %q (status %d), want %q at 1", out, st, want)
			}
		})
	}
}

// **`limit` sets the soft limit and `limit -h` sets the hard one**, and the
// second brings the soft one down with it where it would otherwise be left
// above the ceiling. The control is the first row: a plain set leaves the
// hard limit exactly where it was.
func TestWhichLimitASetReaches(t *testing.T) {
	out, st := runLimit(t, limitFixture(),
		"limit filesize 2m\nlimit filesize\nlimit -h filesize")
	if want := "filesize        2MB\nfilesize        unlimited\n"; out != want || st != 0 {
		t.Errorf("soft set = %q (status %d), want %q", out, st, want)
	}
	out, st = runLimit(t, limitFixture(),
		"limit -h filesize 2m\nlimit filesize\nlimit -h filesize")
	if want := "filesize        2MB\nfilesize        2MB\n"; out != want || st != 0 {
		t.Errorf("hard set = %q (status %d), want %q", out, st, want)
	}
}

// A soft limit above the ceiling is this builtin's own sentence, which names
// no resource at all — and the control beside it is the same value set under
// a hard limit that allows it.
func TestASoftLimitPastTheHardOneIsRefused(t *testing.T) {
	out, st := runLimit(t, limitFixture(), "limit maxproc 20000")
	if want := "zsh:limit:1: limit exceeds hard limit\n"; out != want || st != 1 {
		t.Errorf("out = %q (status %d), want %q at 1", out, st, want)
	}
	out, st = runLimit(t, limitFixture(), "limit maxproc 12000\nlimit maxproc")
	if want := "maxproc         12000\n"; out != want || st != 0 {
		t.Errorf("control = %q (status %d), want %q", out, st, want)
	}
}

// `unlimit` raises a soft limit to its hard one, which is the only thing a
// process without privilege can do — so the row that matters is the one
// whose ceiling is **not** infinity: `stacksize` comes back at the hard
// limit's 63MB rather than at `unlimited`.
func TestUnlimitRaisesASoftLimitToItsCeiling(t *testing.T) {
	out, st := runLimit(t, limitFixture(),
		"limit filesize 1m\nunlimit filesize\nlimit filesize\nunlimit stacksize\nlimit stacksize")
	want := "filesize        unlimited\nstacksize       63MB\n"
	if out != want || st != 0 {
		t.Errorf("unlimit = %q (status %d), want %q", out, st, want)
	}
}

// With no operand it reaches every resource, which the pair below separates
// from "it reached the first one": both a lowered `filesize` and the
// untouched `stacksize` move.
func TestUnlimitWithNoOperandReachesEveryResource(t *testing.T) {
	out, st := runLimit(t, limitFixture(),
		"limit filesize 1m\nunlimit\nlimit filesize\nlimit stacksize\nlimit maxproc")
	want := "filesize        unlimited\nstacksize       63MB\nmaxproc         16000\n"
	if out != want || st != 0 {
		t.Errorf("unlimit = %q (status %d), want %q", out, st, want)
	}
}

// And the module the two builtins are: it loads, it lists its three features,
// and `whence -w` calls all three builtins. The last row is the one the
// table's own rule is about — an entry without the builtins would make the
// module load and leave `limit` as `command not found`.
func TestTheRlimitsModuleLoadsWithItsThreeBuiltins(t *testing.T) {
	out, st := runLimit(t, limitFixture(),
		"zmodload zsh/rlimits\nprint l=$?\nzmodload -lF zsh/rlimits\nwhence -w limit unlimit ulimit")
	want := "l=0\n+b:limit\n+b:ulimit\n+b:unlimit\n" +
		"limit: builtin\nunlimit: builtin\nulimit: builtin\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
}
