// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// A name a dialect's `set -o` listing writes must be a name `set` takes, in
// both directions — unless the shell being imitated refuses it, and then it
// must be refused for the same name.
//
// The invariant is the roster's half of the one
// [TestNoLetterIsBothImplementedAndNot] keeps for the letters, and it is
// worth having for the same reason: a listing is a **capture surface**. A
// script saves its option state with `eval "$(set +o)"`, so a row the listing
// writes that `set` will not take is a line the shell hands out and then
// rejects — which is worse than a short listing, because the short listing at
// least never promised. That is #3128, and it arrived because #2925 grew
// ksh93's listing to all thirty-two of its rows while the mover stayed where
// it was.
//
// The refusals below are measured rather than declared, 2026-09-16, by asking
// each reference shell for every name in its *own* listing in both
// directions:
//
//	bash 5.3.20   refuses nothing
//	bash 3.2.57   refuses nothing
//	dash 0.5.12   refuses nothing
//	BusyBox ash   refuses nothing it lists
//	ksh93u+       interactive, login_shell, rc — `bad option(s)`, both ways
//	zsh 5.9.2     interactive, shinstdin, singlecommand, zle, and monitor
//
// So five of the six columns take every name they advertise, and the two that
// do not refuse only names about *being interactive* — which is why a table of
// exceptions here is a handful of lines rather than a per-dialect roster.
//
// The **sign** is part of the exception and is measured too, because the two
// shells do not refuse alike. ksh93 answers `bad option(s)` to its three in
// both directions. zsh refuses only the request that would *move* one: in a
// shell that is not interactive, `set +o interactive` is the state it is
// already in and is granted at 0, which is the same bargain `set +o posix`
// strikes in a shell with no posix mode. A list of bare names would have
// asserted zsh into refusing four requests it grants.
//
// `monitor` is left out of the sweep in every column. It is the one request
// that can be refused for a reason outside the option table — job control
// wants a terminal, and these runners have none — so its answer here is about
// the test's environment rather than about the roster. zsh's entry above is
// the same name refused for the other reason, and is exercised by
// dialect/zsh's own tests.
func TestEveryListedOptionNameIsOneAScriptCanMove(t *testing.T) {
	for _, d := range []struct {
		preset dialecttest.Preset
		// refused are the requests this shell lists and will not take,
		// written as they are asked — the sign is part of the fact.
		refused []string
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, nil},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, nil},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, nil},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, []string{
			"-o interactive", "+o interactive",
			"-o login_shell", "+o login_shell",
			"-o rc", "+o rc",
		}},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, []string{"-o interactive", "-o shinstdin", "-o singlecommand", "-o zle"}},
	} {
		t.Run(d.preset.Name, func(t *testing.T) {
			refused := make(map[string]bool, len(d.refused))
			for _, n := range d.refused {
				refused[n] = true
			}
			// The rows this shell would write, read from the same place
			// `set -o` reads them so the two can never disagree about which
			// names exist.
			var out, errs strings.Builder
			listing := d.preset.Runner(dialecttest.Base{Stdout: &out, Stderr: &errs}).ListedOptions()
			if len(listing) == 0 {
				t.Fatal("this dialect lists no options at all")
			}
			for _, row := range listing {
				if row.Name == "monitor" {
					continue
				}
				for _, sign := range []string{"-", "+"} {
					asked := sign + "o " + row.Name
					src := "set " + asked + "\n"
					// A fresh runner per request: `set` is a special
					// builtin, so a refusal ends the shell and everything
					// after it would be asked of a runner that had stopped.
					var said strings.Builder
					r := d.preset.Runner(dialecttest.Base{Stdout: &strings.Builder{}, Stderr: &said})
					if _, err := r.Run(context.Background(), d.preset.Parse(t, src)); err != nil &&
						!refused[asked] {
						t.Errorf("%q: %v", src, err)
						continue
					}
					switch complaint := said.String(); {
					case complaint == "" && refused[asked]:
						t.Errorf("%q was taken; this shell refuses that request", src)
					case complaint != "" && !refused[asked]:
						t.Errorf("%q said %q; the listing writes this row, so the shell has to "+
							"take it back", src, complaint)
					}
				}
			}
		})
	}
}
