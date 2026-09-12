// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"strconv"
	"strings"
)

// The variables a session is configured with.
//
// SH_-prefixed rather than HIST-shaped because they are ours: no shell in the
// panel has a variable by any of these names, so none can collide with
// something an existing rc file sets and means differently.
const (
	// DirVar names the store. An empty value turns it off, which is the same
	// idiom an empty HISTFILE already is, so there is one thing to learn.
	DirVar = "SH_BLOCKS_DIR"
	// OutputVar turns the output half on, off, or leaves it to the front end.
	// Unset is the default and means "keep output if a terminal can be put
	// behind it"; empty turns it off outright, the same idiom an empty
	// HISTFILE already is; anything else keeps output whatever it costs. See
	// CaptureFrom, and Capture.Stream for what the cost is.
	OutputVar = "SH_BLOCKS_OUTPUT"
	// MaxOutputVar bounds one block's body, in bytes.
	MaxOutputVar = "SH_BLOCKS_MAX_OUTPUT"
)

// DirFrom resolves where the store lives, and empty means there is none.
//
// The lookup is passed in rather than read from the process, and that is the
// same rule the rest of the tree follows: a Runner holds its own variables, so
// the front end at a prompt asks the shell and a tool inspecting a store asks
// the environment. A package that reached for os.Getenv would be answering for
// whichever of those the process happened to be.
//
// An empty HISTFILE turns the store off too, and the coupling is deliberate:
// `HISTFILE=` is what a person types when they mean *do not remember this
// session*, and a shell that honored it for the line file while writing a
// richer record — with the output attached — would be doing the opposite of
// what was asked in the one moment it matters most.
//
// # Unset is off, and naming the directory is how a person opts in
//
// There is no default location any more. A session that was told nothing
// keeps no blocks, and a session that names a directory keeps them there —
// which makes the one variable both the switch and the address, with no
// second thing to set and no sentinel to learn.
//
// It used to fall back to $XDG_STATE_HOME/sh/blocks and then to the home, so
// the store was on for everybody who had a home and history. That was the
// first of the open questions in docs/design/blocks.md, flagged there as a
// default chosen without an answer and cheap to reverse, and #2274 is the
// answer. The short of it: the case for on-by-default was written about the
// *command* half — one small line per command, and free — and neither premise
// survived the output half, which puts a pseudo-terminal in front of every
// child and writes down what the person was *shown* rather than what they
// typed. Nothing reads the store yet either, so on-by-default was collecting
// what nobody was consuming; and a default is easy to loosen later and a
// regression to tighten, which is the argument for doing it before v0.0.0
// rather than after.
//
// Empty still means off, unchanged, so a rc file that says `SH_BLOCKS_DIR=`
// keeps meaning what it meant. It is now the same answer as unset rather than
// a distinct gesture, which costs nothing and keeps the line working.
func DirFrom(get func(string) (string, bool)) string {
	// Asked first: a session that is not recording lines must not be recording
	// blocks, whatever else it was told.
	if h, ok := get("HISTFILE"); ok && h == "" {
		return ""
	}
	// Named or nowhere. An unset variable is a person who has not asked for
	// this, and inventing a directory for them is what this stopped doing.
	dir, _ := get(DirVar)
	return dir
}

// A CaptureMode is what a session was told to do about keeping output.
//
// Three answers rather than two, and none of them is what a session that said
// nothing gets — saying nothing is CaptureOff. Keeping output costs a child
// its terminal unless the front end can put a pseudo-terminal behind the
// capture: interp hands a child r.Stdout directly, so anything that is not an
// *os.File makes os/exec build a pipe and `isatty` is false downstream of it.
// A front end that can pays nothing for it and one that cannot pays the whole
// price, so the setting says which of those a session is willing to accept and
// the front end says which it is.
type CaptureMode int

const (
	// CaptureOff keeps no output. What an unset or empty OutputVar asks for,
	// which is to say what a session gets unless it asked for the other
	// thing (#2274).
	CaptureOff CaptureMode = iota
	// CaptureIfTerminal keeps output where the front end can do it without a
	// child noticing, and keeps none where it cannot. Asked for by name: see
	// CaptureTerminalValue, and note this was the default once.
	CaptureIfTerminal
	// CaptureAlways keeps output whatever it costs, which is what any other
	// value asks for. Still worth having, and it is not a legacy spelling: a
	// session recording a build in a pipeline has no terminal to preserve and
	// wants the bodies anyway.
	CaptureAlways
)

// CaptureTerminalValue asks for the output half only where it is free.
//
// A word rather than the absence of one, which is the whole of what changed:
// this used to be what an unset variable meant, so every session at a terminal
// wrote down what its commands printed without anyone asking. It is worth
// keeping as a spelling — a session that wants bodies but will not take a
// child's terminal away is a real thing to want, and #720 is the measurement
// that says the two cost different amounts — so it keeps a name instead of
// becoming a mode nothing can reach.
const CaptureTerminalValue = "terminal"

// CaptureFrom reports what this session keeps of what it printed, and how much.
//
// Unset is off. Recording what a person was *shown* is not the same class of
// thing as recording what they typed — a history file holds keystrokes, and a
// body holds whatever was on the screen, credentials included — and it is the
// only half of this store whose harm a backup makes permanent. So it is asked
// for by name or it does not happen; see DirFrom for the rest of the argument
// and #2274 for the whole of it.
//
// A max that is not a number leaves the default rather than zero, which would
// turn the output half back off through a typo — the same rule HISTFILESIZE
// follows.
func CaptureFrom(get func(string) (string, bool)) (mode CaptureMode, max int) {
	v, ok := get(OutputVar)
	switch {
	case !ok, v == "":
		// Unset and empty are one answer now. Empty is still worth accepting
		// on its own: it is the same gesture an empty HISTFILE is, and a rc
		// file that spells the refusal out keeps working.
		return CaptureOff, 0
	case v == CaptureTerminalValue:
		mode = CaptureIfTerminal
	default:
		mode = CaptureAlways
	}
	max = DefaultMaxOutput
	if s, ok := get(MaxOutputVar); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			max = n
		}
	}
	return mode, max
}
