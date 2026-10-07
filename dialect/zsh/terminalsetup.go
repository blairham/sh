// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// Which of the description's two readings `$terminfo` and `$termcap` answer
// from, and what moves the shell from one to the other (#5315).
//
// A description has a stored reading and a converted one —
// repl.ConvertedCapabilities says what differs, and the difference is real
// keys: `is3` in one is `OTi2` in the other. zsh 5.9.2 hands a script either,
// depending on what the shell has done with its terminal so far, so the same
// expansion answers differently at two points in one script:
//
//	zsh -fc 'zmodload zsh/terminfo; print ${+terminfo[is3]}'    0
//	zsh -fic 'zmodload zsh/terminfo; print ${+terminfo[is3]}'   1
//
// under `TERM=d217-unix`, whose description carries `is3`. Neither is the
// broken one. A script sees the converted reading once the shell has *set
// its terminal up* while a capability module was loaded, and the stored
// reading otherwise.
//
// # The rule, measured
//
// zsh 5.9.2 under `env -i`, 2026-10-02, reading an enumeration of
// `$terminfo` for `OTbc` — present in the converted reading of `d217-unix`
// and absent from the stored one — after each of these:
//
//	-c, nothing                                     stored
//	-c, ${terminfo[bel]} or ${+terminfo[bel]}       converted
//	-c, echoti bel / echotc bl / ${termcap[bl]}     converted
//	-c, ${(kv)termcap} or ${terminfo[(I)b*]}        stored — a listing sets nothing up
//	-c, zmodload zsh/terminfo; print -P %B          converted
//	-c, print -P %B; zmodload zsh/terminfo          stored, and it stays stored
//	                                                after ${terminfo[bel]}
//	-c, print -P %B; echotc bl                      stored, the same way
//	-c, zmodload zsh/termcap; print -P %B           converted — either module
//	-c, print -P %B | od -c                         stored — the pipeline's left
//	                                                side is a subshell
//	-i (a terminal or not), anything                stored
//	-i, zmodload zsh/terminfo; TERM=other           converted
//	-i, TERM=other; then the first reference        stored
//	-c, converted, then TERM=other                  stored, until a lookup
//	-c, converted, then TERM=$TERM                  converted — the same value
//	                                                changes nothing
//
// Which comes to one state machine. The shell **sets its terminal up** for a
// `$TERM`, and the reading in force is the converted one exactly when the
// last setting up was for the `$TERM` the shell has now and yielded the
// converted reading. Setting up yields the converted reading if a capability
// module has been reached by then — a reference to either parameter, `echoti`,
// `echotc`, or `zmodload` of either module — and the stored one if not.
//
// When it sets up is where the two kinds of shell part. An interactive shell
// sets up at startup, before anything can have been reached, and again on
// every assignment to `$TERM`, the same value included. Any other shell sets
// up only when something needs the terminal — a lookup, `echoti`, `echotc` or
// a prompt attribute — and the last setting up was for some other `$TERM`, or
// there was none. A lookup reaches its module before it sets up, so a lookup
// always yields the converted reading; a prompt drawn first does not, and the
// shell then stays in the stored reading because it is already set up. An
// enumeration or a pattern subscript reaches the module and sets nothing up.
//
// Measured on the rows above and on these, the same way:
//
//	-c, ${terminfo[bel]}; TERM=xterm; TERM=$T        converted — the setting up
//	                                                 was for this $TERM
//	-c, ${terminfo[bel]} under xterm; TERM=d217-unix stored, until a lookup
//	-i, ${terminfo[bel]}; TERM=$T                    converted — an assignment
//	                                                 sets up, and the lookup had
//	                                                 reached the module
//	-i, zmodload zsh/terminfo; TERM=xterm; TERM=$T   converted
//	-i, TERM=xterm; TERM=$T                          stored — nothing reached
//
// The state belongs to the shell and not to the process, which the pipeline
// row says: the left side of `print -P %B | od -c` set its own terminal up
// and the shell after it had not. So it is kept in a store a subshell gets a
// copy of, the way `emulate` keeps its mode.

// terminalSetupStore is where the state lives.
const terminalSetupStore = zshEngineStorePrefix + "terminal"

// terminalSetup is the state, decoded.
type terminalSetup struct {
	// reached says a capability module has been reached.
	reached bool
	// setUp says the shell has set its terminal up, for term.
	setUp bool
	term  string
	// converted says that setting up yielded the converted reading.
	converted bool
}

// setUpFor sets the terminal up for term.
func (s *terminalSetup) setUpFor(term string) {
	s.setUp, s.term, s.converted = true, term, s.reached
}

// advanceTerminalSetup applies one use of the description to the shell's
// state and says whether the converted reading is in force afterwards.
func advanceTerminalSetup(r *interp.Runner, use terminalUse) bool {
	term, _ := r.GetVar("TERM")
	s := loadTerminalSetup(r)
	before := s
	switch use {
	case listingCapabilities:
		s.reached = true
	case readingCapability:
		s.reached = true
		if !s.setUp || s.term != term {
			s.setUpFor(term)
		}
	case drawingAttribute:
		if !s.setUp || s.term != term {
			s.setUpFor(term)
		}
	}
	if s != before {
		storeTerminalSetup(r, s)
	}
	return s.setUp && s.converted && s.term == term
}

// terminalAssigned is what an assignment to `$TERM` does: in an interactive
// shell it sets the terminal up again, and in any other nothing at all.
func terminalAssigned(r *interp.Runner, term string) {
	if interactive, _ := r.DialectOption("interactive"); !interactive {
		return
	}
	s := loadTerminalSetup(r)
	s.setUpFor(term)
	storeTerminalSetup(r, s)
	// And a terminal called `emacs` takes the line editor away. See
	// editorOffInsideEmacs.
	editorOffInsideEmacs(r, term)
}

// loadTerminalSetup reads the store. Three flags and the `$TERM` they were
// decided under, the flags first so that a `$TERM` holding anything at all
// cannot be mistaken for one.
//
// A store never written is the shell as it started: set up for the `$TERM` it
// has now if it is interactive, with nothing reached, and not set up at all
// otherwise.
func loadTerminalSetup(r *interp.Runner) terminalSetup {
	v, ok := r.GetVar(terminalSetupStore)
	if !ok || len(v) < 3 {
		s := terminalSetup{}
		if interactive, _ := r.DialectOption("interactive"); interactive {
			term, _ := r.GetVar("TERM")
			s.setUpFor(term)
		}
		return s
	}
	return terminalSetup{
		reached: v[0] == '1', setUp: v[1] == '1', converted: v[2] == '1',
		term: v[3:],
	}
}

// storeTerminalSetup writes the store.
func storeTerminalSetup(r *interp.Runner, s terminalSetup) {
	var b strings.Builder
	for _, flag := range []bool{s.reached, s.setUp, s.converted} {
		if flag {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	b.WriteString(s.term)
	r.SetVar(terminalSetupStore, b.String())
}
