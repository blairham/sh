// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `zsh/rlimits`: `limit` and `unlimit`, which are `ulimit` addressed by
// **word** rather than by letter — `limit cputime` where `ulimit -t` asks the
// same question.
//
// `ulimit` is the third feature of the same module and is already here, so
// this file is the other two and the module's table entry lands with them
// (#4736).
//
// # What the two builtins are
//
// Measured 2026-09-26 on zsh 5.9.2, `-f` under `env -i PATH=/usr/bin:/bin`,
// on macOS (aarch64-apple-darwin25.4.0) and again in the digest-pinned
// `ghcr.io/blairham/sh/zsh` image (zsh 5.9.2, aarch64-unknown-linux-gnu), one
// probe at a time:
//
//	limit                 every resource this kernel has, one per line
//	limit -h              the same, reading the hard limits
//	limit cputime         one resource
//	limit filesize 2m     set the soft limit
//	limit -h filesize 2m  set the hard limit, which brings the soft one down
//	unlimit               raise every soft limit to its hard one
//	unlimit filesize      raise one
//	unlimit -h filesize   raise its hard limit
//
// The letters are `-h` and `-s` and nothing else: `limit -x` and `unlimit -f`
// are both `bad option`, measured over all 26.
//
// # The listing is the same table `ulimit -a` prints, in the same order
//
// Row for row, on both kernels: nine rows on macOS and sixteen on Linux, in
// the kernel's own numbering — which is what `ulimit -a` is already laid out
// by here, so the presence of a row and its position come from
// [interp.Runner.RlimitOrder] and [interp.Runner.HasRlimit] rather than from a
// second list that could drift from the first.
//
// What is genuinely this builtin's own is the **vocabulary** — the word for
// each resource, and the unit each word's value is written in — which is why
// that is the whole of the table below.
//
// # Three renderings, measured one value at a time
//
// A size is written in **kB or MB**, from the raw byte count: `0kB` for
// nought, `1023kB` for 1047552 bytes, `1MB` for 1048576 and `1024MB` for a
// gigabyte — so it is MB from one megabyte up and kB below, by integer
// division either way and never rounded. `ulimit -f 1` is one 512-byte block
// and reads back `0kB`, which is the row that says the division truncates.
//
// A time is `H:MM:SS`, unpadded in the hours: `0:00:59`, `0:01:30`, `1:01:01`
// and `100:00:00`.
//
// `rt_time` is microseconds and is written `500us`. Everything else — a count
// of processes, of descriptors, of file locks, of queued signals, the nice
// ceiling, the real-time priority, and the **bytes** in the message queues —
// is written plain: `limit msgqueue 1000` reads back `1000` and not `0kB`.
//
// `unlimited` is the word for no limit in every one of them.
//
// # How a resource is named
//
// By an **unambiguous prefix** of a word, or by the kernel's own number.
// Measured: `cpu` and `cput` are both `cputime`, `c` is `ambiguous resource
// specification: c` at 1 — cputime and coredumpsize — and `rt` is ambiguous on
// Linux where `rt_priority` and `rt_time` are both rows. `limit 2` is the
// third row of the listing and `limit 99` is `can't read limit: invalid
// argument`, the same sentence `ulimit -N 99` gives, while `limit -1` is `no
// such resource: -1` because the word never reaches the number.
//
// # What a value may be
//
// `unlimited`, or digits with an optional scaling letter — `k`, `m` or `g`,
// case-insensitively — and a **bare number is kilobytes** for a size:
// `limit filesize 1024` and `limit filesize 1m` are the same line. A word
// that is neither is `unknown scaling factor: <word>` at 1, which is what
// `limit filesize xyz` and `limit filesize 1:30` both get — so the `M:SS`
// spelling a time is *printed* in is not one it may be set from.
//
// A value above the hard limit is `limit exceeds hard limit` at 1, which is
// this builtin's own sentence rather than the kernel's reason.

// limitUnit is how one resource's value is written back, which is a fact
// about the resource rather than about the shell — see the file comment for
// the three, each measured one value at a time.
type limitUnit int

const (
	// limitBytes is written kB below a megabyte and MB from one up.
	limitBytes limitUnit = iota
	// limitSeconds is written `H:MM:SS`.
	limitSeconds
	// limitMicroseconds is written with a `us` suffix.
	limitMicroseconds
	// limitPlain is written as the number it is.
	limitPlain
)

// limitResource is one row of this builtin's vocabulary: the word that names
// a resource here, and the unit its value is written in.
//
// Deliberately **not** a field on [interp.UlimitListingRow]. That struct is
// the layout of one shell's `ulimit -a` — a label, a letter, a scale — and
// this is a different shell's second name for the same limit; folding them
// would put one builtin's vocabulary in every dialect's table, including the
// four that have no such builtin. What *is* shared is which rows this kernel
// has and what order they come in, and that is read from the runner.
type limitResource struct {
	word string
	res  interp.Resource
	unit limitUnit
}

// limitResources is the word for each resource and the unit it is written in.
//
// The order here is not the listing's — that is the kernel's, through
// RlimitOrder — but it *is* the order a prefix is resolved against, which
// only matters for whether a prefix is ambiguous, and ambiguity does not
// depend on order.
//
// Sixteen rows, which is every resource this tree names. Nine of them are on
// macOS and all sixteen on Linux, measured on both; a row the kernel has no
// limit behind is not listed and its word is `no such resource`, which is the
// same rule `ulimit -a` already follows for its letters.
var limitResources = []limitResource{
	{"cputime", interp.ResourceCPUTime, limitSeconds},
	{"filesize", interp.ResourceFileSize, limitBytes},
	{"datasize", interp.ResourceData, limitBytes},
	{"stacksize", interp.ResourceStack, limitBytes},
	{"coredumpsize", interp.ResourceCore, limitBytes},
	{"resident", interp.ResourceResidentSet, limitBytes},
	{"maxproc", interp.ResourceProcesses, limitPlain},
	{"descriptors", interp.ResourceOpenFiles, limitPlain},
	{"memorylocked", interp.ResourceLockedMemory, limitBytes},
	{"addressspace", interp.ResourceAddressSpace, limitBytes},
	{"maxfilelocks", interp.ResourceFileLocks, limitPlain},
	{"sigpending", interp.ResourcePendingSignals, limitPlain},
	// Bytes, and written plain all the same: `limit msgqueue 1000` reads
	// back `1000`. Measured in the pinned Linux image, and it is the row
	// that says the unit is the resource's rather than "anything counted in
	// bytes is a size".
	{"msgqueue", interp.ResourceMessageQueues, limitPlain},
	{"nice", interp.ResourceSchedulingPriority, limitPlain},
	{"rt_priority", interp.ResourceRealtimePriority, limitPlain},
	{"rt_time", interp.ResourceRealtimeTime, limitMicroseconds},
}

const (
	limitKilobyte = int64(1024)
	limitMegabyte = limitKilobyte * 1024
)

// registerRlimits installs `limit` and `unlimit`.
//
// Unconditionally, like every other module builtin here: `whence -w limit` is
// `builtin` on a `zsh -f` that has loaded nothing, so the name is the shell's
// from the start and `zmodload zsh/rlimits` only reports whether it is there.
func registerRlimits(r *interp.Runner) {
	r.Register("limit", limitBuiltin)
	r.Register("unlimit", unlimitBuiltin)
}

// limitOptions reads the `-h` and `-s` letters off the front of a call. Every
// other letter is `bad option`, measured over all 26 on both builtins.
func limitOptions(r *interp.Runner, name string, args []string) (hard bool, rest []string, code int) {
	for len(args) > 0 {
		word := args[0]
		if word == "--" {
			return hard, args[1:], 0
		}
		if len(word) < 2 || word[0] != '-' {
			break
		}
		if _, err := strconv.Atoi(word); err == nil {
			// A negative number is an operand rather than an option bundle,
			// and it is the one that says so: `limit -1` is `no such
			// resource: -1` and not `bad option: -1`.
			break
		}
		for i := 1; i < len(word); i++ {
			switch word[i] {
			case 'h':
				hard = true
			case 's':
				hard = false
			default:
				r.Diagnosef("bad option: -%c\n", word[i])
				return false, nil, 1
			}
		}
		args = args[1:]
	}
	return hard, args, 0
}

// limitBuiltin is `limit`.
func limitBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	hard, rest, code := limitOptions(r, "limit", args)
	if code != 0 {
		return code
	}
	if r.GetRlimit == nil {
		// A Runner an embedder gave no limits to. The same shape every other
		// hook-backed builtin here takes: say so rather than report nothing,
		// which would read as a kernel with no limits at all.
		r.Diagnosef("can't read limit: invalid argument\n")
		return 1
	}
	if len(rest) == 0 {
		for _, row := range limitRows(r) {
			if code := limitReport(r, row, hard); code != 0 {
				return code
			}
		}
		return 0
	}
	for len(rest) > 0 {
		row, ok := limitLookup(r, "limit", rest[0])
		if !ok {
			return 1
		}
		if len(rest) == 1 {
			return limitReport(r, row, hard)
		}
		if code := limitSet(r, row, rest[1], hard); code != 0 {
			return code
		}
		rest = rest[2:]
	}
	return 0
}

// unlimitBuiltin is `unlimit`: a limit taken back off, which is the same
// write with `unlimited` as its value.
//
// With no operand it reaches **every** resource, measured — `limit filesize
// 1m; unlimit; limit filesize` is `unlimited`, and the same call under `-h`
// leaves the hard limits where they were rather than refusing.
//
// Raising a *soft* limit means raising it to the hard one rather than to
// infinity, which is the only thing a process without privilege can do:
// measured, `unlimit filesize` after `limit filesize 1m` reads back
// `unlimited` on a shell whose hard limit is unlimited, and the same call
// where the hard limit is 1MB leaves the soft one at 1MB rather than failing.
func unlimitBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	hard, rest, code := limitOptions(r, "unlimit", args)
	if code != 0 {
		return code
	}
	if r.GetRlimit == nil || r.SetRlimit == nil {
		r.Diagnosef("can't read limit: invalid argument\n")
		return 1
	}
	rows := limitRows(r)
	if len(rest) > 0 {
		rows = rows[:0]
		for _, word := range rest {
			row, ok := limitLookup(r, "unlimit", word)
			if !ok {
				return 1
			}
			rows = append(rows, row)
		}
	}
	for _, row := range rows {
		soft, max, err := r.GetRlimit(row.res)
		if err != nil {
			r.Diagnosef("can't read limit: invalid argument\n")
			return 1
		}
		newSoft, newHard := max, max
		if hard {
			newSoft, newHard = soft, interp.RlimitInfinity
			if soft == interp.RlimitInfinity {
				newSoft = interp.RlimitInfinity
			}
		}
		if err := r.SetRlimit(row.res, newSoft, newHard); err != nil {
			r.Diagnosef("can't change limit: %v\n", err)
			return 1
		}
	}
	return 0
}

// limitRows is the resources this kernel has, in the order it numbers them —
// the same order and the same membership `ulimit -a` is laid out by here, read
// from the runner rather than restated.
func limitRows(r *interp.Runner) []limitResource {
	byRes := make(map[interp.Resource]limitResource, len(limitResources))
	for _, row := range limitResources {
		byRes[row.res] = row
	}
	out := make([]limitResource, 0, len(limitResources))
	for _, res := range r.RlimitOrder {
		if row, ok := byRes[res]; ok {
			out = append(out, row)
		}
	}
	return out
}

// limitLookup resolves an operand to a resource: an unambiguous prefix of a
// word, or this kernel's own number for one.
//
// The prefix set is the *listed* rows and not every word this file knows,
// which is what keeps a name the kernel has no limit behind from making a
// prefix ambiguous on a platform where that row is not there.
func limitLookup(r *interp.Runner, name, word string) (limitResource, bool) {
	rows := limitRows(r)
	if n, err := strconv.Atoi(word); err == nil && n >= 0 {
		if n >= len(rows) {
			r.Diagnosef("can't read limit: invalid argument\n")
			return limitResource{}, false
		}
		return rows[n], true
	}
	var hits []limitResource
	for _, row := range rows {
		if row.word == word {
			return row, true
		}
		if strings.HasPrefix(row.word, word) {
			hits = append(hits, row)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], true
	case 0:
		r.Diagnosef("no such resource: %s\n", word)
	default:
		r.Diagnosef("ambiguous resource specification: %s\n", word)
	}
	return limitResource{}, false
}

// limitReport writes one row: the word, padded, and the value in its unit.
func limitReport(r *interp.Runner, row limitResource, hard bool) int {
	soft, max, err := r.GetRlimit(row.res)
	if err != nil {
		r.Diagnosef("can't read limit: invalid argument\n")
		return 1
	}
	v := soft
	if hard {
		v = max
	}
	fmt.Fprintf(r.Out(), "%-16s%s\n", row.word, limitValue(row.unit, v))
	return 0
}

// limitValue is a limit written the way this builtin writes it. See the file
// comment for the measurements behind each of the four.
func limitValue(unit limitUnit, v int64) string {
	if v == interp.RlimitInfinity {
		return "unlimited"
	}
	switch unit {
	case limitSeconds:
		return fmt.Sprintf("%d:%02d:%02d", v/3600, (v/60)%60, v%60)
	case limitMicroseconds:
		return strconv.FormatInt(v, 10) + "us"
	case limitBytes:
		if v >= limitMegabyte {
			return strconv.FormatInt(v/limitMegabyte, 10) + "MB"
		}
		return strconv.FormatInt(v/limitKilobyte, 10) + "kB"
	}
	return strconv.FormatInt(v, 10)
}

// limitSet writes one limit from an operand.
func limitSet(r *interp.Runner, row limitResource, word string, hard bool) int {
	v, ok := limitOperand(row.unit, word)
	if !ok {
		r.Diagnosef("unknown scaling factor: %s\n", word)
		return 1
	}
	soft, max, err := r.GetRlimit(row.res)
	if err != nil {
		r.Diagnosef("can't read limit: invalid argument\n")
		return 1
	}
	newSoft, newHard := v, max
	if hard {
		// The hard limit comes down and takes the soft one with it where it
		// would otherwise be left above the ceiling: measured, `limit -h
		// filesize 2m` reads back `2MB` from both `limit` and `limit -h`.
		newHard = v
		if aboveLimit(soft, v) {
			newSoft = v
		} else {
			newSoft = soft
		}
	} else if aboveLimit(v, max) {
		// This builtin's own sentence rather than the kernel's reason: the
		// refusal is measured as `limit exceeds hard limit` at 1, with the
		// resource named nowhere in it.
		r.Diagnosef("limit exceeds hard limit\n")
		return 1
	}
	if err := r.SetRlimit(row.res, newSoft, newHard); err != nil {
		r.Diagnosef("can't change limit: %v\n", err)
		return 1
	}
	return 0
}

// aboveLimit reports whether v is past ceiling, with infinity above
// everything and above itself never.
func aboveLimit(v, ceiling int64) bool {
	if ceiling == interp.RlimitInfinity {
		return false
	}
	return v == interp.RlimitInfinity || v > ceiling
}

// limitOperand reads a value: `unlimited`, or digits with an optional scaling
// letter. A bare number is kilobytes for a size and the resource's own unit
// otherwise — measured, `limit filesize 1024` and `limit filesize 1m` are the
// same line, while `limit descriptors 100` is a hundred descriptors.
func limitOperand(unit limitUnit, word string) (int64, bool) {
	if word == "unlimited" {
		return interp.RlimitInfinity, true
	}
	if word == "" {
		return 0, false
	}
	scale := int64(1)
	if unit == limitBytes {
		scale = limitKilobyte
	}
	digits := word
	switch last := word[len(word)-1]; last {
	case 'k', 'K':
		digits, scale = word[:len(word)-1], limitKilobyte
	case 'm', 'M':
		digits, scale = word[:len(word)-1], limitMegabyte
	case 'g', 'G':
		digits, scale = word[:len(word)-1], limitMegabyte*1024
	}
	if digits == "" {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, false
	}
	return n * scale, true
}
