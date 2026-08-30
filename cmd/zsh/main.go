// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command zsh is a zsh dialect built on the core.
//
// It exists to prove the core is a library rather than a program with
// options: everything here uses only the public API, so it could be lifted
// into its own repository without the core changing. That is the property
// worth protecting, and keeping this in cmd/ while importing nothing internal
// is what keeps proving it.
//
// Almost all of the dialect is one value. The vector is what makes a shell
// bash rather than zsh; the Go below it is only what shell cannot express.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// prelude is the part of the dialect that needs no Go.
//
// A function shadows a builtin and an external command alike, so anything
// here replaces the core's answer without the core knowing. Most of a real
// dialect belongs in a file like this.
const prelude = `
pushd() { cd "$1"; }
popd()  { cd "$OLDPWD"; }
`

func main() {
	command := flag.String("c", "", "run the given command")
	flag.Parse()

	if *command == "" {
		fmt.Fprintln(os.Stderr, "zsh: -c is required")
		os.Exit(2)
	}
	os.Exit(run(*command))
}

func run(src string) int {
	// The whole of "which shell am I", as data.
	sem := interp.ZshSemantics()
	dial := syntax.Bash()

	r := &interp.Runner{Semantics: &sem, Dialect: &dial, Name: "zsh"}
	registerPrimitives(r)

	if code := source(r, prelude, dial); code != 0 {
		return code
	}

	f, err := syntax.Parse(src, dial)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh:", err)
		return 2
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh:", err)
		return 2
	}
	return status
}

// source runs a script on an existing runner, which is how a prelude is
// installed: no special entry point, just the same Run.
func source(r *interp.Runner, src string, d syntax.Dialect) int {
	f, err := syntax.Parse(src, d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh: prelude:", err)
		return 2
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		fmt.Fprintln(os.Stderr, "zsh: prelude:", err)
		return 2
	}
	return 0
}

// registerPrimitives adds the commands shell cannot express and the core does
// not already provide.
//
// It is empty: cd, pwd and read were the examples, and they turned out to
// belong in the core — each changes the runner's own state and every dialect
// needs them. What remains for a dialect to register is whatever *it* has and
// the common denominator does not.
func registerPrimitives(r *interp.Runner) {}
