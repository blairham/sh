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
// holds, and -u reads a descriptor from the shell's own table, -C names a command
// run every -c elements as the array fills, and -c is how many that is.
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
func biMapfile(r *Runner, ctx context.Context, name string, args []string) int {
	// The letters are fixed rather than a semantics axis, because exactly
	// one dialect has the command at all. Through the shared reader, so a
	// bundle splits — `-tn 1` is `-t -n 1` — and an unknown letter is
	// refused with the dialect's wording and usage line.
	args, opts, optArg, code := r.builtinOptionsArg(name, args, "C:c:d:n:O:s:tu:")
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

	// The callback and how often it runs. The quantum is validated whenever
	// -c is written, with or without a -C to call — measured, `mapfile -c 0`
	// alone is the same refusal — and zero is refused rather than meaning no
	// cap, which is the opposite of what zero means to -n above. The default
	// is 5000, so a -C with no -c on a short list calls nothing at all.
	callback, hasCallback := optArg['C']
	quantum := 5000
	if word, ok := optArg['c']; ok {
		n, numeric := atoi(word)
		if !numeric || n <= 0 {
			r.diagf("%s: %s: invalid callback quantum\n", name, word)
			return 1
		}
		quantum = n
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
	//
	// The third stream a builtin waits on, and so the third place a
	// background job's pid has to settle before it does — see
	// settleBackgroundJobBeforeABlockingRead.
	//
	// The elements land one at a time rather than in one write at the end,
	// because a -C callback can see the array and it sees a partly filled
	// one: measured, the first call with `-c 1` prints an *empty* array and
	// the second prints the first element. That is also when the replacing
	// form empties what was there — before the first line is read, not after
	// the last — so `arr=(x y z)` is already gone by the first callback.
	if !originSet {
		r.setArray(target, nil)
	}
	r.settleBackgroundJobBeforeABlockingRead(in)
	next := directByteSource(in)
	var b strings.Builder
	seen, kept := 0, 0
	keep := func(elem string) {
		at := origin + kept
		kept++
		// Before the assignment, and only on a multiple of the quantum: the
		// callback is handed the subscript this element is *about* to get and
		// the element itself. -O moves the subscript with it, and -s does not
		// — a skipped line is never an element and never counts here.
		if hasCallback && kept%quantum == 0 {
			r.runMapfileCallback(ctx, callback, at, elem)
		}
		r.setArrayElem(target, at, itoa(at), elem)
	}
	for count == 0 || seen < skip+count {
		c, ev := next()
		if ev != evByte {
			if b.Len() > 0 {
				seen++
				if seen > skip {
					keep(b.String())
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
				keep(b.String())
			}
			b.Reset()
		}
	}

	if _, ok := r.Arrays[target]; !ok {
		// Nothing arrived and nothing was there: the name still becomes an
		// empty array, exactly as the replacing form leaves it.
		r.setArray(target, nil)
	}
	return 0
}

// runMapfileCallback runs one -C call.
//
// It is text joined and evaluated, not a command with two arguments appended,
// and the difference is measurable in three directions: `-C 'echo A |'` runs
// the subscript as a command on the right of a pipe, `-C 'echo x; exit'` hands
// the arguments to the *last* command in the string, and `-C 'echo "'` fails
// to parse. So the callback is a fragment of shell source and the two
// arguments are appended to it as source.
//
// Which is why the element is quoted on the way in. It is one argument
// whatever it holds — measured with embedded spaces, a `*`, a `$V` and a `;`,
// all of which arrive literally — so joining it raw would let a line of data
// become a line of program.
//
// The status is not the builtin's: a callback that fails leaves `mapfile` at
// 0, measured with `-C false`.
//
// The label is `stdin` rather than `eval`, which is measured and is not the
// same answer `eval` itself gives on the same route: a malformed `eval` is
// `bash: eval: line 1: …` and a malformed callback is `bash: stdin: line 1: …`,
// from a script file as well as from standard input. Fixed here rather than
// asked of the dialect for the reason the letters are: one dialect has the
// command at all.
func (r *Runner) runMapfileCallback(ctx context.Context, callback string, at int, elem string) {
	status := r.status
	text := callback + " " + itoa(at) + " " + singleQuoted(elem, `'\''`, false)
	r.runSourced(ctx, text, sourced{
		eval:         true,
		label:        "stdin",
		syntaxStatus: r.diag().SyntaxStatus(),
	})
	r.status = status
}
