// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strings"

	"github.com/blairham/sh/interp"
)

// `setopt` and `unsetopt` are how zsh scripts change options — far more often
// than `set -o`, which zsh also has. The two builtins are one machinery with
// the request inverted, and the names they take are zsh's own namespace:
// case-insensitive, underscores ignored, and a single `no` prefix negating
// whatever follows, so `No_Glob`, `NOGLOB` and `noglob` are one request.
// Measured: the prefix strips once and once only — `no_no_glob` is "no such
// option", not glob restored.
//
// The option table below is this shell's honest subset of zsh's (~180 names).
// Each entry is one of three kinds:
//
//   - backed by the substrate's `set -o` machinery, so `setopt err_exit` and
//     `set -o errexit` are the same switch read and written through one seam;
//   - backed by a semantics axis — `shwordsplit`, `nomatch`, `ksharrays` are
//     zsh's own names for three axes the vector already carries, and flipping
//     them is what `emulate` does too (see emulate.go);
//   - fixed: a name zsh has whose state this shell cannot change. Asking for
//     the state it is already in succeeds, the same bargain setoptions.go
//     strikes for `set +o posix`; asking it to move is refused out loud with
//     zsh's own wording for an option that will not budge — measured on
//     `setopt monitor` in a non-interactive zsh: `can't change option`, 1.
//
// A name outside the table is `no such option`, status 1, and the remaining
// operands are still acted on — measured: `setopt zzqq no_glob` complains and
// still turns globbing off, in either order.
//
// Bare `setopt` lists the options whose state differs from zsh's own default,
// spelled canonically — the base name when on, `no` plus it when off — and
// sorted by the base name, which is measured rather than guessed: zsh prints
// `noclobber` between `allexport` and `errexit`. `nohashdirs` is the one line
// a bare `-c` run shows, and this shell prints it for the honest reason: we
// never hash directories, and zsh's default is to.
//
// Bare `unsetopt` in real zsh lists every option that is off — all ~170 of
// them. Ours lists the table's, which is the same claim over the options this
// shell can speak about; docs/spec/semantics.md records the difference.

// zshOption is one name in this dialect's option namespace.
type zshOption struct {
	// base is the canonical spelling: lower case, no underscores, no `no`.
	base string
	// def is the state a zsh default run has, which is what the bare listing
	// compares against.
	def bool
	// get reads the live state.
	get func(*interp.Runner) bool
	// set moves it, returning zero or a status after its own complaint. Nil
	// marks a fixed option: the current state can be asked for and granted,
	// and anything else is refused.
	set func(*interp.Runner, bool) int
}

// zshOptions is the table, in listing order (sorted by base).
var zshOptions = []zshOption{
	// Alias expansion happens here, which is what the name asks about. Which
	// routes into the shell it happens on is the parser's question and this
	// is not it (syntax.Dialect.ExpandAliases); the state is that the shell
	// does the thing, and it is not this shell's to switch off.
	fixedConstant("aliases", true, true),
	setOptBacked("allexport", false, "allexport", false),
	// banghist is history expansion's own name; histexpand below is the
	// sh-style alias, and both read one switch.
	fixedOptBacked("banghist", false, "histexpand", false),
	fixedOptBacked("braceexpand", true, "braceexpand", false),
	setOptBacked("clobber", true, "noclobber", true),
	fixedOptBacked("emacs", true, "emacs", false),
	setOptBacked("errexit", false, "errexit", false),
	// noexec has a real apply now, one-way on under both spellings the way
	// all four shells treat it.
	setOptBacked("exec", true, "noexec", true),
	// `$0` inside a function is the function's name here, which is what the
	// name asks for and what this shell already does.
	fixedConstant("functionargzero", true, true),
	setOptBacked("glob", true, "noglob", true),
	// Command tracking is permission to cache rather than a promise to, and
	// the substrate holds the switch; off at start, which keeps the listing
	// at zsh's own `-c` baseline.
	setOptBacked("hashall", false, "hashall", false),
	// Directories are never hashed here, and zsh's default is to — the one
	// line its own `-c` baseline lists, and ours.
	fixedConstant("hashdirs", true, false),
	fixedOptBacked("histexpand", false, "histexpand", false),
	setOptBacked("histignoredups", false, "histignoredups", false),
	fixedOptBacked("ignoreeof", false, "ignoreeof", false),
	// Whether this is an interactive shell — a fact the front end brought in
	// rather than a switch, which is why it is read and not set. The
	// third-party integration this machine's startup files load opens on
	// `[[ -o interactive ]]`, and ksh93 has the same name for the same fact,
	// measured in its own `set -o` listing.
	{
		base: "interactive", def: false,
		get: func(r *interp.Runner) bool { return r.Interactive },
	},
	// Comments are honored wherever they are written; the set -o table says
	// the same, but this dialect does not declare the name there, so the
	// state is a constant rather than a read through it.
	fixedConstant("interactivecomments", true, true),
	{
		base: "ksharrays", def: false,
		get: func(r *interp.Runner) bool { return r.Semantics.ArrayBaseIsZero == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.ArrayBaseIsZero = answer(on) })
			return 0
		},
	},
	// Job control cannot be turned on in a non-interactive shell, and zsh
	// says so rather than pretending: measured, `can't change option`, 1.
	fixedConstant("monitor", false, false),
	{
		base: "nomatch", def: true,
		get: func(r *interp.Runner) bool { return r.Semantics.GlobNoMatchIsError == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.GlobNoMatchIsError = answer(on) })
			return 0
		},
	},
	// Background jobs are announced here, which is what the name means.
	fixedConstant("notify", true, true),
	fixedOptBacked("physical", false, "physical", false),
	setOptBacked("pipefail", false, "pipefail", false),
	fixedOptBacked("privileged", false, "privileged", false),
	// sh-style globbing narrows the pattern language to the standard's. This
	// shell does not narrow it, which is the state, and zsh's default is the
	// same, so the listings do not move. The prompt theme this machine loads
	// reads the name at its third line.
	fixedConstant("shglob", false, false),
	{
		base: "shwordsplit", def: false,
		get: func(r *interp.Runner) bool { return r.Semantics.SplitParamExpansion == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.SplitParamExpansion = answer(on) })
			return 0
		},
	},
	fixedOptBacked("unset", true, "nounset", true),
	setOptBacked("verbose", false, "verbose", false),
	fixedOptBacked("vi", false, "vi", false),
	setOptBacked("xtrace", false, "xtrace", false),
}

// setOptBacked binds a zsh name to a `set -o` name this shell really applies,
// inverted where zsh's base is the substrate's negation — zsh's `clobber` is
// the same switch as `set -o noclobber`, read from the other end.
func setOptBacked(base string, def bool, opt string, inv bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(r *interp.Runner) bool { on, _ := r.NamedOption(opt); return on != inv },
		set: func(r *interp.Runner, on bool) int { return r.ApplyNamedOption(opt, on != inv) },
	}
}

// fixedOptBacked reads its state through the same seam and refuses to move
// it, because the substrate does not do the thing the name asks for.
func fixedOptBacked(base string, def bool, opt string, inv bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(r *interp.Runner) bool { on, _ := r.NamedOption(opt); return on != inv },
	}
}

// fixedConstant is a name whose state here never moves and is not the
// substrate's to hold.
func fixedConstant(base string, def, state bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(*interp.Runner) bool { return state },
	}
}

// swapAxes changes semantics copy-on-write: a subshell clone shares the
// vector by pointer, so the mutation goes on a fresh copy and the swap stays
// this runner's own — which is also what makes a subshell's `setopt` stay in
// the subshell.
func swapAxes(r *interp.Runner, change func(*interp.Semantics)) {
	s := *r.Semantics
	change(&s)
	r.Semantics = &s
}

func answer(on bool) interp.Answer {
	if on {
		return interp.Yes
	}
	return interp.No
}

// normalizeOption reduces a spelling to the namespace's canonical form.
func normalizeOption(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", "")
}

// lookupZshOption resolves one normalized name to its table entry and the
// direction the spelling asked for: `noglob` is `glob`, inverted. An exact
// match wins first, which is what keeps `nomatch` an option rather than a
// negated `match`.
func lookupZshOption(name string) (zshOption, bool, bool) {
	for _, o := range zshOptions {
		if o.base == name {
			return o, false, true
		}
	}
	rest, ok := strings.CutPrefix(name, "no")
	if !ok {
		return zshOption{}, false, false
	}
	for _, o := range zshOptions {
		if o.base == rest {
			return o, true, true
		}
	}
	return zshOption{}, false, false
}

// conditionOption answers `[[ -o name ]]` out of this namespace rather than
// out of the `set -o` names, which is the whole of why the substrate takes a
// function for it: `[[ -o no_brace_expand ]]` is one question here and three
// unknown names anywhere else.
//
// It is the same lookup `setopt` does, read rather than written, so a name
// this dialect can speak about answers the same way through either — and a
// name it cannot is unknown to both, which is what lets the substrate's axis
// decide what to say about it.
func conditionOption(r *interp.Runner, name string) (on, known bool) {
	o, inverted, ok := lookupZshOption(normalizeOption(name))
	if !ok {
		return false, false
	}
	return o.get(r) != inverted, true
}

// registerSetopt installs the pair, and the namespace they share with the
// condition.
func registerSetopt(r *interp.Runner) {
	r.Register("setopt", setoptBuiltin(true))
	r.Register("unsetopt", setoptBuiltin(false))
	r.SetOptionNamespace(func(name string) (bool, bool) { return conditionOption(r, name) })
}

// setoptBuiltin builds either half; they differ in the direction a bare base
// name means and in what an empty command lists.
func setoptBuiltin(setting bool) interp.Builtin {
	return func(r *interp.Runner, _ context.Context, args []string) int {
		if len(args) == 0 {
			listZshOptions(r, setting)
			return 0
		}
		status := 0
		for _, arg := range args {
			o, inverted, ok := lookupZshOption(normalizeOption(arg))
			if !ok {
				r.Diagnosef("no such option: %s\n", arg)
				status = 1
				continue
			}
			want := setting != inverted
			if o.set != nil {
				if code := o.set(r, want); code != 0 {
					status = code
				}
				continue
			}
			if o.get(r) == want {
				// Already where it was asked to be: granted, the same
				// bargain the substrate's own option table strikes.
				continue
			}
			r.Diagnosef("can't change option: %s\n", arg)
			status = 1
		}
		return status
	}
}

// listZshOptions is the bare command. `setopt` lists what differs from zsh's
// defaults; `unsetopt` lists what is off. Both spell an option canonically —
// the base when the line says it is on, `no` plus it when off — in the order
// of the base names, which is zsh's measured order: `noclobber` prints
// between `allexport` and `errexit`. The table already holds that order.
func listZshOptions(r *interp.Runner, setting bool) {
	for _, o := range zshOptions {
		on := o.get(r)
		switch {
		case setting && on != o.def:
			_, _ = fmt.Fprintf(r.Out(), "%s\n", spellOption(o.base, on))
		case !setting && !on:
			_, _ = fmt.Fprintf(r.Out(), "%s\n", o.base)
		}
	}
}

func spellOption(base string, on bool) string {
	if on {
		return base
	}
	return "no" + base
}
