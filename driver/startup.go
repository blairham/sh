// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The files a shell reads before it starts asking.
//
// Two of them, and they answer different questions. A *login* shell reads
// ~/.profile — the things a person wants set once, for everything started from
// that session. An *interactive* shell reads $ENV — the things that only make
// sense at a prompt, and which a script must not inherit.
//
// $ENV rather than a name of our own. Measured: dash and ksh93 both read it
// when interactive and neither reads anything else; bash reads ~/.bashrc and
// zsh ~/.zshrc, which are names those shells own. This binary is not those
// shells — the repository says so at the top of cmd/sh — so it reads the one
// that is not anybody's brand.

// startup sources the files this session should read, reporting a status if
// one of them failed.
//
// A file that is not there is not a failure. Every shell starts for the first
// time with none of these, and a complaint about it would be the first thing
// anyone saw.
func (sh Shell) startup(r *interp.Runner, login bool) int {
	if login {
		if code := sh.sourceFile(r, homeFile(r, ".profile")); code != 0 {
			return code
		}
	}
	// $ENV is expanded first: it is a path with parameters in it more often
	// than not, and `$HOME/.shrc` is the usual spelling. An unset or empty
	// one expands to nothing and names nothing, which sourceFile answers.
	env, _ := r.GetVar("ENV")
	return sh.sourceFile(r, r.Expand(env))
}

// sourceFile runs a file on the runner, as `.` would.
//
// Guarded per file, and a caught panic costs the file rather than the session.
// That is the opposite of what a *parse* error in the same file does, and the
// difference is whose fault it is: a file that will not parse is wrong, and a
// shell that started anyway would be running with settings a person wrote and
// the shell silently declined. A file that parsed and then tickled an
// interpreter bug is the shell being wrong, and a half-configured prompt is a
// far better answer to that than no prompt at all.
func (sh Shell) sourceFile(r *interp.Runner, path string) (status int) {
	if path == "" {
		return 0
	}
	// Through the gate, which is what makes $ENV an access rather than a
	// blind spot: the path comes from a shell variable, so a line of script
	// can point it anywhere, and a policy that refuses every open a script
	// makes should not be walked around by setting a variable and starting a
	// session. ~/.profile is here for the same reason and by the same route.
	b, err := sh.readFile(path)
	if err != nil {
		// Missing, unreadable, a directory, refused: none of them is worth
		// stopping for. A shell that refused to start because ~/.profile was
		// not there would be unusable on a fresh machine, and one that
		// refused to start because a policy hid it would be worse — the
		// policy meant to keep the file out of the session, not to keep the
		// person out of a shell.
		return 0
	}
	if sh.guard().Do(func() { status = sh.sourceText(r, path, string(b)) }) {
		return 0
	}
	return status
}

// sourceText is sourceFile once the bytes are in hand and a guard is around
// it.
func (sh Shell) sourceText(r *interp.Runner, path, text string) int {
	f, perr := syntax.Parse(text, sh.Dialect)
	if perr != nil {
		sh.errf("%s", sh.Diagnostics.ParseDiagnostic(path, text, perr, text))
		return sh.Diagnostics.StatusForParseError(perr)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		sh.errf("%s", sh.Diagnostics.Report(path, 1, err.Error()+"\n"))
		return usageStatus
	}
	return 0
}

// homeFile names a file in the shell's home directory, or nothing when there
// is no home to name it under.
//
// Nothing rather than a path built on an empty home: joining would give
// `/.profile`, which is a real path on a real machine and belongs to root. A
// shell started without HOME must not read it.
func homeFile(r *interp.Runner, name string) string {
	home, _ := r.GetVar("HOME")
	if home == "" {
		return ""
	}
	return home + "/" + name
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
