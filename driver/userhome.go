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
func userHomeDir(name string) (string, bool) {
	u, err := user.Lookup(name)
	if err != nil || u.HomeDir == "" {
		return "", false
	}
	return u.HomeDir, true
}
