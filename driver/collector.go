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

// tuneCollector raises the collector's threshold for a shell binary, unless
// the environment has already named one.
//
// An explicit GOGC is left alone because the runtime has already applied it
// and the person who set it knows more about their machine than this constant
// does — including that they may have set it to `off`, which SetGCPercent
// spells as a negative number and which this must not quietly undo.
func tuneCollector() {
	if _, named := os.LookupEnv("GOGC"); named {
		return
	}
	debug.SetGCPercent(startupGCPercent)
}
