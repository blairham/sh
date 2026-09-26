// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// Argv0Name is the variable one dialect reads as the name a command is
// started under, rather than as something to hand the command.
//
// It is exported for the reason ShellLevelName is: a front end and an
// embedder both need to spell the name the shell reserves, and spelling it
// twice is how the two drift apart.
const Argv0Name = "ARGV0"

// namedByTheEnvironment answers what a command should find in argv[0], and
// what environment it should be handed, where the two are one question.
//
// **The noun is "exported", not "prefix".** `ARGV0=sh cmd` looks like an
// assignment prefix and is not one: what decides is whether the name would
// have reached the child's environment at all, and every spelling that puts
// it there does this. Measured 2026-09-26 on zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `-f`, with the child asking for `$0`:
//
//	ARGV0=sh cmd                          sh    a prefix, which exports
//	export ARGV0=sh; cmd                  sh    no prefix at all
//	typeset -x ARGV0=sh; cmd              sh
//	ARGV0=sh; export ARGV0; cmd           sh    exported after the fact
//	setopt allexport; ARGV0=sh; cmd       sh    exported by the option
//	env ARGV0=sh zsh -c cmd               sh    inherited, never assigned here
//	ARGV0=sh                              —     assigned and not exported
//	cmd                                         so the name is the path
//	export ARGV0=sh; typeset +x ARGV0; cmd —    exported and then not
//
// The last two rows are the discriminating pair, and they are the reason a
// rule written around the prefix would have been wrong while agreeing with
// every row above them: hold "there is a variable named ARGV0" fixed and vary
// only whether it is exported, and the answer moves. The sixth row is the
// other half — hold "exported" fixed and take the prefix away entirely, and
// the answer does not move.
//
// **And it is consumed rather than copied.** The child is named and the
// variable does not reach it: `ARGV0=sh printenv ARGV0` exits 1 there, on
// every row above that renames. This shell's own copy survives, which is the
// half that says only the child's environment is touched — `export ARGV0=kk;
// cmd; print $ARGV0` still writes `kk`.
//
// A dialect that answers No is not asked to do anything: the name stays in
// the environment and the child is named by the word that was typed, which is
// what bash 5.3.20, bash 3.2.57, ksh93u+ 2012-08-01, dash 0.5.12 and BusyBox
// ash 1.37.0 all do. The axis is only consulted when the name is actually
// there, so a core with no dialect chosen refuses the script that uses it
// rather than every command it starts.
func (r *Runner) namedByTheEnvironment(name string, env []string) (string, []string) {
	chosen, kept, named := splitArgv0(env)
	if !named {
		return name, env
	}
	if !r.ask(r.sem().ExportedArgv0NamesTheCommand, "`"+Argv0Name+"` naming the command it starts") {
		return name, env
	}
	return chosen, kept
}

// splitArgv0 pulls ARGV0 out of an environment, reporting the value and what
// is left.
//
// **The last entry wins and every entry goes**, which is the rule the rest of
// a child's environment already follows here: this shell's exported names are
// laid down first and an assignment prefix is appended after them, so
// `PATH=/a PATH=/b cmd` resolves to `/b` by being last. A name that decides
// argv[0] cannot be left behind in one of its other spellings.
func splitArgv0(env []string) (value string, kept []string, found bool) {
	for _, entry := range env {
		v, ok := strings.CutPrefix(entry, Argv0Name+"=")
		if !ok {
			continue
		}
		value, found = v, true
	}
	if !found {
		return "", env, false
	}
	kept = make([]string, 0, len(env)-1)
	for _, entry := range env {
		if strings.HasPrefix(entry, Argv0Name+"=") {
			continue
		}
		kept = append(kept, entry)
	}
	return value, kept, true
}
