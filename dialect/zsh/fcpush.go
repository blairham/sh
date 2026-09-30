// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `fc -p` puts the history list aside and starts an empty one; `fc -P` puts
// it back.
//
// All three letters were `bad option` here, and the status was the tell: a
// script that brackets its own work with `fc -p` … `fc -P` — which is what the
// idiom is for — got two complaints and a history it did not want to keep
// (#4970).
//
// # What the issue established, and what it left
//
// The issue measured the **acceptance**: zsh is silent at 0 for all four
// spellings, so the visible half of the bug was the refusal, and it said in
// as many words that "what the letters *do* to the history needs measuring
// beyond the acceptance". Accepting them and doing nothing would have been
// the worse answer — a letter accepted and ignored hands a script a success it
// did not earn — so the doing was measured too.
//
// Measured 2026-09-28 on zsh 5.9.2 under `-f` from a script file with `env -i
// PATH=/usr/bin:/bin TERM=dumb` and a scratch `HOME`:
//
//	print -s one; print -s two      ${#history} is 1
//	fc -p                           0, and ${#history} is 0
//	print -s inner                  the pushed list is its own
//	fc -P                           0, and ${#history} is 1 again, holding `one`
//
// and the stack is a stack: `fc -p; fc -P; fc -P` is 0, 0, **1** — popping
// what was never pushed is a failure rather than a silent nothing.
//
// (`${#history}` reads one fewer than the list holds outside the line editor,
// which is that view's own rule and not this one's: see zshHistoryEvents. The
// rows above are quoted as the view reports them because that is what a script
// can see.)
//
// # The operands
//
// `fc -p [file [hist [save]]]`: the file is **read into** the new list the way
// `fc -R` reads one, and the two numbers are the sizes the pushed session
// runs under. Measured with a three-entry list, `fc -p /tmp/hf 2 2` then three
// `print -s` leaves two entries — so the first number is `HISTSIZE` and it
// trims, exactly as an assignment to that parameter does.
//
// # `-a` is accepted and does nothing here, which is measured rather than
// assumed
//
// `-a` says the pushed history should be **appended to its file** when it is
// popped. Five probes could not produce that write: `fc -ap FILE 10 10` with
// three entries pushed and then `fc -P` leaves no file, and neither does the
// same line without `-a`. So in a non-interactive script the reference writes
// nothing on either spelling, and a write modeled here would be a behavior
// only this shell has. The letter is read, recorded on the frame and carried
// through the pop; what it would trigger is left for an instrument that can
// reach it — the same split #4993, #4994 and #4995 draw for a value with no
// reader.
//
// # The storage
//
// A depth counter and one array per level, under names no script can reach —
// the way `zmodload`, `zstyle` and `emulate` keep theirs, which is also what
// gives a subshell its own copy.
const (
	fcPushDepth = ".zsh.history.pushdepth"
	fcPushList  = ".zsh.history.push."
	fcPushSize  = ".zsh.history.pushsize."
)

// fcPushLetter finds `-p`, `-P` or `-a` among the leading option words, and
// reports the letters the call carried.
//
// Read the way fcFileLetter reads its three and for the same reason: anything
// this does not claim is left to the core builtin, so a letter arriving later
// does not have to be added in two places. `-a` is claimed only beside `-p`,
// which is what the reference does with it — `fc -ap` is the spelling, and a
// lone `-a` is a push with no list of its own to append.
func fcPushLetter(args []string) (push, pop, appendOnPop bool, rest []string, found bool) {
	i := 0
	for ; i < len(args); i++ {
		word := args[i]
		if !strings.HasPrefix(word, "-") || len(word) < 2 || word == "--" {
			break
		}
		for j := 1; j < len(word); j++ {
			switch word[j] {
			case 'p':
				push = true
			case 'P':
				pop = true
			case 'a':
				appendOnPop = true
			case 'R':
				// Claimed and not acted on, because a push **already** reads
				// the file it is given: `fc -p file` and `fc -p -R file` are
				// the same list in the reference, measured. Letting the `-R`
				// through instead handed it to fcPush as the *file operand*
				// and the real name to the core's own `-R`, so the file was
				// read twice and the new list held it twice over.
				//
				// Only alongside a push: with no `p` or `P` in the call the
				// loop below falls through and `fc -R file` goes to the core
				// exactly as before.
			default:
				// A letter this does not claim, mixed into the same word:
				// the whole call goes to the core rather than being read
				// half here.
				return false, false, false, nil, false
			}
		}
	}
	// After **every** leading option word, not after the first one holding a
	// `p`. Returning from inside the loop left `fc -p -R file` handing `-R`
	// to fcPush as its file operand and the real name to the core's own
	// `-R`, so the file was read twice and the new list held it twice over.
	if push || pop {
		return push, pop, appendOnPop, args[i:], true
	}
	return false, false, false, nil, false
}

// fcPush is `fc -p [file [hist [save]]]`.
func fcPush(r *interp.Runner, appendOnPop bool, rest []string) int {
	// **The two sizes are checked before anything is pushed**, and that order
	// is measured rather than tidy: a refused `fc -p` leaves the history, the
	// size and the save count exactly as they were. Measured 2026-09-28 on
	// zsh 5.9.2, with three entries in the list and `HISTSIZE=SAVEHIST=100`:
	//
	//	fc -p /dev/null a 0   fc: HISTSIZE must be an integer   1
	//	                      and the list, HISTSIZE and SAVEHIST all unmoved
	//
	// Before this the words were stored unread, so `a` became a HISTSIZE of
	// **one** and the push went ahead — the list emptied, the sizes moved and
	// the status was 0. That is the one assertion `B06fc.ztst` turns on
	// (#4436).
	if len(rest) > 1 {
		if code := fcPushSizeRefused(r, "HISTSIZE", rest[1]); code != 0 {
			return code
		}
	}
	if len(rest) > 2 {
		if code := fcPushSizeRefused(r, "SAVEHIST", rest[2]); code != 0 {
			return code
		}
	}
	depth := fcPushedDepth(r)
	level := strconv.Itoa(depth)
	// The list and the size it is running under, saved together: the sizes
	// are part of the session being put aside, which is why `-p` takes two
	// numbers for the *new* one.
	r.SetArray(fcPushList+level, fcEntries(r))
	r.SetVar(fcPushSize+level, strconv.Itoa(fcHistorySize(r)))
	if appendOnPop {
		r.SetVar(fcPushAppendName(level), "1")
	} else {
		r.SetVar(fcPushAppendName(level), "")
	}
	r.SetVar(fcPushDepth, strconv.Itoa(depth+1))
	r.SetArray(fcHistoryStore, nil)
	if len(rest) > 1 {
		// The sizes before the file, so that reading it trims to the size
		// the push asked for rather than to the one the outer session had.
		//
		// Through the parameter rather than through fcSizeAssigned, which
		// is the *action* that runs behind an assignment to it and reads
		// the value the assignment already stored — calling it directly set
		// the size in force and left `$HISTSIZE` at thirty, which a script
		// can see.
		r.SetVar("HISTSIZE", rest[1])
	}
	if len(rest) > 2 {
		r.SetVar("SAVEHIST", rest[2])
	}
	if len(rest) > 0 && rest[0] != "" {
		// Read the way `fc -R` reads one, and silently where there is
		// nothing to read: measured, `fc -p /no/such/file` is 0 and leaves
		// an empty list, which is what a session started on a file that
		// does not exist yet has to do.
		if text, ok := fcReadFile(r, rest[0]); ok {
			fcLoadText(r, text)
		}
	}
	return 0
}

// fcPop is `fc -P`.
//
// One on an empty stack, which is measured and is the row that says the
// letters are a stack rather than a toggle: `fc -p; fc -P; fc -P` is 0, 0, 1.
func fcPop(r *interp.Runner) int {
	depth := fcPushedDepth(r)
	if depth == 0 {
		return 1
	}
	level := strconv.Itoa(depth - 1)
	entries, _ := r.GetArray(fcPushList + level)
	r.SetArray(fcHistoryStore, append([]string(nil), entries...))
	if held, ok := r.GetVar(fcPushSize + level); ok && held != "" {
		r.SetVar("HISTSIZE", held)
	}
	r.SetArray(fcPushList+level, nil)
	r.SetVar(fcPushSize+level, "")
	r.SetVar(fcPushAppendName(level), "")
	r.SetVar(fcPushDepth, strconv.Itoa(depth-1))
	return 0
}

// fcPushAppendName is where one level's `-a` is remembered.
//
// Kept even though nothing reads it back yet, which is deliberate: the letter
// is part of the frame it was written on, and a pop that had to guess would
// be the shape this file exists to avoid. See the note on `-a` above.
func fcPushAppendName(level string) string { return ".zsh.history.pushappend." + level }

// fcPushedDepth is how many lists are on the stack.
func fcPushedDepth(r *interp.Runner) int {
	held, ok := r.GetVar(fcPushDepth)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(held)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// fcPushSizeRefused refuses one of `fc -p`'s two size words when it is not an
// integer, and says which of the two it was.
//
// **Each word names its own parameter**, which is what says this is two checks
// and not one: measured, `fc -p /dev/null a 0` is `HISTSIZE must be an
// integer` and `fc -p /dev/null 0 a` is `SAVEHIST must be an integer`. The
// first bad word decides, so `fc -p /dev/null a b` names HISTSIZE alone.
//
// What counts as an integer is the decimal reading and nothing wider:
// `10`, `-1` and `010` are taken, and `1.5`, `0x10` and `1e2` are all refused
// — the last two being the rows that say it is not "does this start like a
// number" and not a shell arithmetic evaluation either.
//
// An **empty** word is taken, which is measured and is why the check is not
// simply "parses as an integer": `fc -p /dev/null ” 0` is 0 in the reference.
func fcPushSizeRefused(r *interp.Runner, name, written string) int {
	if written == "" {
		return 0
	}
	if _, err := strconv.Atoi(written); err != nil {
		r.Diagnosef("%s must be an integer\n", name)
		return 1
	}
	return 0
}
