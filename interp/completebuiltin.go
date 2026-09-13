// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"
)

// `complete` records completion specifications.
//
// bash alone: dash, ksh93 and zsh have no such command, so it is registered
// here and taken away by the three without it, the way `compgen` is.
//
// Nothing here completes anything — there is no interactive line to complete —
// but every bash_completion.d file is written to run in a non-interactive
// shell, registering specs and exiting 0, and dying at 127 on the first
// `complete` is what broke carapace's setup in the run sweep. So the spec is
// *kept*, verbatim: registration succeeds, `complete -p` and a bare
// `complete` print the specs back the way bash prints them, and `-r` takes
// one away. That is the whole of what a script can observe without a
// terminal, measured against bash.
func init() {
	builtins["complete"] = biComplete
	builtins["compopt"] = biCompopt
}

// completionSpec is one registered specification.
//
// Two halves rather than one string, because they are printed back
// differently and one of them can be *moved* after registration. bash renders
// the `-o` options first, sorted and deduplicated, whatever order they were
// written in — measured, `complete -F f -o nospace x` comes back as
// `complete -o nospace -F f x` and `complete -o nospace -o dirnames x` as
// `-o dirnames -o nospace` — and `compopt` adds and removes them one at a
// time, which a rendered string could only do by taking itself apart again.
//
// Everything else is kept as written. bash canonicalizes some of it (`-A
// file` comes back as `-f`) and that is deliberately not followed: what a
// script can observe here is that its spec survived, and inventing a
// normalization we have not measured in full would be a second answer to
// maintain.
type completionSpec struct {
	// options are the `-o` names that are on, sorted and unique.
	options []string
	// words is the rest of the specification, in the order it was written.
	words []string
}

// render writes the spec back the way bash prints it: the options first, then
// the words, each word a space away from the next.
func (c completionSpec) render() string {
	var b strings.Builder
	for _, o := range c.options {
		b.WriteString("-o " + o + " ")
	}
	for _, w := range c.words {
		if strings.ContainsAny(w, " \t") {
			b.WriteString("'" + w + "' ")
			continue
		}
		b.WriteString(w + " ")
	}
	return b.String()
}

// setOption turns one `-o` name on or off, keeping the list sorted and
// unique. Reports whether anything moved, which nothing reads today and which
// is what says the two directions are one operation.
func (c *completionSpec) setOption(name string, on bool) bool {
	at := sort.SearchStrings(c.options, name)
	held := at < len(c.options) && c.options[at] == name
	switch {
	case on && !held:
		c.options = append(c.options, "")
		copy(c.options[at+1:], c.options[at:])
		c.options[at] = name
	case !on && held:
		c.options = append(c.options[:at], c.options[at+1:]...)
	default:
		return false
	}
	return true
}

// completionOptions are the `-o` names bash has, which both builtins validate
// against. Measured from `compopt name` on a spec, which lists every one of
// them in this order:
//
//	compopt +o bashdefault +o default +o dirnames +o filenames +o fullquote
//	        +o noquote +o nosort +o nospace +o plusdirs name
//
// A name that is not one of these is refused at 2 by `complete` and `compopt`
// alike, before either touches the table — measured, `complete -o nosuchopt
// x` registers nothing.
var completionOptions = []string{
	"bashdefault", "default", "dirnames", "filenames", "fullquote",
	"noquote", "nosort", "nospace", "plusdirs",
}

// completionOption reports whether a word is one of them.
func completionOption(name string) bool {
	at := sort.SearchStrings(completionOptions, name)
	return at < len(completionOptions) && completionOptions[at] == name
}

// completionPseudoNames are what `-D`, `-E` and `-I` name instead of a
// command: bash keeps the default, the empty-line and the initial-word
// specifications in the same table under names no command can have, and says
// so out loud — `compopt -D` with none registered is `compopt: _DefaultCmD_:
// no completion specification`. Printed back as the letter, measured:
// `complete -D -F f` lists as `complete -F f -D`.
var completionPseudoNames = map[byte]string{
	'D': "_DefaultCmD_", 'E': "_EmptycmD_", 'I': "_InitialWorD_",
}

// completionPseudoLetters is the same map read the other way, for the
// listing. One table would need a search at every print; two would drift, so
// this is built from the first.
var completionPseudoLetters = func() map[string]string {
	m := make(map[string]string, len(completionPseudoNames))
	for letter, name := range completionPseudoNames {
		m[name] = "-" + string(letter)
	}
	return m
}()

// completionDisplayName is how a stored name is written back: the letter for
// one of the three pseudo-names, and the name itself for a command.
func completionDisplayName(name string) string {
	if letter, ok := completionPseudoLetters[name]; ok {
		return letter
	}
	return name
}

func biComplete(r *Runner, _ context.Context, args []string) int {
	// The spec's own options are stored, not interpreted, so the reader
	// splits only the ones that change what the command *does*: -p prints,
	// -r removes, -o names an option this shell can move afterwards, and
	// -D/-E/-I name a spec that is not a command's. They come first in every
	// use bash's own manual shows, and bash reads them anywhere; reading
	// them anywhere costs nothing.
	var print, remove bool
	spec := completionSpec{}
	var names []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-p":
			print = true
		case a == "-r":
			remove = true
		case a == "-o" || a == "+o":
			// The one letter whose argument this builtin reads rather than
			// stores. It went unread until #2412, so `complete -o nospace -F
			// _foo foo` registered a completion for a command called
			// `nospace` as well as for `foo`, and printed the spec back with
			// the option's name missing.
			i++
			if i >= len(args) {
				r.diagf("complete: %s: option requires an argument\n", a)
				r.completeUsage()
				return 2
			}
			if !completionOption(args[i]) {
				r.diagf("complete: %s: invalid option name\n", args[i])
				return 2
			}
			spec.setOption(args[i], a == "-o")
		case len(a) > 1 && a[0] == '-' && completionPseudoNames[a[1]] != "" && len(a) == 2:
			names = append(names, completionPseudoNames[a[1]])
		case len(a) > 1 && a[0] == '-':
			spec.words = append(spec.words, a)
			// The letters whose argument is the next word, from bash's
			// synopsis: actions, words, globs, prefixes, suffixes, the
			// function and the command.
			letter := a[len(a)-1]
			if strings.ContainsRune("AGWFCXPS", rune(letter)) && i+1 < len(args) {
				i++
				spec.words = append(spec.words, args[i])
			}
		default:
			names = append(names, a)
		}
	}

	switch {
	case remove:
		if len(names) == 0 {
			// `complete -r` with no names forgets everything.
			r.completions = nil
			return 0
		}
		status := 0
		for _, name := range names {
			if _, ok := r.completions[name]; !ok {
				status = r.noCompletionSpec("complete", name)
				continue
			}
			delete(r.completions, name)
		}
		return status
	case print || (len(spec.options) == 0 && len(spec.words) == 0 && len(names) == 0):
		// `-p`, and a bare `complete`, print what is registered — sorted,
		// because bash's own order is its hash table's and no script can
		// rely on it.
		if len(names) == 0 {
			for _, name := range sortedCompletionNames(r.completions) {
				r.printCompletion(name)
			}
			return 0
		}
		status := 0
		for _, name := range names {
			if _, ok := r.completions[name]; !ok {
				status = r.noCompletionSpec("complete", name)
				continue
			}
			r.printCompletion(name)
		}
		return status
	}

	if r.completions == nil {
		r.completions = map[string]completionSpec{}
	}
	for _, name := range names {
		r.completions[name] = spec
	}
	return 0
}

// biCompopt changes the `-o` options of specs that are already registered, or
// reports them.
//
// bash's own use for it is from *inside* a completion function, where it
// changes the spec of the completion being generated — which is why a call
// with no names outside one is `not currently executing completion function`.
// This shell never runs a completion, so that is the only answer it can give
// to the no-name form, and it happens to be the measured one for every route
// a script can reach.
//
// What a script *can* observe without a terminal is the rest: a name with no
// spec is refused and named, a listing writes every option's state, and a
// change survives to be printed back by `complete -p`. All measured against
// bash 5.3.15 on 2026-09-12.
func biCompopt(r *Runner, _ context.Context, args []string) int {
	type change struct {
		name string
		on   bool
	}
	var changes []change
	var names []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o" || a == "+o":
			i++
			if i >= len(args) {
				r.diagf("compopt: %s: option requires an argument\n", a)
				r.compoptUsage()
				return 2
			}
			if !completionOption(args[i]) {
				// Before any name is looked at: measured, `compopt -o
				// nosuchopt true` with no spec for `true` complains about
				// the option and not about the name.
				r.diagf("compopt: %s: invalid option name\n", args[i])
				return 2
			}
			changes = append(changes, change{args[i], a == "-o"})
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				pseudo, ok := completionPseudoNames[a[j]]
				if !ok {
					r.diagf("compopt: -%c: invalid option\n", a[j])
					r.compoptUsage()
					return 2
				}
				names = append(names, pseudo)
			}
		default:
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		// Nothing to change and nowhere to change it. bash means this
		// literally — `compopt` inside a completion function operates on the
		// completion in progress — and there is never one here.
		r.diagf("%s\n", Wording(r.diag().CompoptNoCompletion,
			"compopt: not currently executing completion function"))
		return 1
	}
	status := 0
	for _, name := range names {
		spec, ok := r.completions[name]
		if !ok {
			status = r.noCompletionSpec("compopt", name)
			continue
		}
		if len(changes) == 0 {
			// The listing: every option bash has, in its own order, with the
			// sign saying whether this spec holds it.
			var b strings.Builder
			b.WriteString("compopt")
			for _, o := range completionOptions {
				sign := "+o"
				if spec.setOption(o, true) {
					// It was not there, so put it back and say so.
					spec.setOption(o, false)
				} else {
					sign = "-o"
				}
				b.WriteString(" " + sign + " " + o)
			}
			r.printf("%s %s\n", b.String(), completionDisplayName(name))
			continue
		}
		for _, c := range changes {
			spec.setOption(c.name, c.on)
		}
		r.completions[name] = spec
	}
	return status
}

// noCompletionSpec is the complaint both builtins make about a name nothing
// was registered for, worded once: they say the same thing about the same
// table and each blames itself.
func (r *Runner) noCompletionSpec(builtin, name string) int {
	r.diagf("%s\n", Wording(r.diag().CompleteNoSpec,
		"%[2]s: %[1]s: no completion specification", name, builtin))
	return 1
}

// printCompletion writes one spec back as the command that would register it.
func (r *Runner) printCompletion(name string) {
	r.printf("complete %s%s\n", r.completions[name].render(), completionDisplayName(name))
}

func (r *Runner) completeUsage() {
	if w := r.diag().BuiltinUsage["complete"]; w != "" {
		r.errf("%s\n", w)
	}
}

func (r *Runner) compoptUsage() {
	if w := r.diag().BuiltinUsage["compopt"]; w != "" {
		r.errf("%s\n", w)
	}
}

func sortedCompletionNames(m map[string]completionSpec) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
