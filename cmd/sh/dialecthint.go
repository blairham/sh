// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"
	"strings"
	"sync"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A name that is somebody's builtin, refused as if it were a typo — what this
// file is for, and the three things that keep it small.
//
// In the core dialect these two failures are the same sentence:
//
//	$ sh -c 'whence ls'      sh: whence: not found
//	$ sh -c 'lss'            sh: lss: not found
//
// The second is a typo. The first is a real command one flag away —
// `sh -dialect ksh -c 'whence ls'` runs — and the core is *right* to refuse
// it: dash has no `whence` either and says exactly what we say, so changing
// the language would be trading fidelity for helpfulness. Everywhere else
// this shell refuses an unimplemented thing by name, and this is the one
// place where the by-name convention and panel fidelity point in opposite
// directions. Fidelity wins in the language, so the affordance goes here
// (#1343).
//
// Three constraints, and each of them narrows it:
//
//   - **This binary only.** `cmd/bash` claims to *be* bash; real bash prints
//     `bash: line 1: lss: command not found` and nothing more, and a hint
//     there would be the same category of lie as accepting a flag bash
//     rejects. `cmd/sh` claims to be nothing, which is what makes it the
//     place a substrate seam gets a way in — the same reason `-highlight`
//     and `-policy` live here.
//   - **A prompt only.** A hint on standard error in a script is output
//     nobody asked for, and no real shell writes one. `interp.Runner.AtPrompt`
//     is the front end's existing answer to "is this a person", so the gate
//     is that rather than a second guess at it.
//   - **A second line only.** The first line is what a script parses and it
//     is byte-identical with or without this, which is what
//     [interp.Runner.NotFoundHint] carries rather than a wording that could
//     rewrite it.
//
// It is not behind a flag of its own. `-highlight` needs one because coloring
// is on every line of every session and no shell in the panel does it; this
// writes nothing at all until a person at a prompt types a name that is a
// builtin of another preset, which is a strictly rarer event than the flag
// would be to remember.

// hintDialects are the presets a name is looked for in, in the order the hint
// lists them. Only the ones with builtins of their own: `core` and `posix`
// register none, so a name found in neither of those is found in none of
// these either and the hint would name every preset for every miss.
var hintDialects = []struct {
	name  string
	apply func(*interp.Runner)
}{
	{"bash", bash.Apply},
	{"zsh", zsh.Apply},
	{"ksh", ksh.Apply},
	{"dash", dash.Apply},
	{"ash", ash.Apply},
}

// builtinOwners answers which presets provide a name, built once and on the
// failure path.
//
// Lazily, because it costs five runners and five registration passes to build
// and a shell that never refuses a name never needs it. The table is read
// through a func so a test can look at what it holds.
var builtinOwners = sync.OnceValue(func() map[string][]string {
	out := map[string][]string{}
	for _, d := range hintDialects {
		r := &interp.Runner{}
		d.apply(r)
		for _, name := range r.BuiltinNames() {
			out[name] = append(out[name], d.name)
		}
	}
	for name := range out {
		sort.Strings(out[name])
	}
	return out
})

// hintFor is the second line for a name, or empty where there is nothing to
// say: a typo, or a name every preset has — which cannot happen here, since a
// preset that had it would have run it.
//
// except is the preset the shell is already running as, which is left out of
// the list. Reached only when that preset did *not* provide the name, so it
// is never in the answer today; naming it anyway would be advice to switch to
// the dialect you are already in, which is the shape a later preset could
// produce.
func hintFor(name, except string) string {
	owners := builtinOwners()[name]
	var keep []string
	for _, o := range owners {
		if o != except {
			keep = append(keep, o)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	for i, o := range keep {
		keep[i] = "-dialect " + o
	}
	return "      `" + name + "` is a builtin of " + joinAnd(keep)
}

// joinAnd lists names the way a sentence does: `a`, `a and b`, `a, b and c`.
func joinAnd(names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// withDialectHint installs the hint on every runner this shell builds,
// chaining whatever registration the preset already had rather than replacing
// it — `core` and `posix` have none, and the other five do.
//
// A function of its own so a test can look at what the wiring produced.
// Registration dropped on the floor inside run() is invisible: the hint is
// written only at a prompt, so one that never arrives looks exactly like a
// person who typed a real typo.
func withDialectHint(sh driver.Shell, dialectName string) driver.Shell {
	outer := sh.Register
	sh.Register = func(r *interp.Runner) {
		if outer != nil {
			outer(r)
		}
		r.NotFoundHint = func(name string) string {
			if !r.AtPrompt {
				// Not in a script. See the note at the top of this file.
				return ""
			}
			return hintFor(name, dialectName)
		}
	}
	return sh
}
