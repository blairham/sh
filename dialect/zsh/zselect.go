// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/internal/fdset"
	"github.com/blairham/sh/interp"
)

// `zsh/zselect`: one builtin, which waits until a descriptor is ready and says
// which one.
//
// Measured 2026-09-10 against zsh 5.9.2 (Homebrew, aarch64) with `zsh -f`.
//
// # It is the wall behind `sysopen` in a prompt theme's worker
//
// #1737 made the theme's `sysopen` line work and the next two lines are these
// (`internal/worker.zsh:196`, inside the process substitution that `sysopen`
// opens):
//
//	zmodload zsh/zselect               || return
//	! { zselect -t0 || (( $? != 1 )) } || return
//	…
//	while syswrite $'\x05'; do zselect -t 1000; done
//
// Two `|| return` guards on consecutive lines, and the module was not in the
// feature table at all, so `zmodload zsh/zselect` refused and the worker's
// body stopped there. The third line is the same builtin used as the loop's
// only sleep.
//
// **The second line is a probe and it is the reason the status vocabulary is
// load-bearing.** It asks for a wait of no time on nothing at all and insists
// the answer is exactly 1 — nothing became ready — so a shell that answered 0
// there, or answered 1 with a diagnostic, fails the guard as surely as one
// that had no builtin.
//
// # What it is
//
//	written                zsh 5.9.2
//	zselect -t 0           1 at once — nothing was ready
//	zselect -t 10          1 after a tenth of a second
//	zselect -r 0 -t 0      0 and `$reply` is `-r 0` where input is waiting
//	zselect -r $fd         blocks until readable, then 0
//	zselect                blocks forever, on nothing
//
// **`-t` is in hundredths of a second.** Measured one value at a time: `-t 10`
// is 0.14s and `-t 100` is 1.03s. So the theme's `zselect -t 1000` is a
// ten-second wait and not a one-second one, and a shell reading the number as
// milliseconds would turn that heartbeat loop into a spin.
//
// # The answer is an array of alternating flags and numbers
//
// `$reply` — or `-a name` — is `(-r 0 -w 1)`: the letter of the set, then
// every descriptor ready in it, then the next letter. `-A name` is an
// association instead, keyed by the descriptor, whose value is the letters it
// was ready in: `-A a -r 0 -w 0` on a readable and writable descriptor is
// `a=(0 rw)`. Both measured, and the sets come out in the order `r`, `w`, `e`
// with the descriptors inside each ascending.
//
// **Nothing is assigned when nothing is ready.** Measured: `reply=(x y);
// zselect -r 1 -t 0` leaves `reply` as `(x y)`. A shell that emptied it would
// be telling a caller with a stale answer that it now has a fresh one.
//
// # A bare number joins the set the last letter named
//
// `zselect -w 1 0 -t 0` watches **both** 1 and 0 for writing, and `zselect 0`
// with no letter before it watches 0 for reading, because reading is where the
// current set starts. Measured. A word that is not a number at all is
// `expecting file descriptor: extra`, and one with a number in front of
// something else is `garbage after file descriptor: x` — which is also what an
// option letter this builtin has not got gets, since an unknown `-q` is read
// as a word that should have been a descriptor.
//
// # A descriptor this shell cannot ask about is never ready
//
// `zselect -r 99 -t 0` is 1 with nothing said, and `zselect -r 99` with no
// timeout waits forever — both measured in zsh, and both are what dropping the
// descriptor before the call produces. It is dropped rather than complained
// about for the reason fdset.Ready gives: the kernel's answer to a set holding
// one dead entry is a complaint about the *call*, so every live descriptor
// beside it would lose its turn.

// zselectHundredth is what one unit of `-t` is worth. See above: it is the
// unit `zsh/system`'s `sysread -t` does not share, and the two are in two
// modules.
const zselectHundredth = 10 * time.Millisecond

// zselectSets is the three sets in the order a listing writes them, with the
// letter each is named by.
var zselectSets = [3]byte{'r', 'w', 'e'}

// zselectOpts is what one `zselect` command line asked for.
type zselectOpts struct {
	// watching holds the descriptors named for reading, writing and errors,
	// indexed the way zselectSets is.
	watching [3][]int
	// current is which of the three a bare number joins.
	current  int
	timeout  time.Duration
	timed    bool
	array    string
	assoc    string
	reported bool
}

// zselectBuiltin is `zselect [-rwe fd]... [-t timeout] [-a array] [-A assoc]`.
func zselectBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, code := zselectOptions(r, args)
	if code != 0 {
		return code
	}
	var wait *time.Duration
	if opts.timed {
		wait = &opts.timeout
	}
	ready, code := zselectWait(r, opts, wait)
	if code != 0 {
		return code
	}
	if !opts.reported {
		return 1
	}
	return zselectReport(r, opts, ready)
}

// zselectWait maps the shell's descriptor numbers on to the operating
// system's, asks, and maps the answer back.
//
// The two tables are not one table — see [interp.Runner.SystemDescriptor] —
// and the numbers a caller wrote are the shell's, so the numbers `$reply`
// gets back have to be the shell's as well. A shell that reported the
// kernel's would answer `-r 12` to a script that asked about 5.
func zselectWait(r *interp.Runner, opts *zselectOpts, wait *time.Duration) ([3][]int, int) {
	var asked [3][]int
	shellOf := map[int]int{}
	for set, fds := range opts.watching {
		for _, fd := range fds {
			sys, ok := r.SystemDescriptor(fd)
			if !ok {
				// Not a descriptor this shell can ask the kernel about. Left
				// out of the call, so it is never ready and never stops the
				// wait — which is what zsh does with a number nothing is open
				// at. See the note above.
				continue
			}
			asked[set] = append(asked[set], sys)
			shellOf[sys] = fd
		}
	}
	read, write, except, err := fdset.Ready(asked[0], asked[1], asked[2], wait)
	if err != nil {
		return asked, 1
	}
	var out [3][]int
	for set, fds := range [3][]int{read, write, except} {
		for _, sys := range fds {
			out[set] = append(out[set], shellOf[sys])
		}
		slices.Sort(out[set])
		opts.reported = opts.reported || len(out[set]) > 0
	}
	return out, 0
}

// zselectReport writes the answer into `$reply`, or into the parameter `-a`
// or `-A` named.
func zselectReport(r *interp.Runner, opts *zselectOpts, ready [3][]int) int {
	if opts.assoc != "" {
		// Keyed by the descriptor, with the letters it was ready in as the
		// value: `-A a -r 0 -w 0` on a readable and writable descriptor is
		// `a=(0 rw)`. Measured, and it is the one shape here where a
		// descriptor appears once rather than once per set.
		letters := map[string]string{}
		for set, fds := range ready {
			for _, fd := range fds {
				key := strconv.Itoa(fd)
				letters[key] += string(zselectSets[set])
			}
		}
		r.SetAssoc(opts.assoc, letters)
		return 0
	}
	answer := []string(nil)
	for set, fds := range ready {
		if len(fds) == 0 {
			continue
		}
		answer = append(answer, "-"+string(zselectSets[set]))
		for _, fd := range fds {
			answer = append(answer, strconv.Itoa(fd))
		}
	}
	name := opts.array
	if name == "" {
		name = "reply"
	}
	r.SetArray(name, answer)
	return 0
}

// zselectOptions reads the whole command line, which is one pass because a
// bare number's meaning depends on the letter before it.
//
// There is no separate operand phase: `zselect -w 1 0 -t 0` has a descriptor
// between two options and it joins the write set, so the words are read in
// order and the current set is carried along. See the note above.
func zselectOptions(r *interp.Runner, args []string) (*zselectOpts, int) {
	opts := &zselectOpts{}
	rest := args
	for len(rest) > 0 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			// Not an end of options: `zselect -- -t 0` still honors the
			// timeout, measured, and `zselect --` waits forever on nothing.
			// So the word decides nothing and is stepped over.
			continue
		}
		if !strings.HasPrefix(word, "-") || len(word) == 1 || isDigit(word[1]) {
			// A word that is not an option, and a word whose dash is a minus
			// sign: `zselect -0 -t 0` watches descriptor 0 and `-1` watches a
			// descriptor no shell has. Measured both ways — `-0` comes back
			// as `-r 0` and `-1` comes back with nothing ready — which is
			// what says the leading dash there is arithmetic rather than an
			// option this builtin has never heard of.
			if code := zselectDescriptor(r, opts, word); code != 0 {
				return opts, code
			}
			continue
		}
		if code := zselectLetters(r, opts, word, &rest); code != 0 {
			return opts, code
		}
	}
	return opts, 0
}

// zselectLetters reads one option word's letters, which bundle.
//
// **A letter for a set takes the rest of its word only when the rest is a
// number.** `-r0` is `-r 0` and `-rw 0` is `-w 0` — the `w` is a second letter
// rather than a descriptor named `w` — and `-rt 0` is a read set with a
// *timeout* of zero. All three measured, and the rule that produces all three
// is that a digit after the letter ends the word and anything else carries on
// bundling. A shell that took the rest of the word unconditionally reads
// `-ra arr` as a descriptor called `a`, which is how this was found.
func zselectLetters(r *interp.Runner, opts *zselectOpts, word string, rest *[]string) int {
	for i := 1; i < len(word); i++ {
		letter := word[i]
		switch letter {
		case 'r', 'w', 'e':
			opts.current = strings.IndexByte(string(zselectSets[:]), letter)
			if i+1 < len(word) && isDigit(word[i+1]) {
				return zselectDescriptor(r, opts, word[i+1:])
			}
		case 't', 'a', 'A':
			value, ok := systemLetterValue(word, rest, i)
			if !ok {
				r.Diagnosef("argument expected after -%c\n", letter)
				return 1
			}
			if code := zselectValue(r, opts, letter, value); code != 0 {
				return code
			}
			i = len(word)
		default:
			// Not a letter this builtin has: the word is read as one that
			// should have been a descriptor, which is what zsh says about it.
			r.Diagnosef("expecting file descriptor: %s\n", word[i:])
			return 1
		}
	}
	return 0
}

// zselectValue reads the value one of the three lettered options took.
func zselectValue(r *interp.Runner, opts *zselectOpts, letter byte, value string) int {
	switch letter {
	case 'a', 'A':
		if !isIdentifier(value) {
			r.Diagnosef("invalid array name: %s\n", value)
			return 1
		}
		if letter == 'a' {
			opts.array = value
		} else {
			opts.assoc = value
		}
		return 0
	}
	hundredths, err := strconv.Atoi(value)
	if err != nil || hundredths < 0 {
		// Measured, and it is not the rule `zsystem flock -t` follows: there
		// a word that is not a number is a zero timeout, and here it stops
		// the command. Two builtins in two modules, each reproduced as it is.
		r.Diagnosef("number expected after -t\n")
		return 1
	}
	opts.timeout, opts.timed = time.Duration(hundredths)*zselectHundredth, true
	return 0
}

// zselectDescriptor reads one word as a descriptor for the current set.
//
// A leading minus is a sign rather than an option marker — see zselectOptions
// — and what follows it has to be digits and nothing else. The two refusals
// are measured and they are different sentences: a word with no number in it
// at all is `expecting file descriptor: extra`, and a word that starts as one
// and does not finish as one is `garbage after file descriptor: x`, which
// names only the part that was not a number.
func zselectDescriptor(r *interp.Runner, opts *zselectOpts, word string) int {
	at := 0
	if strings.HasPrefix(word, "-") {
		at = 1
	}
	digits := at
	for digits < len(word) && isDigit(word[digits]) {
		digits++
	}
	switch {
	case digits == at:
		r.Diagnosef("expecting file descriptor: %s\n", word)
		return 1
	case digits < len(word):
		r.Diagnosef("garbage after file descriptor: %s\n", word[digits:])
		return 1
	}
	fd, err := strconv.Atoi(word)
	if err != nil {
		r.Diagnosef("expecting file descriptor: %s\n", word)
		return 1
	}
	opts.watching[opts.current] = append(opts.watching[opts.current], fd)
	return 0
}

// registerZselectModule installs `zsh/zselect`'s one builtin.
//
// The whole module, and the name shadows nothing on any machine — which is
// the test #1668 set for whether a name may be registered at all, and the
// reason `zsh/stat`'s `stat` and `zsh/files`' nine plain spellings are not.
func registerZselectModule(r *interp.Runner) {
	if !zselectSupported {
		// A platform with no descriptor set to wait on. Registering the name
		// would give a script a `zselect` that answered without waiting,
		// which is worse than not having one: the heartbeat loop it is the
		// sleep in would spin. Left unregistered it refuses by its own name,
		// and `zmodload zsh/zselect` refuses with it.
		return
	}
	r.Register("zselect", zselectBuiltin)
}
