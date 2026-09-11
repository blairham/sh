// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"errors"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `zcompile`: zsh's "compile these scripts so they load faster".
//
// # What it produces here, and why that is the honest answer
//
// Real zsh writes **wordcode** — a binary form of its own parse tree, opening
// `\x07\x06\x05\x04` and carrying the version that wrote it. This shell cannot
// write that and is not going to: the format is zsh's internal representation,
// and reading it out of zsh's source is what CLEANROOM.md forbids as squarely
// as the C source itself.
//
// So the file this writes is the **text of the inputs, concatenated**, which
// is a valid shell program equivalent to what it was given. That is a real
// choice and not a stub, and it rests on one measurement:
//
//	# a file zsh cannot read as wordcode, beside a script it can
//	printf 'NOT WORDCODE\x00\x01' > lib.zsh.zwc
//	zsh -f -c 'source lib.zsh'       ->  the script runs, exit 0, silence
//
// **Real zsh silently ignores a `.zwc` it cannot read and falls back to the
// source.** Measured on 5.9 with junk, with an empty file, and with plain
// shell text, all three: the script ran, status 0, nothing on standard error.
// That is what makes writing here safe in a home directory two shells share —
// the worst this can do to somebody's real zsh is waste the bytes.
//
// Concatenated text rather than a marker of our own, because it is the choice
// that degrades best: anything that sources the product gets a program that
// does what the inputs did, in either shell.
//
// # Why it was worth implementing before the other two
//
// #1405 names `vared`, `zcompile` and `zregexparse` together. This one is not
// like the others: powerlevel10k calls it while writing its instant-prompt
// cache — `internal/p10k.zsh` has `zcompile -R -- $tmp.zwc $root_file ||
// return` — so a shell without it abandons that dump on **every** startup.
// Measured through a pseudo-terminal on a real `~/.zshrc`, those two lines
// were the *entire* error output of an interactive start, against a real zsh
// that is silent:
//
//	_p9k_dump_instant_prompt:374: command not found: zcompile
//	_p9k_dump_state:23: command not found: zcompile
//
// # The interface, measured on zsh 5.9.2, 2026-09-11
//
//	zcompile a.zsh                  writes a.zsh.zwc, 0
//	zcompile out.zwc a.zsh b.zsh    writes out.zwc, 0 — first operand is the
//	                                output once there is more than one
//	zcompile -- o.zwc a.zsh         `--` ends the options
//	zcompile                        `too few arguments`, 1
//	zcompile nosuch.zsh             `can't open file: nosuch.zsh`, 1
//	zcompile bad.zsh                the parse error, then
//	                                `can't read file: bad.zsh`, 1, and
//	                                **no output file at all**
//	zcompile -q a.zsh               `bad option: -q`, 1
//	zcompile -t nosuch.zwc          `can't open zwc file: nosuch.zwc`, 1
//
// The output is mode 0444 in zsh, and is here: a compiled file is a cache and
// writing to it by hand is not a thing to make easy.
//
// Two divergences, stated rather than hidden:
//
//   - A file that does not parse is refused with the right sentence and the
//     right status, but **without zsh's parse error in front of it**. Zsh
//     prints `zsh:2: parse error near '((('` first. Reproducing a located
//     parse failure from inside a builtin is the front end's machinery, and
//     the operand is refused either way.
//   - `-t` reports `can't open zwc file` for everything, because nothing this
//     writes is a zwc file and nothing else can be read as one. Saying so is
//     better than inventing a listing.
const zcompileLetters = "URMzkcamt"

// zcompileFlags is what the letters set. Only `t` changes what the builtin
// does; the rest are accepted and shape a compilation this does not perform
// differently — `-U` is "do not expand aliases while reading", `-z`/`-k` pick
// which shell's autoloading a digest is for, `-R`/`-M` say whether the file is
// read into memory or mapped. Each is recorded rather than acted on, and the
// product is the same text either way.
type zcompileFlags struct {
	test bool
}

func registerZcompile(r *interp.Runner) { r.Register("zcompile", zcompileBuiltin) }

func zcompileBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	flags, rest, code := zcompileOptions(r, args)
	if code != 0 {
		return code
	}
	if len(rest) == 0 {
		r.Diagnosef("too few arguments\n")
		return 1
	}
	if flags.test {
		// Nothing this shell writes is wordcode, and nothing else here can
		// read what zsh writes — so the honest answer is the one zsh gives
		// for a file it cannot open as a zwc.
		r.Diagnosef("can't open zwc file: %s\n", rest[0])
		return 1
	}

	out, inputs := rest[0]+".zwc", rest
	if len(rest) > 1 {
		out, inputs = rest[0], rest[1:]
	}

	// Every input is read and parsed *before* anything is written, which is
	// zsh's order and is observable: `zcompile out.zwc good.zsh bad.zsh`
	// leaves no `out.zwc` behind at all.
	var b strings.Builder
	for _, name := range inputs {
		text, err := r.ReadFileGated(name)
		if err != nil {
			r.Diagnosef("can't open file: %s\n", name)
			return 1
		}
		if _, perr := syntax.Parse(string(text), Dialect()); perr != nil {
			r.Diagnosef("can't read file: %s\n", name)
			return 1
		}
		b.Write(text)
		// A file that does not end in a newline would otherwise run its last
		// command into the next file's first.
		if len(text) > 0 && text[len(text)-1] != '\n' {
			b.WriteByte('\n')
		}
	}

	// Through filesgate.go rather than the `os` package directly, which is
	// this dialect's discipline for every path a script named — see the file's
	// own comment, and internal/boundary's guard, which is what makes the
	// property checkable by reading one file (#1819).
	//
	// 0444 is zsh's mode for a compiled file, measured. A cache is not a thing
	// to make easy to edit by hand.
	if err := fileWriteWhole(r, ctx, out, []byte(b.String()), 0o444); err != nil {
		// A refusal by the gate has already been reported, in the words a
		// refused redirection gets. Anything else is the kernel declining,
		// and zsh's sentence for that is measured: `zcompile out.zwc a.zsh`
		// into a directory with no write permission says `can't write zwc
		// file: out.zwc` at 1.
		if !errors.Is(err, errGateRefused) {
			r.Diagnosef("can't write zwc file: %s\n", out)
		}
		return 1
	}
	return 0
}

// zcompileOptions reads the leading option words, stopping at `--` or at the
// first operand.
func zcompileOptions(r *interp.Runner, args []string) (flags zcompileFlags, rest []string, code int) {
	rest = args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			letter := rune(word[i])
			if !strings.ContainsRune(zcompileLetters, letter) {
				r.Diagnosef("bad option: -%c\n", letter)
				return flags, nil, 1
			}
			if letter == 't' {
				flags.test = true
			}
		}
	}
	return flags, rest, 0
}
