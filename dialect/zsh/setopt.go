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

// `setopt` and `unsetopt` are how zsh scripts change options — far more often
// than `set -o`, which zsh also has. The two builtins are one machinery with
// the request inverted, and the names they take are zsh's own namespace:
// case-insensitive, underscores ignored, and a single `no` prefix negating
// whatever follows, so `No_Glob`, `NOGLOB` and `noglob` are one request.
// Measured: the prefix strips once and once only — `no_no_glob` is "no such
// option", not glob restored.
//
// # The name set
//
// zsh 5.9.2 has 185 options and twelve further spellings borrowed from sh and
// ksh, and all 197 are in the table below. The set was derived by probing the
// binary rather than read anywhere: `set -o` lists every option in the
// spelling that is off by default, `zmodload zsh/parameter` then exposes
// `$options` whose keys are the canonical names and whose values are the live
// states, and the twelve extras were each identified by flipping them one at
// a time and reading which canonical entry moved with it.
//
// The default recorded for each name is that same measured state in a
// non-interactive `zsh -c` with an empty HOME, with one correction that a
// bare `setopt` reports itself: `hashdirs` is the single option whose
// compiled-in default differs from its state in such a shell, which is why
// `nohashdirs` is the one line a bare `setopt` prints there.
//
// # Recognized, recorded, and implemented are three different claims
//
// Every entry is one of four kinds, and the difference is the honest part of
// this file:
//
//   - backed by the substrate's `set -o` machinery, so `setopt err_exit` and
//     `set -o errexit` are the same switch read and written through one seam;
//   - backed by a semantics axis or a match option — `shwordsplit`, `nomatch`,
//     `ksharrays` are zsh's own names for three axes the vector already
//     carries, and `nullglob`, `globdots` and `caseglob` are zsh's names for
//     three the pattern matcher carries. Flipping the first three is what
//     `emulate` does too (see emulate.go);
//   - fixed: a name zsh has whose state this shell cannot change. Asking for
//     the state it is already in succeeds, the same bargain setoptions.go
//     strikes for `set +o posix`; asking it to move is refused out loud with
//     zsh's own wording for an option that will not budge — measured on
//     `setopt monitor` in a non-interactive zsh: `can't change option`, 1.
//     Real zsh refuses exactly five of the 185 that way in a `-c` run, and
//     they are the five about being interactive — `interactive`, `monitor`,
//     `shinstdin`, `singlecommand` and `zle`. Every other name it takes, in
//     both directions, which is measured and is what says the rest belong in
//     one of the other three kinds rather than in a refusal;
//   - **recorded**: a name this shell recognizes and remembers and does not
//     act on. `setopt auto_cd` succeeds, `setopt` then reports `autocd`, and
//     typing a directory name still does not change directory. 150 of the 185
//     are this, and they are marked `recorded(…)` below so the distinction can
//     be read off the table rather than taken on trust.
//
// Recording is worth doing and is not the same as implementing. A real rc
// file opens with a dozen `setopt` lines about completion, correction and
// history — features this shell does not have — and a shell that answers each
// with `no such option` sprays complaints at every startup over nothing it was
// ever going to do. Recording stops the complaint and reports the request back
// faithfully; it promises nothing further, and docs/spec/semantics.md says so
// in the same words.
//
// A name outside the table is `no such option`, status 1, and the remaining
// operands are still acted on — measured: `setopt zzqq no_glob` complains and
// still turns globbing off, in either order.
//
// # The listings
//
// Measured, and simpler than it first looks. Each option has one printed
// spelling: the one that is off by default, so `noclobber` for an option that
// defaults on and `allexport` for one that defaults off. A bare `setopt`
// prints the spellings that are currently on and a bare `unsetopt` prints the
// ones that are currently off, both ordered by the canonical name and both
// naming the canonical option rather than the compat spelling that may have
// set it — measured, `setopt dotglob; setopt` answers `globdots`.
//
// So `setopt` is the list of deviations from zsh's defaults and `unsetopt` is
// its complement, which is why the first is two lines long and the second is
// 184 in a shell that has changed nothing.

// zshOption is one name in this dialect's option namespace.
type zshOption struct {
	// base is the canonical spelling: lower case, no underscores, no `no`.
	base string
	// def is the state a zsh default run has, which is what the listings
	// compare against.
	//
	// Four of the entries this file inherited hold this shell's own state
	// here instead of zsh's — `banghist`, `emacs`, `hashcmds` and
	// `interactivecomments` are all measured the other way round in real zsh
	// — which silences four deviations the listing exists to show. They are
	// left as they were found rather than corrected in a change about the
	// name set; see docs/spec/semantics.md.
	def bool
	// recorded marks a name that is remembered and not acted on. It is what
	// tells the listings and `emulate` that the state lives in the store
	// below rather than in anything the shell does.
	recorded bool
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
	recorded("aliasfuncdef", false),
	setOptBacked("allexport", false, "allexport", false),
	recorded("alwayslastprompt", true),
	recorded("alwaystoend", false),
	recorded("appendcreate", false),
	recorded("appendhistory", true),
	recorded("autocd", false),
	recorded("autocontinue", false),
	recorded("autolist", true),
	recorded("automenu", true),
	recorded("autonamedirs", false),
	recorded("autoparamkeys", true),
	recorded("autoparamslash", true),
	recorded("autopushd", false),
	recorded("autoremoveslash", true),
	recorded("autoresume", false),
	recorded("badpattern", true),
	fixedOptBacked("banghist", false, "histexpand", false),
	recorded("bareglobqual", true),
	recorded("bashautolist", false),
	recorded("bashrematch", false),
	recorded("beep", true),
	recorded("bgnice", true),
	recorded("braceccl", false),
	recorded("bsdecho", false),
	matchBacked("caseglob", true, interp.GlobFoldsCase, true),
	recorded("casematch", true),
	recorded("casepaths", false),
	recorded("cbases", false),
	recorded("cdablevars", false),
	recorded("cdsilent", false),
	recorded("chasedots", false),
	fixedOptBacked("chaselinks", false, "physical", false),
	recorded("checkjobs", true),
	recorded("checkrunningjobs", true),
	setOptBacked("clobber", true, "noclobber", true),
	recorded("clobberempty", false),
	recorded("combiningchars", false),
	recorded("completealiases", false),
	recorded("completeinword", false),
	recorded("continueonerror", false),
	recorded("correct", false),
	recorded("correctall", false),
	recorded("cprecedences", false),
	recorded("cshjunkiehistory", false),
	recorded("cshjunkieloops", false),
	recorded("cshjunkiequotes", false),
	recorded("cshnullcmd", false),
	recorded("cshnullglob", false),
	recorded("debugbeforecmd", true),
	recorded("dvorak", false),
	fixedOptBacked("emacs", true, "emacs", false),
	recorded("equals", true),
	setOptBacked("errexit", false, "errexit", false),
	recorded("errreturn", false),
	recorded("evallineno", true),
	setOptBacked("exec", true, "noexec", true),
	recorded("extendedglob", false),
	recorded("extendedhistory", false),
	recorded("flowcontrol", true),
	recorded("forcefloat", false),
	// `$0` inside a function is the function's name here, which is what the
	// name asks for and what this shell already does.
	fixedConstant("functionargzero", true, true),
	setOptBacked("glob", true, "noglob", true),
	recorded("globalexport", true),
	recorded("globalrcs", true),
	recorded("globassign", false),
	recorded("globcomplete", false),
	matchBacked("globdots", false, interp.PatternsMatchHidden, false),
	recorded("globstarshort", false),
	recorded("globsubst", false),
	setOptBacked("hashcmds", false, "hashall", false),
	fixedConstant("hashdirs", true, false),
	recorded("hashexecutablesonly", false),
	recorded("hashlistall", true),
	recorded("histallowclobber", false),
	recorded("histbeep", true),
	recorded("histexpiredupsfirst", false),
	recorded("histfcntllock", false),
	recorded("histfindnodups", false),
	recorded("histignorealldups", false),
	setOptBacked("histignoredups", false, "histignoredups", false),
	recorded("histignorespace", false),
	recorded("histlexwords", false),
	recorded("histnofunctions", false),
	recorded("histnostore", false),
	recorded("histreduceblanks", false),
	recorded("histsavebycopy", true),
	recorded("histsavenodups", false),
	recorded("histsubstpattern", false),
	recorded("histverify", false),
	recorded("hup", true),
	fixedOptBacked("ignorebraces", false, "braceexpand", true),
	recorded("ignoreclosebraces", false),
	fixedOptBacked("ignoreeof", false, "ignoreeof", false),
	recorded("incappendhistory", false),
	recorded("incappendhistorytime", false),
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
	recorded("kshautoload", false),
	recorded("kshglob", false),
	recorded("kshoptionprint", false),
	recorded("kshtypeset", false),
	recorded("kshzerosubscript", false),
	recorded("listambiguous", true),
	recorded("listbeep", true),
	recorded("listpacked", false),
	recorded("listrowsfirst", false),
	recorded("listtypes", true),
	recorded("localloops", false),
	recorded("localoptions", false),
	recorded("localpatterns", false),
	recorded("localtraps", false),
	recorded("login", false),
	recorded("longlistjobs", false),
	recorded("magicequalsubst", false),
	recorded("mailwarning", false),
	recorded("markdirs", false),
	recorded("menucomplete", false),
	// The five that refuse to move, all of them about being interactive.
	// Every one is off here and off in a `-c` zsh, so turning it off is
	// granted and turning it on is the measured `can't change option`, 1.
	fixedConstant("monitor", false, false),
	recorded("multibyte", true),
	recorded("multifuncdef", true),
	recorded("multios", true),
	{
		base: "nomatch", def: true,
		get: func(r *interp.Runner) bool { return r.Semantics.GlobNoMatchIsError == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.GlobNoMatchIsError = answer(on) })
			return 0
		},
	},
	fixedConstant("notify", true, true),
	matchBacked("nullglob", false, interp.UnmatchedPatternIsEmpty, false),
	recorded("numericglobsort", false),
	recorded("octalzeroes", false),
	recorded("overstrike", false),
	recorded("pathdirs", false),
	recorded("pathscript", false),
	setOptBacked("pipefail", false, "pipefail", false),
	recorded("posixaliases", false),
	recorded("posixargzero", false),
	recorded("posixbuiltins", false),
	recorded("posixcd", false),
	recorded("posixidentifiers", false),
	recorded("posixjobs", false),
	recorded("posixstrings", false),
	recorded("posixtraps", false),
	recorded("printeightbit", false),
	recorded("printexitvalue", false),
	fixedOptBacked("privileged", false, "privileged", false),
	recorded("promptbang", false),
	recorded("promptcr", true),
	recorded("promptpercent", true),
	recorded("promptsp", true),
	recorded("promptsubst", false),
	recorded("pushdignoredups", false),
	recorded("pushdminus", false),
	recorded("pushdsilent", false),
	recorded("pushdtohome", false),
	recorded("rcexpandparam", false),
	recorded("rcquotes", false),
	recorded("rcs", true),
	recorded("recexact", false),
	recorded("rematchpcre", false),
	recorded("restricted", false),
	recorded("rmstarsilent", false),
	recorded("rmstarwait", false),
	recorded("sharehistory", false),
	recorded("shfileexpansion", false),
	// sh-style globbing narrows the pattern language to the standard's. This
	// shell does not narrow it, which is the state, and zsh's default is the
	// same, so the listings do not move. The prompt theme this machine loads
	// reads the name at its third line.
	fixedConstant("shglob", false, false),
	fixedConstant("shinstdin", false, false),
	recorded("shnullcmd", false),
	recorded("shoptionletters", false),
	recorded("shortloops", true),
	recorded("shortrepeat", false),
	{
		base: "shwordsplit", def: false,
		get: func(r *interp.Runner) bool { return r.Semantics.SplitParamExpansion == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.SplitParamExpansion = answer(on) })
			return 0
		},
	},
	fixedConstant("singlecommand", false, false),
	recorded("singlelinezle", false),
	recorded("sourcetrace", false),
	recorded("sunkeyboardhack", false),
	recorded("transientrprompt", false),
	recorded("trapsasync", false),
	recorded("typesetsilent", false),
	recorded("typesettounset", false),
	setOptBacked("unset", true, "nounset", true),
	setOptBacked("verbose", false, "verbose", false),
	fixedOptBacked("vi", false, "vi", false),
	recorded("warncreateglobal", false),
	recorded("warnnestedvar", false),
	setOptBacked("xtrace", false, "xtrace", false),
	fixedConstant("zle", false, false),
}

// zshOptionAlias is one of the compat spellings: a second name for an option
// the table already holds, negated where the two names mean opposite states.
type zshOptionAlias struct {
	base string
	inv  bool
}

// zshOptionAliases are the twelve names zsh carries for sh and ksh
// compatibility. Each was identified by flipping it in a real zsh and reading
// which canonical option moved: `setopt dotglob` turns `globdots` on,
// `unsetopt braceexpand` turns `ignorebraces` on, and `setopt nolog` turns
// `histnofunctions` on. `hashall` and `trackall` are two names for one option,
// which is why both land on `hashcmds`.
//
// They resolve to the canonical entry before anything else happens, so they
// share its state exactly and are never what a listing prints — measured, and
// the reason they are a map here rather than entries of their own.
var zshOptionAliases = map[string]zshOptionAlias{
	"braceexpand": {"ignorebraces", true},
	"dotglob":     {"globdots", false},
	"hashall":     {"hashcmds", false},
	"histappend":  {"appendhistory", false},
	"histexpand":  {"banghist", false},
	"log":         {"histnofunctions", true},
	"mailwarn":    {"mailwarning", false},
	"onecmd":      {"singlecommand", false},
	"physical":    {"chaselinks", false},
	"promptvars":  {"promptsubst", false},
	"stdin":       {"shinstdin", false},
	"trackall":    {"hashcmds", false},
}

// zshOptionIndex is the table by canonical name, built once. A linear scan
// answered when the table held twenty-six names; at 185 it is walked twice
// per operand and a startup file writes dozens of them.
var zshOptionIndex = func() map[string]int {
	m := make(map[string]int, len(zshOptions))
	for i, o := range zshOptions {
		m[o.base] = i
	}
	return m
}()

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

// matchBacked binds a zsh name to one of the pattern matcher's run-time
// options, inverted where zsh names the state the matcher's flag turns off:
// `caseglob` on is `GlobFoldsCase` off. These are implemented, not recorded —
// `setopt nullglob` really does delete a word that matched nothing.
func matchBacked(base string, def bool, opt interp.MatchOption, inv bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(r *interp.Runner) bool { return r.MatchOption(opt) != inv },
		set: func(r *interp.Runner, on bool) int { r.SetMatchOption(opt, on != inv); return 0 },
	}
}

// recorded is a name this shell remembers and does not act on. See the four
// kinds at the top of this file: the state is real, readable and reported,
// and nothing in the shell reads it.
func recorded(base string, def bool) zshOption {
	return zshOption{
		base: base, def: def, recorded: true,
		get: func(r *interp.Runner) bool { return def != recordedDeviates(r, base) },
		set: func(r *interp.Runner, on bool) int {
			setRecordedDeviation(r, base, on != def)
			return 0
		},
	}
}

// zshRecordedStore is where the recorded options live: the canonical names
// whose state differs from the table's default, in an array under a name no
// script can reach — the shape `zstyle` and `emulate` already use, and for
// the same reason. A subshell deep-copies the Vars table, so `(setopt
// auto_cd)` stays in the subshell exactly as an axis-backed option does.
//
// Deviations rather than states, so that a runner that has never run `setopt`
// holds an empty array and every name reads back at its default.
const zshRecordedStore = ".zsh.setopt"

// recordedDeviates reports whether one recorded name has been moved off its
// default.
func recordedDeviates(r *interp.Runner, base string) bool {
	names, _ := r.GetArray(zshRecordedStore)
	for _, n := range names {
		if n == base {
			return true
		}
	}
	return false
}

// setRecordedDeviation records or clears one name's deviation, keeping the
// store sorted so the array is a function of the set and not of the order the
// rc file happened to write.
func setRecordedDeviation(r *interp.Runner, base string, dev bool) {
	names, _ := r.GetArray(zshRecordedStore)
	out := make([]string, 0, len(names)+1)
	for _, n := range names {
		if n != base {
			out = append(out, n)
		}
	}
	if dev {
		out = append(out, base)
		sort.Strings(out)
	}
	r.SetArray(zshRecordedStore, out)
}

// recordedOptions is the store itself, for `emulate` to save and put back.
func recordedOptions(r *interp.Runner) []string {
	names, _ := r.GetArray(zshRecordedStore)
	return append([]string(nil), names...)
}

// setRecordedOptions replaces the store wholesale, which is how an emulation
// resets every recorded name to its default at once.
func setRecordedOptions(r *interp.Runner, names []string) {
	r.SetArray(zshRecordedStore, names)
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

// resolveOptionName takes one normalized name to its table entry and the
// direction the spelling asked for. The name is looked up whole first, which
// is what keeps `nomatch` an option rather than a negated `match` — it and
// `notify` are the only two option names that begin with `no`, measured — and
// only then is a single `no` stripped. A compat spelling resolves to its
// canonical entry at either step, so `nophysical` is `chaselinks` off.
func resolveOptionName(name string) (zshOption, bool, bool) {
	if o, inv, ok := exactOptionName(name); ok {
		return o, inv, true
	}
	rest, ok := strings.CutPrefix(name, "no")
	if !ok {
		return zshOption{}, false, false
	}
	o, inv, ok := exactOptionName(rest)
	return o, !inv, ok
}

// exactOptionName looks one spelling up without stripping anything, through
// the alias table first.
func exactOptionName(name string) (zshOption, bool, bool) {
	inv := false
	if a, ok := zshOptionAliases[name]; ok {
		name, inv = a.base, a.inv
	}
	if i, ok := zshOptionIndex[name]; ok {
		return zshOptions[i], inv, true
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
	o, inverted, ok := resolveOptionName(normalizeOption(name))
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
			o, inverted, ok := resolveOptionName(normalizeOption(arg))
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

// listZshOptions is the bare command. Every option has one printed spelling —
// the one that is off by default — and `setopt` prints the spellings that are
// on where `unsetopt` prints the ones that are off. The table is already in
// the measured order, which is by canonical name.
func listZshOptions(r *interp.Runner, setting bool) {
	for _, o := range zshOptions {
		if o.get(r) != o.def == setting {
			_, _ = fmt.Fprintf(r.Out(), "%s\n", spellOption(o.base, !o.def))
		}
	}
}

// spellOption writes the name in the direction asked for: the base for the
// state a listing calls on, `no` and the base for the other.
func spellOption(base string, on bool) string {
	if on {
		return base
	}
	return "no" + base
}
