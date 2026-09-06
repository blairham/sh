// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
)

// `zmodload` loads one of this shell's binary modules.
//
// Measured 2026-09-06 against zsh 5.9.2, with a scratch HOME and no startup
// files. It was `command not found: zmodload`, which is the wrong answer for
// a reason worth stating: a real rc file's first line about modules is
//
//	zmodload zsh/zutil || { print -P "…required, aborting"; return 1; }
//
// and "no such command" is not what a script asking that can act on.
//
// **This shell cannot load a compiled module and never will**, so the
// interesting question is not how to load one but what to say about one. The
// answer here is *per module*, and it is mechanical rather than a claim:
// every zsh module is a set of named **features** — `+b:zparseopts` is a
// builtin, `+p:functions` a parameter — and a module loads here exactly when
// this shell already has every feature that module names. Nothing is
// simulated and nothing is stubbed: the features are the ones this shell
// implements anyway, under their own names, and `zmodload` only reports
// whether they are all there.
//
// That is what keeps it from becoming a silent success, which is the failure
// this builtin is most able to cause. A script told that `zsh/zutil` loaded
// and then calling `zparseopts` fails several hundred lines later, in a
// function whose caller has gone, with a message about a command nobody
// wrote. Refusing at the `zmodload` line instead names the module *and* the
// features that are missing, which is the same line a person would have to
// find anyway.
//
// It also means the gate opens by itself. `zsh/zutil` refuses today because
// three of its four builtins are missing; the day `zparseopts`, `zformat` and
// `zregexparse` are implemented, `zmodload zsh/zutil` starts succeeding with
// no change here — because the table says what the module *is*, and the shell
// answers whether it has it.
//
// The feature lists are measured, one module at a time, with
// `zmodload -lF <module>` after loading it in a real zsh. They are the
// module's contents, not this shell's opinion of them.

// zmodloadStore is the set of modules a script has loaded, kept as an
// indexed array under a name no script can reach — the way `zstyle` and
// `emulate` keep theirs, which is also what gives a subshell its own copy.
const zmodloadStore = ".zsh.zmodload"

// zmodloadAlwaysLoaded is the module a fresh shell already has.
//
// Measured: `zsh -f -c zmodload` writes exactly `zsh/main`, `zmodload -L`
// writes `zmodload zsh/main`, and `zmodload -e zsh/main` is 0 while the same
// question about `zsh/complete` or `zsh/zle` is 1. It names no features —
// `zmodload -lF zsh/main` on a loaded one is `module 'zsh/main' does not
// support features` — so the rule below loads it vacuously, which is the
// same answer arrived at rather than a special case.
const zmodloadAlwaysLoaded = "zsh/main"

// zmodloadFeatures is what each module provides, measured with
// `zmodload -lF` in zsh 5.9.2 after loading the module.
//
// Only the modules this shell has any part of, or that a real rc file asks
// for, are listed. A module absent from the table is refused the same way as
// one whose features are missing — see zmodloadLoad — because the answer a
// script can act on is the same either way: this shell will not provide it.
//
// The prefix is the kind zsh's own listing writes: `b` a builtin, `p` a
// parameter, `c` a condition, `f` a function, `a` a math function.
var zmodloadFeatures = map[string][]string{
	zmodloadAlwaysLoaded: nil,
	"zsh/zutil":          {"b:zformat", "b:zparseopts", "b:zregexparse", "b:zstyle"},
	"zsh/zle":            {"b:bindkey", "b:vared", "b:zle"},
	"zsh/complete": {
		"b:compadd", "b:compset",
		"c:after", "c:between", "c:prefix", "c:suffix",
	},
	"zsh/computil": {
		"b:comparguments", "b:compdescribe", "b:compfiles", "b:compgroups",
		"b:compquote", "b:comptags", "b:comptry", "b:compvalues",
	},
	"zsh/terminfo": {"b:echoti", "p:terminfo"},
	"zsh/termcap":  {"b:echotc", "p:termcap"},
	"zsh/system": {
		"b:syserror", "b:sysopen", "b:sysread", "b:sysseek", "b:syswrite",
		"b:zsystem", "f:systell", "p:errnos", "p:sysparams",
	},
	// The largest of them, and the one a plugin manager leans on hardest:
	// 34 parameters and not one builtin.
	"zsh/parameter": {
		"p:aliases", "p:builtins", "p:commands", "p:dirstack",
		"p:dis_aliases", "p:dis_builtins", "p:dis_functions",
		"p:dis_functions_source", "p:dis_galiases", "p:dis_patchars",
		"p:dis_reswords", "p:dis_saliases", "p:funcfiletrace",
		"p:funcsourcetrace", "p:funcstack", "p:functions",
		"p:functions_source", "p:functrace", "p:galiases", "p:history",
		"p:historywords", "p:jobdirs", "p:jobstates", "p:jobtexts",
		"p:modules", "p:nameddirs", "p:options", "p:parameters",
		"p:patchars", "p:reswords", "p:saliases", "p:userdirs",
		"p:usergroups",
	},
}

// zmodloadHasFeature reports whether this shell already provides one feature.
//
// A builtin and a parameter are asked of the runner, so the answer moves when
// the shell does rather than when somebody remembers to edit a list. The
// other three kinds — a condition, a function and a math function — have no
// registry to ask, so they count as missing and are named as missing. That is
// the honest answer today and it is not a guess in the wrong direction: a
// module counted as loaded on the strength of a feature nobody can find is
// exactly the silent success this builtin exists to avoid.
func zmodloadHasFeature(r *interp.Runner, feature string) bool {
	kind, name, ok := strings.Cut(feature, ":")
	if !ok {
		return false
	}
	switch kind {
	case "b":
		return r.KnownBuiltin(name)
	case "p":
		return r.DynamicParameter(name)
	}
	return false
}

// zmodloadMissing is the features of a module this shell does not have, in
// the order the table names them — which is the order zsh's own `-lF` listing
// writes, so a refusal reads against that listing.
func zmodloadMissing(r *interp.Runner, module string) []string {
	var missing []string
	for _, f := range zmodloadFeatures[module] {
		if !zmodloadHasFeature(r, f) {
			_, name, _ := strings.Cut(f, ":")
			missing = append(missing, name)
		}
	}
	return missing
}

// zmodloadLoaded is the modules loaded now, sorted — which is the order
// zsh's listing is in.
//
// A store that was never written means the shell as it started, which is
// `zsh/main` alone. An *empty* store is not the same thing and must not read
// as the default: `zmodload -u zsh/main` succeeds and empties the listing in
// zsh, so the emptied set has to survive being written.
func zmodloadLoaded(r *interp.Runner) []string {
	stored, ok := r.GetArray(zmodloadStore)
	if !ok {
		return []string{zmodloadAlwaysLoaded}
	}
	out := append([]string(nil), stored...)
	// Sorted because that is the order zsh's listing is in, and *not*
	// because a test can see it: only a module with no features loads in
	// this shell and there is exactly one of those, so no script can get two
	// names into this list. Kept rather than dropped so the order is right
	// the day a second module can load — see the surviving mutant noted in
	// the pull request rather than left for somebody to rediscover.
	sort.Strings(out)
	return out
}

// zmodloadSetLoaded records that a module is loaded, or that it is not.
func zmodloadSetLoaded(r *interp.Runner, module string, loaded bool) {
	kept := make([]string, 0, len(zmodloadLoaded(r))+1)
	for _, m := range zmodloadLoaded(r) {
		if m != module {
			kept = append(kept, m)
		}
	}
	if loaded {
		kept = append(kept, module)
	}
	r.SetArray(zmodloadStore, kept)
}

// zmodloadOpts is what the letters asked for.
type zmodloadOpts struct {
	exists   bool // -e: ask rather than load
	unload   bool // -u
	commands bool // -L: the listing as the commands that would make it
	features bool // -F: act on features rather than on the module
	list     bool // -l: list them, only allowed with -F
	silent   bool // -s: no complaint about a module that will not load
	quietIf  bool // -i: no complaint about one already loaded
}

// zmodloadLetters are the letters implemented here, and
// zmodloadUnimplemented the ones zsh has and this shell does not — autoloaded
// builtins and conditions (`-a` with `-b`, `-c`, `-f`, `-p`), module aliases
// (`-A`, `-R`), the dependency table (`-d`), pattern arguments (`-m`), and
// `-I` and `-P`. Each is named as missing rather than as unknown, so a script
// can tell a shell that lacks one from a typo.
//
// Anything outside both sets is `bad option: -q` and 1, measured against
// twenty-two letters zsh does not have.
const (
	zmodloadLetters       = "eulLFsi"
	zmodloadUnimplemented = "aAbcdfImpPR"
)

func registerZmodload(r *interp.Runner) {
	r.Register("zmodload", zmodloadBuiltin)
}

func zmodloadBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := zmodloadOptions(r, args)
	if code != 0 {
		return code
	}
	switch {
	case opts.list && !opts.features:
		// Measured: `-l` alone is refused rather than treated as a listing.
		r.Diagnosef("-l is only allowed with -F\n")
		return 1
	case opts.features:
		return zmodloadFeatureCommand(r, opts, rest)
	case opts.exists:
		return zmodloadExists(r, rest)
	case opts.unload:
		return zmodloadUnload(r, rest)
	case len(rest) == 0:
		return zmodloadListing(r, opts.commands)
	case opts.commands:
		// `-L` with modules named writes nothing at all — measured, and for
		// a loaded module as much as for one that is not: with an operand
		// the letter is asking what a module *autoloads*, which is `-a`'s
		// table and refused by name here. Status 0 either way.
		return 0
	}
	// Every module is attempted, and one that will not load does not stop
	// the ones after it: measured, `zmodload zsh/zutil zsh/nosuch
	// zsh/parameter` complains once and leaves `zsh/parameter` loaded, with
	// status 1 for the one that failed.
	status := 0
	for _, m := range rest {
		if !zmodloadLoad(r, opts, m) {
			status = 1
		}
	}
	return status
}

// zmodloadOptions reads the leading option words.
func zmodloadOptions(r *interp.Runner, args []string) (opts zmodloadOpts, rest []string, code int) {
	rest = args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			letter := word[i]
			switch {
			case strings.IndexByte(zmodloadLetters, letter) >= 0:
				setZmodloadLetter(&opts, letter)
			case strings.IndexByte(zmodloadUnimplemented, letter) >= 0:
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return opts, nil, 1
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return opts, nil, 1
			}
		}
	}
	return opts, rest, 0
}

func setZmodloadLetter(opts *zmodloadOpts, letter byte) {
	switch letter {
	case 'e':
		opts.exists = true
	case 'u':
		opts.unload = true
	case 'l':
		opts.list = true
	case 'L':
		opts.commands = true
	case 'F':
		opts.features = true
	case 's':
		opts.silent = true
	case 'i':
		opts.quietIf = true
	}
}

// zmodloadLoad is one module, and the whole of the judgement this builtin
// makes: it is loaded when this shell has every feature the module names, and
// refused by the names of the ones it does not.
//
// A module already loaded is loaded again without a word and without work,
// measured — and `-i` is therefore about a complaint this shell does not make
// rather than about the status, which is why the letter is accepted and
// changes nothing here.
func zmodloadLoad(r *interp.Runner, opts zmodloadOpts, module string) bool {
	if containsWord(zmodloadLoaded(r), module) {
		return true
	}
	if _, known := zmodloadFeatures[module]; !known {
		// A module this shell has no part of. Not worded as though the
		// module did not exist — most of the ones that reach here are real
		// zsh modules — but as what it is here: not implemented.
		if !opts.silent {
			r.DiagnoseAsTheShellf("failed to load module `%s': not implemented yet\n", module)
		}
		return false
	}
	features := zmodloadFeatures[module]
	if missing := zmodloadMissing(r, module); len(missing) > 0 {
		if !opts.silent {
			r.DiagnoseAsTheShellf("failed to load module `%s': %s\n",
				module, zmodloadShortfall(missing, len(features)))
		}
		return false
	}
	zmodloadSetLoaded(r, module, true)
	return true
}

// zmodloadUnload is `-u`: forget that a module was loaded.
//
// **`no such module` means "not loaded", not "no such name"** — measured, and
// it is the one thing here that a first reading got wrong. `zmodload -u
// zsh/mathfunc` is `no such module zsh/mathfunc` and 1 for a module zsh
// certainly ships, and the same line after the module has been loaded is
// silence and 0. So `-u` says nothing about whether a name exists, and a
// list of the modules zsh ships — which the first version of this file
// carried, to tell "absent here" from "no such thing anywhere" — turned out
// to answer no question this builtin asks. It is gone.
//
// Nothing is unloaded in the sense zsh means it: the features this shell has
// are the ones it has, and `-u` only takes the module out of the listing.
// Said plainly rather than refused, because a script's `zmodload -u` is
// tidying up after itself and has nothing to act on either way.
func zmodloadUnload(r *interp.Runner, modules []string) int {
	status := 0
	for _, m := range modules {
		if !containsWord(zmodloadLoaded(r), m) {
			r.Diagnosef("no such module %s\n", m)
			status = 1
			continue
		}
		zmodloadSetLoaded(r, m, false)
	}
	return status
}

// zmodloadExists is `-e`: ask whether the modules are loaded and say nothing.
// Every one of them must be, measured — `zmodload -e zsh/zutil zsh/nosuch`
// is 1 with `zsh/zutil` loaded.
func zmodloadExists(r *interp.Runner, modules []string) int {
	loaded := zmodloadLoaded(r)
	for _, m := range modules {
		if !containsWord(loaded, m) {
			return 1
		}
	}
	return 0
}

// zmodloadListing is the bare form and `-L`: the loaded modules, as names or
// as the commands that would load them.
func zmodloadListing(r *interp.Runner, commands bool) int {
	for _, m := range zmodloadLoaded(r) {
		if commands {
			zmodloadPrintf(r, "zmodload %s\n", m)
			continue
		}
		zmodloadPrintf(r, "%s\n", m)
	}
	return 0
}

// zmodloadFeatureCommand is `-F`. Only the `-lF` listing is built: it is the
// one form that answers a question rather than changing what is loaded, and
// it is how a script finds out what a module would have brought.
//
// A module that is not loaded has no features to list — measured, `module
// 'zsh/zutil' is not yet loaded` and 1, with the builtin named in the
// location, which is where this shell puts it. `zsh/main` reaches that same
// refusal in zsh for a different stated reason; here it is refused for having
// no features to list, which is the same answer and one rule.
func zmodloadFeatureCommand(r *interp.Runner, opts zmodloadOpts, args []string) int {
	if len(args) == 0 {
		// The operand is required by the letter and not by the builtin: a
		// bare `zmodload` with no letters at all is a listing and 0. Asked
		// before the refusal below so that the missing operand is what a
		// bare `-F` is told about, which is what zsh says too.
		r.Diagnosef("-F requires a module name\n")
		return 1
	}
	if !opts.list {
		// Turning a feature on or off one at a time is what this shell has
		// no way to do: a feature here is a builtin or a parameter the shell
		// either has or has not, and `+zparseopts` cannot conjure one.
		r.Diagnosef("-F without -l is not implemented yet\n")
		return 1
	}
	status := 0
	for _, m := range args {
		features := zmodloadFeatures[m]
		switch {
		case !containsWord(zmodloadLoaded(r), m):
			r.Diagnosef("module `%s' is not yet loaded\n", m)
			status = 1
		case len(features) == 0:
			// Loaded and with nothing to list, which is `zsh/main` and its
			// own measured sentence rather than the one above.
			r.Diagnosef("module `%s' does not support features\n", m)
			status = 1
		default:
			for _, f := range features {
				zmodloadPrintf(r, "+%s\n", f)
			}
		}
	}
	return status
}

func zmodloadPrintf(r *interp.Runner, format string, args ...any) {
	_, _ = fmt.Fprintf(r.Out(), format, args...)
}

// zmodloadShortfall says what a module is short of, and it is two sentences
// rather than one because a module can be short of three features or of
// thirty-three.
//
// Naming them is the honest-refusal convention and it is what a person can
// act on — `zparseopts` and two others is a to-do list. Thirty-three names on
// one line is not: `zsh/parameter` names thirty-three features, all of them
// parameters, and none of them are here. So beyond a handful the count speaks
// instead, and it still says the two numbers that matter — how much is
// missing and how much there is — which is the difference between a module
// this shell has part of and one it has none of.
func zmodloadShortfall(missing []string, total int) string {
	if len(missing) <= zmodloadNamedShortfall {
		return joinNames(missing) + " " + isOrAre(len(missing)) + " not implemented yet"
	}
	return fmt.Sprintf("%d of its %d features are not implemented yet", len(missing), total)
}

// zmodloadNamedShortfall is how many missing features are named one by one
// before the count speaks for them.
//
// Six, measured against the table: every module this shell has *part* of
// names four or fewer — `zsh/zutil` four, `zsh/zle` three, `zsh/terminfo`
// and `zsh/termcap` two — so a module with a gap is always named in full,
// and only the wholly absent large ones (`zsh/parameter` at thirty-three,
// `zsh/system` at nine, `zsh/computil` at eight) reach the count.
const zmodloadNamedShortfall = 6

// joinNames writes a list of names the way a sentence does: commas between
// all but the last two and `and` before the last, so a refusal naming three
// missing builtins reads as a sentence rather than as a slice.
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// isOrAre agrees the verb with the list joinNames wrote.
func isOrAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
