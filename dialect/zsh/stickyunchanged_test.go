// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A sticky emulation already in force is not entered again: nothing is saved
// at the call and nothing put back at the return, so what the callee sets its
// caller keeps (#5144). Which emulations count as "the same", and which
// definitions are made under one, is in dialect/zsh/sticky.go.
//
// Every row is a caller that turns `alwayslastprompt` off, calls a sticky
// function that turns it on, and reads it: `on` is "not entered", `off` is
// "entered and put back". Measured on zsh 5.9.2, `-f`, 2026-09-30; on main
// before this change every `on` row below read `off`.
const stickyIsALP = "isalp() { if [[ -o alwayslastprompt ]]; then print on; else print off; fi; }\n"

// stickyPair defines the callee under `emulate <inner> -c` and a caller under
// `emulate <outer> -c`, and calls the caller.
func stickyPair(outer, inner string) string {
	return "emulate " + inner + " -c 'inr() { setopt alwayslastprompt; }'\n" +
		"emulate " + outer + " -c 'outr() { unsetopt alwayslastprompt; inr; isalp; }'\n" +
		"outr\n"
}

func TestAStickyEmulationInForceIsNotEnteredAgain(t *testing.T) {
	const inSh = "emulate sh -c 'in_sh() { setopt alwayslastprompt; }'\n"
	rows := []struct{ name, src, want string }{
		{"sh calls sh", stickyPair("sh", "sh"), "on"},
		{"csh calls csh", stickyPair("csh", "csh"), "on"},
		{"zsh calls zsh", stickyPair("zsh", "zsh"), "on"},
		{"sh calls csh", stickyPair("sh", "csh"), "off"},
		{"sh calls zsh", stickyPair("sh", "zsh"), "off"},
		{"-R sh calls -R sh", stickyPair("-R sh", "-R sh"), "on"},
		{"-R sh calls sh", stickyPair("-R sh", "sh"), "off"},
		{"sh calls -R sh", stickyPair("sh", "-R sh"), "off"},
		// In force through a plain function in between, and in the `-c` run
		// itself.
		{"sh, plain, sh", inSh + "pl() { in_sh; }\n" +
			"emulate sh -c 'outr() { unsetopt alwayslastprompt; pl; isalp; }'\noutr\n", "on"},
		{"the -c run calls sh", inSh +
			"emulate sh -c 'unsetopt alwayslastprompt; in_sh; isalp'\n", "on"},
		// And nothing is in force at the top level, even after a bare
		// `emulate sh`, nor in a plain function called from there.
		{"top level", inSh + "unsetopt alwayslastprompt\nin_sh\nisalp\n", "off"},
		{"top level after emulate sh", inSh +
			"emulate sh\nunsetopt alwayslastprompt\nin_sh\nisalp\n", "off"},
		{"plain from the top level", inSh +
			"p2() { unsetopt alwayslastprompt; in_sh; isalp; }\np2\n", "off"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), stickyIsALP+row.src)
			if st != 0 || out != row.want+"\n" {
				t.Errorf("out %q status %d, want %q", out, st, row.want+"\n")
			}
		})
	}
}

// The option words beside the mode are part of which emulation it is, as
// written rather than as they come out: order does not count and a name's
// spellings are one name, but a redundant word does, and so does how many
// times a name is written. A name keeps its count and its last direction.
func TestTheStickyOptionWordsArePartOfTheEmulation(t *testing.T) {
	rows := []struct{ name, outer, inner, want string }{
		{"same -o", "sh -o nullglob", "sh -o nullglob", "on"},
		{"order", "sh -o nullglob -o markdirs", "sh -o markdirs -o nullglob", "on"},
		{"spelling", "sh -o nullglob", "sh -o NULL_GLOB", "on"},
		{"no- is +o", "sh -o nonullglob", "sh +o nullglob", "on"},
		{"redundant word", "sh", "sh -o shwordsplit", "off"},
		{"both redundant, different names", "sh -o shwordsplit", "sh +o markdirs", "off"},
		{"written twice", "sh -o nullglob", "sh -o nullglob -o nullglob", "off"},
		{"set and unset is not nothing", "sh", "sh -o nullglob +o nullglob", "off"},
		{"last direction, same", "sh -o nullglob +o nullglob", "sh +o nullglob +o nullglob", "on"},
		{"last direction, different", "sh -o nullglob +o nullglob", "sh +o nullglob -o nullglob", "off"},
		{"counts, same", "sh -o nullglob -o markdirs -o nullglob", "sh -o markdirs -o nullglob -o nullglob", "on"},
		{"counts, different", "sh -o nullglob -o markdirs -o nullglob", "sh -o markdirs -o nullglob -o markdirs", "off"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), stickyIsALP+stickyPair(row.outer, row.inner))
			if st != 0 || out != row.want+"\n" {
				t.Errorf("out %q status %d, want %q", out, st, row.want+"\n")
			}
		})
	}
}

// The option words are entered at every call, not only during the `-c` run,
// and go back at the return (#5244). The `-o` direction, because `+o
// nullglob` is off on the call whether or not it is applied.
func TestAStickyCallAppliesItsOptionWords(t *testing.T) {
	src := "ng() { if [[ -o nullglob ]]; then print ng-on; else print ng-off; fi; }\n" +
		"sw() { if [[ -o shwordsplit ]]; then print sw-on; else print sw-off; fi; }\n" +
		"emulate -R sh -o nullglob -c 'f() { ng; sw; }'\n" +
		"f\nng\nsw\n"
	out, st := runZsh(t, t.TempDir(), src)
	want := "ng-on\nsw-on\nng-off\nsw-off\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// A function defined while a sticky emulation is in force is sticky, whoever
// defines it — the sticky body itself or a plain function it calls (#5245).
// Each is then called from the top level, where it must enter sh and put
// `alwayslastprompt` back: `off`. The control is the same plain definer run
// at the top level, whose function is not sticky: `on`.
func TestAFunctionDefinedUnderAStickyCallIsSticky(t *testing.T) {
	const call = "unsetopt alwayslastprompt\n%s\nisalp\n"
	rows := []struct{ name, src, want string }{
		{
			"defined by the sticky body",
			"emulate sh -c 'mk() { g() { setopt alwayslastprompt; }; }'\nmk\n" +
				strings.Replace(call, "%s", "g", 1), "off",
		},
		{
			"defined by a plain function a sticky one called",
			"plain() { h() { setopt alwayslastprompt; }; }\n" +
				"emulate sh -c 'st() { plain; }'\nst\n" +
				strings.Replace(call, "%s", "h", 1), "off",
		},
		{
			"control: defined at the top level",
			"plain() { h() { setopt alwayslastprompt; }; }\nplain\n" +
				strings.Replace(call, "%s", "h", 1), "on",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), stickyIsALP+row.src)
			if st != 0 || out != row.want+"\n" {
				t.Errorf("out %q status %d, want %q", out, st, row.want+"\n")
			}
		})
	}
}
