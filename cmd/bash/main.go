// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command bash is a bash dialect built on the core.
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
	"strings"

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
		fmt.Fprintln(os.Stderr, "bash: -c is required")
		os.Exit(2)
	}
	os.Exit(run(*command))
}

func run(src string) int {
	// The whole of "which shell am I", as data.
	sem := interp.BashSemantics()
	dial := syntax.Bash()

	r := &interp.Runner{Semantics: &sem, Dialect: &dial, Name: "bash"}
	registerPrimitives(r)

	if code := source(r, prelude, dial); code != 0 {
		return code
	}

	f, err := syntax.Parse(src, dial)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bash:", err)
		return 2
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bash:", err)
		return 2
	}
	return status
}

// source runs a script on an existing runner, which is how a prelude is
// installed: no special entry point, just the same Run.
func source(r *interp.Runner, src string, d syntax.Dialect) int {
	f, err := syntax.Parse(src, d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bash: prelude:", err)
		return 2
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		fmt.Fprintln(os.Stderr, "bash: prelude:", err)
		return 2
	}
	return 0
}

// registerPrimitives adds the commands shell cannot express, and only those.
func registerPrimitives(r *interp.Runner) {
	r.Register("cd", func(r *interp.Runner, _ context.Context, args []string) int {
		dir := ""
		if len(args) > 0 {
			dir = args[0]
		}
		if dir == "" {
			dir, _ = r.Vars["HOME"]
			if dir == "" {
				dir = os.Getenv("HOME")
			}
		}
		old := r.Dir
		if old == "" {
			old, _ = os.Getwd()
		}
		if !strings.HasPrefix(dir, "/") && old != "" {
			dir = old + "/" + dir
		}
		if err := os.Chdir(dir); err != nil {
			fmt.Fprintln(os.Stderr, "bash: cd:", err)
			return 1
		}
		// The runner's own directory, which no shell function could set.
		r.Dir = dir
		r.SetVar("OLDPWD", old)
		r.SetVar("PWD", dir)
		return 0
	})

	r.Register("pwd", func(r *interp.Runner, _ context.Context, _ []string) int {
		dir := r.Dir
		if dir == "" {
			dir, _ = os.Getwd()
		}
		// The write error is discarded: a builtin whose output stream is
		// closed has nowhere to report that, and the shell should carry on.
		_, _ = fmt.Fprintln(r.Out(), dir)
		return 0
	})
}
