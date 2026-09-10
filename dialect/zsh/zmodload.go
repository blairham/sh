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
// **A missing feature holds a module shut when nothing would tell a script it
// was missing.** That is the rule, and it is a refinement of the one this
// file was written with (#1058, #1146). The first version said the kind
// decided it — a builtin never held, a parameter always did — and the
// reasoning was right while the wording was not, because what it was reaching
// for was never *presence*. It was **legibility**.
//
// A missing *builtin* fails loudly, by name, at its own call site:
// `command not found: zregexparse`, on the line that wrote it. Nothing is
// silently wrong in between, so the module refusing as well buys a script
// nothing and costs it everything — `zmodload zsh/zutil || return 1` is the
// first line of a real plugin manager, and holding it shut over a builtin
// that file never calls stops the file for a feature it does not use. Counted
// on this machine: `zi.zsh` names `zparseopts` and `zformat` five times
// between them and `zregexparse` not once.
//
// A missing *parameter* had no such call site, and that was the whole of the
// difference: `${#functions}` on a shell without `$functions` is `0` at status
// 0, a plausible answer to a different question, reaching the caller as data
// rather than as a diagnostic. So a module short of one refused.
//
// **A parameter can have a call site too**, and now does — see
// [interp.Runner.SetAbsentParameter]. A parameter this shell has not got is
// registered as absent rather than left undefined, and reading it is
// `jobstates: parameter not implemented yet` at the expansion that asked,
// with the command not run. That is the same three things `command not found`
// does, so the same answer follows: the module loads, and the script finds out
// at the line that depends on it.
//
// Which leaves the rule as one sentence about what a script is told rather
// than two about kinds of feature: **a module loads when everything it names
// is either implemented or refuses by name on access, and nothing it names
// may read as empty when it is absent.** Applied to `zsh/parameter`, five of
// the thirty-three are implemented, ten are empty and right to be, eighteen
// refuse — and the module loads. Applied to `zsh/zutil` it is the answer that
// file already gave.
//
// The other three kinds — a condition, a function and a math function — have
// no registry to ask and no call site to refuse at, so they still hold: a
// module counted as loaded on the strength of a feature nobody can find is
// the silent success this builtin exists to avoid.
//
// It also means the gate opens by itself. The day a module's parameters
// exist, `zmodload` starts succeeding for it with no change here — because
// the table says what the module *is*, and the shell answers whether it has
// it.
//
// The feature lists are measured, one module at a time, with
// `zmodload -lF <module>` after loading it in a real zsh. They are the
// module's contents, not this shell's opinion of them.
//
// **`-F` asks the same question of a subset** (#1619). A caller writing
// `zmodload -F zsh/complete b:compadd` has named the one feature of six it
// wants, so the other five have no bearing on the answer — and the verdict
// moves with the question, which is what makes the letter worth having here
// rather than an unimplementable one. Which features a module is left showing
// is state, kept beside the loaded set and reported by `-lF`; zmodloadSelect
// is the whole of the rule and zmodloadFeatureStore the whole of the state.

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
	// Both of `zsh/sched`'s features are here, which makes it the second
	// module after `zsh/datetime` that loads because everything it names is
	// implemented rather than because a rule forgave it. See sched.go.
	"zsh/sched": {"b:sched", "p:zsh_scheduled_events"},
	"zsh/complete": {
		"b:compadd", "b:compset",
		"c:after", "c:between", "c:prefix", "c:suffix",
	},
	"zsh/computil": {
		"b:comparguments", "b:compdescribe", "b:compfiles", "b:compgroups",
		"b:compquote", "b:comptags", "b:comptry", "b:compvalues",
	},
	// All four implemented, which is the only module here that can say so:
	// three clock reads and a formatter, none of which needs a seam this
	// shell has not got. See datetime.go.
	"zsh/datetime": {"b:strftime", "p:EPOCHSECONDS", "p:EPOCHREALTIME", "p:epochtime"},
	"zsh/terminfo": {"b:echoti", "p:terminfo"},
	"zsh/termcap":  {"b:echotc", "p:termcap"},
	"zsh/system": {
		"b:syserror", "b:sysopen", "b:sysread", "b:sysseek", "b:syswrite",
		"b:zsystem", "f:systell", "p:errnos", "p:sysparams",
	},
	// The largest of them, and the one a plugin manager leans on hardest:
	// 33 parameters and not one builtin. Measured 2026-09-07 as
	// `zmodload -lF zsh/parameter` against zsh 5.9.2, and held name for name
	// by TestTheThreeRostersAccountForTheModuleExactlyOnce.
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
// the shell does rather than when somebody remembers to edit a list. The other
// three kinds — a condition, a function and a math function — have no registry
// to ask, so they count as missing: a module counted as loaded on the strength
// of a feature nobody can find is the silent success this builtin exists to
// avoid.
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

// zmodloadHolds reports whether a feature's absence should keep its module
// from loading.
//
// One question, asked of the feature rather than of the shell's progress:
// **would a script that depended on this be told?** A builtin is told by
// `command not found` at the word that ran it, so a missing one never holds —
// however many of its neighbors exist. A parameter is told when it has been
// registered as absent, and is not when it has not: an unregistered one reads
// `0` at status 0 and holds its module shut, which is the case the rule was
// written for and the only one left in it.
//
// The runner is asked rather than a list consulted, so the gate opens by
// itself the day a dialect names one — the same property the implemented half
// has through zmodloadHasFeature.
func zmodloadHolds(r *interp.Runner, feature string) bool {
	kind, name, ok := strings.Cut(feature, ":")
	if !ok {
		return true
	}
	switch kind {
	case "b":
		return false
	case "p":
		return !r.AbsentParameter(name)
	}
	return true
}

// zmodloadMissing is which of the features handed to it this shell does not
// have *and will not load without*, in the order they were given — which is
// the order the table names them and therefore the order zsh's own `-lF`
// listing writes, so a refusal reads against that listing.
//
// A list rather than a module name, because `-F` asks the same question of a
// *subset*: `zmodload -F zsh/complete b:compadd` is a script naming the one
// feature it wants, and the five it did not name have no bearing on whether
// it gets it. See zmodloadSelect.
//
// A builtin the shell has not got is absent from this list on purpose and is
// still absent from the shell: `zmodload zsh/zutil` succeeds and
// `zregexparse` is `command not found` on the line that calls it. The two
// statements are consistent because the second one is the loud half — see
// zmodloadHolds. A parameter registered as absent is out of this list for the
// same reason and by the same test, and `$jobstates` refuses at the expansion
// that reads it.
func zmodloadMissing(r *interp.Runner, features []string) []string {
	var missing []string
	for _, f := range features {
		if zmodloadHolds(r, f) && !zmodloadHasFeature(r, f) {
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
	// Sorted because that is the order zsh's listing is in. This was once
	// unobservable — only a module with no features loaded, and there is
	// exactly one of those — and it is observable now: `zsh/datetime` has
	// all four of its features here, so `zmodload zsh/datetime; zmodload`
	// writes two names and the order is the shell's answer rather than a
	// map's (#1154).
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

// zmodloadFeatureStore is which of a module's features are switched on, for
// the modules `-F` has narrowed. An association from the module's name to its
// enabled features, joined by spaces, under a name no script can reach — the
// same idiom as zmodloadStore above and cloned into a subshell for the same
// reason.
//
// **A module absent from this table has all its features on**, which is what
// a plain `zmodload zsh/zutil` leaves behind and is not the same as an entry
// holding the empty string. `zmodload -F zsh/zutil` with no features named
// loads the module with every one of them *off* — measured, and it is the
// line `_fzf_completion` writes as `zmodload -F zsh/compctl` — so the empty
// selection has to survive being written, exactly as the emptied module
// listing does.
const zmodloadFeatureStore = ".zsh.zmodload.features"

// zmodloadEnabled is the features of a module that are on now.
//
// Three answers, and which of them applies is the whole of what makes `-F` a
// selection in one case and a delta in the other.
//
// A module that is **not loaded** has nothing on, so the first `-F` at it
// starts from silence and turns on only what it names. That answer is also
// what makes an unload need no cleanup of its own: an entry left in the store
// for a module nobody has loaded cannot be reached, because every reader is
// this function. A module **loaded whole** has everything on, so a later `-F`
// moves only the features it names and leaves the rest — measured, `zmodload
// zsh/zutil; zmodload -F zsh/zutil -b:zparseopts` leaves the other three
// standing. A module **`-F` has already narrowed** has whatever it was left
// with.
func zmodloadEnabled(r *interp.Runner, module string) []string {
	if !containsWord(zmodloadLoaded(r), module) {
		return nil
	}
	if selected, ok := r.GetAssoc(zmodloadFeatureStore); ok {
		if list, narrowed := selected[module]; narrowed {
			return strings.Fields(list)
		}
	}
	return zmodloadFeatures[module]
}

// zmodloadNarrow records which of a module's features are on.
func zmodloadNarrow(r *interp.Runner, module string, features []string) {
	selected, _ := r.GetAssoc(zmodloadFeatureStore)
	if selected == nil {
		selected = map[string]string{}
	}
	selected[module] = strings.Join(features, " ")
	r.SetAssoc(zmodloadFeatureStore, selected)
}

// zmodloadWiden puts a module back to all-features-on, which is what a plain
// load leaves behind.
//
// Measured: `zmodload -F zsh/zutil +b:zstyle; zmodload zsh/zutil; zmodload -lF
// zsh/zutil` writes `+` against all four, so a whole load is not a no-op on a
// module that is already loaded narrowed. An unload does not call this — see
// zmodloadUnload for why one rule is enough.
func zmodloadWiden(r *interp.Runner, module string) {
	selected, ok := r.GetAssoc(zmodloadFeatureStore)
	if !ok {
		return
	}
	delete(selected, module)
	r.SetAssoc(zmodloadFeatureStore, selected)
}

// zmodloadSpec reads one `[+-]feature` operand: the feature it names and
// whether it is being switched on.
//
// A bare name means `+` — measured, `zmodload -F zsh/zutil b:zstyle` is the
// spelling every real caller uses and it switches the feature on. The sign is
// stripped before the name is looked up, which is also what puts `-` in the
// complaint about `zmodload -F zsh/zutil -- b:zstyle`: `--` after the module
// name is an operand like any other, and what is left of it is one character
// that names no feature.
func zmodloadSpec(word string) (feature string, on bool) {
	switch {
	case strings.HasPrefix(word, "+"):
		return word[1:], true
	case strings.HasPrefix(word, "-"):
		return word[1:], false
	}
	return word, true
}

// zmodloadCheckSpecs reports the first operand that names no feature of the
// module, having stripped its sign.
//
// **Every operand is checked before any is applied**, which is measured and
// is the half a first reading would get wrong: `zmodload -F zsh/zutil
// +b:zstyle +b:nosuch` complains about the second and leaves the module
// *unloaded*, so the good spec in front of the bad one buys nothing. A shell
// that applied as it went would load the module and enable `zstyle`.
func zmodloadCheckSpecs(module string, specs []string) (bad string, ok bool) {
	features := zmodloadFeatures[module]
	for _, word := range specs {
		feature, _ := zmodloadSpec(word)
		if !containsWord(features, feature) {
			return feature, false
		}
	}
	return "", true
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
	case opts.unload && opts.features:
		// Measured, and worded as zsh words it: the letters it names are all
		// letters zsh has, and the sentence is about the combination rather
		// than about any one of them. The other four reach the refusal for
		// an unimplemented letter first, so `-u` is the one that gets here.
		r.Diagnosef("-b, -c, -f, -p and -u cannot be combined with -F\n")
		return 1
	case opts.exists && opts.features:
		// `-eF` is still the question `-e` asks, and it asks it of the module
		// alone: measured, `zmodload -eF zsh/zutil b:zstyle` is 0 once the
		// module is loaded, whether or not `zstyle` is one of the features it
		// was left showing. So the operands after the module are along for
		// the ride — and the module itself is required, the way it is for
		// `-lF` and unlike `-LF`.
		if len(rest) == 0 {
			r.Diagnosef("-F requires a module name\n")
			return 1
		}
		return zmodloadExists(r, rest[:1])
	case opts.exists:
		return zmodloadExists(r, rest)
	case opts.features:
		return zmodloadFeatureCommand(r, opts, rest)
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
// A module already loaded is loaded again without a word, measured — and `-i`
// is therefore about a complaint this shell does not make rather than about
// the status, which is why the letter is accepted and changes nothing here.
// It is not *without work*, which is what a first reading had: a whole load
// puts back every feature `-F` had switched off, so it is the one command
// that widens a narrowed module. See zmodloadWiden.
func zmodloadLoad(r *interp.Runner, opts zmodloadOpts, module string) bool {
	features, known := zmodloadFeatures[module]
	if !known {
		// A module this shell has no part of. Not worded as though the
		// module did not exist — most of the ones that reach here are real
		// zsh modules — but as what it is here: not implemented.
		if !opts.silent {
			r.DiagnoseAsTheShellf("failed to load module `%s': not implemented yet\n", module)
		}
		return false
	}
	if missing := zmodloadMissing(r, features); len(missing) > 0 {
		if !opts.silent {
			r.DiagnoseAsTheShellf("failed to load module `%s': %s\n",
				module, zmodloadShortfall(missing, len(features)))
		}
		return false
	}
	zmodloadWiden(r, module)
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
		// The narrowing is deliberately *not* cleared here. It would be a
		// second rule saying what zmodloadEnabled's first line already says
		// — a module that is not loaded has nothing on — and the entry left
		// behind cannot be read by anything, because every reader goes
		// through that function. Clearing it as well passed every test with
		// the clearing removed, which is what a redundant rule looks like:
		// one notion of whose features these are, not two that can drift.
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

// zmodloadFeatureCommand is `-F`, and its three forms are three different
// commands sharing one letter: `-lF` lists a module's features, `-LF` writes
// the listing as the command that would reproduce it, and `-F` on its own
// **selects** which of them the module exposes.
//
// The third is what a plugin manager writes and what this builtin refused
// outright until #1619 — `zmodload -F zsh/stat b:zstat || return` and
// `zmodload -F zsh/parameter p:functions` are lines from two real ones, and
// `-F without -l is not implemented yet` is not an answer either can act on.
//
// **`-F` narrows the question rather than conjuring a feature.** A feature
// here is a builtin or a parameter this shell either has or has not, and
// `+zparseopts` cannot make one — which is why the letter looked
// unimplementable. But that is not what a caller writes it for: `zmodload -F
// zsh/complete b:compadd` is a script naming the one feature of six that it
// wants, and the answer to *that* is a question this shell can answer.
// Narrowing therefore moves the verdict, and moves it in the direction a
// script needs — `zmodload zsh/complete` is refused here for four conditions
// nobody can find, and the narrowed form does not have to be.
//
// The one thing it does not do is take a feature *away*. A module narrowed to
// one builtin leaves the other three of `zsh/zutil` callable here, where zsh
// removes them from its table outright — measured, `zmodload zsh/zutil;
// zmodload -F zsh/zutil -b:zparseopts; zparseopts` is `command not found` in
// zsh. Nothing in this shell provides those builtins *through* the module —
// they are the dialect's, registered before any script runs — so withdrawing
// one would take away a command the line before `zmodload` could already run.
// The selection is recorded and reported truthfully by `-lF`, which is what a
// script reads, and #1635 is the divergence written down where it can be
// argued with rather than only here.
func zmodloadFeatureCommand(r *interp.Runner, opts zmodloadOpts, args []string) int {
	if opts.commands && len(args) == 0 {
		// `-LF` alone is the whole shell's selection, one line per loaded
		// module that has features — measured, and the one form of `-F` that
		// does not want a module name. `-lF` alone is refused, which is the
		// asymmetry below and is also measured.
		for _, m := range zmodloadLoaded(r) {
			if len(zmodloadFeatures[m]) == 0 {
				continue
			}
			zmodloadFeatureListing(r, opts, m, nil)
		}
		return 0
	}
	if len(args) == 0 {
		// The operand is required by the letter and not by the builtin: a
		// bare `zmodload` with no letters at all is a listing and 0. Asked
		// before everything below so that the missing operand is what a bare
		// `-F` is told about, which is what zsh says too, and `-lF` and `-eF`
		// with nothing after them say the same. `-LF` is the exception and
		// is answered above.
		r.Diagnosef("-F requires a module name\n")
		return 1
	}
	module, specs := args[0], args[1:]
	if opts.list || opts.commands {
		return zmodloadFeatureListing(r, opts, module, specs)
	}
	return zmodloadSelect(r, opts, module, specs)
}

// zmodloadSelect is `-F` without a listing letter: load the module with the
// features named switched on, and the ones not named left as they were.
//
// The starting point is the whole of what makes this a selection rather than
// a set of deltas, and it depends on one thing — whether the module is loaded
// already. It is not, so `zmodload -F zsh/zutil b:zstyle` starts from nothing
// on and finishes with `zstyle` alone; it is, so `zmodload zsh/zutil;
// zmodload -F zsh/zutil -b:zparseopts` starts from all four and finishes with
// three. Both measured, and zmodloadEnabled is where the two answers live.
//
// Nothing is applied until every operand has been checked, which is measured
// and is the difference between a refusal and a half-loaded module.
func zmodloadSelect(r *interp.Runner, opts zmodloadOpts, module string, specs []string) int {
	features, known := zmodloadFeatures[module]
	switch {
	case !known:
		// The same sentence a plain load gives, because it is the same fact
		// about the same module: this shell has no part of it. `-s` silences
		// it there and here.
		if !opts.silent {
			r.DiagnoseAsTheShellf("failed to load module `%s': not implemented yet\n", module)
		}
		return 1
	case len(features) == 0:
		// `zsh/main`, which is loaded and has nothing to select from.
		// Measured as the shell speaking rather than the builtin, which is
		// the other way round from the `-lF` listing's identical sentence —
		// so the two paths keep their own locations.
		r.DiagnoseAsTheShellf("module `%s' does not support features\n", module)
		return 1
	}
	if bad, ok := zmodloadCheckSpecs(module, specs); !ok {
		r.DiagnoseAsTheShellf("module `%s' has no such feature: `%s'\n", module, bad)
		return 1
	}
	on := make(map[string]bool, len(features))
	for _, f := range zmodloadEnabled(r, module) {
		on[f] = true
	}
	for _, word := range specs {
		feature, enable := zmodloadSpec(word)
		// Last spec wins: measured, `+b:zstyle -b:zstyle` leaves it off.
		on[feature] = enable
	}
	// In the table's order, which is the order `-lF` writes and the order
	// `-LF` replays, so a selection reads against the listing it produces.
	selected := make([]string, 0, len(features))
	for _, f := range features {
		if on[f] {
			selected = append(selected, f)
		}
	}
	if missing := zmodloadMissing(r, selected); len(missing) > 0 {
		if !opts.silent {
			r.DiagnoseAsTheShellf("failed to load module `%s': %s\n",
				module, zmodloadShortfall(missing, len(selected)))
		}
		return 1
	}
	zmodloadNarrow(r, module, selected)
	zmodloadSetLoaded(r, module, true)
	return 0
}

// zmodloadFeatureListing is `-lF` and `-LF`: what a module exposes now, as a
// list of features or as the command that would reproduce it.
//
// A module that is not loaded has no features to list — measured, `module
// 'zsh/zutil' is not yet loaded` and 1, with the builtin named in the
// location, which is where this shell puts it. `zsh/main` reaches that same
// refusal in zsh for a different stated reason; here it is refused for having
// no features to list, which is the same answer and one rule.
//
// The operands after the module are a filter, and they are compared to the
// feature names *as written*. So a bare `b:zstyle` narrows the listing to one
// line and a signed `+b:zstyle` narrows it to none, which is measured and
// looks like a slip until the two rules are separated: the sign is stripped to
// decide whether the operand names a feature at all, and is not stripped again
// to decide which lines it matches.
func zmodloadFeatureListing(r *interp.Runner, opts zmodloadOpts, module string, filters []string) int {
	features := zmodloadFeatures[module]
	switch {
	case !containsWord(zmodloadLoaded(r), module):
		r.Diagnosef("module `%s' is not yet loaded\n", module)
		return 1
	case len(features) == 0:
		// Loaded and with nothing to list, which is `zsh/main` and its
		// own measured sentence rather than the one above.
		r.Diagnosef("module `%s' does not support features\n", module)
		return 1
	}
	if bad, ok := zmodloadCheckSpecs(module, filters); !ok {
		r.Diagnosef("module `%s' has no such feature: `%s'\n", module, bad)
		return 1
	}
	on := make(map[string]bool, len(features))
	for _, f := range zmodloadEnabled(r, module) {
		on[f] = true
	}
	if opts.commands {
		line := "zmodload -F " + module
		for _, f := range features {
			if on[f] && (len(filters) == 0 || containsWord(filters, f)) {
				line += " " + f
			}
		}
		// With a newline, which zsh writes only when the last feature in the
		// module's list happens to be one of the enabled ones — measured
		// byte for byte, and a line that ends in a space and stops is not a
		// behavior to reproduce. It is the one place here that answers what
		// zsh meant rather than what it did.
		zmodloadPrintf(r, "%s\n", line)
		return 0
	}
	for _, f := range features {
		if len(filters) > 0 && !containsWord(filters, f) {
			continue
		}
		if on[f] {
			zmodloadPrintf(r, "+%s\n", f)
			continue
		}
		zmodloadPrintf(r, "-%s\n", f)
	}
	return 0
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
// and `zsh/termcap` two — so a module with a gap is always named in full.
//
// **Nothing in the table reaches the count today**, and that is worth writing
// down rather than deleting the branch over. `zsh/parameter` was the one that
// did — twenty-eight missing parameters on one line is not something anyone
// can read — and since #1146 those twenty-eight refuse by name instead, so it
// loads and is short of nothing. The rule it settles is still the right one
// for the next large module, and dropping it would mean rediscovering that a
// thirty-name refusal is unreadable. A mutant that lowers this number
// survives; that is the shape of a rule with nothing left to apply it to, and
// it is noted rather than papered over — the same call zmodloadLoaded's sort
// makes a few lines up.
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
