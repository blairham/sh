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
//
//   - `-v`: the sentence — the core `type`, whose ksh wording is already this
//     shell's, plus the alias line the core cannot know.
//
//   - `-p`: the PATH search alone, functions and builtins invisible: `whence
//     -p echo` is /bin/echo and `whence -p f` says nothing at 1. With `-v`
//     the found path is worded as a tracked alias, and a name PATH does not
//     hold falls back to `-v`'s own not-found line.
//
//   - `-q`: the status with the words withheld.
//
//   - `-a`: every resolution rather than the first, in the order below, and
//     always in `-v`'s sentences. Measured name by name; the rule that
//     decides the last line is the one worth writing down, because #633
//     recorded it as needing FPATH machinery and it does not:
//
//     alias, if any            N is an alias for V
//     keyword, if any          N is a keyword
//     function, if any         N is a function
//     builtin, if any and no   N is a shell builtin — or `N is a special
//     function shadows it      shell builtin`, for one POSIX marks special
//     every PATH hit           N is <path>, or `N is a tracked alias for
//     <path>` when the PATH hit is the only line
//     the FPATH candidate      N is an undefined function
//
//     The last line is **not** an FPATH fact. It appears exactly when the
//     name had a builtin or function resolution *and* a PATH hit, and it
//     appears with FPATH unset, set to an empty directory, or exported:
//     `whence -a typeset` and `whence -a whence` — builtins with no file on
//     PATH — do not print it, while `whence -a alias` and `whence -a ls`
//     with a function defined do. So it is a PATH search, which this
//     substrate has, rather than the function-path walk #633 took it for.
//
// `-f` is ksh93's too and is not implemented here — the letter is refused the
// way the substrate refuses an option a dialect has but this shell does not,
// and docs/spec/semantics.md records the boundary. An unknown letter is
// `unknown option` with the usage line after it, 2; so is a whence with
// nothing to ask about.
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
			case 'v', 'p', 'q', 'a':
				opts += string(letter)
			case 'f':
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
	if lastP := strings.LastIndexByte(opts, 'p'); lastP >= 0 {
		// `-p` always restricts the answer to the PATH search: a builtin, a
		// function and an alias are invisible with the letter written, in
		// any order and with `-a` beside it.
		//
		// What is decided by *position* is the wording, and by the last of
		// the three letters rather than by two of them. Measured 2026-09-12
		// and again 2026-09-18 on ksh93u+ 2012-08-01 over eight orderings:
		//
		//	-p    path     -a    sentences   -ap   paths      -pa   sentences
		//	-pv   sentence -vp   path        -apv  sentences  -avp  paths
		//	                                 -pav  sentences  -vap  paths
		//
		// So the bare path is written when `p` is the last of `a`, `p` and
		// `v` to appear, and the sentence otherwise — which is the same rule
		// the two-letter rows already followed, with `a` joining it. The
		// not-found report follows the wording: `-pv` and `-pa` say `not
		// found` where `-p` and `-vp` say nothing at all.
		//
		// `-a` decides something else entirely: **how many rows there are**.
		// With it the PATH walk is every hit rather than the first, which is
		// what composes the two letters — `whence -ap dup` is both copies as
		// paths where `whence -p dup` is the first as a path (#3198).
		verbose := max(strings.LastIndexByte(opts, 'v'), strings.LastIndexByte(opts, 'a')) > lastP
		return whencePath(r, name, verbose, quiet, strings.ContainsRune(opts, 'a'))
	}
	if strings.ContainsRune(opts, 'a') {
		// `-a` speaks in the sentences whether or not `-v` was written:
		// `whence -a echo` and `whence -av echo` are the same three lines.
		return whenceAll(r, name, quiet)
	}
	if value, ok := r.ReportedAlias(name); ok {
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

// whenceAll is `-a`: every resolution the shell can see, in the order ksh93
// lists them, and always as sentences.
//
// The order is alias, keyword, function, builtin, PATH — and a *defined*
// function hides the builtin of the same name, which is what the shell would
// actually run, while an alias hides nothing: `alias echo=x; whence -a echo`
// is four lines and `echo() { :; }; whence -a echo` is three with no builtin
// among them.
func whenceAll(r *interp.Runner, name string, quiet bool) int {
	var lines []string
	shadowed := false
	if value, ok := r.ReportedAlias(name); ok {
		lines = append(lines, fmt.Sprintf("%s is an alias for %s", name, quoteWhenNeeded(value)))
	}
	switch kind, _ := r.ResolveName(name); kind {
	case interp.NameReserved:
		lines = append(lines, name+" is a keyword")
	case interp.NameFunction:
		lines = append(lines, name+" is a function")
		shadowed = true
	case interp.NameBuiltin:
		// The core's sentence and not a literal, which is the whole of #3273
		// in this file: `whence -v .` goes through the shared wording and had
		// the special/ordinary split, and this letter wrote its own line and
		// did not — so one builtin was `a special shell builtin` under `-v`
		// and `a shell builtin` under `-a`, from the same shell, one line
		// apart. ksh93u+ 2012-08-01 says `special` under both.
		lines = append(lines, r.BuiltinSentence(name))
		shadowed = true
	case interp.NameNotFound, interp.NameFile:
		// A file is listed by the PATH loop below, which shows every hit
		// rather than the first.
	}
	paths := r.LookPathAll(name)
	for _, path := range paths {
		if len(lines) == 0 {
			// The PATH hit standing alone keeps the sentence `-v` gives it,
			// asked through the core so that the *pathname operand* wording
			// comes with it: a word already holding a slash was never
			// searched for, and this shell writes the plain sentence for
			// one. Written out here as a literal, it wrote `./bb/tool is a
			// tracked alias for …` where the real shell writes `./bb/tool is
			// …` (#2953).
			lines = append(lines, r.TypeExternalSentence(name, path))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s is %s", name, path))
	}
	if shadowed && len(paths) > 0 {
		// The FPATH candidate, which is a PATH fact here rather than a
		// function-path one — see the table above.
		lines = append(lines, name+" is an undefined function")
	}
	if len(lines) == 0 {
		if !quiet {
			r.Diagnosef("whence: %s: not found\n", r.NameReportWord(name))
		}
		return 1
	}
	if quiet {
		return 0
	}
	for _, line := range lines {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", line)
	}
	return 0
}

// whencePath is `-p`: the PATH search with everything else invisible.
//
// all is `-a` written beside it, which makes the walk every hit rather than
// the first. The two letters compose rather than one winning: `-a` says how
// many rows there are and the wording is decided by position — see whenceOne
// for the eight orderings that were measured.
func whencePath(r *interp.Runner, name string, verbose, quiet, all bool) int {
	paths := []string{}
	if all {
		paths = r.LookPathAll(name)
	} else if path, ok := r.LookPath(name); ok {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		if verbose && !quiet {
			// The later letter's wording again: `whence -pv` on a name PATH
			// does not hold says what `-v` says, where plain `-p` and
			// `whence -vp` say nothing.
			r.Diagnosef("whence: %s: not found\n", r.NameReportWord(name))
		}
		return 1
	}
	if quiet {
		return 0
	}
	for i, path := range paths {
		if !verbose {
			_, _ = fmt.Fprintf(r.Out(), "%s\n", path)
			continue
		}
		if i == 0 {
			// The wording a path gets from `-v`, measured on `whence -pv
			// echo`: the sentence `type` uses for an external, and through
			// the core so that a pathname operand gets the plain one.
			_, _ = fmt.Fprintf(r.Out(), "%s\n", r.TypeExternalSentence(name, path))
			continue
		}
		// And a later hit is the plain sentence, which is the same shape
		// `whence -a` writes: the first is how the name resolves and the
		// rest are what else is there.
		_, _ = fmt.Fprintf(r.Out(), "%s is %s\n", name, path)
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
