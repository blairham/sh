// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestAnEmulationIsKeyedOnTheDefaultAndNotOnTheRoute is the other half of
// TestAStrictEmulationPutsBackAnOptionTheInvocationTurnedOff, and it is here
// because that one cannot fail for the reason this one can.
//
// #4506 was found in a `zsh -f`, so the rule it fixed can be stated two ways
// that agree on every row of that measurement: *a reset puts the name at the
// emulation's default*, or *a reset undoes what the invocation did*. A grid
// over names and modes varies neither of those, because the invocation is
// fixed in all of it — which is the shape a wide grid keyed on the wrong noun
// has, and the pair below is what tells them apart.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, 2026-09-25. Same name, same wanted answer, the
// current state reached two ways:
//
//	zsh -f -c 'emulate -R zsh; …'                      rcs on
//	ZDOTDIR=<empty> zsh -d -c 'setopt norcs;
//	                           emulate -R zsh; …'      rcs on
//
// So the noun is the **option's default** and the route is not part of the
// question. The third row is the control on the other side — a name already
// at the emulation's default is left at it rather than flipped, which a fix
// that wrote the bit unconditionally would get wrong and which no row of a
// suppressed-startup grid can see.
func TestAnEmulationIsKeyedOnTheDefaultAndNotOnTheRoute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		option string
		// shell is how the invocation left the runner, and moved is a
		// `setopt`/`unsetopt` run before the emulation. Between them they
		// are the two routes to one state.
		shell func(*interp.Runner)
		moved *bool
		want  bool
	}{
		{
			"the invocation turned rcs off",
			"rcs", func(r *interp.Runner) { r.StartupFilesSuppressed = true }, nil, true,
		},
		{
			"a setopt turned rcs off",
			"rcs", nil, boolFor(false), true,
		},
		{
			"the shell's own kind turned hashdirs off",
			"hashdirs", func(r *interp.Runner) { r.Interactive = false }, nil, true,
		},
		{
			"an unsetopt turned hashdirs off at a prompt",
			"hashdirs", func(r *interp.Runner) { r.Interactive = true }, boolFor(false), true,
		},
		{
			"rcs stays on where the invocation left it on",
			"rcs", nil, nil, true,
		},
		{
			"hashdirs stays on where the shell's own kind left it on",
			"hashdirs", func(r *interp.Runner) { r.Interactive = true }, nil, true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := optionStateRunner(t)
			if tc.shell != nil {
				tc.shell(r)
			}
			o, _, ok := exactOptionName(tc.option)
			if !ok {
				t.Fatalf("%s is not in the option table", tc.option)
			}
			if tc.moved != nil {
				if code := o.set(r, *tc.moved); code != 0 {
					t.Fatalf("setting %s to %v answered %d", tc.option, *tc.moved, code)
				}
				if got := o.get(r); got != *tc.moved {
					t.Fatalf("%s reads %v after being set to %v, so the row's route was never taken",
						tc.option, got, *tc.moved)
				}
			}
			applyEmulation(r, "zsh", true)
			if got := o.get(r); got != tc.want {
				t.Errorf("%s reads %v after emulate -R zsh, want %v", tc.option, got, tc.want)
			}
		})
	}
}

func boolFor(b bool) *bool { return &b }
