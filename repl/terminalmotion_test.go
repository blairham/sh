// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// described is a description holding the given string capabilities, the way
// TerminalCapabilities answers for one.
func described(strs map[string]string) []TerminalCapability {
	var caps []TerminalCapability
	for name, v := range strs {
		caps = append(caps, TerminalCapability{Terminfo: name, Value: v, Kind: StringCapability})
	}
	return caps
}

// xterm's own description draws every byte the ANSI the editor spoke before
// it read one: the guarantee that taking the sequences from the description
// changed nothing on the terminals people use (#6324). The values are
// xterm-256color's, as `infocmp -1` prints them.
func TestXtermsDescriptionMovesAsANSIDid(t *testing.T) {
	xterm := motionOf(described(map[string]string{
		"cub1": "\b", "cub": "\x1b[%p1%dD",
		"cuf1": "\x1b[C", "cuf": "\x1b[%p1%dC",
		"cuu1": "\x1b[A", "cuu": "\x1b[%p1%dA",
		"cud1": "\n", "cud": "\x1b[%p1%dB",
		"el": "\x1b[K", "ed": "\x1b[J", "clear": "\x1b[H\x1b[2J",
		"sgr0": "\x1b(B\x1b[m",
	}), 0)
	for fromRow := range 3 {
		for toRow := range 3 {
			for fromCol := 0; fromCol < 120; fromCol += 7 {
				for toCol := 0; toCol < 120; toCol += 5 {
					var want, got strings.Builder
					ansiMotion.moveCursor(&want, fromRow, fromCol, toRow, toCol, nil)
					xterm.moveCursor(&got, fromRow, fromCol, toRow, toCol, nil)
					if got.String() != want.String() {
						t.Fatalf("(%d,%d) to (%d,%d): xterm moved with %q, ANSI with %q",
							fromRow, fromCol, toRow, toCol, got.String(), want.String())
					}
				}
			}
		}
	}
	for _, c := range []struct{ name, got, want string }{
		{"el", xterm.eraseToRowEnd(), "\x1b[K"},
		{"ed", xterm.eraseToScreenEnd(), "\x1b[J"},
		{"clear", xterm.clearScreen(), clearScreenSequence},
		{"the reset", xterm.resetBeforeErase(), highlightReset},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}
}

// A terminal with single steps and no counted moves is moved with the steps,
// by the shorter of walking back and returning to the start of the row. vt52's
// sequences; measured 2026-10-07 on bash 5.3.20 under `TERM=vt52`, `P> echo
// abc` then ^A is `\r\eC\eC\eC` and three left is `\eD\eD\eD` (#6324).
func TestATerminalWithOnlySingleStepsMovesWithThem(t *testing.T) {
	vt52 := motionOf(described(map[string]string{
		"cub1": "\x1bD", "cuf1": "\x1bC", "cuu1": "\x1bA", "cud1": "\x1bB",
		"el": "\x1bK", "ed": "\x1bJ", "clear": "\x1bH\x1bJ",
	}), 0)
	for _, c := range []struct {
		name                           string
		fromRow, fromCol, toRow, toCol int
		want                           string
	}{
		{"three left", 0, 14, 0, 11, "\x1bD\x1bD\x1bD"},
		{"to the start of the line, past the prompt", 0, 11, 0, 3, "\r\x1bC\x1bC\x1bC"},
		{"to the start of the row", 0, 11, 0, 0, "\r"},
		{"right", 0, 3, 0, 6, "\x1bC\x1bC\x1bC"},
		{"up two rows", 2, 5, 0, 5, "\x1bA\x1bA"},
		{"down a row", 0, 5, 1, 5, "\x1bB"},
	} {
		var b strings.Builder
		vt52.moveCursor(&b, c.fromRow, c.fromCol, c.toRow, c.toCol, nil)
		if b.String() != c.want {
			t.Errorf("%s: moved with %q, want %q", c.name, b.String(), c.want)
		}
	}
	if got := vt52.eraseToScreenEnd(); got != "\x1bJ" {
		t.Errorf("ed: %q", got)
	}
	if got := vt52.resetBeforeErase(); got != "" {
		t.Errorf("a terminal with no attributes was reset with %q", got)
	}
	if got := vt52.clearScreen(); got != "\x1bH\x1bJ" {
		t.Errorf("clear: %q", got)
	}
}

// A delay is taken off a sequence at no speed, and written as NULs at one.
func TestAMovesDelayFollowsTheSpeed(t *testing.T) {
	caps := described(map[string]string{"cuf1": "\x1b[C$<2>"})
	slow, fast := motionOf(caps, 0), motionOf(caps, 9600)
	if got := slow.rightBy(2); got != "\x1b[C\x1b[C" {
		t.Errorf("with no speed: %q", got)
	}
	if got := fast.rightBy(1); got != "\x1b[C\x00\x00" {
		t.Errorf("at 9600: %q", got)
	}
}

// A generic description is not a terminal unless it can clear and address the
// cursor: measured 2026-10-07, bash 5.3.20 asks for no paste markers under
// `unknown` and `ibm327x`, and does under `gn` with both `clear` and `cup`
// (#6332). No description at all is not one either.
func TestAGenericDescriptionIsNotATerminal(t *testing.T) {
	flag := func(name string) TerminalCapability {
		return TerminalCapability{Terminfo: name, Value: "yes", Kind: BooleanCapability}
	}
	str := func(name string) TerminalCapability {
		return TerminalCapability{Terminfo: name, Value: "x", Kind: StringCapability}
	}
	for _, c := range []struct {
		name string
		caps []TerminalCapability
		want bool
	}{
		{"nothing", nil, false},
		{"unknown: am gn cr", []TerminalCapability{flag("am"), flag("gn"), str("cr")}, false},
		{"gn and clear", []TerminalCapability{flag("gn"), str("clear")}, false},
		{"gn and cup", []TerminalCapability{flag("gn"), str("cup")}, false},
		{"gn, clear and cup", []TerminalCapability{flag("gn"), str("clear"), str("cup")}, true},
		{"am and cr, not generic", []TerminalCapability{flag("am"), str("cr")}, true},
		{"gn stored as no", []TerminalCapability{{Terminfo: "gn", Value: "no", Kind: BooleanCapability}, str("cr")}, true},
	} {
		if got := terminalIsUsable(c.caps); got != c.want {
			t.Errorf("%s: usable %v, want %v", c.name, got, c.want)
		}
	}
}
