// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blairham/sh/internal/promptimport"
	"github.com/blairham/sh/internal/prompttheme"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `prompt import` — the half of the theme engine that reads somebody else's
// configuration.
//
// docs/spec/prompt-theme.md's rules, and every one of them is a property of
// where this code sits rather than a promise:
//
//   - it runs **once**, as a command a person types, and writes our settings
//     into our configuration file;
//   - **nothing on the render path reads another project's file, name or
//     format** — internal/promptimport is imported from here and from
//     nowhere else, and this function is reached only from the `prompt`
//     prelude function;
//   - what comes out is an ordinary configuration afterwards.
//
// # Every path here goes through the gate
//
// Both files a conversion touches are named by a person at a prompt, which
// is exactly the case a policy exists for: the source is **read** and the
// destination is **written**, and a shell under a policy must refuse both
// where the policy does not permit them. So every open asks
// Runner.AllowReadPath or Runner.AllowModify first — the same route a
// dialect's own file builtins take — and internal/boundary's guard is what
// says so rather than a convention, since it reads this package and refuses
// an ungated `os` call in it.
//
// # Evaluating rather than parsing, and where the runner comes from
//
// A powerlevel10k configuration is a zsh program and the spec's decision is
// to source it and harvest the parameter namespace. The runner that does the
// sourcing is a **child of this session's**, built from the same vectors and
// seeded with the same variables, and thrown away afterwards.
//
// Both halves of that matter. A child, because sourcing 280 settings and a
// handful of functions into a person's live session is not what they asked
// for when they asked to convert a file. Seeded from the session, because
// the file opens with a version gate that reads `$ZSH_VERSION` — a child
// with an empty namespace applies **none** of the configuration and produces
// an empty import that looks exactly like a file with nothing in it.
//
// **The dialect is the session's**, and that is the one thing here worth
// arguing about. `repl` imports no dialect package — the property #1323 says
// must survive this work — so there is no zsh in reach to build a runner
// from, and the honest alternatives were the session's own vectors or a new
// field on `driver.Shell` that five binaries would each have to fill in. So:
// run the import from a zsh session, which is where the file's author
// already is. A file that will not parse is reported with the dialect that
// refused it named, because "it did not work" and "you are in bash" are
// different sentences and only one of them is actionable.

// importUsage is what the subcommand takes.
const importUsage = "usage: prompt import p10k|starship <file> [<destination>]"

// importConfig converts another prompt program's configuration.
func (t *Theme) importConfig(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) < 2 || len(args) > 3 {
		r.DiagnoseAsf(promptName, "%s\n", importUsage)
		return 2
	}
	kind, source := args[0], args[1]
	destination := ""
	if len(args) == 3 {
		destination = args[2]
	} else if t.get != nil {
		destination, _ = t.get(prompttheme.Prefix + "CONFIG")
	}
	if strings.TrimSpace(destination) == "" {
		// Named or nowhere, which is the rule the configuration file already
		// follows and the one the block store deliberately stopped breaking:
		// inventing a location for somebody is how a tool ends up owning a
		// path nobody chose.
		r.DiagnoseAsf(promptName, "%s: import: no destination — "+
			"name one, or set %sCONFIG to the file you want written\n",
			promptName, prompttheme.Prefix)
		return 2
	}

	var result promptimport.Result
	switch kind {
	case "p10k", "powerlevel10k":
		params, code := t.harvest(r, ctx, source, "POWERLEVEL9K_")
		if code != 0 {
			return code
		}
		result = promptimport.Powerlevel10k(params, t.draws)
	case "starship":
		if !r.AllowReadPath(ctx, source) {
			r.DiagnoseAsf(promptName, "%s: import: %s: refused by the policy\n", promptName, source)
			return 1
		}
		text, err := os.ReadFile(source)
		if err != nil {
			r.DiagnoseAsf(promptName, "%s: import: %v\n", promptName, err)
			return 1
		}
		var perr error
		result, perr = promptimport.Starship(string(text), t.draws)
		if perr != nil {
			r.DiagnoseAsf(promptName, "%s: import: %s: %v\n", promptName, source, perr)
			return 1
		}
	default:
		r.DiagnoseAsf(promptName, "%s: import: %s: no converter for that; %s\n",
			promptName, kind, importUsage)
		return 2
	}

	text := promptimport.Write(source, result)
	if !r.AllowModify(ctx, destination) {
		r.DiagnoseAsf(promptName, "%s: import: %s: refused by the policy\n", promptName, destination)
		return 1
	}
	if err := os.WriteFile(destination, []byte(text), 0o600); err != nil {
		r.DiagnoseAsf(promptName, "%s: import: %v\n", promptName, err)
		return 1
	}

	// The report, on standard output beside the count, because this is the
	// asked half of the surface and a person who ran a conversion is looking
	// at its answer. Every setting that was read and not honored, by name:
	// a converter that reported only what it carried would be reporting the
	// half nobody needs to check.
	out := r.Out()
	_, _ = fmt.Fprintf(out, "wrote %s: %d settings carried\n", destination, result.Carried)
	if len(result.NotCarried) == 0 {
		return 0
	}
	_, _ = fmt.Fprintf(out, "%d read and not honored:\n", len(result.NotCarried))
	for _, line := range result.NotCarried {
		_, _ = fmt.Fprintf(out, "  %s\n", line)
	}
	return 0
}

// draws answers the converters' one question about this shell: is there a
// segment for this element name?
//
// The roster's own answer, so it includes a shell function the person has
// already defined and a plugin they are already running — a converter with a
// list of its own would grade a configuration against a shell nobody is
// running. Asked without rendering, for the reason Roster.Source exists: an
// element asked about must not join the report's "not yet" list.
func (t *Theme) draws(element string) bool {
	_, ok := t.roster.Source(element)
	return ok
}

// harvest sources a program in a child of this session and collects the
// parameters under a prefix.
//
// The child is discarded, so nothing the file did reaches the session: not
// its settings, not its functions, not its options. What crosses back is two
// maps of strings, which is what keeps internal/promptimport free of an
// interpreter.
func (t *Theme) harvest(r *interp.Runner, ctx context.Context, path, prefix string) (promptimport.Params, int) {
	abs, err := filepath.Abs(path)
	if err != nil {
		r.DiagnoseAsf(promptName, "%s: import: %v\n", promptName, err)
		return promptimport.Params{}, 1
	}
	if !r.AllowReadPath(ctx, abs) {
		r.DiagnoseAsf(promptName, "%s: import: %s: refused by the policy\n", promptName, path)
		return promptimport.Params{}, 1
	}
	if !r.AllowProbe(ctx, abs) {
		// Hidden rather than refused, which is what a probe's false means
		// everywhere else here: a refusal that identified itself would be an
		// oracle for what the policy hides.
		r.DiagnoseAsf(promptName, "%s: import: %s: no such file\n", promptName, path)
		return promptimport.Params{}, 1
	}
	if _, err := os.Stat(abs); err != nil {
		r.DiagnoseAsf(promptName, "%s: import: %v\n", promptName, err)
		return promptimport.Params{}, 1
	}

	child := childOf(r)
	// Sourced rather than parsed-and-run, and the difference is the gate.
	// A configuration of this kind opens with a version check that ends in
	// `return`, and `return` means "stop reading this file" only inside a
	// file the shell is reading. Running the parsed program directly instead
	// carries on past the gate and converts a configuration the shell would
	// never have applied — the silent wrong answer, at the one point the
	// whole file hangs on.
	f, err := syntax.Parse(". "+shellQuoted(abs)+"\n", dialectOf(r))
	if err != nil {
		// Named with the dialect that refused it. A powerlevel10k
		// configuration is a zsh program, and "this shell is not running the
		// zsh dialect" is the actionable half of almost every refusal here.
		r.DiagnoseAsf(promptName, "%s: import: %s: %v — "+
			"that file is a zsh program, so run the import from a zsh session\n",
			promptName, path, err)
		return promptimport.Params{}, 1
	}
	if err := child.RunPart(ctx, f); err != nil {
		r.DiagnoseAsf(promptName, "%s: import: %s: %v\n", promptName, path, err)
		return promptimport.Params{}, 1
	}

	params := promptimport.Params{
		Scalars: map[string]string{},
		Arrays:  map[string][]string{},
	}
	for name, value := range child.Vars {
		if strings.HasPrefix(name, prefix) {
			params.Scalars[name] = value
		}
	}
	for name := range child.Arrays {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if values, ok := child.GetArray(name); ok {
			params.Arrays[name] = values
		}
	}
	return params, 0
}

// childOf is a runner that starts where this session is and is thrown away.
//
// The variables are copied rather than shared, which is the whole of why the
// session survives an import: a configuration that assigns two hundred names
// assigns them here. The vectors are shared, because they are what the
// session *is* and a copy of them would be a different shell reading the
// file.
//
// Its streams go nowhere. A configuration that prints something is printing
// to the shell that sourced it in the program it was written for, and here
// that would be the middle of a person's conversion report.
func childOf(r *interp.Runner) *interp.Runner {
	child := &interp.Runner{
		Dialect:     r.Dialect,
		Semantics:   r.Semantics,
		Diagnostics: r.Diagnostics,
		Dir:         r.Dir,
		Name:        r.Name,
		Env:         slices.Clone(r.Env),
		Vars:        maps.Clone(r.Vars),
		Stdout:      nopWriter{},
		Stderr:      nopWriter{},
	}
	if child.Vars == nil {
		child.Vars = map[string]string{}
	}
	child.Arrays = map[string]interp.Array{}
	maps.Copy(child.Arrays, r.Arrays)
	return child
}

// shellQuoted wraps a path so the shell reads it as one word, whatever is in
// it. Single quotes, because inside them nothing at all is special and a
// path is data.
func shellQuoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// dialectOf is the grammar this session parses with, or the core's where a
// session has none.
func dialectOf(r *interp.Runner) syntax.Dialect {
	if r.Dialect == nil {
		return syntax.Dialect{}
	}
	return *r.Dialect
}

// nopWriter swallows what a sourced configuration prints.
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
