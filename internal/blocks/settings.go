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
	// OutputVar turns the output half on. Empty — the default — records the
	// command half only, because capture costs a child its terminal and the
	// command half costs nothing. See Capture.Stream.
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

// CaptureFrom reports whether this session keeps output, and how much.
//
// Off unless asked, and the cost is the reason: see Capture.Stream. A max that
// is not a number leaves the default rather than zero, which would turn the
// output half back off through a typo — the same rule HISTFILESIZE follows.
func CaptureFrom(get func(string) (string, bool)) (on bool, max int) {
	v, ok := get(OutputVar)
	if !ok || v == "" {
		return false, 0
	}
	max = DefaultMaxOutput
	if s, ok := get(MaxOutputVar); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			max = n
		}
	}
	return true, max
}
