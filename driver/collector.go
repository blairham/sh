// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"runtime/debug"
)

// What a shell binary tells the garbage collector, and why a binary is the
// only place that may say it.
//
// # The measurement
//
// A warm interactive startup on a real `~/.zshrc` — powerlevel10k and a
// plugin manager — spends most of its time in the collector and the
// allocator rather than in anything this repository wrote. The flat profile
// of one, at 1000Hz: `runtime.madvise` 21%, `memclrNoHeapPointers` 9%,
// `tryDeferToSpanScan` 9%, `scanObjectsSmall` 10% cumulative,
// `mallocgc` 16% cumulative. Nothing of ours reaches 1% flat.
//
// That is the shape of a **short allocation burst** collected at the default
// GOGC=100 the whole way through. Measured on the same configuration, CPU
// time, each binary in its own block after four warm-up runs, median of
// seven, with peak RSS taken from a run of its own:
//
//	real zsh          137ms    20MB
//	default           454ms    60MB
//	GOGC=400          312ms    90MB
//	GOGC=800          287ms    98MB
//	GOGC=1600         282ms   111MB
//	GOGC=off          267ms   155MB
//
// **The knee moves as the shell allocates less, so it is re-measured rather
// than inherited.** #2066 chose 400 on the evidence then; 800 replaced it
// when a startup's allocation fell from 154MB to ~127MB, because the same
// threshold holds a smaller heap and the headroom is cheaper. A constant
// like this is a measurement, not a decision.
//
// It has moved again, and for the same reason. With the two per-command
// copies gone — a discarded [syntax.Core] per nested parse and a 3280-byte
// semantics vector per option that was already where it was being set — the
// same startup allocates 228MB where it allocated 494MB, and the threshold
// that suits that is higher. Measured 2026-09-24 on the same configuration,
// the settings interleaved so a load spike lands on all of them, median of
// seven, peak RSS from `/usr/bin/time -l`:
//
//	GOGC=800          520ms   190MB
//	GOGC=1600         473ms   205MB
//	GOGC=2400         483ms   178MB
//	GOGC=4000         471ms   185MB
//	GOGC=8000         465ms   253MB
//	GOGC=off          458ms   253MB
//
// 1600 is the knee: −9% against 800, and everything above it is within the
// noise of itself until the collector is off altogether, which buys 3% for
// another 50MB. Paired against 800 — thirty runs, alternating which went
// first — it is −38ms, faster in 24 of 24.
//
// # How it has to be measured, because the obvious way does not work
//
// Blocks of one binary and then blocks of the other **cannot resolve this**
// on a machine doing anything else. Measured that way the same pair answered
// −8%, then −1.7%, and a control — `GOGC=800` in the environment against the
// same binary — came back 320ms and then 285ms. A 35ms swing between rounds
// is larger than the 25ms being looked for, and on that evidence this change
// was discarded once as unshowable.
//
// What works is **interleaving the two builds and pairing the results**: one
// run of each per repetition, alternating which goes first, and reading the
// per-repetition difference rather than two medians. Drift that moves both
// arms cancels. Thirty pairs, twice:
//
//	run 1   median −22ms   p25 −26   p75 −19
//	run 2   median −24ms   p25 −26   p75 −20   faster in 30 of 30
//
// Every quartile negative, and the direction unanimous.
//
// This is safe here and is *not* safe against real zsh: two builds of this
// shell write the same completion dump, where alternating with zsh makes the
// two invalidate each other's `~/.zcompdump` and produces a clean, repeatable,
// entirely false bimodal split (#2036).
//
// # Why this is in driver and not in interp
//
// The GC percent is **process-wide state**, and nothing under `interp/` may
// change any: a Runner is embedded in other programs, and a library that
// retunes its host's collector is borrowing exactly what the `os.Chdir` and
// `syscall.Exec` rules exist to stop it borrowing. A shell *binary* is the
// one place the process-wide question is the right one to ask — the same
// reasoning that puts Runner.ReplaceProcess and Runner.DieBySignal here.
//
// It is called from [Main] and deliberately not from [MainArgs]: MainArgs is
// what the tests run, in their own process, and a front end that retuned the
// test binary's collector on every call would be doing the thing this comment
// says not to do.
const startupGCPercent = 1600

// # The ceiling beside the percent
//
// A percent has no ceiling of its own. At 1600 the collector lets the heap
// grow to seventeen times what survived the last collection, which is the
// right trade for a startup's burst and the wrong one for a script whose live
// heap is itself large: `for i in {1..2000000}` held 484MB where the default
// percent holds 143MB, and #6364 saw a 1.5GB outlier on a string append under
// load. A shell is rarely one process — `make -j`, a test suite, a
// `find -exec sh -c` — and #6307 declined an 820MB startup peak on exactly
// that reasoning.
//
// So the percent is paired with a soft memory limit, the runtime's own
// GOMEMLIMIT, set here rather than lowered for scripts. Below the limit it
// does nothing at all: the collector runs at 1600 and every heap that fits
// keeps the fast policy. Near it the collector works harder, so a runaway heap
// is held at the ceiling rather than at seventeen times its live size.
// Lowering the percent for scripts was the alternative, and it costs 10 to 80%
// CPU on every loop to save memory most of them never needed.
//
// **The value is set by the one workload it must not touch**: the
// interactive real-config start, which is what 1600 was chosen for. Measured
// 2026-10-11 with #5873's instrument (a pty at 120x40, a throwaway HOME with
// the rc symlinked and the zi tree cloned, `getrusage` of the child), main at
// `ee80b4336`, load 4-17. First the same binary in both arms, GOMEMLIMIT set
// in one, to choose the value:
//
//	limit      paired median   faster    peak RSS (none / limit)
//	128MiB       +198ms          0/12      259MB / 135MB
//	384MiB        +10ms          9/20      261MB / 261MB
//	512MiB        -62ms         15/20      261MB / 260MB
//
// A start allocates about 485MB (its peak with the collector off), collects
// twice at 1600 with a largest goal of about 190MB, and peaks at 260-340MB
// RSS depending on load. 128MiB binds on it — six or seven collections, a
// fifth more CPU — which is the instrument showing it can see a limit at all;
// 384MiB is the lowest round value with room above that peak. Then this
// change against its parent, interleaved, three batches with the order
// swapped in one: +31ms (12/24 slower), +56ms (10/24 faster), −34ms (15/30
// faster), against a same-binary control at −5ms (11/20). Null, and not only
// by CPU: with GODEBUG=gctrace=1 both builds collect the same two times at the
// same goals, so below the ceiling the collector is doing exactly what it did.
//
// The rows it was asked to bound, at the shipped setting, before and after,
// twelve pairs each:
//
//	program (bash dialect)             peak RSS       CPU, paired
//	for i in {1..2000000}              484 → 383MB    −0.2ms of 1.7s, 7/12 faster
//	1,000,000 `a+=(x$i)`               531 → 416MB    +30ms of 2.8s, 5/12 faster
//
// Every row of the #6352 sweep now peaks well under it — about 120MB at the
// worst, and the loops at about 80MB, which is the percent's floor of sixteen
// times four megabytes rather than anything they keep — because the
// allocation behind its worst rows has been cut at the source (#6357, #6359):
// `{1..200000}`, 40,000 appends and 30,000 assoc keys, at 628, 323 and 311MB
// when the sweep was taken, are 77, 80 and 118MB on this main. So the limit
// leaves every one of them alone: a gctrace run of every row in all four
// dialect binaries collects the same number of times in both builds (two rows
// differed once, each where the parent overshot under load, and agreed on
// three repeats), and eighteen rows paired twenty times in bash and zsh moved
// by 3ms at most.
//
// Like the percent, this is a measurement and not a decision. A configuration
// that allocates more, or a percent that moves, moves the peak it has to clear,
// and it is re-measured with the same instrument rather than inherited.
//
// The limit is soft. It is a target the collector steers toward, not a
// refusal: a script whose *live* heap is larger than it still runs, and the
// runtime caps the collector's share of the CPU rather than letting it spin.
const runawayHeapLimit = 384 << 20

// tuneCollector raises the collector's threshold for a shell binary and puts
// a ceiling beside it, unless the environment has already named them.
//
// An explicit GOGC is left alone because the runtime has already applied it
// and the person who set it knows more about their machine than this constant
// does — including that they may have set it to `off`, which SetGCPercent
// spells as a negative number and which this must not quietly undo.
//
// An explicit GOMEMLIMIT is left alone for the same reason. And a GOGC alone
// leaves the limit unset as well, because the limit is the ceiling on *this*
// percent: a person who chose their own, `off` most of all, has taken the
// collector's policy into their own hands, and a ceiling they did not ask for
// would turn `off` into "collect at 384MiB".
func tuneCollector() {
	if _, named := os.LookupEnv("GOGC"); named {
		return
	}
	debug.SetGCPercent(startupGCPercent)
	if _, named := os.LookupEnv("GOMEMLIMIT"); named {
		return
	}
	debug.SetMemoryLimit(runawayHeapLimit)
}
