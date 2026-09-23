// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A written `~` that has no home to become.
//
// `HOME` unset is not a rare shape. `env -i` has none, a login that never set
// one has none, and on the column that answers a `~` from a cached copy —
// Semantics.TildeReadsACachedHome — `export -n HOME` and then any construct
// that builds an environment for a child leaves the copy absent even though
// the variable is still there to read.
//
// This package used to leave the word as written in every such case, and said
// so in the TildeReadsACachedHome doc: "this package carries no password
// database, so a `~` whose cached home is absent is left as written". That is
// one column's answer and three others disagree.
//
// Measured 2026-09-23, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> -c` with
// `printf "<%s>" ~` and the same line again as `v=a:~:b`, so that both roads a
// written tilde takes to a home are on the same rows:
//
//	HOME never set                      HOME set, then `unset HOME`
//	bash 5.3.20   the password entry    the cached copy, still /seeded
//	bash 3.2.57   the password entry    the password entry
//	zsh 5.9.2     the password entry    the empty string
//	dash 0.5.12   `~`, as written       `~`, as written
//	ksh93u+       the *login name*      the login name
//
// zsh's two columns differ because zsh seeds `HOME` itself at startup from
// the password entry, which bash does not — `env -i bash -c 'echo "[$HOME]"'`
// is empty and `~` is a path all the same. So zsh's own answer to the
// question is the right-hand column: an unset `HOME` is the empty string
// there, exactly as a `HOME=` that is set and empty is in every column.
//
// ksh93 is measured and deliberately not modeled. Its answer is not a home
// at all: `cd ~` is `cd: root: [No such file or directory]` and the word it
// substitutes is what `logname` prints — the session's login name, which
// depends on who owns the controlling terminal rather than on the shell. A
// dialect answering that would be writing a value no CI run could reproduce.
type TildeWithNoHomePolicy int

const (
	// TildeWithNoHomeUnspecified is no answer, and reads as the word being
	// left exactly as it was written — the standard's description of the
	// tilde prefix is a replacement by the value of `HOME`, and with no
	// value there is nothing to replace it with.
	TildeWithNoHomeUnspecified TildeWithNoHomePolicy = iota
	// TildeWithNoHomeStaysWritten is dash 0.5.12 and BusyBox ash: the two
	// characters `~/` reach the command as themselves.
	TildeWithNoHomeStaysWritten
	// TildeWithNoHomeIsEmpty is zsh 5.9.2: an unset `HOME` answers a `~`
	// the way an empty one does, so `~/x` is `/x`.
	TildeWithNoHomeIsEmpty
	// TildeWithNoHomeReadsThePasswordEntry is bash 5.3.20 and bash 3.2.57:
	// the home of the user the process runs as, which is the same database
	// `~user` reads and is asked through the same hook. A Runner with no
	// Runner.UserHomeDir has no database to ask and leaves the word as
	// written, which is what a library embedded in a program with no
	// business reading a password file should do.
	TildeWithNoHomeReadsThePasswordEntry
)

func (p TildeWithNoHomePolicy) String() string {
	switch p {
	case TildeWithNoHomeStaysWritten:
		return "stays written"
	case TildeWithNoHomeIsEmpty:
		return "is empty"
	case TildeWithNoHomeReadsThePasswordEntry:
		return "reads the password entry"
	}
	return "unspecified"
}

// homeForAWrittenTilde is the home a written `~` becomes, including the answer
// for a shell that has none.
//
// The one place both roads a written tilde takes — Runner.tildeSplit for a
// leading one and Runner.expandColonTildes for one after a colon — ask what
// the home is. They asked Runner.tildeHome separately before, and a fallback
// added to one of them would have been a second helper carrying half the
// answer, which is the shape that keeps being found here.
func (r *Runner) homeForAWrittenTilde() (string, bool) {
	if home, ok := r.tildeHome(); ok {
		return home, true
	}
	switch r.sem().TildeWithNoHome {
	case TildeWithNoHomeIsEmpty:
		return "", true
	case TildeWithNoHomeReadsThePasswordEntry:
		if r.UserHomeDir == nil {
			return "", false
		}
		return r.UserHomeDir("")
	}
	return "", false
}
