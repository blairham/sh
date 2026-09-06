// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"path/filepath"
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
// A state directory rather than a dotfile in the home, because the bodies make
// this a growing directory of arbitrary size, which is what a state directory
// is for.
func DirFrom(get func(string) (string, bool)) string {
	// Asked first: a session that is not recording lines must not be recording
	// blocks, whatever else it was told.
	if h, ok := get("HISTFILE"); ok && h == "" {
		return ""
	}
	if dir, ok := get(DirVar); ok {
		return dir
	}
	if state, ok := get("XDG_STATE_HOME"); ok && state != "" {
		return filepath.Join(state, "sh", "blocks")
	}
	home, _ := get("HOME")
	if home == "" {
		// Nowhere to put it, which is the same answer the history file gives.
		return ""
	}
	return filepath.Join(home, ".local", "state", "sh", "blocks")
}

// A CaptureMode is what a session was told to do about keeping output.
//
// Three answers rather than two, and the third is the default. Keeping output
// used to cost a child its terminal — interp hands a child r.Stdout directly,
// so anything that is not an *os.File makes os/exec build a pipe and `isatty`
// is false downstream of it — which is why this was off unless asked. A front
// end that can put a pseudo-terminal behind the capture pays nothing for it,
// and one that cannot still pays the whole price. So the setting says which of
// those a session is willing to accept, and the front end says which it is.
type CaptureMode int

const (
	// CaptureOff keeps no output. What an empty OutputVar asks for.
	CaptureOff CaptureMode = iota
	// CaptureIfTerminal keeps output where the front end can do it without a
	// child noticing, and keeps none where it cannot. The default: a session
	// that said nothing gets the whole feature at a terminal and no surprises
	// anywhere else.
	CaptureIfTerminal
	// CaptureAlways keeps output whatever it costs, which is what a
	// non-empty OutputVar asks for. Still worth having, and it is not a
	// legacy spelling: a session recording a build in a pipeline has no
	// terminal to preserve and wants the bodies anyway.
	CaptureAlways
)

// CaptureFrom reports what this session keeps of what it printed, and how much.
//
// A max that is not a number leaves the default rather than zero, which would
// turn the output half back off through a typo — the same rule HISTFILESIZE
// follows.
func CaptureFrom(get func(string) (string, bool)) (mode CaptureMode, max int) {
	v, ok := get(OutputVar)
	switch {
	case ok && v == "":
		// Set to nothing is the one way to say "not this session", and it is
		// the same gesture an empty HISTFILE is. It has to exist now that the
		// default is on; before, unset already meant off and nobody needed it.
		return CaptureOff, 0
	case !ok:
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
