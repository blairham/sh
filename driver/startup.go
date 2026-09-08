// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The files a shell reads before it starts asking.
//
// Four slots, and they answer different questions. One file is read on *every*
// invocation, whatever the shell was asked to do. A *login* shell reads a
// profile — the things a person wants set once, for everything started from
// that session. An *interactive* shell reads a run-commands file — the things
// that only make sense at a prompt, and which a script must not inherit. And
// one shell has a second login file that comes after the run-commands one, so
// that it sees what the prompt's own settings did.
//
// The names are the dialect's and every one of them may be empty; see
// Semantics.UnconditionalStartupFile and the fields beside it. The run-commands
// file is the one exception, and it is the standard's rather than nobody's: a
// shell with no name of its own reads `$ENV`, which is what dash, ksh93 and
// POSIX itself say, and so does a shell of any dialect once it is in POSIX
// mode. docs/spec/invocation.md has the measured grid.

// startupFlags is what the invocation said about which startup files to skip —
// the escape hatches from a startup file that is wrong.
//
// Read off the argument vector by the option loop and carried on the source,
// for the reason login-ness and being called `sh` are carried there: deciding
// them is part of reading an invocation, and the routes that run a program
// without one are never asked.
type startupFlags struct {
	// login makes this a login shell whatever argv[0] said: `-l`,
	// `--login`. Not the same fact as source.login, and the difference is
	// measured — the option reads the profile even with a script to run,
	// where the dashed argv[0] does not in every dialect.
	login bool
	// none suppresses every file: zsh's `-f`.
	none bool
	// noLogin suppresses the profile: bash's `--noprofile`.
	noLogin bool
	// noInteractive suppresses the run-commands file: bash's `--norc`.
	noInteractive bool
	// file is read in place of the run-commands file: bash's `--rcfile`.
	file string
}

// startup sources the files this invocation should read, reporting a status if
// one of them failed.
//
// One function for every route, which is the shape the measurements force: the
// unconditional file is read by a script as much as by a prompt, and `sh -i
// script.sh` reads the run-commands file that `sh script.sh` does not. It was
// two functions — one the prompt used and one the script routes reached past —
// and that split is exactly what made `~/.bashrc` unreachable from a terminal.
//
// A file that is not there is not a failure. Every shell starts for the first
// time with none of these, and a complaint about it would be the first thing
// anyone saw.
func (sh Shell) startup(r *interp.Runner, in source) int {
	// The prompt parameters, before the first file can read them — and
	// before the escape hatch below, because suppressing the files does not
	// suppress these: measured, `--norc`, `--noprofile` and `-f` all leave
	// the dialect's default sitting in PS1. See promptDefaults.
	sh.promptDefaults(r, in, false)
	defer sh.promptDefaults(r, in, true)
	if in.startup.none {
		// The escape hatch, and it is first because it is the whole answer:
		// a person whose only startup file will not run has to be able to get
		// a shell without it.
		return 0
	}
	for _, step := range []func(*interp.Runner, source) int{
		sh.unconditionalStartupFile,
		sh.loginProfile,
		sh.interactiveStartupFile,
		sh.lateLoginProfile,
	} {
		if code := step(r, in); code != 0 {
			return code
		}
		if r.Exited() {
			// `exit 3` in a startup file exits 3, and the files after it are
			// not read — measured in every shell that reads more than one.
			// Reported as no failure of its own: the caller asks the runner
			// what happened, the same way it does for the profile alone.
			return 0
		}
	}
	return 0
}

// unconditionalStartupFile sources the file read on every invocation, which is
// the only startup file any shell in the panel reads for a plain `sh -c cmd`.
//
// Empty in three of the four dialects, which is what makes this do nothing at
// all for them; see Semantics.UnconditionalStartupFile.
func (sh Shell) unconditionalStartupFile(r *interp.Runner, _ source) int {
	return sh.sourceFile(r, sh.startupPath(r, sh.Semantics.UnconditionalStartupFile))
}

// loginProfile sources the profile a login shell reads, which is the half of
// startup a shell with a script to run may also want.
//
// Whether a login shell with a script to run reads it at all is
// Semantics.LoginProfileWhenNonInteractive: at a prompt the panel is unanimous
// that it is read, so only the script routes have a question to ask.
//
// Which file is Semantics.LoginStartupFiles, and the list is tried in order
// with the *first one that can be read* winning. That is bash's rule —
// `.bash_profile`, then `.bash_login`, then `.profile`, and exactly one of them
// — and it reduces to "the file" for every shell that has one name for it.
func (sh Shell) loginProfile(r *interp.Runner, in source) int {
	if !sh.readsLoginProfile(in) {
		return 0
	}
	for _, name := range strings.Fields(sh.Semantics.LoginStartupFiles) {
		if code, found := sh.sourceFoundFile(r, sh.startupPath(r, name)); found {
			return code
		}
	}
	return 0
}

// lateLoginProfile sources the login file that comes *after* the run-commands
// one, so that it sees what the prompt's own settings did.
//
// zsh alone has one. Its position is the whole of why it is a second field
// rather than another entry in the list above: measured, an interactive login
// zsh reads `.zprofile`, then `.zshrc`, then `.zlogin`.
func (sh Shell) lateLoginProfile(r *interp.Runner, in source) int {
	if !sh.readsLoginProfile(in) {
		return 0
	}
	return sh.sourceFile(r, sh.startupPath(r, sh.Semantics.LateLoginStartupFile))
}

// readsLoginProfile answers whether this invocation reads a profile at all.
//
// Being interactive settles it whatever the dialect says: measured, all four
// shells read a profile for an interactive login shell, so the axis below is
// only ever asked of a shell with a script to run. `--noprofile` overrides
// both.
func (sh Shell) readsLoginProfile(in source) bool {
	if !in.loginShell() || in.startup.noLogin {
		return false
	}
	// An explicit option reads it whatever the dialect says, which is the
	// other measured half: `bash --login -c cmd` reads its profile where the
	// same shell under a dashed argv[0] reads nothing. So the axis is about
	// login-ness *inferred* from argv[0] and the option overrides it.
	return in.interactive || in.startup.login || sh.Semantics.LoginProfileWhenNonInteractive
}

// interactiveStartupFile sources the file a shell reads because there is a
// person on the other end, which is the counterpart of
// nonInteractiveStartupFile below.
//
// Two files under one name, and which of them is a mode question rather than a
// dialect one. A shell reads the file of *its own* name — `.bashrc`, `.zshrc` —
// unless it is in POSIX mode, where it reads the standard's `$ENV` instead;
// and a shell with no name of its own reads `$ENV` always, which is dash,
// ksh93 and the POSIX preset. The two never both happen, which is measured:
// bash at a prompt reads `.bashrc` and does nothing with `$ENV`, and bash
// invoked as `sh` reads `$ENV` and does nothing with `.bashrc`.
//
// That makes this the interactive half of what Semantics.NonInteractiveStartupVariable
// records for `$BASH_ENV`, and it is asked the same way and for the same
// reason: the mode is runtime state, so it is read off the runner and off how
// the shell was named rather than off a second axis that would have recorded
// the accident instead of the rule (#691, #733).
//
// The name is asked as well as the runner because the two are not the same
// moment. Being called `sh` turns the mode on *after* the startup files have
// run — measured, `set -o` in a file read by `-sh -i` reports `posix off` —
// so a shell that only asked the runner would pick its file before the answer
// existed.
func (sh Shell) interactiveStartupFile(r *interp.Runner, in source) int {
	if !in.interactive {
		return 0
	}
	if name := sh.Semantics.InteractiveStartupFile; name != "" && !in.posix && !r.PosixMode() {
		if in.startup.noInteractive {
			return 0
		}
		if in.loginShell() && sh.Semantics.InteractiveStartupFileWhenLogin != interp.Yes {
			// A login shell that does not read its run-commands file reads
			// nothing here at all — it does not fall through to `$ENV`.
			// Measured: `bash -l -i` reads `~/.bash_profile` and nothing
			// else, with `$ENV` set and pointing somewhere real.
			return 0
		}
		if in.startup.file != "" {
			// `--rcfile` replaces the name rather than adding to it, and it
			// is read from here rather than from the invocation directly so
			// that it loses to everything this branch already refused.
			return sh.sourceFile(r, in.startup.file)
		}
		return sh.sourceFile(r, sh.startupPath(r, name))
	}
	// $ENV is expanded first: it is a path with parameters in it more often
	// than not, and `$HOME/.shrc` is the usual spelling. An unset or empty
	// one expands to nothing and names nothing, which sourceFile answers.
	env, _ := r.GetVar("ENV")
	return sh.sourceFile(r, r.Expand(env))
}

// nonInteractiveStartupFile sources the file a shell reads when it is *not*
// going to prompt, which is the counterpart of the run-commands file above.
//
// The name of the variable is the dialect's and empty for most of them, which
// is what makes this do nothing at all in three of the four presets; see
// Semantics.NonInteractiveStartupVariable. The value is expanded before it is
// opened, exactly as $ENV's is and for the same reason — `$HOME/…` is how such
// a path is written.
//
// Reached from the script routes and never from a shell that is interactive,
// which is the whole of what separates it from the file above. The two are
// complementary rather than alternatives: measured, the shell that has this
// reads it when it is not interactive and reads a file of its own name when it
// is, and never reads both — `bash -i -c cmd` with `$BASH_ENV` set reads
// `~/.bashrc` and not the named file.
//
// **Not in POSIX mode.** The same shell reads nothing here when it was started
// with the standard's posix option or invoked as `sh`, which is measured and
// is the reason this asks the runner rather than the dialect: the mode is
// runtime state by then, and a second axis keyed on the invocation would have
// recorded the accident instead of the rule (#691, #733).
func (sh Shell) nonInteractiveStartupFile(r *interp.Runner, in source) int {
	name := sh.Semantics.NonInteractiveStartupVariable
	if name == "" || in.interactive || in.startup.none || r.PosixMode() {
		return 0
	}
	value, _ := r.GetVar(name)
	return sh.sourceFile(r, r.Expand(value))
}

// sourceFile runs a file on the runner, as `.` would.
func (sh Shell) sourceFile(r *interp.Runner, path string) int {
	status, _ := sh.sourceFoundFile(r, path)
	return status
}

// sourceFoundFile is sourceFile, also reporting whether the file was there.
//
// The second answer is what a fallback chain needs and nothing else does:
// bash reads the first of `.bash_profile`, `.bash_login` and `.profile` that
// exists, so "ran and said nothing" and "was not there" have to be told apart
// — a status of zero is both.
//
// Guarded per file, and a caught panic costs the file rather than the session.
// That is the opposite of what a *parse* error in the same file does, and the
// difference is whose fault it is: a file that will not parse is wrong, and a
// shell that started anyway would be running with settings a person wrote and
// the shell silently declined. A file that parsed and then tickled an
// interpreter bug is the shell being wrong, and a half-configured prompt is a
// far better answer to that than no prompt at all.
func (sh Shell) sourceFoundFile(r *interp.Runner, path string) (status int, found bool) {
	if path == "" {
		return 0, false
	}
	// Through the gate, which is what makes $ENV an access rather than a
	// blind spot: the path comes from a shell variable, so a line of script
	// can point it anywhere, and a policy that refuses every open a script
	// makes should not be walked around by setting a variable and starting a
	// session. The profile is here for the same reason and by the same route.
	b, err := sh.readFile(path)
	if err != nil {
		// Missing, unreadable, a directory, refused: none of them is worth
		// stopping for. A shell that refused to start because ~/.profile was
		// not there would be unusable on a fresh machine, and one that
		// refused to start because a policy hid it would be worse — the
		// policy meant to keep the file out of the session, not to keep the
		// person out of a shell.
		return 0, false
	}
	if sh.guard().Do(func() { status = sh.sourceText(r, path, string(b)) }) {
		return 0, true
	}
	return status, true
}

// sourceText is sourceFile once the bytes are in hand and a guard is around
// it.
func (sh Shell) sourceText(r *interp.Runner, path, text string) int {
	f, perr := syntax.Parse(text, sh.Dialect)
	if perr != nil {
		sh.errf("%s", sh.Diagnostics.ParseDiagnostic(path, text, perr, text))
		return sh.Diagnostics.StatusForParseError(perr)
	}
	// As the sourced script it is, which is what gives a `return` in it
	// something to return from: every shell in the panel accepts one in a
	// startup file, stops reading the file there and says nothing (#1422).
	// Run would have made this a script's own top level, where the refusal
	// belongs.
	if _, err := r.RunStartupFile(context.Background(), f); err != nil {
		sh.errf("%s", sh.Diagnostics.Report(path, 1, err.Error()+"\n"))
		return usageStatus
	}
	// A startup file is a file of its own, so an error in it costs that file
	// and not the session. Measured in both shells that read a startup file
	// without a person on the other end: a `~/.zshenv` or a `$BASH_ENV` whose
	// third line is `echo X${NOPE}` under `set -u` stops there, the startup
	// files after it are still read — zsh goes on to `.zprofile`, `.zshrc`
	// and `.zlogin` — and the script the shell was started for still runs.
	//
	// Not `exit`, which GiveUpTheFile deliberately does not catch: `exit 3`
	// in a startup file exits 3 and the files after it are not read, which is
	// the neighboring rule the loop in startup already models on Exited.
	r.GiveUpTheFile()
	return 0
}

// startupPath names one of this dialect's startup files, or nothing when there
// is no directory to name it under.
//
// The directory is Semantics.StartupDirectoryVariable's value where the
// dialect has one and `$HOME` otherwise. Read afresh for every file rather
// than once, which is measured and is the reason a person's `~/.zshenv`
// setting `ZDOTDIR` works at all: the file that sets it is found under the
// home directory and every file after it under the directory it named.
//
// Nothing rather than a path built on an empty directory: joining would give
// `/.profile`, which is a real path on a real machine and belongs to root. A
// shell started without HOME must not read it, and neither must one whose
// ZDOTDIR is set to nothing.
func (sh Shell) startupPath(r *interp.Runner, name string) string {
	if name == "" {
		return ""
	}
	dir, named := "", false
	if v := sh.Semantics.StartupDirectoryVariable; v != "" {
		dir, named = r.GetVar(v)
	}
	if !named {
		dir, _ = r.GetVar("HOME")
	}
	if dir == "" {
		// Set to nothing is not the same as unset, and the difference is
		// measured: a zsh whose ZDOTDIR is the empty string reads none of its
		// files rather than falling back to the home directory.
		return ""
	}
	return dir + "/" + name
}

// LoginShell reports whether this invocation is a login shell.
//
// The convention is argv[0] beginning with a dash, which is what `login` and
// every terminal emulator that offers "run as a login shell" does. It is a
// convention rather than a flag because there is nowhere else to put it: the
// shell is exec'd with no arguments of its own.
func LoginShell(argv []string) bool {
	return len(argv) > 0 && len(argv[0]) > 0 && argv[0][0] == '-'
}
