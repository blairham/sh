// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// The last two of `zsh/system`'s six builtins: `sysseek`, which moves where a
// descriptor is reading from, and `syserror`, which turns an error number into
// the sentence the operating system has for it.
//
// Measured 2026-09-10 against zsh 5.9.2 (Homebrew, aarch64) with `zsh -f`.
//
// # Neither is written anywhere in the tree this shell starts with
//
// That is why they were split out of #1737 and left refused, and it is worth
// keeping in view now that they are here: nothing about this pair was learned
// from a workload. What settles them instead is the module — six names, three
// of which had become "implemented" and three "refused by name", which is a
// state a module is supposed to pass through rather than rest in. A caller
// reading `zmodload -F zsh/system b:sysseek` gets an answer that no longer
// depends on which half of the module it happened to ask about.
//
// # `sysseek` is one lseek and its status is the whole report
//
//	written                       zsh 5.9.2
//	sysseek -u 7 0                back to the start of the file at 7
//	sysseek -u 7 -w end 0         to its end, so `systell` is its size
//	sysseek -u 7 -w current 1     one byte on from where it is
//	sysseek 0                     descriptor 0, which is the default
//	sysseek -u 7 -1               2, and **nothing said**
//	sysseek -u 7 bogus            offset zero — the word is read as a number
//
// The silent 2 is the same shape `syswrite` uses for a descriptor that
// refuses, and the reason is the same: this is a call rather than a command
// line, and the status is where a call says what happened.
//
// # `syserror` is a formatter and it never fails at what it was asked
//
// It writes on standard error and answers 0 whatever it printed — `syserror
// 9999` is `Unknown error: 9999` at status 0 — because there is no such thing
// as an error number it cannot describe. The one nonzero it has is 2, for an
// operand that is neither a number nor a name in `$errnos`, and that one is
// silent as well.
//
// `-e var` diverts the sentence into a parameter instead of printing it, which
// is the whole of what the letter does, and `-p` puts a prefix on the front of
// either. Measured: no space is added, so `syserror -p oops 2` is
// `oopsNo such file or directory` and a caller wanting one writes it.
//
// **The capitalization is the platform's and it differs from every other
// diagnostic here.** `syserror 2` is `No such file or directory` and this
// shell's own `sysopen /no/such` is `no such file or directory` — two
// spellings of one string, both measured in zsh, and errnoText and
// sysErrnoText are which one each caller gets.
//
// # `syserror` with no operand refuses, and that is a divergence on purpose
//
// In zsh the operand defaults to the C library's `errno` *at that instant* —
// whatever the last system call the shell made left behind. Measured twice in
// one session it was `No such file or directory` and then `Interrupted system
// call`, which is not a behavior a shell can be held to.
//
// This shell has no such variable. It is a Go program: the failures its
// builtins see are values they handle and there is no per-process errno left
// lying around for a later command to read. Inventing one would mean answering
// `Undefined error: 0` — the sentence that means *nothing went wrong* — to a
// script asking what went wrong, at status 0, which is the accepting-and-inert
// failure this module has produced twice already.
//
// So the no-operand form refuses **by name**, and the one part of it this
// shell can answer honestly is answered: zsh's `ERRNO` is writable and
// `ERRNO=13; syserror` is `Permission denied` there, so a script that has put
// a number in `ERRNO` gets that number's sentence here. See sysErrorNumber.
// `$ERRNO` as a live view of a process errno is filed separately.

// sysseekWhence is `-w`'s three words and the lseek origin each names.
//
// Exactly three, and not abbreviated: `-w start` and `-w end` work, `-w s`,
// `-w e`, `-w cur` and `-w beginning` are each `unknown argument to -w`, and
// `-w END` works — so the comparison is case-insensitive over whole words and
// is not the prefix matching `zstat`'s `+element` does. All measured.
var sysseekWhence = map[string]int{
	"start":   io.SeekStart,
	"current": io.SeekCurrent,
	"end":     io.SeekEnd,
}

// sysseekBuiltin is `sysseek [-u fd] [-w start|current|end] offset`.
func sysseekBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := systemOptions(r, args, "uw", "")
	if code != 0 {
		return code
	}
	if len(rest) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if len(rest) > 1 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	fd, code := opts.number(r, 'u', 0)
	if code != 0 {
		return code
	}
	whence := io.SeekStart
	if word, ok := opts.value('w'); ok {
		origin, known := sysseekWhence[strings.ToLower(word)]
		if !known {
			r.Diagnosef("unknown argument to -w: %s\n", word)
			return 1
		}
		whence = origin
	}
	// An offset that is not a number is zero, measured — the same rule
	// `zsystem flock -t` follows for its own value, and reproduced for the
	// same reason: a script written against it would change behavior under a
	// shell that refused instead.
	offset, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil {
		offset = 0
	}
	if !r.SeekDescriptor(fd, offset, whence) {
		// Silent, measured: `sysseek -u 7 -1` is 2 and says nothing. A
		// descriptor with no position at all — a pipe, a terminal — is the
		// same 2, which is what the caller's question had as its answer.
		return 2
	}
	return 0
}

// syserrorBuiltin is `syserror [-e errvar] [-p prefix] [errno|errname]`.
func syserrorBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := systemOptions(r, args, "ep", "")
	if code != 0 {
		return code
	}
	if len(rest) > 1 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	dest, diverting := opts.value('e')
	if diverting && !isIdentifier(dest) {
		r.Diagnosef("not an identifier: %s\n", dest)
		return 1
	}
	number, code := sysErrorNumber(r, rest)
	if code != 0 {
		return code
	}
	prefix, _ := opts.value('p')
	text := prefix + errnoText(number)
	if diverting {
		r.SetVar(dest, text)
		return 0
	}
	_, _ = fmt.Fprintf(r.Err(), "%s\n", text)
	return 0
}

// sysErrorNumber is which error `syserror` was asked about.
//
// A number or a name from `$errnos` where there is an operand — status 2 and
// nothing said for a word that is neither, measured — and the number the last
// system call left where there is not.
//
// That last case used to refuse by name, because this shell kept no errno at
// all and reading the absent parameter as zero would have printed `Undefined
// error: 0` — the sentence that means *nothing went wrong* — at status 0 to a
// script asking what did. There is a number now (interp/errno.go), so zero
// here means what it says: no system call this shell made for the script has
// failed. Which is the answer the shell being modeled gives in a fresh
// session, byte for byte (#1802).
//
// It reads the *number* and not `$ERRNO`, and the two are not the same: the
// parameter is empty until a script assigns it, and this is not.
func sysErrorNumber(r *interp.Runner, rest []string) (int, int) {
	if len(rest) == 1 {
		number, known := errnoNumber(rest[0])
		if !known {
			return 0, 2
		}
		return number, 0
	}
	return r.LastErrno(), 0
}
