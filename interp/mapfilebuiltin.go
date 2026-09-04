// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `mapfile` reads a stream into an indexed array, one element per line, and
// `readarray` is the same command under its other name.
//
// bash alone: dash, ksh93 and zsh have no such command, so it is registered
// here and taken away by the three without it, the way `compgen` and
// `complete` are.
//
// It matters because `mapfile -t arr` is the idiomatic file-to-array read
// with no subshell in it — the `while read` loop it replaces runs in one when
// piped, and the array vanishes with it. Everything here is measured against
// bash: the default name with no operand is MAPFILE, -t strips the trailing
// delimiter from each element, -d renames the delimiter and an empty -d means
// NUL, -n caps how many elements arrive (0 is no cap), -s throws away the
// first N, -O writes from a given subscript into whatever the array already
// holds, and -u reads a descriptor from the shell's own table. The -C/-c
// callbacks are deferred: the dialect's letter table refuses them by name
// rather than parsing and ignoring them.
func init() {
	builtins["mapfile"] = func(r *Runner, ctx context.Context, args []string) int {
		return biMapfile(r, ctx, "mapfile", args)
	}
	builtins["readarray"] = func(r *Runner, ctx context.Context, args []string) int {
		return biMapfile(r, ctx, "readarray", args)
	}
}

// biMapfile is both names; the complaints and the usage line carry whichever
// one the script used, measured — `readarray -q` says readarray.
func biMapfile(r *Runner, _ context.Context, name string, args []string) int {
	// The letters are fixed rather than a semantics axis, because exactly
	// one dialect has the command at all. Through the shared reader, so a
	// bundle splits — `-tn 1` is `-t -n 1` — and an unknown letter is
	// refused with the dialect's wording and usage line.
	args, opts, optArg, code := r.builtinOptionsArg(name, args, "d:n:O:s:tu:")
	if code != 0 {
		return code
	}

	// The delimiter: a newline unless -d renamed it. The argument's first
	// character speaks, as `read -d` reads it, and an empty argument means
	// NUL. A NUL delimiter is stripped from the elements even without -t:
	// measured, `printf 'a\0' | mapfile -d '' x` leaves ${#x[0]} at 1.
	delim := byte('\n')
	if word, ok := optArg['d']; ok {
		delim = 0
		if word != "" {
			delim = word[0]
		}
	}
	strip := strings.Contains(opts, "t") || delim == 0

	// The counts: how many elements to keep, and how many to throw away
	// first. One wording for both letters, measured — `-s x` complains about
	// a line count too. Zero for -n is no cap rather than nothing, so an
	// absent -n and `-n 0` land in the same place.
	count := 0
	if word, ok := optArg['n']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			r.diagf("%s: %s: invalid line count\n", name, word)
			return 1
		}
		count = n
	}
	skip := 0
	if word, ok := optArg['s']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			r.diagf("%s: %s: invalid line count\n", name, word)
			return 1
		}
		skip = n
	}

	// The origin. Giving it at all is what changes the write: with -O the
	// elements land at their subscripts and the rest of the array survives,
	// without it the whole array is replaced — measured with `arr=(x y z)`
	// before both.
	origin, originSet := 0, false
	if word, ok := optArg['O']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			r.diagf("%s: %s: invalid array origin\n", name, word)
			return 1
		}
		origin, originSet = n, true
	}

	// The stream: standard input, or the descriptor -u names — resolved
	// against the shell's own table, where `exec 5<file` put it. Two
	// complaints, measured: an argument that is not a descriptor at all, and
	// a number nothing is open at.
	in := r.In()
	if word, ok := optArg['u']; ok {
		fd, numeric := atoi(word)
		if !numeric || fd < 0 {
			r.diagf("%s: %s: invalid file descriptor specification\n", name, word)
			return 1
		}
		rd, open := r.readerForFd(fd)
		if !open {
			r.diagf("%s: %d: invalid file descriptor: Bad file descriptor\n", name, fd)
			return 1
		}
		in = rd
	}

	// The array: the first operand, MAPFILE with none, and any operands
	// after the first ignored — all three measured. A name that is not one
	// is refused, and a name already declared associative is refused as the
	// wrong kind of array rather than silently rewritten.
	target := "MAPFILE"
	if len(args) > 0 {
		target = args[0]
	}
	if !isPlainName(target) {
		r.diagf("%s: `%s': not a valid identifier\n", name, target)
		return 1
	}
	if _, ok := r.AssocArrays[target]; ok {
		r.diagf("%s: %s: not an indexed array\n", name, target)
		return 1
	}

	// The read, a byte at a time so nothing past the cap is consumed:
	// measured, `mapfile -n 1` leaves the rest of the stream for whoever
	// reads next. End of input with no trailing delimiter still yields the
	// element, and is not a failure — unlike `read`, a mapfile that reaches
	// the end read everything it was asked for.
	next := directByteSource(in)
	var elems []string
	var b strings.Builder
	seen := 0
	for count == 0 || seen < skip+count {
		c, ev := next()
		if ev != evByte {
			if b.Len() > 0 {
				seen++
				if seen > skip {
					elems = append(elems, b.String())
				}
			}
			break
		}
		if !strip || c != delim {
			b.WriteByte(c)
		}
		if c == delim {
			seen++
			if seen > skip {
				elems = append(elems, b.String())
			}
			b.Reset()
		}
	}

	if !originSet {
		r.setArray(target, elems)
		return 0
	}
	for i, e := range elems {
		r.setArrayElem(target, origin+i, e)
	}
	if _, ok := r.Arrays[target]; !ok {
		// Nothing arrived and nothing was there: the name still becomes an
		// empty array, exactly as the replacing form leaves it.
		r.setArray(target, nil)
	}
	return 0
}
