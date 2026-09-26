// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// #4640: a shell whose argv[0] names another shell did not start in that
// shell's emulation, so a binary copied to a file called `sh` answered every
// question as itself.
//
// Named for the question and not for a shell, the way the emulation option
// next door is: what is under test is that a dialect naming the letters has
// its emulation builtin run with the mode the name picked, before the
// invocation's own options, and that a dialect naming none is untouched.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) — `go version
// -m` says *not a Go executable* for it — by copying the reference binary to
// a file with each name and running it. A symlink answers as the copy does,
// which is the control that says the name and not the file is what is read.
func nameInitials() interp.EmulationOption {
	e := wholeEmulationOption()
	e.NameInitials = "s=sh b=sh k=ksh c=csh"
	e.NameDropsInitial = "r"
	return e
}

// TestEmulationNamed pins which letter starts which mode.
//
// The rule is keyed on the **first letter** of the basename, and the two rows
// that say so are `b` — which enters sh emulation with no `s` in the word at
// all — and `xsh`, which does not enter it while containing one. Any table
// that read the word `sh`, or a name beginning with `sh`, agrees with this
// one everywhere else and is wrong on exactly those two.
func TestEmulationNamed(t *testing.T) {
	for _, tc := range []struct {
		argv0 string
		want  string
	}{
		{"sh", "sh"},
		{"/bin/sh", "sh"},
		{"./sh", "sh"},
		// The login convention, which prepends exactly one dash.
		{"-sh", "sh"},
		{"-/bin/sh", "sh"},
		// The basename is taken *before* the dash is stripped, which is the
		// one place this reading and PosixNamed's part company: measured,
		// `./d/-sh` is sh emulation in zsh and is not the name `sh` to bash.
		{"./d/-sh", "sh"},
		{"-x/sh", "sh"},
		// Two dashes is not the convention.
		{"--sh", ""},
		{"--s", ""},
		// The first letter and not the word. `s` and `b` carry the whole
		// claim: no grid of plausible shell names reaches them.
		{"s", "sh"},
		{"sx", "sh"},
		{"shx", "sh"},
		{"shell", "sh"},
		{"b", "sh"},
		{"bash", "sh"},
		{"bsh", "sh"},
		{"k", "ksh"},
		{"ksh", "ksh"},
		{"c", "csh"},
		{"csh", "csh"},
		// And the letters that name no mode.
		{"zsh", ""},
		{"xsh", ""},
		{"mysh", ""},
		{"ash", ""},
		{"dash", ""},
		{"fish", ""},
		// Case-sensitively: the same word in the other case is not it.
		{"SH", ""},
		{"", ""},
		{"sh/", ""},
		// One leading `r` is dropped first — the restricted-shell prefix,
		// which this shell does not otherwise have. Exactly one: `rr` is
		// `r`, which names nothing.
		{"rsh", "sh"},
		{"rs", "sh"},
		{"rksh", "ksh"},
		{"rzsh", ""},
		{"rr", ""},
		{"r", ""},
		{"-rsh", "sh"},
	} {
		t.Run(tc.argv0, func(t *testing.T) {
			if got := driver.EmulationNamed([]string{tc.argv0}, nameInitials()); got != tc.want {
				t.Errorf("EmulationNamed(%q) = %q, want %q", tc.argv0, got, tc.want)
			}
		})
	}
	if got := driver.EmulationNamed(nil, nameInitials()); got != "" {
		t.Errorf("EmulationNamed(nil) = %q, want no mode — an argv with nothing in it names nothing", got)
	}
	// And a dialect that names no letters is never asked, whatever it was
	// called. This is the row every other column of the panel sits on: bash
	// invoked as `sh` is in POSIX mode and emulates nothing.
	if got := driver.EmulationNamed([]string{"sh"}, wholeEmulationOption()); got != "" {
		t.Errorf("EmulationNamed(sh) with no letters = %q, want no mode", got)
	}
}

// TestTheNameRunsTheEmulationBuiltin is the end of the wire: the letter the
// dialect named reaches the builtin, with the mode behind an end-of-options
// marker exactly as the option's own route delivers it.
func TestTheNameRunsTheEmulationBuiltin(t *testing.T) {
	for _, c := range []struct {
		name  string
		argv0 string
		calls []string
	}{
		{"the name picks the mode", "sh", []string{"-- sh"}},
		{"another letter, another mode", "ksh", []string{"-- ksh"}},
		{"the first letter and not the word", "b", []string{"-- sh"}},
		{"a letter that names nothing runs nothing", "xsh", nil},
		{"and the shell's own name runs nothing", "testsh", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			sh := emulationShell(nameInitials(), &calls)
			if got := driver.MainArgs(sh, []string{c.argv0, "-c", ":"}); got != 0 {
				t.Fatalf("status %d, want 0", got)
			}
			if len(calls) != len(c.calls) {
				t.Fatalf("calls %q, want %q", calls, c.calls)
			}
			for i := range calls {
				if calls[i] != c.calls[i] {
					t.Errorf("call %d = %q, want %q", i, calls[i], c.calls[i])
				}
			}
		})
	}
}

// The option wins outright over the name, in both directions, and that is
// measured rather than assumed from precedence: with the reference copied to
// a file called `sh`, `--emulate zsh` gives a shell reporting `zsh`, and
// under its own name `--emulate sh` gives one reporting `sh`.
//
// Outright is the load-bearing word. `--emulate fish` — a mode no shell knows
// and that the builtin passes over in silence — leaves a binary called `sh`
// reporting `zsh`, so writing the option is what puts the name aside and the
// name is not a fallback for a word that meant nothing.
func TestTheEmulationOptionWinsOverTheName(t *testing.T) {
	for _, c := range []struct {
		name  string
		argv  []string
		calls []string
	}{
		{"the option replaces the name's mode", []string{"sh", emulationSpelling, "other", "-c", ":"}, []string{"-- other"}},
		{"even where its word names nothing", []string{"sh", emulationSpelling, "fish", "-c", ":"}, []string{"-- fish"}},
		{"and the name is not applied as well", []string{"sh", emulationSpelling, "zsh", "-c", ":"}, []string{"-- zsh"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			sh := emulationShell(nameInitials(), &calls)
			if got := driver.MainArgs(sh, c.argv); got != 0 {
				t.Fatalf("status %d, want 0", got)
			}
			if len(calls) != len(c.calls) || (len(calls) == 1 && calls[0] != c.calls[0]) {
				t.Fatalf("calls %q, want %q", calls, c.calls)
			}
		})
	}
}

// And it is a fact about the name rather than about how the program arrived,
// which is the shape PosixNamed's own route test has next door: the same
// three routes, the same one word, the same answer.
func TestTheNamesEmulationReachesEveryRoute(t *testing.T) {
	script := writeScript(t, ":\n")
	for _, r := range []struct {
		name  string
		argv  func(argv0 string) []string
		typed string
	}{
		{"a command string", func(a string) []string { return []string{a, "-c", ":"} }, ""},
		{"a script operand", func(a string) []string { return []string{a, script} }, ""},
		{"standard input", func(a string) []string { return []string{a} }, ":\n"},
	} {
		for _, c := range []struct {
			argv0 string
			want  []string
		}{
			{"sh", []string{"-- sh"}},
			{"testsh", nil},
		} {
			t.Run(r.name+"/"+c.argv0, func(t *testing.T) {
				var calls []string
				sh := emulationShell(nameInitials(), &calls)
				runPipedShell(t, sh, r.typed, r.argv(c.argv0)...)
				if len(calls) != len(c.want) || (len(calls) == 1 && calls[0] != c.want[0]) {
					t.Errorf("calls %q, want %q", calls, c.want)
				}
			})
		}
	}
}
