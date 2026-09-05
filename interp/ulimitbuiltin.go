// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
)

// `ulimit` reads and changes the limits this shell and its children run under.
//
// The other half of #117. Before the names were reserved, `ulimit -n 256` found
// /usr/bin/ulimit on PATH, set the limit in a child, and exited 0 — so a script
// that raised its file-descriptor limit before opening a thousand files got no
// error and no headroom.
//
// The limits live in the process, so the work is done by hooks the caller
// supplies. See Runner.GetRlimit and Runner.SetRlimit.

func init() {
	builtins["ulimit"] = biUlimit
}

// UlimitListingRow is one line of `ulimit -a`, whose table no two shells lay
// out alike: the labels, the order, which rows exist at all and what unit
// each is counted in are all the dialect's. The prefix is everything before
// the value, spacing included, so a row is the prefix and then the number.
type UlimitListingRow struct {
	// Prefix is the label column exactly as the engine writes it.
	Prefix string
	// Fixed, when non-empty, is the whole value: the rows about pipe and
	// socket buffers, and the ones an engine lists as unsupported, are not
	// resource limits and never move.
	Fixed string
	// Res and Scale are the live rows' inputs — the resource, and what one
	// printed unit is worth, zero meaning the dialect's block unit.
	Res   Resource
	Scale int64
}

// ulimitListing is `ulimit -a`: every row the dialect lists, soft limits
// unless -H asked for the hard ones — the same choice a single report makes.
func (r *Runner) ulimitListing(hard bool) int {
	rows := r.diag().UlimitListing
	if len(rows) == 0 {
		r.diagf("%s\n", r.unanswered("how `ulimit -a` is laid out"))
		r.status = 2
		r.unspecified = true
		return 2
	}
	blockUnit := int64(512)
	if r.ask(r.sem().UlimitBlockIsKilobyte, "`ulimit -f` counting in 1024-byte blocks") {
		blockUnit = kilobyte
	}
	if r.unspecified {
		return r.status
	}
	for _, row := range rows {
		if row.Fixed != "" {
			r.printf("%s%s\n", row.Prefix, row.Fixed)
			continue
		}
		unit := row.Scale
		if unit == 0 {
			unit = blockUnit
		}
		soft, max, err := r.GetRlimit(row.Res)
		if err != nil {
			r.diagf("ulimit: %v\n", err)
			return 1
		}
		v := soft
		if hard {
			v = max
		}
		if v == RlimitInfinity {
			r.printf("%sunlimited\n", row.Prefix)
			continue
		}
		r.printf("%s%d\n", row.Prefix, v/unit)
	}
	return 0
}

func biUlimit(r *Runner, _ context.Context, args []string) int {
	// `-H` and `-S` choose which limit is read or written; without either,
	// reading gives the soft one and writing sets both. Unanimous.
	var hard, soft, all bool
	// The default resource is `-f`, which is why bare `ulimit` reports the
	// file-size limit rather than a summary.
	res, scale := ResourceFileSize, int64(0)
	for len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		letters := args[0][1:]
		for i := 0; i < len(letters); i++ {
			switch c := letters[i]; c {
			case 'H':
				hard = true
			case 'S':
				soft = true
			case 'a':
				// The whole table, in the dialect's own layout — see
				// UlimitListingRow. All four shells have the letter.
				all = true
			default:
				var found bool
				res, scale, found = lookupResource(c)
				if !found || !r.hasResource(res) {
					r.diagf("%s\n", Wording(r.diag().UlimitBadOption,
						"ulimit: -%[1]s: invalid option", string(c)))
					// The usage line every other builtin's refusal is
					// followed by, in the dialects that print one. This
					// builtin has a refusal of its own — the letters are
					// resources rather than a fixed set — and had been
					// left out of the line that comes after it (#825).
					r.builtinUsageLine("ulimit")
					return orDefault(r.diag().UlimitBadOptionStatus, 2)
				}
			}
		}
		args = args[1:]
	}
	if r.GetRlimit == nil || r.SetRlimit == nil {
		r.diagf("ulimit: this shell was not given any limits to read or change\n")
		return 2
	}
	if all {
		return r.ulimitListing(hard)
	}
	unit := scale
	if unit == 0 {
		// The block scale, which is the one dialect question in the numbers:
		// 1024 bytes in bash, 512 in the other three.
		unit = 512
		if r.ask(r.sem().UlimitBlockIsKilobyte, "`ulimit -f` counting in 1024-byte blocks") {
			unit = kilobyte
		}
	}
	if len(args) == 0 {
		return r.reportLimit(res, hard, unit)
	}
	want, ok := parseLimit(args[0], unit)
	if !ok {
		r.diagf("%s\n", Wording(r.diag().UlimitBadNumber,
			"ulimit: %[1]s: invalid number", args[0]))
		return orDefault(r.diag().UlimitBadNumberStatus, 1)
	}
	cur, curHard, err := r.GetRlimit(res)
	if err != nil {
		r.diagf("ulimit: %v\n", err)
		return 1
	}
	// With neither flag, three of the four lower the ceiling as well as the
	// floor — which is what makes `ulimit -t 3600` irreversible. zsh moves
	// only the floor, so the same line there can be undone.
	newSoft, newHard := want, want
	switch {
	case hard && !soft:
		newSoft = cur
	case soft && !hard:
		newHard = curHard
	case !hard && !soft:
		if !r.ask(r.sem().UlimitSetsBothLimits, "`ulimit -t N` lowering the hard limit too") {
			newHard = curHard
		}
	}
	if err := r.SetRlimit(res, newSoft, newHard); err != nil {
		r.diagf("%s\n", Wording(r.diag().UlimitCannotChange,
			"ulimit: cannot change limit: %[1]s", err.Error()))
		return 1
	}
	return 0
}

// reportLimit prints one limit, or the word every shell uses for no limit.
func (r *Runner) reportLimit(res Resource, hard bool, unit int64) int {
	soft, max, err := r.GetRlimit(res)
	if err != nil {
		r.diagf("ulimit: %v\n", err)
		return 1
	}
	v := soft
	if hard {
		v = max
	}
	if v == RlimitInfinity {
		r.printf("unlimited\n")
		return 0
	}
	r.printf("%d\n", v/unit)
	return 0
}

// lookupResource maps an option letter to what it addresses.
func lookupResource(c byte) (Resource, int64, bool) {
	for _, e := range resourceLetters {
		if e.letter == c {
			return e.res, e.scale, true
		}
	}
	return 0, 0, false
}

// hasResource reports whether this dialect's `ulimit` addresses one.
//
// Two of the ten are not universal, which is measured rather than assumed:
// zsh has no `-m` and dash has no `-u`, and each reports the letter as an
// option it does not know.
func (r *Runner) hasResource(res Resource) bool {
	switch res {
	case ResourceResidentSet:
		return r.ask(r.sem().UlimitHasResidentSet, "`ulimit -m`")
	case ResourceProcesses:
		return r.ask(r.sem().UlimitHasProcessCount, "`ulimit -u`")
	}
	return true
}

// parseLimit reads a limit, in the units the shell prints it in.
func parseLimit(s string, unit int64) (int64, bool) {
	if s == "unlimited" {
		return RlimitInfinity, true
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n * unit, true
}
