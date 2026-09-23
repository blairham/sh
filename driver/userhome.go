// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "os/user"

// userHomeDir answers `~user` out of the system's user database.
//
// Here rather than in interp for the reason replaceProcess and setUmask are
// here: a library Runner must not read the process's world on an embedder's
// behalf, and a shell binary must. See interp.Runner.UserHomeDir, which is
// nil until this is wired and leaves the word as written.
//
// A name with no entry is left as written rather than refused, which is what
// bash, ksh93 and dash do — `echo ~nosuchuser12345` prints the two words back
// in all three. zsh refuses it, `no such user or named directory`, and that
// is a divergence this hook does not decide: it reports what the database
// holds and nothing about what a miss costs.
// An empty name is the user the process runs as, which is the convention
// interp.Runner.UserHomeDir describes: a bare `~` with no `HOME` to read is
// the password entry's home in bash, and there is no name in the script to
// look up. os/user answers it from the process's own uid, so this is the
// same database and one call rather than a name this program would have to
// invent.
func userHomeDir(name string) (string, bool) {
	var u *user.User
	var err error
	if name == "" {
		u, err = user.Current()
	} else {
		u, err = user.Lookup(name)
	}
	if err != nil || u.HomeDir == "" {
		return "", false
	}
	return u.HomeDir, true
}
