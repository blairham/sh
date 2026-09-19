// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// #3156: an invocation option that starts the shell under another shell's
// semantics was read as the name of a `set` option, so the word was refused
// and the mode after it became the script operand.
//
// Named for the axis rather than for a shell, as the help and version options
// next door are: what is under test is that a dialect naming an emulation
// option has its builtin run with the mode, in the position the option was
// granted in, and before the invocation's own options.
const emulationSpelling = "--become"

// emulationShell is a dialect with one registered builtin, which records what
// it was handed and in what order. `calls` is appended to rather than
// overwritten so that a second grant is visible as a second call.
func emulationShell(e interp.EmulationOption, calls *[]string) driver.Shell {
	sem := interp.PosixSemantics()
	sem.EmulationOption = e
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: interp.Diagnostics{InvocationBadLongOption: "%[1]s: invalid option"},
		Register: func(r *interp.Runner) {
			r.Register("become", func(_ *interp.Runner, _ context.Context, args []string) int {
				*calls = append(*calls, strings.Join(args, " "))
				return 0
			})
		},
	}
}

func wholeEmulationOption() interp.EmulationOption {
	return interp.EmulationOption{
		Spellings:       emulationSpelling,
		Builtin:         "become",
		MissingArgument: "%s: argument required",
		OutOfOrder:      "%s: must precede other options",
		Status:          1,
	}
}

func TestAnEmulationOptionRunsItsBuiltinWithTheModeWord(t *testing.T) {
	for _, c := range []struct {
		name   string
		opt    interp.EmulationOption
		argv   []string
		out    string
		errs   string
		status int
		calls  []string
	}{
		{
			// The mode reaches the builtin behind an end-of-options marker,
			// which is what keeps a mode that looks like an option from
			// being read as one.
			name:   "the mode reaches the builtin",
			opt:    wholeEmulationOption(),
			argv:   []string{"testsh", emulationSpelling, "other", "-c", "echo ran"},
			out:    "ran\n",
			calls:  []string{"-- other"},
			status: 0,
		},
		{
			// Unconditionally: a word the front end would otherwise claim is
			// still the mode, and the operand after it is what is left.
			name:   "the next word is taken whatever it is",
			opt:    wholeEmulationOption(),
			argv:   []string{"testsh", emulationSpelling, "--", "-c", "echo ran"},
			out:    "ran\n",
			calls:  []string{"-- --"},
			status: 0,
		},
		{
			// Written twice, granted twice, and the shell starts in the
			// last mode — measured, `--emulate ksh --emulate sh` is sh. The
			// word is recorded rather than applied as it is read, so the
			// earlier one is overwritten and the builtin runs once; which
			// of those two shapes real zsh has is not observable, since an
			// emulation superseded before a line is read leaves nothing
			// behind.
			name:   "the last grant is the one applied",
			opt:    wholeEmulationOption(),
			argv:   []string{"testsh", emulationSpelling, "one", emulationSpelling, "two", "-c", ":"},
			calls:  []string{"-- two"},
			status: 0,
		},
		{
			// The empty word is a mode like any other and is handed over as
			// one, rather than read as no mode at all.
			name:   "the empty mode is still a mode",
			opt:    wholeEmulationOption(),
			argv:   []string{"testsh", emulationSpelling, "", "-c", ":"},
			calls:  []string{"-- "},
			status: 0,
		},
		{
			name:   "the last word is a refusal at the dialect's status",
			opt:    wholeEmulationOption(),
			argv:   []string{"testsh", emulationSpelling},
			errs:   "testsh: --become: argument required\n",
			status: 1,
		},
		{
			// Behind another option word it is refused, and the refusal is
			// the dialect's sentence at the dialect's status — not the usage
			// block and not this front end's usage status.
			name:   "behind another option word it is refused",
			opt:    wholeEmulationOption(),
			argv:   []string{"testsh", "-x", emulationSpelling, "other", "-c", ":"},
			errs:   "testsh: --become: must precede other options\n",
			status: 1,
		},
		{
			// A dialect with no rule about position grants it anywhere,
			// which is what an empty OutOfOrder means.
			name: "an empty position rule grants it anywhere",
			opt: interp.EmulationOption{
				Spellings: emulationSpelling, Builtin: "become",
				MissingArgument: "%s: argument required", Status: 1,
			},
			argv:   []string{"testsh", "-x", emulationSpelling, "other", "-c", ":"},
			errs:   "+ :\n",
			calls:  []string{"-- other"},
			status: 0,
		},
		{
			// The zero value is a shell with no such option — five of the
			// six columns — and the word is refused as any unknown long
			// option is, at this front end's usage status.
			name:   "refused where the dialect names none",
			opt:    interp.EmulationOption{},
			argv:   []string{"testsh", emulationSpelling, "other", "-c", ":"},
			errs:   "testsh: --become: invalid option\n",
			status: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var calls []string
			var out, errs strings.Builder
			sh := emulationShell(c.opt, &calls)
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, c.argv); got != c.status {
				t.Errorf("status %d, want %d (out %q, err %q)", got, c.status, out.String(), errs.String())
			}
			if out.String() != c.out {
				t.Errorf("stdout %q, want %q", out.String(), c.out)
			}
			if errs.String() != c.errs {
				t.Errorf("stderr %q, want %q", errs.String(), c.errs)
			}
			if strings.Join(calls, "|") != strings.Join(c.calls, "|") {
				t.Errorf("builtin calls %q, want %q", calls, c.calls)
			}
		})
	}
}

// The order the two are applied in, asked so that it cannot pass either way
// round. An emulation resets the option table to its mode's defaults, so one
// applied after the invocation's own options would undo them — and the
// cheapest way to see which ran first is an invocation option that *stops* the
// shell: the builtin has already been called when the refusal lands.
func TestAnEmulationIsAppliedBeforeTheInvocationsOwnOptions(t *testing.T) {
	var calls []string
	var out, errs strings.Builder
	sh := emulationShell(wholeEmulationOption(), &calls)
	sh.Stdout, sh.Stderr = &out, &errs
	status := driver.MainArgs(sh, []string{"testsh", emulationSpelling, "other", "-Z", "-c", "echo ran"})
	if status == 0 || strings.Contains(out.String(), "ran") {
		t.Fatalf("status %d out %q, want the bad option to have stopped the shell", status, out.String())
	}
	if len(calls) != 1 {
		t.Errorf("builtin calls %q, want the emulation applied before the option that refused", calls)
	}
}
