// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"context"
	"fmt"
	"strings"

	"github.com/blairham/sh/interp"
)

// `whence` is ksh93's own question about a name, and the shape real ksh
// scripts ask it in — its `type` is spelled `whence -v`, which the dialect's
// diagnostics already say.
//
// Measured, each mode:
//
//   - bare: the resolution and nothing more — a builtin, keyword or function
//     answers as its own name, an external as its path, an alias as its value
//     quoted when it needs it, and a name that resolves to nothing is silence
//     and 1. The lookup and the spelling are `command -v`'s exactly, measured
//     side by side, so that is what answers here — aliases excepted, which
//     the substrate's `command -v` does not consult.
//   - `-v`: the sentence — the core `type`, whose ksh wording is already this
//     shell's, plus the alias line the core cannot know.
//   - `-p`: the PATH search alone, functions and builtins invisible: `whence
//     -p echo` is /bin/echo and `whence -p f` says nothing at 1. With `-v`
//     the found path is worded as a tracked alias, and a name PATH does not
//     hold falls back to `-v`'s own not-found line.
//   - `-q`: the status with the words withheld.
//
// `-a` and `-f` are ksh93's too and are not implemented here — the letters
// are refused the way the substrate refuses an option a dialect has but this
// shell does not, and docs/spec/semantics.md records the boundary. An unknown
// letter is `unknown option` with the usage line after it, 2; so is a whence
// with nothing to ask about.
const whenceUsage = "Usage: whence [-afpqv] name  ..."

// registerWhence installs the builtin.
func registerWhence(r *interp.Runner) {
	r.Register("whence", whenceBuiltin)
}

func whenceBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	names, opts, code := whenceOptions(r, args)
	if code != 0 {
		return code
	}
	status := 0
	for _, name := range names {
		if st := whenceOne(r, ctx, name, opts); st != 0 {
			status = st
		}
	}
	return status
}

// whenceOptions reads the leading option words, refusing what is not
// implemented the way the substrate does and what is unknown the way ksh93
// does.
func whenceOptions(r *interp.Runner, args []string) (names []string, opts string, code int) {
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for _, letter := range word[1:] {
			switch letter {
			case 'v', 'p', 'q':
				opts += string(letter)
			case 'a', 'f':
				r.Diagnosef("whence: -%c is not implemented yet\n", letter)
				return nil, "", 2
			default:
				r.Diagnosef("whence: -%c: unknown option\n", letter)
				_, _ = fmt.Fprintf(r.Err(), "%s\n", whenceUsage)
				return nil, "", 2
			}
		}
	}
	if len(rest) == 0 {
		// Nothing to ask about is a usage error, the bare line and 2.
		_, _ = fmt.Fprintf(r.Err(), "%s\n", whenceUsage)
		return nil, "", 2
	}
	return rest, opts, 0
}

// whenceOne answers for one name.
func whenceOne(r *interp.Runner, ctx context.Context, name, opts string) int {
	quiet := strings.ContainsRune(opts, 'q')
	verbose := strings.ContainsRune(opts, 'v')
	if strings.ContainsRune(opts, 'p') {
		return whencePath(r, name, verbose, quiet)
	}
	if value, ok := r.LookupAlias(name); ok {
		// The one resolution the core's lookup cannot see: the table is the
		// runner's, but whether a word expands is the parser's fact, so
		// `type` never speaks for aliases and this dialect does.
		switch {
		case quiet:
		case verbose:
			_, _ = fmt.Fprintf(r.Out(), "%s is an alias for %s\n", name, quoteWhenNeeded(value))
		default:
			_, _ = fmt.Fprintf(r.Out(), "%s\n", quoteWhenNeeded(value))
		}
		return 0
	}
	inner := "command"
	innerArgs := []string{"-v", name}
	if verbose {
		// The sentence is the core `type`, whose ksh wording — the tracked
		// alias, the whence-prefixed not-found — is already this dialect's.
		inner, innerArgs = "type", []string{name}
	}
	fn, ok := r.Builtin(inner)
	if !ok {
		return 1
	}
	if quiet {
		return runQuietly(r, ctx, fn, innerArgs)
	}
	return fn(r, ctx, innerArgs)
}

// whencePath is `-p`: the PATH search with everything else invisible.
func whencePath(r *interp.Runner, name string, verbose, quiet bool) int {
	path, ok := r.LookPath(name)
	if !ok {
		if verbose && !quiet {
			// Measured: `-pv` on a name PATH does not hold says what `-v`
			// says, where plain `-p` says nothing.
			r.Diagnosef("whence: %s: not found\n", name)
		}
		return 1
	}
	switch {
	case quiet:
	case verbose:
		// The wording a path gets from `-v`, measured on `whence -pv echo`:
		// the sentence `type` uses for an external.
		_, _ = fmt.Fprintf(r.Out(), "%s is a tracked alias for %s\n", name, path)
	default:
		_, _ = fmt.Fprintf(r.Out(), "%s\n", path)
	}
	return 0
}

// runQuietly is `-q`: the delegate's status with its output withheld. The
// diagnostic stream stays: a quiet whence is quiet about answers, and its
// not-found complaints were measured as silence already.
func runQuietly(r *interp.Runner, ctx context.Context, fn interp.Builtin, args []string) int {
	savedOut, savedErr := r.Stdout, r.Stderr
	r.Stdout, r.Stderr = discard{}, discard{}
	defer func() { r.Stdout, r.Stderr = savedOut, savedErr }()
	return fn(r, ctx, args)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// quoteWhenNeeded spells an alias value the way ksh93's listing does: bare
// while every character can stand bare, single-quoted otherwise.
func quoteWhenNeeded(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`&|;<>(){}[]*?~#=") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
