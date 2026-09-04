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
}

func biComplete(r *Runner, _ context.Context, args []string) int {
	// The spec's own options are stored, not interpreted, so the reader
	// splits only the three that change what the command *does*: -p prints,
	// -r removes, and everything else registers. They come first in every
	// use bash's own manual shows, and bash reads them anywhere; reading
	// them anywhere costs nothing.
	var print, remove bool
	var spec []string
	var names []string
	expectArg := false
	for _, a := range args {
		switch {
		case expectArg:
			spec = append(spec, a)
			expectArg = false
		case a == "-p":
			print = true
		case a == "-r":
			remove = true
		case len(a) > 1 && a[0] == '-':
			spec = append(spec, a)
			// The letters whose argument is the next word, from bash's
			// synopsis: actions, words, globs, prefixes, suffixes, the
			// function and the command.
			letter := a[len(a)-1]
			if strings.ContainsRune("AGWFCXPS", rune(letter)) {
				expectArg = true
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
				r.diagf("%s\n", Wording(r.diag().CompleteNoSpec,
					"complete: %[1]s: no completion specification", name))
				status = 1
				continue
			}
			delete(r.completions, name)
		}
		return status
	case print || (len(spec) == 0 && len(names) == 0):
		// `-p`, and a bare `complete`, print what is registered — sorted,
		// because bash's own order is its hash table's and no script can
		// rely on it.
		if len(names) == 0 {
			for _, name := range sortedCompletionNames(r.completions) {
				r.printf("complete %s%s\n", r.completions[name], name)
			}
			return 0
		}
		status := 0
		for _, name := range names {
			stored, ok := r.completions[name]
			if !ok {
				r.diagf("%s\n", Wording(r.diag().CompleteNoSpec,
					"complete: %[1]s: no completion specification", name))
				status = 1
				continue
			}
			r.printf("complete %s%s\n", stored, name)
		}
		return status
	}

	// A registration. The options are kept as written, quoted the way bash
	// prints them back: a word with a space goes back in single quotes.
	var b strings.Builder
	for _, w := range spec {
		if strings.ContainsAny(w, " \t") {
			b.WriteString("'" + w + "' ")
			continue
		}
		b.WriteString(w + " ")
	}
	if r.completions == nil {
		r.completions = map[string]string{}
	}
	for _, name := range names {
		r.completions[name] = b.String()
	}
	return 0
}

func sortedCompletionNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
