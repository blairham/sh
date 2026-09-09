// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command shfmt lays out shell scripts, in every dialect the substrate
// parses: the core, posix, dash, bash, ksh93 and zsh.
//
// On the name: #1401 argued against it, on the grounds that `mvdan.cc/sh`
// ships a formatter called `shfmt` and a matching name invites a comparison
// the license does not permit. The maintainer chose it anyway, 2026-09-09, and
// the clean-room position is unchanged by it — nothing here was written from
// that project's source, and this formatter works by a different principle:
// it emits every token from the source extent it was read from, where a
// canonicalizing printer rebuilds it. The name collides; the code does not.
//
// It is the first thing here that consumes the parser as a product rather than
// as test infrastructure, which is the criterion `AGENTS.md` sets for taking a
// package seriously.
//
// # What it promises
//
// Nothing is lost. Every token is written back verbatim by its source extent,
// so a word is never respelled — `$x` does not become `${x}`, backticks do not
// become `$( )`, a here-document body is copied byte for byte — and every
// comment survives. What this command owns is the space *between* tokens:
// indentation, keyword placement, operator spacing, statement separation, and
// where a comment stands.
//
// That is why it does not print through [syntax.Print]. That printer is a
// canonicalizer: it promises the tree, not the spelling. The distinction is
// not academic — #1221 had it escape a word-leading pattern group into a
// program that ran and did something else, and #1406 had it drop a `function`
// keyword, which changes whether a function's locals leak. Both are fixed, and
// neither could ever have reached a formatter that does not rebuild a token in
// the first place.
//
// # The dialect decides the layout
//
// `dialect/<shell>.Style()` is the fourth vector beside Dialect, Semantics and
// Diagnostics, and it answers the questions the grammar does not: what one
// indent is, whether `then` shares its header's line, what becomes of a body
// the author spelled with braces. docs/spec/style.md holds the measurements
// behind every answer.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/diff"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/syntax"
)

const name = "shfmt"

var (
	lang  = flag.String("dialect", "auto", "which shell to read as: auto, core, posix, bash, zsh, ksh, dash")
	write = flag.Bool("w", false, "write the result back to each file instead of to standard output")
	list  = flag.Bool("l", false, "list the files whose layout differs, and exit 1 if any do")
	diffM = flag.Bool("d", false, "print a unified diff where the layout differs, and exit 1 if any does")
)

func main() {
	flag.Usage = usage
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		os.Exit(1)
	}
	if differs {
		os.Exit(1)
	}
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "usage: %s [flags] [file ...]\n\n", name)                        //nolint:errcheck // usage on a closed stderr is not worth a branch
	fmt.Fprint(out, "Lay out shell scripts. With no file, reads standard input.\n\n") //nolint:errcheck // as above
	flag.PrintDefaults()
}

// differs records that -l or -d found something, which is an exit status
// rather than an error: nothing went wrong, the answer is just "not
// formatted", and CI wants to know.
var differs bool

func run() error {
	if flag.NArg() == 0 {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		out, err := render("<standard input>", string(src))
		if err != nil {
			return err
		}
		_, err = io.WriteString(os.Stdout, out)
		return err
	}
	var failed []error
	for _, path := range flag.Args() {
		if err := one(path); err != nil {
			fmt.Fprintln(os.Stderr, name+":", err)
			failed = append(failed, err)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d files could not be read", len(failed), flag.NArg())
	}
	return nil
}

func one(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := render(path, string(src))
	if err != nil {
		return err
	}
	switch {
	case *list:
		if out != string(src) {
			fmt.Println(path)
			differs = true
		}
	case *diffM:
		if d := diff.Unified(path+".orig", path, string(src), out); d != "" {
			fmt.Print(d)
			differs = true
		}
	case *write:
		if out != string(src) {
			// The original mode, because a formatter must not quietly make a
			// script unexecutable.
			mode := os.FileMode(0o644)
			if fi, err := os.Stat(path); err == nil {
				mode = fi.Mode().Perm()
			}
			return os.WriteFile(path, []byte(out), mode)
		}
	default:
		_, err = io.WriteString(os.Stdout, out)
		return err
	}
	return nil
}

func render(path, src string) (string, error) {
	d, st, which, err := chosen(path, src)
	if err != nil {
		return "", err
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		// Name the dialect the file was read as. A construct that needs
		// another one fails here, and "unexpected" on its own does not tell
		// anyone that the answer is `-dialect zsh`.
		return "", fmt.Errorf("%s: read as %s: %w", path, which, err)
	}
	return printer.Format(src, f, comments.Recover(src, f), st), nil
}

// chosen picks the grammar and the layout together, because they are one
// decision: a file read as zsh is laid out as zsh.
//
// The flag first, then the extension, then the shebang or a zsh function's
// `#compdef`/`#autoload` marker, then the core — the substrate's common
// denominator, the grammar every panel shell accepts, and the honest default
// for a file that declares nothing.
func chosen(path, src string) (syntax.Dialect, syntax.Style, string, error) {
	if *lang != "auto" {
		d, st, err := named(*lang)
		return d, st, *lang, err
	}
	switch strings.TrimPrefix(filepath.Ext(path), ".") {
	case "bash":
		return bash.Dialect(), bash.Style(), "bash", nil
	case "zsh", "zsh-theme":
		return zsh.Dialect(), zsh.Style(), "zsh", nil
	case "ksh":
		return ksh.Dialect(), ksh.Style(), "ksh", nil
	case "dash":
		return dash.Dialect(), dash.Style(), "dash", nil
	}
	first, _, _ := strings.Cut(src, "\n")
	switch {
	case strings.HasPrefix(first, "#compdef"), strings.HasPrefix(first, "#autoload"):
		// Not a comment to be re-flowed: zsh's autoload machinery reads it,
		// and it is on line 1 of all 1,004 completion files measured.
		return zsh.Dialect(), zsh.Style(), "zsh", nil
	case strings.HasPrefix(first, "#!"):
		if d, st, which, ok := fromShebang(first); ok {
			return d, st, which, nil
		}
	}
	return syntax.Core(), syntax.CoreStyle(), "core", nil
}

func fromShebang(first string) (syntax.Dialect, syntax.Style, string, bool) {
	interp := first
	if i := strings.LastIndexByte(first, '/'); i >= 0 {
		interp = first[i+1:]
	}
	if f := strings.Fields(interp); len(f) > 0 {
		interp = f[0]
		// `#!/usr/bin/env bash` names the shell in the argument.
		if interp == "env" {
			if f := strings.Fields(first); len(f) > 1 {
				interp = f[len(f)-1]
			}
		}
	}
	switch interp {
	case "bash":
		return bash.Dialect(), bash.Style(), "bash", true
	case "zsh":
		return zsh.Dialect(), zsh.Style(), "zsh", true
	case "ksh", "ksh93":
		return ksh.Dialect(), ksh.Style(), "ksh", true
	case "dash":
		return dash.Dialect(), dash.Style(), "dash", true
	}
	return syntax.Dialect{}, syntax.Style{}, "", false
}

func named(which string) (syntax.Dialect, syntax.Style, error) {
	switch which {
	case "core":
		return syntax.Core(), syntax.CoreStyle(), nil
	case "posix":
		return syntax.POSIX(), syntax.CoreStyle(), nil
	case "bash":
		return bash.Dialect(), bash.Style(), nil
	case "zsh":
		return zsh.Dialect(), zsh.Style(), nil
	case "ksh":
		return ksh.Dialect(), ksh.Style(), nil
	case "dash":
		return dash.Dialect(), dash.Style(), nil
	}
	return syntax.Dialect{}, syntax.Style{}, errors.New("unknown dialect " + which +
		" (want one of: auto, core, posix, bash, zsh, ksh, dash)")
}
