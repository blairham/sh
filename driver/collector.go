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
//	real zsh          129ms    21MB
//	default           454ms    60MB
//	GOGC=400          325ms    90MB
//	GOGC=800          300ms   116MB
//	GOGC=1600         291ms   130MB
//
// So 400 is −29% for +30MB, and the curve flattens hard after it: another
// 56MB buys a further 8%. It is the knee, which is why it is the number
// rather than the largest one that helps (#2066).
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
const startupGCPercent = 400

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
