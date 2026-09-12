// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/blairham/sh/internal/fdset"
	"github.com/blairham/sh/internal/tty"
)

// `read -k`: characters from the terminal, as they are typed.
//
// The letter is zsh's and it is not a variation on reading a line. Three
// things about it are different, and all three are measured 2026-09-12
// against zsh 5.9.2:
//
//   - **It reads the terminal, not the stream.** `printf abc | read -k v` is
//     `not interactive and can't open terminal` at status 1, and so is `read
//     -k 2 v < f.txt` — redirecting standard input does not redirect it.
//     Under a pseudo-terminal it still works after `exec 0</dev/null`, which
//     is what says the question is "does this shell hold a terminal" and not
//     "what is this builtin reading". `-u` is the one thing that overrides it,
//     and that is also what makes every fact here testable without a
//     pseudo-terminal.
//   - **It counts what the locale calls a character.** `read -k 2 -u 3 v` over
//     `héllo` gives `hé` under a UTF-8 locale, three bytes for two characters,
//     and `h` plus one byte of the accented letter under `LC_ALL=C` — measured
//     both ways, and it is the same split `${#s}` and `${s:off:len}` already
//     answer through countsTheLocalesCharacters. A reader that always decoded
//     would disagree with the panel in the locale the corpus harness runs in.
//   - **Nothing is a terminator.** `read -k 3 -u 3 v` over `a\nb` gives all
//     three, newline included, and `${#v}` is 3.
//
// A short read assigns what arrived and reports 1 — `read -k 5 -u 3 v` over
// `ab` leaves `v=ab` at status 1 — which is the same shape `read`'s ordinary
// end-of-input has and is why the count and the status are answered
// separately below. `read -k 0` is status 1 with nothing read.
//
// The line discipline is put into cbreak and **echo is left alone**, which is
// measured rather than assumed: a character read this way still appears on the
// screen. Borrowing the editor's raw mode would swallow the keystroke a script
// just asked a person for. See internal/tty's mode.go.

// readKeyCount is how many characters `read -k` was asked for, and whether the
// letter was given at all.
//
// One when the letter is bare, which is measured: `read -k -u 3 v` reads a
// single character. The count rides the shared option reader's `#` shape — see
// builtinOptionsArg — so a word that is not a number was never the argument
// and is still the name to read into.
func readKeyCount(opts string, optArg map[byte]string) (count int, asked bool) {
	if !strings.ContainsRune(opts, 'k') {
		return 0, false
	}
	word, given := optArg['k']
	if !given {
		return 1, true
	}
	n, numeric := atoi(word)
	if !numeric || n < 0 {
		return -1, true
	}
	return n, true
}

// readKeysFrom reads count characters from a stream, and is the whole of the
// read once the source has been settled.
//
// `chars` is asked whether a character is the locale's or a byte, and it is a
// function rather than a flag so that it is asked **only when a byte above
// ASCII actually arrives**. That is the discipline countsCharacters keeps for
// a length — a value whose bytes are all ASCII is the same either way, so the
// axis goes unasked — and keeping it here is what stops every `read -k` in the
// corpus from putting a locale question to a core whose answer is
// "unanswered".
//
// ok is false for a read that ended early, which the caller reports as status
// 1 with what arrived still assigned.
func readKeysFrom(next func() (byte, int), count int, chars func() bool) (text string, ok bool) {
	var buf []byte
	var out []byte
	// A count of nought reads nothing and is still a failure: measured,
	// `read -k 0 -u 3 v` is status 1 with `v` empty, which the loop below
	// would have called a satisfied read of nothing.
	if count <= 0 {
		return "", false
	}
	for read := 0; read < count; {
		c, ev := next()
		if ev != evByte {
			return string(out), false
		}
		buf = append(buf, c)
		if !utf8.FullRune(buf) && len(buf) < utf8.UTFMax && chars() {
			// The rest of a character that has begun to arrive. A terminal
			// delivers bytes and a character outside ASCII is several of
			// them; counting the first as a character would end the read in
			// the middle of one. FullRune is true of every ASCII byte, so
			// this is the only place the locale is asked at all.
			continue
		}
		out = append(out, buf...)
		buf = buf[:0]
		read++
	}
	return string(out), true
}

// pollingKeySource reads a terminal a byte at a time, waiting for each byte to
// be *there* before asking for it, and never leaving a read in flight.
//
// **This is not an optimization; it is the difference between a timeout that
// works and one that eats the next keystroke.** The shared timedByteSource
// reads on a goroutine, because an io.Reader cannot be told to stop waiting,
// and its own comment says what that costs: a read still in flight when the
// deadline passes is abandoned, and if its byte ever arrives it is lost —
// "the cost of a timeout over a plain pipe, confined to the stream the timeout
// was used on".
//
// For `read -k` the stream is the **editor's own input**, so it is not
// confined to anything. The plugin this letter was implemented for ends every
// history search with `read -k -t 1`; the timeout expires a second later with
// the abandoned read still parked on the terminal, and the next key the person
// presses is swallowed by it. Driving the built shell through a
// pseudo-terminal with the real plugins, a second Up arrow arriving after one
// such timeout lost its escape byte and typed `[A` into the line.
//
// A descriptor can be asked whether a read would block, which an io.Reader
// cannot — so where the source is a file this waits for readability first and
// reads only what is already there. Nothing is ever in flight, so nothing is
// ever abandoned.
//
// asked is false where the question cannot be put to this descriptor at all,
// and the caller then falls back to the shared machinery: a `read -k` that
// reported a timeout it never waited for would be worse than one that loses a
// keystroke.
func pollingKeySource(f *os.File, deadline time.Time, bounded bool) (next func() (byte, int), asked bool) {
	fd := int(f.Fd())
	if _, can := fdset.ReadableWithin(fd, 0); !can {
		return nil, false
	}
	var ch [1]byte
	first := true
	return func() (byte, int) {
		// Where the deadline bounds only the wait for the *first* byte, the
		// rest of the read is made without one — the axis
		// ReadTimeoutBoundsReadability decides which this is, and it is
		// settled by the caller and handed in.
		if first || bounded {
			ready, can := fdset.ReadableWithin(fd, time.Until(deadline))
			if !can {
				return 0, evEOF
			}
			if !ready {
				return 0, evTimeout
			}
		}
		first = false
		n, err := f.Read(ch[:])
		if n == 0 || err != nil {
			return 0, evEOF
		}
		return ch[0], evByte
	}, true
}

// readKeySource is the stream `read -k` reads: the terminal this shell holds,
// put into cbreak for the length of the read, unless the caller already
// settled on a descriptor.
//
// explicit is a source `-u` or `-p` named, which is handed straight back — no
// terminal is looked for and no mode is changed, because the descriptor may
// not be one. That is measured: `read -k 2 -u 3 v` on a file reads two bytes
// of the file with the shell's own terminal untouched.
//
// The returned stop is never nil, so a caller can defer it without asking.
func (r *Runner) readKeySource(explicit io.Reader) (in io.Reader, stop func(), ok bool) {
	if explicit != nil {
		return explicit, func() {}, true
	}
	f, held := r.terminal()
	if !held {
		return nil, func() {}, false
	}
	mode, err := tty.Cbreak(f)
	if err != nil {
		// A terminal that will not take the mode is still a terminal, and
		// reading it is still the right thing to try: inside a line editor it
		// is already in a mode that delivers characters, which is exactly the
		// case this must not refuse.
		return f, func() {}, true
	}
	return f, func() { _ = mode.Restore() }, true
}

// readNoTerminal is what `read -k` says when this shell holds no terminal.
//
// Measured 2026-09-12 against zsh 5.9.2, and the wording carries no location
// and no builtin name — it is the bare sentence, which is why it is a wording
// rather than a diagnostic built from the usual parts.
func (r *Runner) readNoTerminal() int {
	// Raw to standard error rather than through the diagnostic path, which is
	// the same choice `read -p`'s prompt makes a few lines away and for a
	// measured reason rather than a stylistic one: every other complaint this
	// builtin makes is located and named — `zsh:read:1: number expected after
	// -k: 2v` — and this one is not. `zsh -c 'read -k v' < /dev/null` writes
	// the bare sentence and nothing else, so a `zsh:read:1:` in front of it
	// would be this shell inventing a prefix the shell it is imitating does
	// not print.
	r.errf("%s\n", Wording(r.diag().ReadNoTerminal,
		"not interactive and can't open terminal"))
	return 1
}

// readKeysInto is the whole of `read -k` once the source is settled: the
// characters, and the one name they go into.
//
// Its own tail rather than the splitting one every other spelling shares,
// because `read -k` shares none of it. Measured 2026-09-12 against zsh 5.9.2
// with `a b c d` on a descriptor:
//
//	w=keep; read -k 5 -u 3 v w   →   v=`a b c`, w=`keep`, status 0
//
// Three facts in one line. There is **no field splitting** — `v` holds the
// five characters, spaces and all, where the ordinary path would have made
// three fields of them. **Exactly one name is filled**, and it is the first.
// And the names after it are **left as they were**, which is where this
// differs from `-N`: that one hands the text to the first name and *clears*
// the rest, measured in both shells that have it, so reusing its tail would
// have wiped `w`.
//
// A leading blank survives too — `read -k 3` over `  x` gives `  x` — which is
// the same fact from the other side and is what a widget reading a keystroke
// needs, since a space is a key somebody pressed.
func (r *Runner) readKeysInto(next func() (byte, int), count int, args []string) int {
	// Memoized: a read of many characters must not put the same locale
	// question to the axis machinery once per character.
	locale, asked := false, false
	text, whole := readKeysFrom(next, count, func() bool {
		if !asked {
			locale, asked = r.countsTheLocalesCharacters(), true
		}
		return locale
	})
	if r.unspecified {
		return 2
	}
	name := "REPLY"
	if len(args) > 0 {
		name = args[0]
	}
	if !r.isReadName(name) {
		if r.unspecified {
			return 2
		}
		return r.badReadName(name)
	}
	// Assigned before the status is decided, not instead of it: a short read
	// keeps what arrived and still reports 1 — `read -k 5 -u 3 v` over `ab`
	// leaves `v=ab` at status 1 — which is the shape `read`'s ordinary end of
	// input has and the reason a caller can tell "nothing came" from "not
	// enough came" by looking at the variable.
	r.storeThroughOperand(name, text)
	if !whole {
		return 1
	}
	return 0
}

// keyTimedSource is pollingKeySource with the deadline computed, and false
// where this read is not one that can be polled.
//
// A nil file is every read that is not `read -k` on a terminal — a `-u`
// descriptor, a pipe, an embedder's buffer — and those keep the shared timed
// source. Splitting it here rather than inside the switch keeps the one
// question the caller has to ask down to "can this be polled".
func keyTimedSource(f *os.File, timeout time.Duration, bounded bool) (next func() (byte, int), can bool) {
	if f == nil || timeout <= 0 {
		return nil, false
	}
	return pollingKeySource(f, timeNow().Add(timeout), bounded)
}

// timeNow is time.Now, named so the deadline has one source.
func timeNow() time.Time { return time.Now() }
